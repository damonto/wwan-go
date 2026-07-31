package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

const (
	providerStateMask = ProviderStateHome | ProviderStateForbidden | ProviderStatePreferred |
		ProviderStateVisible | ProviderStateRegistered | ProviderStatePreferredMultiCarrier
	cellularClassMask = CellularClassGSM | CellularClassCDMA
)

type ProviderState uint32

const (
	ProviderStateUnknown               ProviderState = 0
	ProviderStateHome                  ProviderState = 1 << 0
	ProviderStateForbidden             ProviderState = 1 << 1
	ProviderStatePreferred             ProviderState = 1 << 2
	ProviderStateVisible               ProviderState = 1 << 3
	ProviderStateRegistered            ProviderState = 1 << 4
	ProviderStatePreferredMultiCarrier ProviderState = 1 << 5
)

type Provider struct {
	ID            string
	State         ProviderState
	Name          string
	CellularClass CellularClass
	RSSI          uint32
	ErrorRate     uint32
}

type Providers struct {
	Providers []Provider
}

type VisibleProvidersAction uint32

const (
	VisibleProvidersActionFullScan       VisibleProvidersAction = 0
	VisibleProvidersActionRestrictedScan VisibleProvidersAction = 1
)

func (p Provider) MarshalBinary() ([]byte, error) {
	if err := validateProvider(p); err != nil {
		return nil, fmt.Errorf("encoding MBIM provider: %w", err)
	}
	return p.marshalBinary(), nil
}

func (p Provider) marshalBinary() []byte {
	providerID := utf16Bytes(p.ID)
	providerName := utf16Bytes(p.Name)
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data[8:12], uint32(p.State))
	binary.LittleEndian.PutUint32(data[20:24], uint32(p.CellularClass))
	binary.LittleEndian.PutUint32(data[24:28], p.RSSI)
	binary.LittleEndian.PutUint32(data[28:32], p.ErrorRate)
	data = appendRefValue(data, 0, providerID)
	data = appendRefValue(data, 12, providerName)
	return data
}

func (p *Provider) UnmarshalBinary(data []byte) error {
	if len(data) < 32 {
		return errors.New("parsing MBIM provider: payload is truncated")
	}
	providerIDRef, err := readOffsetSizeRef(data, 0)
	if err != nil {
		return fmt.Errorf("parsing MBIM provider ID: %w", err)
	}
	providerNameRef, err := readOffsetSizeRef(data, 12)
	if err != nil {
		return fmt.Errorf("parsing MBIM provider name: %w", err)
	}
	if providerIDRef.size > 12 {
		return fmt.Errorf("parsing MBIM provider ID: size %d exceeds 12 bytes", providerIDRef.size)
	}
	if providerNameRef.size > 40 {
		return fmt.Errorf("parsing MBIM provider name: size %d exceeds 40 bytes", providerNameRef.size)
	}
	if err := validateDataBufferRefs(data, 32, []valueRef{providerIDRef, providerNameRef}); err != nil {
		return fmt.Errorf("parsing MBIM provider data buffer: %w", err)
	}
	if err := validateUTF16Refs(data, []valueRef{providerIDRef, providerNameRef}); err != nil {
		return fmt.Errorf("parsing MBIM provider strings: %w", err)
	}
	providerID, err := utf16String(data, providerIDRef)
	if err != nil {
		return fmt.Errorf("parsing MBIM provider ID: %w", err)
	}
	providerName, err := utf16String(data, providerNameRef)
	if err != nil {
		return fmt.Errorf("parsing MBIM provider name: %w", err)
	}
	provider := Provider{
		ID:            providerID,
		State:         ProviderState(binary.LittleEndian.Uint32(data[8:12])),
		Name:          providerName,
		CellularClass: CellularClass(binary.LittleEndian.Uint32(data[20:24])),
		RSSI:          binary.LittleEndian.Uint32(data[24:28]),
		ErrorRate:     binary.LittleEndian.Uint32(data[28:32]),
	}
	if err := validateProvider(provider); err != nil {
		return fmt.Errorf("parsing MBIM provider: %w", err)
	}
	*p = provider
	return nil
}

func (p Providers) MarshalBinary() ([]byte, error) {
	if err := validateProviders(p.Providers); err != nil {
		return nil, fmt.Errorf("encoding MBIM providers: %w", err)
	}
	return p.marshalBinary(), nil
}

func validateProvider(provider Provider) error {
	if err := validateProviderID(provider.ID); err != nil {
		return err
	}
	if size := len(utf16Bytes(provider.Name)); size > 40 {
		return fmt.Errorf("provider name length %d exceeds 40 bytes", size)
	}
	if provider.State&^providerStateMask != 0 {
		return fmt.Errorf("provider state %#x contains reserved bits", provider.State)
	}
	if !validCellularClass(provider.CellularClass) {
		return fmt.Errorf("cellular class %#x contains reserved bits", provider.CellularClass)
	}
	if !validRSSI(provider.RSSI) {
		return fmt.Errorf("RSSI %d is outside 0..31 and is not the unknown value 99", provider.RSSI)
	}
	if !validErrorRate(provider.ErrorRate) {
		return fmt.Errorf("error rate %d is outside 0..7 and is not the unknown value 99", provider.ErrorRate)
	}
	return nil
}

func validateProviderID(providerID string) error {
	if size := len(utf16Bytes(providerID)); size > 12 {
		return fmt.Errorf("provider ID length %d exceeds 12 bytes", size)
	}
	for _, digit := range providerID {
		if digit < '0' || digit > '9' {
			return fmt.Errorf("provider ID %q contains a non-decimal character", providerID)
		}
	}
	return nil
}

func validCellularClass(class CellularClass) bool {
	return class&^cellularClassMask == 0
}

func validRSSI(rssi uint32) bool {
	return rssi <= 31 || rssi == 99
}

func validErrorRate(errorRate uint32) bool {
	return errorRate <= 7 || errorRate == 99
}

func validateProviders(providers []Provider) error {
	for i, provider := range providers {
		if err := validateProvider(provider); err != nil {
			return fmt.Errorf("provider %d: %w", i, err)
		}
	}
	return nil
}

func (p Providers) marshalBinary() []byte {
	elements := make([][]byte, len(p.Providers))
	for i, provider := range p.Providers {
		elements[i] = provider.marshalBinary()
	}
	header := binary.LittleEndian.AppendUint32(nil, uint32(len(elements)))
	return appendOffsetSizeElements(header, elements)
}

func (p *Providers) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM providers: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[0:4])
	refs, err := offsetSizeRefs(data, 4, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM providers: %w", err)
	}
	providers := make([]Provider, count)
	for i, ref := range refs {
		if err := providers[i].UnmarshalBinary(data[ref.offset : ref.offset+ref.size]); err != nil {
			return fmt.Errorf("parsing MBIM provider %d: %w", i, err)
		}
	}
	p.Providers = providers
	return nil
}

type HomeProviderRequest struct {
	TransactionID uint32
	Response      *Provider
}

func (r *HomeProviderRequest) Request() *Request {
	r.Response = new(Provider)
	return singleProviderRequest(r.TransactionID, command(ServiceBasicConnect, CIDHomeProvider, CommandTypeQuery, nil), r.Response)
}

type HomeProviderSetRequest struct {
	TransactionID uint32
	Provider      Provider
	Response      *Provider
}

func (r *HomeProviderSetRequest) Request() *Request {
	data, err := r.Provider.MarshalBinary()
	r.Response = new(Provider)
	return singleProviderRequest(
		r.TransactionID,
		commandWithError(ServiceBasicConnect, CIDHomeProvider, CommandTypeSet, data, err),
		r.Response,
	)
}

type PreferredProvidersRequest struct {
	TransactionID uint32
	Response      *Providers
}

func (r *PreferredProvidersRequest) Request() *Request {
	r.Response = new(Providers)
	return providersRequest(r.TransactionID, command(ServiceBasicConnect, CIDPreferredProviders, CommandTypeQuery, nil), r.Response)
}

type PreferredProvidersSetRequest struct {
	TransactionID uint32
	Providers     []Provider
	Response      *Providers
}

func (r *PreferredProvidersSetRequest) Request() *Request {
	data, err := (Providers{Providers: r.Providers}).MarshalBinary()
	r.Response = new(Providers)
	return providersRequest(
		r.TransactionID,
		commandWithError(ServiceBasicConnect, CIDPreferredProviders, CommandTypeSet, data, err),
		r.Response,
	)
}

type VisibleProvidersRequest struct {
	TransactionID uint32
	Action        VisibleProvidersAction
	Response      *Providers
}

func (r *VisibleProvidersRequest) Request() *Request {
	data, err := r.Action.MarshalBinary()
	r.Response = new(Providers)
	return providersRequest(
		r.TransactionID,
		commandWithError(ServiceBasicConnect, CIDVisibleProviders, CommandTypeQuery, data, err),
		r.Response,
	)
}

func (a VisibleProvidersAction) MarshalBinary() ([]byte, error) {
	if a > VisibleProvidersActionRestrictedScan {
		return nil, fmt.Errorf("encoding MBIM visible providers action: value %d is outside 0..%d", a, VisibleProvidersActionRestrictedScan)
	}
	return binary.LittleEndian.AppendUint32(nil, uint32(a)), nil
}

type MulticarrierProvidersRequest struct {
	TransactionID uint32
	Response      *Providers
}

func (r *MulticarrierProvidersRequest) Request() *Request {
	r.Response = new(Providers)
	return providersRequest(r.TransactionID, command(ServiceBasicConnect, CIDMulticarrierProviders, CommandTypeQuery, nil), r.Response)
}

type MulticarrierProvidersSetRequest struct {
	TransactionID uint32
	Providers     []Provider
	Response      *Providers
}

func (r *MulticarrierProvidersSetRequest) Request() *Request {
	data, err := (Providers{Providers: r.Providers}).MarshalBinary()
	r.Response = new(Providers)
	return providersRequest(
		r.TransactionID,
		commandWithError(ServiceBasicConnect, CIDMulticarrierProviders, CommandTypeSet, data, err),
		r.Response,
	)
}

func providersRequest(transactionID uint32, command *Command, response *Providers) *Request {
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: transactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command:       command,
		Response:      response,
	}
}

func singleProviderRequest(transactionID uint32, command *Command, response *Provider) *Request {
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: transactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command:       command,
		Response:      response,
	}
}

func (c *Client) HomeProvider(ctx context.Context) (Provider, error) {
	request := HomeProviderRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return Provider{}, fmt.Errorf("reading MBIM home provider: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetHomeProvider(ctx context.Context, provider Provider) (Provider, error) {
	if err := validateProvider(provider); err != nil {
		return Provider{}, fmt.Errorf("setting MBIM home provider: %w", err)
	}
	request := HomeProviderSetRequest{TransactionID: c.nextTransactionID(), Provider: provider}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return Provider{}, fmt.Errorf("setting MBIM home provider: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) PreferredProviders(ctx context.Context) ([]Provider, error) {
	request := PreferredProvidersRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM preferred providers: %w", err)
	}
	return slices.Clone(request.Response.Providers), nil
}

func (c *Client) SetPreferredProviders(ctx context.Context, providers []Provider) ([]Provider, error) {
	if err := validateProviders(providers); err != nil {
		return nil, fmt.Errorf("setting MBIM preferred providers: %w", err)
	}
	request := PreferredProvidersSetRequest{TransactionID: c.nextTransactionID(), Providers: slices.Clone(providers)}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("setting MBIM preferred providers: %w", err)
	}
	return slices.Clone(request.Response.Providers), nil
}

func (c *Client) VisibleProviders(ctx context.Context, action VisibleProvidersAction) ([]Provider, error) {
	if action > VisibleProvidersActionRestrictedScan {
		return nil, fmt.Errorf("reading MBIM visible providers: action %d is outside 0..%d", action, VisibleProvidersActionRestrictedScan)
	}
	request := VisibleProvidersRequest{TransactionID: c.nextTransactionID(), Action: action}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM visible providers: %w", err)
	}
	return slices.Clone(request.Response.Providers), nil
}

func (c *Client) MulticarrierProviders(ctx context.Context) ([]Provider, error) {
	request := MulticarrierProvidersRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM multicarrier providers: %w", err)
	}
	return slices.Clone(request.Response.Providers), nil
}

func (c *Client) SetMulticarrierProviders(ctx context.Context, providers []Provider) ([]Provider, error) {
	if err := validateProviders(providers); err != nil {
		return nil, fmt.Errorf("setting MBIM multicarrier providers: %w", err)
	}
	request := MulticarrierProvidersSetRequest{TransactionID: c.nextTransactionID(), Providers: slices.Clone(providers)}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("setting MBIM multicarrier providers: %w", err)
	}
	return slices.Clone(request.Response.Providers), nil
}
