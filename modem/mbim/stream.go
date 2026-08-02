package mbim

import (
	"context"
	"fmt"
	"slices"
	"time"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/contract"
)

const (
	watchPollInterval   = 2 * time.Second
	watchResyncInterval = time.Minute
)

func pollStream[T any](ctx context.Context, query func(context.Context) (T, error)) <-chan Result[T] {
	return contract.PollStream(ctx, watchPollInterval, query)
}

func queryAndSend[T any](ctx context.Context, out chan<- Result[T], query func(context.Context) (T, error)) bool {
	value, err := query(ctx)
	if ctx.Err() != nil {
		return false
	}
	if !contract.SendStreamResult(ctx, out, Result[T]{Value: value, Err: err}) {
		return false
	}
	return err == nil
}

func forwardPollStream[T any](ctx context.Context, out chan<- Result[T], query func(context.Context) (T, error)) {
	for result := range pollStream(ctx, query) {
		if !contract.SendStreamResult(ctx, out, result) {
			return
		}
	}
}

func sendWatchError[T any](ctx context.Context, out chan<- Result[T], result Result[T]) {
	// The watcher terminates immediately after this best-effort error report.
	_ = contract.SendStreamResult(ctx, out, result)
}

func watchErrorResult[T any](value T, action string, err error) Result[T] {
	return Result[T]{Value: value, Err: fmt.Errorf("%s: %w", action, err)}
}

func (b *Backend) ensureWatchNotifications(ctx context.Context, requested ...mbimproto.DeviceServiceSubscribeEntry) error {
	b.notificationMu.Lock()
	defer b.notificationMu.Unlock()

	entries, changed := mergeWatchNotifications(b.notificationEntries, requested)
	if !changed {
		return nil
	}

	list, err := b.client.SetDeviceServiceSubscribeList(ctx, mbimproto.DeviceServiceSubscribeList{Entries: entries})
	if err != nil {
		return fmt.Errorf("enabling MBIM watch notifications: %w", err)
	}
	b.notificationEntries = list.Entries
	return nil
}

func mergeWatchNotifications(
	current []mbimproto.DeviceServiceSubscribeEntry,
	requested []mbimproto.DeviceServiceSubscribeEntry,
) ([]mbimproto.DeviceServiceSubscribeEntry, bool) {
	entries := make([]mbimproto.DeviceServiceSubscribeEntry, len(current))
	for i, entry := range current {
		entries[i] = mbimproto.DeviceServiceSubscribeEntry{ServiceID: entry.ServiceID, CIDs: slices.Clone(entry.CIDs)}
	}

	changed := false
	for _, request := range requested {
		index := slices.IndexFunc(entries, func(entry mbimproto.DeviceServiceSubscribeEntry) bool {
			return entry.ServiceID == request.ServiceID
		})
		if index < 0 {
			var cids []uint32
			for _, cid := range request.CIDs {
				if !slices.Contains(cids, cid) {
					cids = append(cids, cid)
				}
			}
			entries = append(entries, mbimproto.DeviceServiceSubscribeEntry{
				ServiceID: request.ServiceID,
				CIDs:      cids,
			})
			changed = true
			continue
		}
		// An empty CID list subscribes to every notification for the service.
		if len(entries[index].CIDs) == 0 {
			continue
		}
		if len(request.CIDs) == 0 {
			entries[index].CIDs = nil
			changed = true
			continue
		}
		for _, cid := range request.CIDs {
			if slices.Contains(entries[index].CIDs, cid) {
				continue
			}
			entries[index].CIDs = append(entries[index].CIDs, cid)
			changed = true
		}
	}
	return entries, changed
}

func (b *Backend) WatchStatus(ctx context.Context) (<-chan Result[Status], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	radioEvents, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDRadioState)
	if err != nil {
		cancel()
		return pollStream(ctx, b.Status), nil
	}
	subscriberEvents, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDSubscriberReadyStatus)
	if err != nil {
		cancel()
		return pollStream(ctx, b.Status), nil
	}
	registrationEvents, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDRegisterState)
	if err != nil {
		cancel()
		return pollStream(ctx, b.Status), nil
	}
	packetEvents, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDPacketService)
	if err != nil {
		cancel()
		return pollStream(ctx, b.Status), nil
	}
	signalEvents, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDSignalState)
	if err != nil {
		cancel()
		return pollStream(ctx, b.Status), nil
	}
	if err := b.ensureWatchNotifications(ctx, mbimproto.DeviceServiceSubscribeEntry{
		ServiceID: mbimproto.ServiceBasicConnect,
		CIDs: []uint32{
			mbimproto.CIDRadioState,
			mbimproto.CIDSignalState,
			mbimproto.CIDRegisterState,
			mbimproto.CIDSubscriberReadyStatus,
			mbimproto.CIDPacketService,
		},
	}); err != nil {
		cancel()
		return pollStream(ctx, b.Status), nil
	}

	version := b.client.Version().MBIMExVersion
	out := make(chan Result[Status], 1)
	go func() {
		defer close(out)
		defer cancel()

		current, err := b.Status(watchCtx)
		if watchCtx.Err() != nil {
			return
		}
		if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current, Err: err}) || err != nil {
			return
		}

		resync := time.NewTimer(watchResyncInterval)
		defer resync.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case result, ok := <-radioEvents:
				if !ok {
					radioEvents = nil
					break
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "watching MBIM radio state", result.Err))
					return
				}
				var radio mbimproto.RadioStateInfo
				if err := radio.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "decoding MBIM radio state indication", err))
					return
				}
				current.Power = powerState(radio)
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current}) {
					return
				}
			case result, ok := <-subscriberEvents:
				if !ok {
					subscriberEvents = nil
					break
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "watching MBIM subscriber status", result.Err))
					return
				}
				ready := mbimproto.SubscriberReadyStatusResponse{MBIMExVersion: version}
				if err := ready.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "decoding MBIM subscriber status indication", err))
					return
				}
				current.SIM = simState(ready.ReadyState)
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current}) {
					return
				}
			case result, ok := <-registrationEvents:
				if !ok {
					registrationEvents = nil
					break
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "watching MBIM registration state", result.Err))
					return
				}
				registration := mbimproto.RegistrationStateInfo{MBIMExVersion: version}
				if err := registration.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "decoding MBIM registration state indication", err))
					return
				}
				applyRegistrationToStatus(&current, registration)
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current}) {
					return
				}
			case result, ok := <-packetEvents:
				if !ok {
					packetEvents = nil
					break
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "watching MBIM packet service", result.Err))
					return
				}
				packet := mbimproto.PacketServiceInfo{MBIMExVersion: version}
				if err := packet.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "decoding MBIM packet service indication", err))
					return
				}
				applyPacketToStatus(&current, packet)
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current}) {
					return
				}
			case result, ok := <-signalEvents:
				if !ok {
					signalEvents = nil
					break
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "watching MBIM signal state", result.Err))
					return
				}
				signal := mbimproto.SignalStateInfo{MBIMExVersion: version}
				if err := signal.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "decoding MBIM signal state indication", err))
					return
				}
				current.SignalQuality = signalFromState(signal).Quality
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current}) {
					return
				}
			case <-resync.C:
				value, readErr := b.Status(watchCtx)
				if watchCtx.Err() != nil {
					return
				}
				current = value
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current, Err: readErr}) || readErr != nil {
					return
				}
				resync.Reset(watchResyncInterval)
			}

			if radioEvents == nil && subscriberEvents == nil && registrationEvents == nil && packetEvents == nil && signalEvents == nil {
				cancel()
				forwardPollStream(ctx, out, b.Status)
				return
			}
		}
	}()
	return out, nil
}

func (b *Backend) WatchSIM(ctx context.Context) (<-chan Result[SIMInfo], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	events, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDSubscriberReadyStatus)
	if err != nil {
		cancel()
		return pollStream(ctx, b.SIMInfo), nil
	}
	if err := b.ensureWatchNotifications(ctx, mbimproto.DeviceServiceSubscribeEntry{
		ServiceID: mbimproto.ServiceBasicConnect,
		CIDs:      []uint32{mbimproto.CIDSubscriberReadyStatus},
	}); err != nil {
		cancel()
		return pollStream(ctx, b.SIMInfo), nil
	}

	version := b.client.Version().MBIMExVersion
	out := make(chan Result[SIMInfo], 1)
	go func() {
		defer close(out)
		defer cancel()
		if !queryAndSend(watchCtx, out, b.SIMInfo) {
			return
		}

		resync := time.NewTimer(watchResyncInterval)
		defer resync.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case result, ok := <-events:
				if !ok {
					cancel()
					forwardPollStream(ctx, out, b.SIMInfo)
					return
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(SIMInfo{}, "watching MBIM subscriber status", result.Err))
					return
				}
				ready := mbimproto.SubscriberReadyStatusResponse{MBIMExVersion: version}
				if err := ready.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(SIMInfo{}, "decoding MBIM subscriber status indication", err))
					return
				}
				if !queryAndSend(watchCtx, out, b.SIMInfo) {
					return
				}
			case <-resync.C:
				if !queryAndSend(watchCtx, out, b.SIMInfo) {
					return
				}
				resync.Reset(watchResyncInterval)
			}
		}
	}()
	return out, nil
}

func (b *Backend) WatchNetwork(ctx context.Context) (<-chan Result[NetworkStatus], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	registrationEvents, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDRegisterState)
	if err != nil {
		cancel()
		return pollStream(ctx, b.NetworkStatus), nil
	}
	packetEvents, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDPacketService)
	if err != nil {
		cancel()
		return pollStream(ctx, b.NetworkStatus), nil
	}
	if err := b.ensureWatchNotifications(ctx, mbimproto.DeviceServiceSubscribeEntry{
		ServiceID: mbimproto.ServiceBasicConnect,
		CIDs:      []uint32{mbimproto.CIDRegisterState, mbimproto.CIDPacketService},
	}); err != nil {
		cancel()
		return pollStream(ctx, b.NetworkStatus), nil
	}

	version := b.client.Version().MBIMExVersion
	out := make(chan Result[NetworkStatus], 1)
	go func() {
		defer close(out)
		defer cancel()

		current, err := b.NetworkStatus(watchCtx)
		if watchCtx.Err() != nil {
			return
		}
		if !contract.SendStreamResult(watchCtx, out, Result[NetworkStatus]{Value: current, Err: err}) || err != nil {
			return
		}

		resync := time.NewTimer(watchResyncInterval)
		defer resync.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case result, ok := <-registrationEvents:
				if !ok {
					registrationEvents = nil
					break
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "watching MBIM registration state", result.Err))
					return
				}
				registration := mbimproto.RegistrationStateInfo{MBIMExVersion: version}
				if err := registration.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "decoding MBIM registration state indication", err))
					return
				}
				applyRegistration(&current, registration)
				if !contract.SendStreamResult(watchCtx, out, Result[NetworkStatus]{Value: current}) {
					return
				}
			case result, ok := <-packetEvents:
				if !ok {
					packetEvents = nil
					break
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "watching MBIM packet service", result.Err))
					return
				}
				packet := mbimproto.PacketServiceInfo{MBIMExVersion: version}
				if err := packet.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(current, "decoding MBIM packet service indication", err))
					return
				}
				applyPacketService(&current, packet)
				if !contract.SendStreamResult(watchCtx, out, Result[NetworkStatus]{Value: current}) {
					return
				}
			case <-resync.C:
				value, readErr := b.NetworkStatus(watchCtx)
				if watchCtx.Err() != nil {
					return
				}
				current = value
				if !contract.SendStreamResult(watchCtx, out, Result[NetworkStatus]{Value: current, Err: readErr}) || readErr != nil {
					return
				}
				resync.Reset(watchResyncInterval)
			}

			if registrationEvents == nil && packetEvents == nil {
				cancel()
				forwardPollStream(ctx, out, b.NetworkStatus)
				return
			}
		}
	}()
	return out, nil
}

func (b *Backend) WatchSignal(ctx context.Context) (<-chan Result[Signal], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	events, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceBasicConnect, mbimproto.CIDSignalState)
	if err != nil {
		cancel()
		return pollStream(ctx, b.Signal), nil
	}
	if err := b.ensureWatchNotifications(ctx, mbimproto.DeviceServiceSubscribeEntry{
		ServiceID: mbimproto.ServiceBasicConnect,
		CIDs:      []uint32{mbimproto.CIDSignalState},
	}); err != nil {
		cancel()
		return pollStream(ctx, b.Signal), nil
	}

	version := b.client.Version().MBIMExVersion
	out := make(chan Result[Signal], 1)
	go func() {
		defer close(out)
		defer cancel()
		if !queryAndSend(watchCtx, out, b.Signal) {
			return
		}

		resync := time.NewTimer(watchResyncInterval)
		defer resync.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case result, ok := <-events:
				if !ok {
					cancel()
					forwardPollStream(ctx, out, b.Signal)
					return
				}
				if result.Err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(Signal{}, "watching MBIM signal state", result.Err))
					return
				}
				signal := mbimproto.SignalStateInfo{MBIMExVersion: version}
				if err := signal.UnmarshalBinary(result.Value.InformationBuffer); err != nil {
					sendWatchError(watchCtx, out, watchErrorResult(Signal{}, "decoding MBIM signal state indication", err))
					return
				}
				if !contract.SendStreamResult(watchCtx, out, Result[Signal]{Value: signalFromState(signal)}) {
					return
				}
			case <-resync.C:
				if !queryAndSend(watchCtx, out, b.Signal) {
					return
				}
				resync.Reset(watchResyncInterval)
			}
		}
	}()
	return out, nil
}

func applyRegistrationToStatus(status *Status, registration mbimproto.RegistrationStateInfo) {
	status.Registration = registrationState(registration.RegisterState)
	status.OperatorID = registration.ProviderID
	status.OperatorName = registration.ProviderName
}

func applyPacketToStatus(status *Status, packet mbimproto.PacketServiceInfo) {
	status.PacketService = PacketServiceState(packet.PacketServiceState)
	status.Technology = technologyFromDataClass(packet.CurrentDataClass)
}

func knownSignal(db float64) SignalValue { return SignalValue{DB: db, Known: true} }
