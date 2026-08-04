package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	eventRegistrationCardStatus         uint32 = 1 << 0
	eventRegistrationPhysicalSlotStatus uint32 = 1 << 4
	monitorCleanupTimeout                      = 5 * time.Second
	uimRefreshFilesMax                         = 100
)

// WatchCardStatus subscribes to logical UIM card and application changes.
func (c *Client) WatchCardStatus(ctx context.Context) (<-chan CardStatus, error) {
	transport, err := c.indicationTransportContext(ctx)
	if err != nil {
		return nil, err
	}
	clientID, err := c.uimClientID(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching QMI UIM card status: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceUIM, clientID, MessageCardStatus)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI UIM card status: %w", err)
	}
	if err := c.acquireUIMEvents(ctx, eventRegistrationCardStatus); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI UIM card status: %w", err)
	}

	out := make(chan CardStatus, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseUIMEvents(eventRegistrationCardStatus)

		for ind := range indications {
			var status CardStatus
			if err := status.UnmarshalTLVs(ind.TLVs); err != nil {
				return
			}
			select {
			case out <- status:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) WatchSlotStatus(ctx context.Context) (<-chan SlotStatus, error) {
	transport, err := c.indicationTransportContext(ctx)
	if err != nil {
		return nil, err
	}
	clientID, err := c.uimClientID(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching QMI UIM slot status: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceUIM, clientID, MessageSlotStatus)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI UIM slot status: %w", err)
	}

	if err := c.acquireUIMEvents(ctx, eventRegistrationPhysicalSlotStatus); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI UIM slot status: %w", err)
	}

	out := make(chan SlotStatus, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseUIMEvents(eventRegistrationPhysicalSlotStatus)

		for ind := range indications {
			var status SlotStatus
			if err := status.UnmarshalTLVs(ind.TLVs); err != nil {
				return
			}
			select {
			case out <- status:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// WatchRefreshFiles watches refresh events for specific UIM files. Only one
// refresh watcher may be active on a Client at a time. The returned channel
// closes if a required refresh protocol response fails.
func (c *Client) WatchRefreshFiles(ctx context.Context, req RefreshRegisterRequest) (<-chan RefreshEvent, error) {
	if len(req.Files) == 0 {
		return nil, errors.New("watching QMI UIM refresh files: file list is empty")
	}
	if err := validateUIMAIDLength(req.AID); err != nil {
		return nil, fmt.Errorf("watching QMI UIM refresh files: %w", err)
	}

	tlvs, err := refreshRegisterTLVs(req, true)
	if err != nil {
		return nil, fmt.Errorf("watching QMI UIM refresh files: %w", err)
	}
	transport, err := c.indicationTransportContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching QMI UIM refresh files: %w", err)
	}
	clientID, err := c.uimClientID(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching QMI UIM refresh files: %w", err)
	}
	if err := c.acquireRefreshWatcher(); err != nil {
		return nil, fmt.Errorf("watching QMI UIM refresh files: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceUIM, clientID, MessageRefresh)
	if err != nil {
		cancel()
		c.releaseRefreshWatcher()
		return nil, fmt.Errorf("watching QMI UIM refresh files: %w", err)
	}

	if err := c.sendMonitorRequest(ctx, MessageRefreshRegister, tlvs); err != nil {
		cancel()
		c.releaseRefreshWatcher()
		return nil, fmt.Errorf("watching QMI UIM refresh files: %w", err)
	}

	cleanupReq := cloneRefreshRegisterRequest(req)
	out := make(chan RefreshEvent, 8)
	go c.forwardRefreshEvents(ctx, cancel, indications, out, func() {
		c.unregisterRefreshFiles(cleanupReq)
		c.releaseRefreshWatcher()
	})
	return out, nil
}

// WatchRefreshAll watches all refresh events for a UIM session. Only one
// refresh watcher may be active on a Client at a time. The returned channel
// closes if a required refresh protocol response fails.
func (c *Client) WatchRefreshAll(ctx context.Context, session Session, aid []byte) (<-chan RefreshEvent, error) {
	if err := validateUIMAIDLength(aid); err != nil {
		return nil, fmt.Errorf("watching all QMI UIM refresh files: %w", err)
	}

	tlvs, err := refreshRegisterAllTLVs(session, aid, true)
	if err != nil {
		return nil, fmt.Errorf("watching all QMI UIM refresh files: %w", err)
	}
	transport, err := c.indicationTransportContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching all QMI UIM refresh files: %w", err)
	}
	clientID, err := c.uimClientID(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching all QMI UIM refresh files: %w", err)
	}
	if err := c.acquireRefreshWatcher(); err != nil {
		return nil, fmt.Errorf("watching all QMI UIM refresh files: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceUIM, clientID, MessageRefresh)
	if err != nil {
		cancel()
		c.releaseRefreshWatcher()
		return nil, fmt.Errorf("watching all QMI UIM refresh files: %w", err)
	}

	if err := c.sendMonitorRequest(ctx, MessageRefreshRegisterAll, tlvs); err != nil {
		cancel()
		c.releaseRefreshWatcher()
		return nil, fmt.Errorf("watching all QMI UIM refresh files: %w", err)
	}

	cleanupAID := slices.Clone(aid)
	out := make(chan RefreshEvent, 8)
	go c.forwardRefreshEvents(ctx, cancel, indications, out, func() {
		c.unregisterRefreshAll(session, cleanupAID)
		c.releaseRefreshWatcher()
	})
	return out, nil
}

func (c *Client) acquireRefreshWatcher() error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.uimRefreshWatcherActive {
		return errors.New("another QMI UIM refresh watcher is active")
	}
	c.uimRefreshWatcherActive = true
	return nil
}

func (c *Client) releaseRefreshWatcher() {
	c.watchMu.Lock()
	c.uimRefreshWatcherActive = false
	c.watchMu.Unlock()
}

func (c *Client) indicationTransport() (IndicationTransport, error) {
	return c.indicationTransportContext(context.Background())
}

func (c *Client) indicationTransportContext(ctx context.Context) (IndicationTransport, error) {
	if err := c.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer c.mu.Unlock()
	if !c.isOpenLocked() {
		return nil, errClientClosed
	}
	transport, ok := c.transport.(IndicationTransport)
	if !ok {
		return nil, errors.New("QMI transport does not support indications")
	}
	return transport, nil
}

func (c *Client) registerEvents(ctx context.Context, mask uint32) error {
	return c.sendMonitorRequest(ctx, MessageRegisterEvents, registerEventsTLVs(mask))
}

func (c *Client) acquireUIMEvents(ctx context.Context, mask uint32) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()

	if c.uimEventRefs == nil {
		c.uimEventRefs = make(map[uint32]int)
	}
	oldMask := combinedUIMEventMask(c.uimEventRefs)
	c.uimEventRefs[mask]++
	newMask := combinedUIMEventMask(c.uimEventRefs)
	if oldMask == newMask {
		return nil
	}
	if err := c.registerEvents(ctx, newMask); err != nil {
		c.uimEventRefs[mask]--
		if c.uimEventRefs[mask] == 0 {
			delete(c.uimEventRefs, mask)
		}
		return err
	}
	return nil
}

func (c *Client) releaseUIMEvents(mask uint32) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.uimEventRefs[mask] == 0 {
		return
	}
	oldMask := combinedUIMEventMask(c.uimEventRefs)
	c.uimEventRefs[mask]--
	if c.uimEventRefs[mask] == 0 {
		delete(c.uimEventRefs, mask)
	}
	newMask := combinedUIMEventMask(c.uimEventRefs)
	if oldMask != newMask {
		_ = c.registerEvents(ctx, newMask)
	}
}

func combinedUIMEventMask(refs map[uint32]int) uint32 {
	var mask uint32
	for eventMask, count := range refs {
		if count > 0 {
			mask |= eventMask
		}
	}
	return mask
}

func (c *Client) sendMonitorRequest(ctx context.Context, id MessageID, tlvs tlv.TLVs) error {
	resp, err := c.request(ctx, id, tlvs)
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (c *Client) unregisterRefreshFiles(req RefreshRegisterRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	tlvs, err := refreshRegisterTLVs(req, false)
	if err != nil {
		return
	}
	_ = c.sendMonitorRequest(ctx, MessageRefreshRegister, tlvs)
}

func (c *Client) unregisterRefreshAll(session Session, aid []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	tlvs, err := refreshRegisterAllTLVs(session, aid, false)
	if err != nil {
		return
	}
	_ = c.sendMonitorRequest(ctx, MessageRefreshRegisterAll, tlvs)
}

func (c *Client) forwardRefreshEvents(
	ctx context.Context,
	cancel context.CancelFunc,
	indications <-chan Indication,
	out chan<- RefreshEvent,
	cleanup func(),
) {
	defer close(out)
	defer cancel()
	defer cleanup()

	for ind := range indications {
		var event RefreshEvent
		if err := event.UnmarshalTLVs(ind.TLVs); err != nil {
			return
		}
		switch {
		case event.Stage == RefreshStageWaitForOK:
			if err := c.allowRefresh(ctx, event); err != nil {
				return
			}
		case refreshNeedsImmediateCompletion(event):
			if err := c.completeRefresh(ctx, event); err != nil {
				return
			}
		}

		if !trySendRefreshEvent(ctx, out, event) {
			return
		}
	}
}

// trySendRefreshEvent drops only the user-facing event; protocol ACK work is
// completed before this point.
func trySendRefreshEvent(ctx context.Context, out chan<- RefreshEvent, event RefreshEvent) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	select {
	case out <- event:
	case <-ctx.Done():
		return false
	default:
	}
	return true
}

func refreshNeedsImmediateCompletion(event RefreshEvent) bool {
	if event.Stage != RefreshStageStart {
		return false
	}
	if event.Mode == RefreshModeFCN {
		return true
	}
	if !isNonProvisioningSession(event.Session) {
		return false
	}
	switch event.Mode {
	case RefreshModeInit,
		RefreshModeInitFCN,
		RefreshModeInitFullFCN,
		RefreshModeApplicationReset,
		RefreshMode3GReset:
		return true
	default:
		return false
	}
}

func isNonProvisioningSession(session Session) bool {
	switch session {
	case SessionNonProvisioningSlot1,
		SessionNonProvisioningSlot2,
		SessionNonProvisioningSlot3,
		SessionNonProvisioningSlot4,
		SessionNonProvisioningSlot5:
		return true
	default:
		return false
	}
}

func (c *Client) allowRefresh(ctx context.Context, event RefreshEvent) error {
	tlvs, err := refreshOKTLVs(event.Session, event.AID, true)
	if err != nil {
		return fmt.Errorf("encoding QMI UIM refresh OK request: %w", err)
	}
	if err := c.sendMonitorRequest(ctx, MessageRefreshOK, tlvs); err != nil {
		return fmt.Errorf("sending QMI UIM refresh OK request: %w", err)
	}
	return nil
}

func (c *Client) completeRefresh(ctx context.Context, event RefreshEvent) error {
	tlvs, err := refreshCompleteTLVs(event.Session, event.AID, true)
	if err != nil {
		return fmt.Errorf("encoding QMI UIM refresh complete request: %w", err)
	}
	if err := c.sendMonitorRequest(ctx, MessageRefreshComplete, tlvs); err != nil {
		return fmt.Errorf("sending QMI UIM refresh complete request: %w", err)
	}
	return nil
}

func registerEventsTLVs(mask uint32) tlv.TLVs {
	return tlv.TLVs{
		tlv.Uint(0x01, mask),
	}
}

func refreshOKTLVs(session Session, aid []byte, allowed bool) (tlv.TLVs, error) {
	sessionValue, err := putSessionValue(session, aid)
	if err != nil {
		return nil, err
	}
	return tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, []byte{boolByte(allowed)}),
	}, nil
}

func refreshCompleteTLVs(session Session, aid []byte, success bool) (tlv.TLVs, error) {
	sessionValue, err := putSessionValue(session, aid)
	if err != nil {
		return nil, err
	}
	return tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, []byte{boolByte(success)}),
	}, nil
}

func refreshRegisterTLVs(req RefreshRegisterRequest, register bool) (tlv.TLVs, error) {
	sessionValue, err := putSessionValue(req.Session, req.AID)
	if err != nil {
		return nil, err
	}

	info, err := encodeRefreshRegisterInfo(req.Files, register, req.VoteForInit)
	if err != nil {
		return nil, err
	}
	return tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, info),
	}, nil
}

func refreshRegisterAllTLVs(session Session, aid []byte, register bool) (tlv.TLVs, error) {
	sessionValue, err := putSessionValue(session, aid)
	if err != nil {
		return nil, err
	}
	flag := uint8(0)
	if register {
		flag = 1
	}
	return tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, []byte{flag}),
	}, nil
}

func encodeRefreshRegisterInfo(files []RefreshFile, register bool, voteForInit bool) ([]byte, error) {
	if len(files) > uimRefreshFilesMax {
		return nil, fmt.Errorf("file count %d exceeds %d", len(files), uimRefreshFilesMax)
	}

	registerFlag := uint8(0)
	if register {
		registerFlag = 1
	}
	initFlag := uint8(0)
	if voteForInit {
		initFlag = 1
	}

	value := []byte{registerFlag, initFlag}
	value = binary.LittleEndian.AppendUint16(value, uint16(len(files)))
	for _, file := range files {
		fileID, path, err := splitFilePath(file.Path)
		if err != nil {
			return nil, err
		}
		if len(path) > uimPathMaxLength {
			return nil, fmt.Errorf("encoding SIM path %X: QMI path length %d exceeds %d", file.Path, len(path), uimPathMaxLength)
		}
		value = binary.LittleEndian.AppendUint16(value, fileID)
		value = append(value, byte(len(path)))
		value = append(value, path...)
	}
	return value, nil
}

// UnmarshalTLVs parses a QMI UIM refresh event.
func (event *RefreshEvent) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*event = RefreshEvent{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return errors.New("reading refresh event: event TLV missing")
	}

	payload := newPayloadReader(value)
	*event = RefreshEvent{
		Stage:   RefreshStage(payload.Uint8()),
		Mode:    RefreshMode(payload.Uint8()),
		Session: Session(payload.Uint8()),
		AID:     payload.Bytes8(),
	}
	fileCount := payload.Uint16()
	if err := payload.Err(); err != nil {
		return fmt.Errorf("reading refresh event: %w", err)
	}

	event.Files = make([]RefreshFile, 0, fileCount)
	for range fileCount {
		fileID := payload.Uint16()
		path := payload.Bytes8()
		if err := payload.Err(); err != nil {
			return fmt.Errorf("reading refresh event: %w", err)
		}
		fullPath, err := joinQMIFilePath(fileID, path)
		if err != nil {
			return fmt.Errorf("reading refresh event: %w", err)
		}
		event.Files = append(event.Files, RefreshFile{
			FileID: fileID,
			Path:   fullPath,
		})
	}
	return nil
}

func joinQMIFilePath(fileID uint16, path []byte) ([]byte, error) {
	if len(path)%2 != 0 {
		return nil, fmt.Errorf("QMI path %X length must be an even number of bytes", path)
	}

	out := make([]byte, 0, len(path)+2)
	for i := 0; i < len(path); i += 2 {
		out = append(out, path[i+1], path[i])
	}
	out = binary.BigEndian.AppendUint16(out, fileID)
	return out, nil
}

func cloneRefreshRegisterRequest(req RefreshRegisterRequest) RefreshRegisterRequest {
	req.AID = slices.Clone(req.AID)
	req.Files = slices.Clone(req.Files)
	for i := range req.Files {
		req.Files[i].Path = slices.Clone(req.Files[i].Path)
	}
	return req
}
