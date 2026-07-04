package uim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

const (
	catOwnerProbeTimeout = 2 * time.Second
	// Qualcomm CAT keeps CATI_MAX_CLIDS at five in the reference stack; probing
	// beyond that can hang older firmware instead of returning InvalidClientId.
	catMaxClientID = 5
)

type CATServiceState struct {
	RawGlobalMask     uint32
	RawClientMask     uint32
	DecodedGlobalMask uint32
	DecodedClientMask uint32
	FullFunctionMask  uint32
}

type CATEventClaimConfig struct {
	RawMask            uint32
	FullFunctionMask   uint32
	CandidateClientIDs []uint8
}

type CATEventClaim struct {
	Service          qcom.ServiceType
	ClientID         uint8
	ReleasedClientID uint8
	StateBefore      CATServiceState
}

func (c *CAT) ServiceState(ctx context.Context) (CATServiceState, error) {
	service, clientID, err := c.reader.catClient(ctx)
	if err != nil {
		return CATServiceState{}, err
	}

	resp, err := c.reader.requestService(ctx, service, clientID, qcom.MessageCATGetServiceState, nil)
	if err != nil {
		return CATServiceState{}, fmt.Errorf("reading QMI CAT service state: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return CATServiceState{}, fmt.Errorf("reading QMI CAT service state: %w", err)
	}
	return decodeCATServiceState(resp.TLVs), nil
}

func (c *CAT) ForceClaimEvents(ctx context.Context, config CATEventClaimConfig) (CATEventClaim, error) {
	if config.RawMask == 0 && config.FullFunctionMask == 0 {
		return CATEventClaim{}, errors.New("claiming QMI CAT events: event mask is empty")
	}

	service, clientID, err := c.reader.catClient(ctx)
	if err != nil {
		return CATEventClaim{}, err
	}

	claim := CATEventClaim{Service: service, ClientID: clientID}
	ok, rawConflict, err := c.trySetEventReport(ctx, service, clientID, config.RawMask, config.FullFunctionMask)
	if err != nil {
		return CATEventClaim{}, err
	}
	if ok {
		return claim, nil
	}
	if !rawConflict {
		_ = c.reader.releaseCATClient(ctx, service, clientID)
		return CATEventClaim{}, fmt.Errorf("claiming QMI CAT events: registration rejected without raw event conflict")
	}

	state, err := c.ServiceState(ctx)
	if err != nil {
		_ = c.reader.releaseCATClient(ctx, service, clientID)
		return CATEventClaim{}, err
	}
	claim.StateBefore = state
	if state.RawGlobalMask&config.RawMask == 0 {
		_ = c.reader.releaseCATClient(ctx, service, clientID)
		return CATEventClaim{}, fmt.Errorf("claiming QMI CAT events: raw global mask 0x%08X does not contain requested mask 0x%08X", state.RawGlobalMask, config.RawMask)
	}

	candidates := config.CandidateClientIDs
	if len(candidates) == 0 {
		candidates = catCandidateClientIDs(clientID)
	}
	for _, candidate := range candidates {
		owner, err := c.eventOwner(ctx, service, clientID, config.RawMask, candidate)
		if err != nil || owner == 0 {
			continue
		}
		if err := c.releaseServiceClientID(ctx, service, owner); err != nil {
			_ = c.reader.releaseCATClient(ctx, service, clientID)
			return CATEventClaim{}, fmt.Errorf("claiming QMI CAT events: release owner client %d: %w", owner, err)
		}
		ok, _, err := c.trySetEventReport(ctx, service, clientID, config.RawMask, config.FullFunctionMask)
		if err != nil {
			_ = c.reader.releaseCATClient(ctx, service, clientID)
			return CATEventClaim{}, err
		}
		if ok {
			claim.ReleasedClientID = owner
			return claim, nil
		}
	}

	_ = c.reader.releaseCATClient(ctx, service, clientID)
	return CATEventClaim{}, fmt.Errorf("claiming QMI CAT events: raw mask 0x%08X is still owned by another CAT client", config.RawMask)
}

func (c *CAT) eventOwner(ctx context.Context, service qcom.ServiceType, ownClientID uint8, rawMask uint32, candidate uint8) (uint8, error) {
	if candidate == 0 || candidate == ownClientID {
		return 0, nil
	}
	state, err := c.serviceStateForClient(ctx, service, candidate, catOwnerProbeTimeout)
	if err != nil {
		return 0, err
	}
	if state.RawClientMask&rawMask == 0 {
		return 0, nil
	}
	return candidate, nil
}

func (c *CAT) serviceStateForClient(ctx context.Context, service qcom.ServiceType, clientID uint8, timeout time.Duration) (CATServiceState, error) {
	c.reader.mu.Lock()
	defer c.reader.mu.Unlock()
	if c.reader.closed || c.reader.transport == nil {
		return CATServiceState{}, errReaderClosed
	}
	resp, err := c.reader.sendRequest(ctx, service, clientID, qcom.MessageCATGetServiceState, nil, timeout)
	if err != nil {
		return CATServiceState{}, err
	}
	if err := resultOK(resp); err != nil {
		return CATServiceState{}, err
	}
	return decodeCATServiceState(resp.TLVs), nil
}

func (c *CAT) releaseServiceClientID(ctx context.Context, service qcom.ServiceType, clientID uint8) error {
	c.reader.mu.Lock()
	defer c.reader.mu.Unlock()
	if c.reader.closed || c.reader.transport == nil {
		return errReaderClosed
	}
	return c.reader.releaseServiceClientID(ctx, service, clientID)
}

func (c *CAT) trySetEventReport(ctx context.Context, service qcom.ServiceType, clientID uint8, rawMask, fullMask uint32) (bool, bool, error) {
	tlvs := tlv.TLVs{
		tlv.Uint(catSetEventReportRawTLV, rawMask),
		tlv.Uint(catSetEventReportSlotTLV, uint8(1<<(c.reader.slot-1))),
	}
	if fullMask != 0 {
		tlvs = append(tlvs, tlv.Uint(catSetEventReportFullFunctionTLV, fullMask))
	}

	resp, err := c.reader.requestService(ctx, service, clientID, qcom.MessageCATSetEventReport, tlvs)
	if err != nil {
		return false, false, fmt.Errorf("registering QMI CAT events: %w", err)
	}
	rawConflict, err := rawEventConflict(resp.TLVs, rawMask)
	if err != nil {
		return false, false, fmt.Errorf("registering QMI CAT events: %w", err)
	}
	if err := checkEventReportRegistration(resp.TLVs, rawMask, fullMask); err != nil {
		return false, rawConflict, nil
	}
	if err := resultOK(resp); err != nil {
		return false, rawConflict, nil
	}
	return true, false, nil
}

func decodeCATServiceState(tlvs tlv.TLVs) CATServiceState {
	var state CATServiceState
	if value, ok := tlv.Value(tlvs, 0x01); ok && len(value) >= 8 {
		state.RawGlobalMask = binary.LittleEndian.Uint32(value[0:4])
		state.RawClientMask = binary.LittleEndian.Uint32(value[4:8])
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok && len(value) >= 8 {
		state.DecodedGlobalMask = binary.LittleEndian.Uint32(value[0:4])
		state.DecodedClientMask = binary.LittleEndian.Uint32(value[4:8])
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok && len(value) >= 4 {
		state.FullFunctionMask = binary.LittleEndian.Uint32(value[0:4])
	}
	return state
}

func rawEventConflict(tlvs tlv.TLVs, rawMask uint32) (bool, error) {
	failed, ok, err := eventReportErrorMask(tlvs, catSetEventReportRawErrorTLV)
	if err != nil {
		return false, err
	}
	return ok && failed&rawMask != 0, nil
}

func catCandidateClientIDs(ownClientID uint8) []uint8 {
	ids := make([]uint8, 0, catMaxClientID-1)
	for id := uint8(1); id <= catMaxClientID; id++ {
		if id != ownClientID {
			ids = append(ids, id)
		}
	}
	return ids
}
