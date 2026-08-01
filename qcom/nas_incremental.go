package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// NASIncrementalNetworkScanStatus describes progress of an incremental scan.
type NASIncrementalNetworkScanStatus uint32

const (
	NASIncrementalNetworkScanComplete NASIncrementalNetworkScanStatus = iota
	NASIncrementalNetworkScanPartial
	NASIncrementalNetworkScanAborted
	NASIncrementalNetworkScanRejectedDuringRLF
	NASIncrementalNetworkScanError
	NASIncrementalNetworkScanPartialPeriodic
)

// NASIncrementalNetworkScanUpdate is one partial or terminal scan indication.
type NASIncrementalNetworkScanUpdate struct {
	Status   NASIncrementalNetworkScanStatus
	Networks []NASVisibleNetwork
}

// NASIncrementalNetworkScanConfig selects the optional scan information type.
type NASIncrementalNetworkScanConfig struct {
	ScanType *NASNetworkScanType
}

// IncrementalNetworkScan starts a scan and streams partial results until a
// terminal status arrives or ctx is canceled.
func (c *Client) IncrementalNetworkScan(
	ctx context.Context,
	config NASIncrementalNetworkScanConfig,
) (<-chan NASIncrementalNetworkScanUpdate, error) {
	tlvs, err := config.MarshalTLVs()
	if err != nil {
		return nil, err
	}
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("scanning QMI NAS networks incrementally: %w", err)
	}
	clientID, releaseClientID, err := c.sessionServiceClientID(ctx, ServiceNAS)
	if err != nil {
		return nil, fmt.Errorf("scanning QMI NAS networks incrementally: %w", err)
	}
	release := func() {
		if !releaseClientID {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
		defer cancel()
		_ = c.releaseServiceClientID(releaseCtx, ServiceNAS, clientID)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceNAS, clientID, MessageNASIncrementalNetworkScan)
	if err != nil {
		cancel()
		release()
		return nil, fmt.Errorf("scanning QMI NAS networks incrementally: %w", err)
	}
	targetTransactionID, resp, err := c.startNASIncrementalNetworkScan(ctx, clientID, tlvs)
	if err != nil {
		cancel()
		release()
		return nil, fmt.Errorf("scanning QMI NAS networks incrementally: %w", err)
	}
	if err := resultOK(resp); err != nil {
		cancel()
		release()
		return nil, fmt.Errorf("scanning QMI NAS networks incrementally: %w", err)
	}

	out := make(chan NASIncrementalNetworkScanUpdate, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer release()
		terminal := false
		defer func() {
			if !terminal {
				c.abortNASIncrementalNetworkScan(clientID, targetTransactionID)
			}
		}()
		for indication := range indications {
			var update NASIncrementalNetworkScanUpdate
			if err := update.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- update:
			case <-watchCtx.Done():
				return
			}
			if update.Status != NASIncrementalNetworkScanPartial {
				terminal = true
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) startNASIncrementalNetworkScan(
	ctx context.Context,
	clientID uint8,
	tlvs tlv.TLVs,
) (uint16, Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isOpenLocked() {
		return 0, Response{}, errClientClosed
	}
	transactionID := c.nextTransactionID(ServiceNAS)
	resp, err := c.doRequest(ctx, Request{
		Service:       ServiceNAS,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     MessageNASIncrementalNetworkScan,
		Timeout:       DefaultRequestTimeout,
		TLVs:          tlvs,
	})
	return transactionID, resp, err
}

func (c *Client) abortNASIncrementalNetworkScan(clientID uint8, targetTransactionID uint16) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()
	resp, err := c.requestServiceWithTimeout(ctx, ServiceNAS, clientID, MessageNASAbort, tlv.TLVs{
		tlv.Uint(0x01, targetTransactionID),
	}, monitorCleanupTimeout)
	if err == nil {
		_ = resultOK(resp)
	}
}

// MarshalTLVs encodes incremental network-scan fields.
func (config NASIncrementalNetworkScanConfig) MarshalTLVs() (tlv.TLVs, error) {
	if config.ScanType == nil {
		return nil, nil
	}
	if *config.ScanType > NASNetworkScanCellSearch {
		return nil, fmt.Errorf("encoding QMI NAS incremental network scan: scan type %d is out of range", *config.ScanType)
	}
	return tlv.TLVs{tlv.Uint(0x10, uint8(*config.ScanType))}, nil
}

// UnmarshalTLVs decodes one QMI NAS incremental network-scan update.
func (u *NASIncrementalNetworkScanUpdate) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI NAS incremental network scan: status TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI NAS incremental network scan: status TLV length %d, want 4", len(value))
	}
	update := NASIncrementalNetworkScanUpdate{
		Status: NASIncrementalNetworkScanStatus(binary.LittleEndian.Uint32(value)),
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		networks, err := decodeNASIncrementalNetworks(value)
		if err != nil {
			return err
		}
		update.Networks = networks
	}
	*u = update
	return nil
}

func decodeNASIncrementalNetworks(value []byte) ([]NASVisibleNetwork, error) {
	if len(value) < 2 {
		return nil, errors.New("parsing QMI NAS incremental network scan: network count is truncated")
	}
	count := int(binary.LittleEndian.Uint16(value[:2]))
	if count > nasMaxVisibleNetworks {
		return nil, fmt.Errorf("parsing QMI NAS incremental network scan: network count %d exceeds %d", count, nasMaxVisibleNetworks)
	}
	networks := make([]NASVisibleNetwork, count)
	offset := 2
	for index := range count {
		if len(value)-offset < 8 {
			return nil, fmt.Errorf("parsing QMI NAS incremental network scan: network %d is truncated", index)
		}
		descriptionLength := int(value[offset+7])
		end := offset + 8 + descriptionLength
		if end > len(value) {
			return nil, fmt.Errorf("parsing QMI NAS incremental network scan: network %d description is truncated", index)
		}
		networks[index] = NASVisibleNetwork{
			PLMN: NASPLMN{
				MCC:                 binary.LittleEndian.Uint16(value[offset : offset+2]),
				MNC:                 binary.LittleEndian.Uint16(value[offset+2 : offset+4]),
				MNCThreeDigits:      value[offset+6] != 0,
				MNCThreeDigitsKnown: true,
				Description:         string(value[offset+8 : end]),
			},
			Status:          NASNetworkStatus(value[offset+4]),
			RadioInterfaces: []NASRadioInterface{NASRadioInterface(value[offset+5])},
		}
		offset = end
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI NAS incremental network scan: network list has %d trailing bytes", len(value)-offset)
	}
	return networks, nil
}
