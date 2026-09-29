package qmi

import (
	"context"
	"time"

	"github.com/damonto/wwan-go/modem/contract"
	"github.com/damonto/wwan-go/qcom"
)

const (
	watchPollInterval         = 2 * time.Second
	watchResyncInterval       = time.Minute
	defaultSIMEnrichmentDelay = 2 * time.Second
)

type statusIndications struct {
	power   <-chan qcom.DMSEvent
	card    <-chan qcom.CardStatus
	network <-chan qcom.NASServingSystem
	system  <-chan qcom.NASSysInfo
	signal  <-chan qcom.NASSignalInfo
}

func (i statusIndications) empty() bool {
	return i.power == nil && i.card == nil && i.network == nil && i.system == nil && i.signal == nil
}

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

func (b *Backend) WatchStatus(ctx context.Context) (<-chan Result[Status], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	// QMI services vary by firmware; keep every supported indication and let
	// the periodic resync cover fields whose subscription is unavailable.
	indications := statusIndications{}
	indications.power, _ = b.client.DMSWatchEvents(watchCtx)
	indications.card, _ = b.client.WatchCardStatus(watchCtx)
	indications.network, _ = b.client.NASWatchServingSystem(watchCtx)
	indications.system, _ = b.client.NASWatchSystemInfo(watchCtx)
	indications.signal, _ = b.client.NASWatchSignalInfo(watchCtx)
	if indications.empty() {
		cancel()
		return pollStream(ctx, b.Status), nil
	}

	out := make(chan Result[Status], 1)
	go func() {
		defer close(out)
		defer cancel()
		var powerPolls <-chan Result[PowerState]

		current, err := b.Status(watchCtx)
		if watchCtx.Err() != nil {
			return
		}
		if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current, Err: err}) || err != nil {
			return
		}
		if indications.power == nil {
			powerPolls = pollStream(watchCtx, b.PowerState)
		}
		resync := time.NewTimer(watchResyncInterval)
		defer resync.Stop()
		sendPowerState := func(state PowerState) bool {
			refresh := powerTransitionNeedsStatusRefresh(current.Power, state)
			applyPowerState(&current, state)
			if !refresh {
				return contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current})
			}

			value, readErr := b.Status(watchCtx)
			if watchCtx.Err() != nil {
				return false
			}
			current = value
			resync.Reset(watchResyncInterval)
			return contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current, Err: readErr}) && readErr == nil
		}
		for {
			select {
			case <-watchCtx.Done():
				return
			case event, ok := <-indications.power:
				if !ok {
					indications.power = nil
					if !indications.empty() {
						powerPolls = pollStream(watchCtx, b.PowerState)
					}
					break
				}
				state, ok := powerStateFromEvent(event)
				if !ok {
					if !event.WirelessDisabledKnown && !event.OperatingModeKnown {
						continue
					}
					var readErr error
					state, readErr = b.PowerState(watchCtx)
					if watchCtx.Err() != nil {
						return
					}
					if readErr != nil {
						if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current, Err: readErr}) {
							return
						}
						return
					}
				}
				if !sendPowerState(state) {
					return
				}
			case result, ok := <-powerPolls:
				if !ok {
					powerPolls = nil
					break
				}
				if result.Err != nil {
					if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current, Err: result.Err}) {
						return
					}
					return
				}
				if current.Power == result.Value {
					continue
				}
				if !sendPowerState(result.Value) {
					return
				}
			case status, ok := <-indications.card:
				if !ok {
					indications.card = nil
					break
				}
				current.SIM = simStateFromCardStatus(status)
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current}) {
					return
				}
			case serving, ok := <-indications.network:
				if !ok {
					indications.network = nil
					break
				}
				applyNetworkStatus(&current, networkStatusFromServing(serving))
				normalizeRadioStatus(&current)
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current}) {
					return
				}
			case _, ok := <-indications.system:
				if !ok {
					indications.system = nil
					break
				}
				// System Info indications may contain only changed RAT fields.
				// Query a coherent snapshot instead of clearing omitted fields.
				value, readErr := b.Status(watchCtx)
				if readErr == nil {
					current = value
				}
				if !contract.SendStreamResult(watchCtx, out, Result[Status]{Value: current, Err: readErr}) {
					return
				}
			case info, ok := <-indications.signal:
				if !ok {
					indications.signal = nil
					break
				}
				current.SignalQuality = signalFromInfo(info).Quality
				normalizeRadioStatus(&current)
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

			if indications.empty() {
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
	// Physical-slot indications are optional on otherwise usable UIM services.
	cardEvents, _ := b.client.WatchCardStatus(watchCtx)
	slotEvents, _ := b.client.WatchSlotStatus(watchCtx)
	if cardEvents == nil && slotEvents == nil {
		cancel()
		return pollStream(ctx, b.SIMInfo), nil
	}

	out := make(chan Result[SIMInfo], 1)
	go func() {
		defer close(out)
		defer cancel()
		initial, err := b.simInfo(watchCtx)
		if watchCtx.Err() != nil {
			return
		}
		if !contract.SendStreamResult(watchCtx, out, Result[SIMInfo]{Value: initial, Err: err}) || err != nil {
			return
		}

		resync := time.NewTimer(watchResyncInterval)
		defer resync.Stop()
		var enrichment *time.Timer
		var enrichmentC <-chan time.Time
		type enrichmentResult struct {
			generation uint64
			result     Result[SIMInfo]
		}
		enrichmentResults := make(chan enrichmentResult, 1)
		var enrichmentCancel context.CancelFunc
		var enrichmentGeneration uint64
		scheduleEnrichment := func(delay time.Duration) {
			enrichmentGeneration++
			if enrichmentCancel != nil {
				enrichmentCancel()
				enrichmentCancel = nil
			}
			enrichment, enrichmentC = resetTimer(enrichment, delay)
		}
		if initial.State != SIMStateAbsent {
			scheduleEnrichment(b.enrichDelay)
		}
		defer func() {
			if enrichmentCancel != nil {
				enrichmentCancel()
			}
			if enrichment != nil {
				enrichment.Stop()
			}
		}()
		for {
			select {
			case <-watchCtx.Done():
				return
			case status, ok := <-cardEvents:
				if !ok {
					cardEvents = nil
					break
				}
				scheduleEnrichment(b.enrichDelay)
				if !contract.SendStreamResult(watchCtx, out, Result[SIMInfo]{Value: b.simInfoFromCardStatus(watchCtx, status)}) {
					return
				}
			case _, ok := <-slotEvents:
				if !ok {
					slotEvents = nil
					break
				}
				scheduleEnrichment(b.enrichDelay)
				if !queryAndSend(watchCtx, out, b.simInfo) {
					return
				}
			case <-enrichmentC:
				enrichmentC = nil
				generation := enrichmentGeneration
				enrichmentCtx, cancelEnrichment := context.WithCancel(watchCtx)
				enrichmentCancel = cancelEnrichment
				go func() {
					defer cancelEnrichment()
					value, readErr := b.SIMInfo(enrichmentCtx)
					if enrichmentCtx.Err() != nil {
						return
					}
					select {
					case enrichmentResults <- enrichmentResult{generation: generation, result: Result[SIMInfo]{Value: value, Err: readErr}}:
					case <-enrichmentCtx.Done():
					}
				}()
			case result := <-enrichmentResults:
				if result.generation != enrichmentGeneration {
					continue
				}
				enrichmentCancel = nil
				if !contract.SendStreamResult(watchCtx, out, result.result) || result.result.Err != nil {
					return
				}
			case <-resync.C:
				scheduleEnrichment(0)
				resync.Reset(watchResyncInterval)
			}

			if cardEvents == nil && slotEvents == nil {
				cancel()
				forwardPollStream(ctx, out, b.SIMInfo)
				return
			}
		}
	}()
	return out, nil
}

func resetTimer(timer *time.Timer, delay time.Duration) (*time.Timer, <-chan time.Time) {
	if timer == nil {
		timer = time.NewTimer(delay)
		return timer, timer.C
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
	return timer, timer.C
}

func (b *Backend) WatchNetwork(ctx context.Context) (<-chan Result[NetworkStatus], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	events, err := b.client.NASWatchServingSystem(watchCtx)
	if err != nil {
		cancel()
		return pollStream(ctx, b.NetworkStatus), nil
	}

	out := make(chan Result[NetworkStatus], 1)
	go func() {
		defer close(out)
		defer cancel()
		if !queryAndSend(watchCtx, out, b.NetworkStatus) {
			return
		}

		resync := time.NewTimer(watchResyncInterval)
		defer resync.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case serving, ok := <-events:
				if !ok {
					cancel()
					forwardPollStream(ctx, out, b.NetworkStatus)
					return
				}
				if !contract.SendStreamResult(watchCtx, out, Result[NetworkStatus]{Value: networkStatusFromServing(serving)}) {
					return
				}
			case <-resync.C:
				if !queryAndSend(watchCtx, out, b.NetworkStatus) {
					return
				}
				resync.Reset(watchResyncInterval)
			}
		}
	}()
	return out, nil
}

func (b *Backend) WatchSignal(ctx context.Context) (<-chan Result[Signal], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	events, err := b.client.NASWatchSignalInfo(watchCtx)
	if err != nil {
		cancel()
		return pollStream(ctx, b.Signal), nil
	}

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
			case info, ok := <-events:
				if !ok {
					cancel()
					forwardPollStream(ctx, out, b.Signal)
					return
				}
				if !contract.SendStreamResult(watchCtx, out, Result[Signal]{Value: signalFromInfo(info)}) {
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

func applyNetworkStatus(status *Status, network NetworkStatus) {
	status.Registration = network.Registration
	status.PacketService = network.PacketService
	status.Technology = network.Technology
	status.OperatorID = network.OperatorID
	status.OperatorName = network.OperatorName
}

func powerStateFromEvent(event qcom.DMSEvent) (PowerState, bool) {
	if event.WirelessDisabledKnown && event.WirelessDisabled {
		if !event.OperatingModeKnown {
			return PowerStateLow, true
		}
		return effectivePowerState(event.OperatingMode, true), true
	}
	if event.OperatingModeKnown {
		state := powerState(event.OperatingMode)
		if state == PowerStateUnknown {
			return PowerStateUnknown, false
		}
		return state, true
	}
	return PowerStateUnknown, false
}

func powerTransitionNeedsStatusRefresh(current, next PowerState) bool {
	return current != PowerStateOn && next == PowerStateOn
}

func applyPowerState(status *Status, state PowerState) {
	status.Power = state
	normalizeRadioStatus(status)
}

func normalizeRadioStatus(status *Status) {
	if status.Power == PowerStateOn {
		return
	}
	status.Registration = RegistrationIdle
	status.PacketService = PacketServiceDetached
	status.Technology = 0
	status.OperatorID = ""
	status.OperatorName = ""
	status.SignalQuality = 0
}
