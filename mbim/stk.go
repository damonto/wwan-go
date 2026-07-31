package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type STKPACProfile byte

const (
	STKPACNotHandledByFunctionCannotBeHandledByHost STKPACProfile = iota
	STKPACNotHandledByFunctionMayBeHandledByHost
	STKPACHandledByFunctionOnlyTransparentToHost
	STKPACHandledByFunctionNotificationToHostPossible
	STKPACHandledByFunctionNotificationsToHostEnabled
	STKPACHandledByFunctionCanBeOverriddenByHost
	STKPACHandledByHostFunctionNotAbleToHandle
	STKPACHandledByHostFunctionAbleToHandle
)

type STKPACType uint32

const (
	STKPACTypeProactiveCommand STKPACType = iota
	STKPACTypeNotification
)

const (
	stkPACHostControlLength  = 32
	stkPACSupportLength      = 256
	stkEnvelopeSupportLength = 32
)

type STKPACQueryRequest struct {
	TransactionID uint32
	Response      *STKPACInfo
}

func (r *STKPACQueryRequest) Request() *Request {
	r.Response = new(STKPACInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKPAC,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type STKPACSetRequest struct {
	TransactionID  uint32
	PacHostControl []byte
	Response       *STKPACInfo
}

func (r *STKPACSetRequest) Request() *Request {
	r.Response = new(STKPACInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKPAC,
			CommandTypeSet,
			r.PacHostControl,
		),
		Response: r.Response,
	}
}

type STKPACInfo struct {
	PacSupport [stkPACSupportLength]STKPACProfile
}

func (r *STKPACInfo) UnmarshalBinary(data []byte) error {
	if len(data) != stkPACSupportLength {
		return fmt.Errorf("parsing MBIM STK PAC info: payload length is %d, want %d", len(data), stkPACSupportLength)
	}
	var pacSupport [stkPACSupportLength]STKPACProfile
	for i, value := range data[:stkPACSupportLength] {
		profile := STKPACProfile(value)
		if profile > STKPACHandledByHostFunctionAbleToHandle {
			return fmt.Errorf("parsing MBIM STK PAC info: profile %d at index %d is reserved", profile, i)
		}
		pacSupport[i] = profile
	}
	r.PacSupport = pacSupport
	return nil
}

type STKPAC struct {
	Type    STKPACType
	Command []byte
}

func (r *STKPAC) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM STK PAC: payload is truncated")
	}
	pacType := STKPACType(binary.LittleEndian.Uint32(data[:4]))
	if pacType > STKPACTypeNotification {
		return fmt.Errorf("parsing MBIM STK PAC: type %d is reserved", pacType)
	}
	r.Type = pacType
	r.Command = slices.Clone(data[4:])
	return nil
}

type STKTerminalResponseRequest struct {
	TransactionID uint32
	Data          []byte
	Response      *STKTerminalResponseInfo
}

func (r *STKTerminalResponseRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(len(r.Data)))
	data = append(data, r.Data...)

	r.Response = new(STKTerminalResponseInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKTerminalResponse,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type STKTerminalResponseInfo struct {
	ResultData  []byte
	StatusWords uint32
}

func (r *STKTerminalResponseInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 12 {
		return errors.New("parsing MBIM STK terminal response: payload is truncated")
	}
	result, err := byteArrayRef(data, data, 0, 12)
	if err != nil {
		return fmt.Errorf("parsing MBIM STK terminal response data: %w", err)
	}
	statusWords := binary.LittleEndian.Uint32(data[8:12])
	if statusWords > 0xffff {
		return fmt.Errorf("parsing MBIM STK terminal response: status words %#x exceed two bytes", statusWords)
	}
	*r = STKTerminalResponseInfo{ResultData: result, StatusWords: statusWords}
	return nil
}

type STKEnvelopeQueryRequest struct {
	TransactionID uint32
	Response      *STKEnvelopeInfo
}

func (r *STKEnvelopeQueryRequest) Request() *Request {
	r.Response = new(STKEnvelopeInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKEnvelope,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type STKEnvelopeInfo struct {
	EnvelopeSupport [stkEnvelopeSupportLength]byte
}

func (r *STKEnvelopeInfo) Supports(tag byte) bool {
	mask := byte(1 << (tag % 8))
	return r.EnvelopeSupport[int(tag)/8]&mask != 0
}

func (r *STKEnvelopeInfo) UnmarshalBinary(data []byte) error {
	if len(data) != stkEnvelopeSupportLength {
		return fmt.Errorf("parsing MBIM STK envelope info: payload length is %d, want %d", len(data), stkEnvelopeSupportLength)
	}
	copy(r.EnvelopeSupport[:], data[:stkEnvelopeSupportLength])
	return nil
}

type STKEnvelopeRequest struct {
	TransactionID uint32
	Data          []byte
	Response      *STKEnvelopeResponse
}

func (r *STKEnvelopeRequest) Request() *Request {
	r.Response = new(STKEnvelopeResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKEnvelope,
			CommandTypeSet,
			r.Data,
		),
		Response: r.Response,
	}
}

type STKEnvelopeResponse struct{}

func (r *STKEnvelopeResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("parsing MBIM STK envelope response: length %d, want 0", len(data))
	}
	return nil
}

func (c *Client) QuerySTKPAC(ctx context.Context) (STKPACInfo, error) {
	request := STKPACQueryRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return STKPACInfo{}, fmt.Errorf("querying MBIM STK PAC: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetSTKPAC(ctx context.Context, pacHostControl []byte) (STKPACInfo, error) {
	if len(pacHostControl) != stkPACHostControlLength {
		return STKPACInfo{}, fmt.Errorf("setting MBIM STK PAC: host control length %d, want %d", len(pacHostControl), stkPACHostControlLength)
	}

	request := STKPACSetRequest{
		TransactionID:  c.nextTransactionID(),
		PacHostControl: slices.Clone(pacHostControl),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return STKPACInfo{}, fmt.Errorf("setting MBIM STK PAC: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) ReadSTKPAC(ctx context.Context) (STKPAC, error) {
	indication, err := c.NextIndication(ctx, ServiceSTK, CIDSTKPAC)
	if err != nil {
		return STKPAC{}, fmt.Errorf("reading MBIM STK PAC: %w", err)
	}
	var pac STKPAC
	if err := pac.UnmarshalBinary(indication.InformationBuffer); err != nil {
		return STKPAC{}, fmt.Errorf("reading MBIM STK PAC: %w", err)
	}
	return pac, nil
}

// WatchSTKPAC streams STK proactive command notifications until ctx is done.
func (c *Client) WatchSTKPAC(ctx context.Context) (<-chan STKPAC, error) {
	results, err := c.WatchSTKPACResults(ctx)
	if err != nil {
		return nil, err
	}
	return watchValues(ctx, results), nil
}

// WatchSTKPACResults streams STK proactive command notifications and reports
// receiver or payload errors through the terminal result.
func (c *Client) WatchSTKPACResults(ctx context.Context) (<-chan WatchResult[STKPAC], error) {
	indications, err := c.WatchIndicationResults(ctx, ServiceSTK, CIDSTKPAC)
	if err != nil {
		return nil, fmt.Errorf("watching MBIM STK PAC: %w", err)
	}
	return watchDecoded(ctx, indications, "watching MBIM STK PAC", func(data []byte) (STKPAC, error) {
		var pac STKPAC
		if err := pac.UnmarshalBinary(data); err != nil {
			return STKPAC{}, err
		}
		return pac, nil
	}), nil
}

func (c *Client) STKTerminalResponse(ctx context.Context, data []byte) (STKTerminalResponseInfo, error) {
	if len(data) == 0 {
		return STKTerminalResponseInfo{}, errors.New("sending MBIM STK terminal response: response is empty")
	}

	request := STKTerminalResponseRequest{
		TransactionID: c.nextTransactionID(),
		Data:          slices.Clone(data),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return STKTerminalResponseInfo{}, fmt.Errorf("sending MBIM STK terminal response: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) QuerySTKEnvelopeSupport(ctx context.Context) (STKEnvelopeInfo, error) {
	info, err := c.querySTKEnvelopeSupport(ctx)
	if err != nil {
		return STKEnvelopeInfo{}, err
	}
	c.setEnvelopeSupport(info)
	return info, nil
}

func (c *Client) STKEnvelope(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return errors.New("running MBIM STK envelope: envelope is empty")
	}

	info, err := c.envelopeSupportInfo(ctx)
	if err != nil {
		return fmt.Errorf("running MBIM STK envelope: %w", err)
	}
	if !info.Supports(data[0]) {
		return fmt.Errorf("running MBIM STK envelope: envelope tag 0x%02X is not expected by function", data[0])
	}

	request := STKEnvelopeRequest{
		TransactionID: c.nextTransactionID(),
		Data:          slices.Clone(data),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return fmt.Errorf("running MBIM STK envelope: %w", err)
	}
	return nil
}

func (c *Client) querySTKEnvelopeSupport(ctx context.Context) (STKEnvelopeInfo, error) {
	request := STKEnvelopeQueryRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return STKEnvelopeInfo{}, fmt.Errorf("querying MBIM STK envelope support: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) envelopeSupportInfo(ctx context.Context) (STKEnvelopeInfo, error) {
	c.ensureState()
	c.mu.Lock()
	if c.envelopeSupport != nil {
		info := *c.envelopeSupport
		c.mu.Unlock()
		return info, nil
	}
	c.mu.Unlock()

	info, err := c.querySTKEnvelopeSupport(ctx)
	if err != nil {
		return STKEnvelopeInfo{}, err
	}
	c.setEnvelopeSupport(info)
	return info, nil
}

func (c *Client) setEnvelopeSupport(info STKEnvelopeInfo) {
	c.ensureState()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.envelopeSupport = new(STKEnvelopeInfo)
	*c.envelopeSupport = info
}

func (c *Client) clearEnvelopeSupport() {
	c.ensureState()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.envelopeSupport = nil
}
