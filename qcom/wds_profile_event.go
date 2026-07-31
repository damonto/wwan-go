package qcom

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const wdsTLVProfileEventRegister = 0x10

const wdsMaxProfileEventRegistrations = 255

// WDSProfileChangeEvent identifies the operation that changed a profile.
type WDSProfileChangeEvent uint8

const (
	WDSProfileCreated WDSProfileChangeEvent = iota + 1
	WDSProfileDeleted
	WDSProfileModified
	WDSProfileSubscriptionChanged
)

// WDSProfileEvent describes one profile change indication.
type WDSProfileEvent struct {
	Profile WDSProfileID
	Change  WDSProfileChangeEvent
}

// WDSConfigureProfileEventListRequest encodes the profiles to monitor.
type WDSConfigureProfileEventListRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Profiles      []WDSProfileID
}

// Request validates and encodes the profile registration list.
func (r WDSConfigureProfileEventListRequest) Request() (Request, error) {
	profiles, err := normalizeWDSProfileEventList(r.Profiles)
	if err != nil {
		return Request{}, err
	}
	value := make([]byte, 1, 1+len(profiles)*2)
	value[0] = byte(len(profiles))
	for _, profile := range profiles {
		value = append(value, byte(profile.Type), profile.Index)
	}
	return Request{
		Service: ServiceWDS, ClientID: r.ClientID, TransactionID: r.TransactionID,
		MessageID: MessageWDSConfigureProfileEventList, Timeout: r.Timeout,
		TLVs: tlv.TLVs{tlv.Bytes(wdsTLVProfileEventRegister, value)},
	}, nil
}

// WDSProfileChangedIndication is the raw profile-change indication payload.
type WDSProfileChangedIndication struct {
	Event WDSProfileEvent
}

// UnmarshalTLVs parses a QMI WDS Profile Changed indication.
func (i *WDSProfileChangedIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = WDSProfileChangedIndication{}
	value, ok := tlv.Value(tlvs, wdsTLVProfileEventRegister)
	if !ok {
		return errors.New("parsing QMI WDS profile changed indication: profile event TLV missing")
	}
	if len(value) != 3 {
		return fmt.Errorf("parsing QMI WDS profile changed indication: profile event TLV length %d, want 3", len(value))
	}
	if WDSProfileType(value[0]) > WDSProfileTypeEPC {
		return fmt.Errorf("parsing QMI WDS profile changed indication: profile type %d is out of range", value[0])
	}
	if value[2] < byte(WDSProfileCreated) || value[2] > byte(WDSProfileSubscriptionChanged) {
		return fmt.Errorf("parsing QMI WDS profile changed indication: change event %d is out of range", value[2])
	}
	i.Event = WDSProfileEvent{
		Profile: WDSProfileID{Type: WDSProfileType(value[0]), Index: value[1]},
		Change:  WDSProfileChangeEvent(value[2]),
	}
	return nil
}

// WDSConfigureProfileEvents replaces the modem's profile-change filter.
func (c *Client) WDSConfigureProfileEvents(ctx context.Context, profiles []WDSProfileID) error {
	profiles, err := normalizeWDSProfileEventList(profiles)
	if err != nil {
		return err
	}
	err = c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		return c.configureWDSProfileEvents(ctx, clientID, profiles)
	})
	if err != nil {
		return fmt.Errorf("configuring QMI WDS profile events: %w", err)
	}
	return nil
}

// WDSWatchProfileChanges subscribes to changes for the selected profiles.
func (c *Client) WDSWatchProfileChanges(ctx context.Context, profiles []WDSProfileID) (<-chan WDSProfileEvent, error) {
	profiles, err := normalizeWDSProfileEventList(profiles)
	if err != nil {
		return nil, err
	}
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceWDS)
	if err != nil {
		return nil, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceWDS, clientID, MessageWDSProfileChanged)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := c.acquireWDSProfileEvents(ctx, clientID, profiles); err != nil {
		cancel()
		return nil, err
	}
	out := make(chan WDSProfileEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseWDSProfileEvents(profiles)
		for indication := range indications {
			var parsed WDSProfileChangedIndication
			if err := parsed.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- parsed.Event:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) configureWDSProfileEvents(ctx context.Context, clientID uint8, profiles []WDSProfileID) error {
	req, err := (WDSConfigureProfileEventListRequest{
		ClientID: clientID, Timeout: DefaultRequestTimeout, Profiles: profiles,
	}).Request()
	if err != nil {
		return err
	}
	resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (c *Client) acquireWDSProfileEvents(ctx context.Context, clientID uint8, profiles []WDSProfileID) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.wdsProfileEventRefs == nil {
		c.wdsProfileEventRefs = make(map[WDSProfileID]int)
	}
	oldList := wdsProfileEventList(c.wdsProfileEventRefs)
	for _, profile := range profiles {
		c.wdsProfileEventRefs[profile]++
	}
	newList := wdsProfileEventList(c.wdsProfileEventRefs)
	if slices.Equal(oldList, newList) {
		return nil
	}
	if err := c.configureWDSProfileEvents(ctx, clientID, newList); err != nil {
		for _, profile := range profiles {
			c.wdsProfileEventRefs[profile]--
			if c.wdsProfileEventRefs[profile] == 0 {
				delete(c.wdsProfileEventRefs, profile)
			}
		}
		return err
	}
	if len(oldList) == 0 {
		enabled := true
		if err := c.setWDSProfileChangeIndication(ctx, clientID, &enabled); err != nil {
			// Restore the modem-side profile filter before returning the
			// registration error. The best-effort rollback keeps a failed
			// acquire from leaving a stale filter active.
			_ = c.configureWDSProfileEvents(ctx, clientID, oldList)
			for _, profile := range profiles {
				c.wdsProfileEventRefs[profile]--
				if c.wdsProfileEventRefs[profile] == 0 {
					delete(c.wdsProfileEventRefs, profile)
				}
			}
			return err
		}
	}
	return nil
}

func (c *Client) releaseWDSProfileEvents(profiles []WDSProfileID) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.wdsProfileEventRefs == nil {
		return
	}
	oldList := wdsProfileEventList(c.wdsProfileEventRefs)
	for _, profile := range profiles {
		if count := c.wdsProfileEventRefs[profile]; count > 1 {
			c.wdsProfileEventRefs[profile] = count - 1
		} else {
			delete(c.wdsProfileEventRefs, profile)
		}
	}
	newList := wdsProfileEventList(c.wdsProfileEventRefs)
	if slices.Equal(oldList, newList) {
		return
	}
	clientID, err := c.serviceClientID(ctx, ServiceWDS)
	if err == nil {
		_ = c.configureWDSProfileEvents(ctx, clientID, newList)
		if len(newList) == 0 {
			disabled := false
			_ = c.setWDSProfileChangeIndication(ctx, clientID, &disabled)
		}
	}
}

func (c *Client) setWDSProfileChangeIndication(ctx context.Context, clientID uint8, enabled *bool) error {
	req := WDSIndicationRegisterRequest{
		ClientID: clientID,
		Timeout:  DefaultRequestTimeout,
		Config: WDSIndicationRegisterConfig{
			ProfileChanges: enabled,
		},
	}.Request()
	resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return fmt.Errorf("configuring QMI WDS profile change indications: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("configuring QMI WDS profile change indications: %w", err)
	}
	return nil
}

func normalizeWDSProfileEventList(profiles []WDSProfileID) ([]WDSProfileID, error) {
	unique := make(map[WDSProfileID]struct{}, len(profiles))
	for _, profile := range profiles {
		if profile.Type > WDSProfileTypeEPC {
			return nil, fmt.Errorf("encoding QMI WDS profile events: profile type %d is out of range", profile.Type)
		}
		unique[profile] = struct{}{}
	}
	result := make([]WDSProfileID, 0, len(unique))
	for profile := range unique {
		result = append(result, profile)
	}
	if len(result) > wdsMaxProfileEventRegistrations {
		return nil, fmt.Errorf(
			"encoding QMI WDS profile events: %d profiles exceeds maximum %d",
			len(result), wdsMaxProfileEventRegistrations,
		)
	}
	slices.SortFunc(result, func(a, b WDSProfileID) int {
		if a.Type != b.Type {
			if a.Type < b.Type {
				return -1
			}
			return 1
		}
		if a.Index < b.Index {
			return -1
		}
		if a.Index > b.Index {
			return 1
		}
		return 0
	})
	return result, nil
}

func wdsProfileEventList(refs map[WDSProfileID]int) []WDSProfileID {
	profiles := make([]WDSProfileID, 0, len(refs))
	for profile, count := range refs {
		if count > 0 {
			profiles = append(profiles, profile)
		}
	}
	profiles, _ = normalizeWDSProfileEventList(profiles)
	return profiles
}
