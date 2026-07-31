package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	locAssistanceDataPartMax = 1024
	locServerURLMaxLength    = 256
)

// LOCServerType identifies an assisted-location server role.
type LOCServerType uint32

const (
	LOCServerUnknown LOCServerType = iota
	LOCServerCDMAPDE
	LOCServerCDMAMPC
	LOCServerUMTSSLP
	LOCServerCustomPDE
)

// LOCServerAddressType is a mask selecting server address forms.
type LOCServerAddressType uint8

const (
	LOCServerAddressNone LOCServerAddressType = 0
	LOCServerAddressIPv4 LOCServerAddressType = 1 << 0
	LOCServerAddressIPv6 LOCServerAddressType = 1 << 1
	LOCServerAddressURL  LOCServerAddressType = 1 << 2
)

// LOCPredictedOrbitsDataFormat identifies an assistance database format.
type LOCPredictedOrbitsDataFormat uint32

const LOCPredictedOrbitsDataFormatXTRA LOCPredictedOrbitsDataFormat = 0

// LOCIPv4Server is an IPv4 assisted-location endpoint.
type LOCIPv4Server struct {
	Address netip.Addr
	Port    uint16
}

// LOCIPv6Server is an IPv6 assisted-location endpoint.
type LOCIPv6Server struct {
	Address netip.Addr
	Port    uint32
}

// LOCServerConfig selects server fields to update.
type LOCServerConfig struct {
	Type LOCServerType
	IPv4 *LOCIPv4Server
	IPv6 *LOCIPv6Server
	URL  *string
}

// LOCServerQuery selects an assisted-location server and desired address
// forms. A nil AddressTypes lets the modem choose its default response.
type LOCServerQuery struct {
	Type         LOCServerType
	AddressTypes *LOCServerAddressType
}

// LOCAssistanceServer contains server fields returned by the modem.
type LOCAssistanceServer struct {
	Type      LOCServerType
	TypeKnown bool
	IPv4      *LOCIPv4Server
	IPv6      *LOCIPv6Server
	URL       *string
}

// LOCPredictedOrbitsSource contains the modem's assistance download limits
// and candidate servers.
type LOCPredictedOrbitsSource struct {
	MaxFileSize       uint32
	MaxPartSize       uint32
	AllowedSizesKnown bool
	Servers           []string
	ServersKnown      bool
}

// LOCAssistanceDataPart is one protocol-level assistance-data fragment. Part
// numbering is passed through unchanged because modem families differ in the
// convention they expect.
type LOCAssistanceDataPart struct {
	TotalSize  uint32
	TotalParts uint16
	PartNumber uint16
	Data       []byte
}

// LOCAssistanceDataPartResult acknowledges an injected fragment.
type LOCAssistanceDataPartResult struct {
	PartNumber      uint16
	PartNumberKnown bool
}

// LOCSetServerRequest encodes QMI LOC Set Server.
type LOCSetServerRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        LOCServerConfig
}

// Request converts the server update into LOC TLVs.
func (r LOCSetServerRequest) Request() (Request, error) {
	if err := validateLOCServerType(r.Config.Type); err != nil {
		return Request{}, fmt.Errorf("encoding QMI LOC server: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint32(r.Config.Type))}
	if r.Config.IPv4 != nil {
		value, err := r.Config.IPv4.MarshalBinary()
		if err != nil {
			return Request{}, fmt.Errorf("encoding QMI LOC server: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	}
	if r.Config.IPv6 != nil {
		value, err := r.Config.IPv6.MarshalBinary()
		if err != nil {
			return Request{}, fmt.Errorf("encoding QMI LOC server: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x11, value))
	}
	if r.Config.URL != nil {
		if len(*r.Config.URL) > locServerURLMaxLength {
			return Request{}, fmt.Errorf("encoding QMI LOC server: URL length %d exceeds %d", len(*r.Config.URL), locServerURLMaxLength)
		}
		tlvs = append(tlvs, tlv.Bytes(0x12, []byte(*r.Config.URL)))
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCSetServer,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCGetServerRequest encodes QMI LOC Get Server.
type LOCGetServerRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Query         LOCServerQuery
}

// Request converts the server query into LOC TLVs.
func (r LOCGetServerRequest) Request() (Request, error) {
	if err := validateLOCServerType(r.Query.Type); err != nil {
		return Request{}, fmt.Errorf("encoding QMI LOC server query: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint32(r.Query.Type))}
	if r.Query.AddressTypes != nil {
		if *r.Query.AddressTypes&^(LOCServerAddressIPv4|LOCServerAddressIPv6|LOCServerAddressURL) != 0 {
			return Request{}, fmt.Errorf("encoding QMI LOC server query: address type mask 0x%02X is invalid", *r.Query.AddressTypes)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*r.Query.AddressTypes)))
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCGetServer,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCGetPredictedOrbitsDataSourceRequest encodes QMI LOC Get Predicted Orbits
// Data Source.
type LOCGetPredictedOrbitsDataSourceRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a LOC request.
func (r LOCGetPredictedOrbitsDataSourceRequest) Request() Request {
	return locEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageLOCGetPredictedOrbitsDataSource)
}

// LOCInjectPredictedOrbitsDataRequest encodes one QMI LOC Inject Predicted
// Orbits Data fragment.
type LOCInjectPredictedOrbitsDataRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Part          LOCAssistanceDataPart
	Format        *LOCPredictedOrbitsDataFormat
}

// Request converts the assistance fragment into LOC TLVs.
func (r LOCInjectPredictedOrbitsDataRequest) Request() (Request, error) {
	tlvs, err := r.Part.MarshalTLVs()
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI LOC predicted-orbits data: %w", err)
	}
	if r.Format != nil {
		if *r.Format != LOCPredictedOrbitsDataFormatXTRA {
			return Request{}, fmt.Errorf("encoding QMI LOC predicted-orbits data: format %d is unsupported", *r.Format)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint32(*r.Format)))
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCInjectPredictedOrbitsData,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCInjectXTRADataRequest encodes one QMI LOC Inject XTRA Data fragment.
type LOCInjectXTRADataRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Part          LOCAssistanceDataPart
}

// Request converts the XTRA fragment into LOC TLVs.
func (r LOCInjectXTRADataRequest) Request() (Request, error) {
	tlvs, err := r.Part.MarshalTLVs()
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI LOC XTRA data: %w", err)
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCInjectXTRAData,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCGetServerIndication is the asynchronous Get Server result.
type LOCGetServerIndication struct {
	Result LOCIndicationResult
	Server LOCAssistanceServer
}

// UnmarshalTLVs parses a QMI LOC Get Server indication.
func (i *LOCGetServerIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCGetServerIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x02); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI LOC server: server type TLV length %d, want 4", len(value))
		}
		i.Server.Type = LOCServerType(binary.LittleEndian.Uint32(value))
		i.Server.TypeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		var server LOCIPv4Server
		if err := server.UnmarshalBinary(value); err != nil {
			return err
		}
		i.Server.IPv4 = &server
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		var server LOCIPv6Server
		if err := server.UnmarshalBinary(value); err != nil {
			return err
		}
		i.Server.IPv6 = &server
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) > locServerURLMaxLength {
			return fmt.Errorf("parsing QMI LOC server: URL length %d exceeds %d", len(value), locServerURLMaxLength)
		}
		url := string(value)
		i.Server.URL = &url
	}
	return nil
}

// LOCGetPredictedOrbitsDataSourceIndication is the asynchronous source query
// result.
type LOCGetPredictedOrbitsDataSourceIndication struct {
	Result LOCIndicationResult
	Source LOCPredictedOrbitsSource
}

// UnmarshalTLVs parses a QMI LOC Get Predicted Orbits Data Source indication.
func (i *LOCGetPredictedOrbitsDataSourceIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCGetPredictedOrbitsDataSourceIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI LOC predicted-orbits source: allowed-sizes TLV length %d, want 8", len(value))
		}
		i.Source.MaxFileSize = binary.LittleEndian.Uint32(value[:4])
		i.Source.MaxPartSize = binary.LittleEndian.Uint32(value[4:8])
		i.Source.AllowedSizesKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		servers, err := decodeLOCServerList(value)
		if err != nil {
			return err
		}
		i.Source.Servers = servers
		i.Source.ServersKnown = true
	}
	return nil
}

// LOCAssistanceDataPartIndication is an asynchronous fragment acknowledgement.
type LOCAssistanceDataPartIndication struct {
	Result LOCIndicationResult
	Part   LOCAssistanceDataPartResult
}

// UnmarshalTLVs parses an assistance-data fragment acknowledgement.
func (i *LOCAssistanceDataPartIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCAssistanceDataPartIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI LOC assistance-data result: part number TLV length %d, want 2", len(value))
		}
		i.Part.PartNumber = binary.LittleEndian.Uint16(value)
		i.Part.PartNumberKnown = true
	}
	return nil
}

// LOCSetServer updates an assisted-location server and waits for the modem's
// asynchronous completion status.
func (c *Client) LOCSetServer(ctx context.Context, config LOCServerConfig) error {
	req, err := (LOCSetServerRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.locOperation(ctx, req); err != nil {
		return fmt.Errorf("setting QMI LOC server: %w", err)
	}
	return nil
}

// LOCServer returns an assisted-location server selected by the query.
func (c *Client) LOCServer(ctx context.Context, query LOCServerQuery) (LOCAssistanceServer, error) {
	req, err := (LOCGetServerRequest{Timeout: DefaultRequestTimeout, Query: query}).Request()
	if err != nil {
		return LOCAssistanceServer{}, err
	}
	indicationTLVs, err := c.locSingleIndication(ctx, req)
	if err != nil {
		return LOCAssistanceServer{}, fmt.Errorf("querying QMI LOC server: %w", err)
	}
	var indication LOCGetServerIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return LOCAssistanceServer{}, err
	}
	if err := indication.Result.Err(); err != nil {
		return LOCAssistanceServer{}, fmt.Errorf("querying QMI LOC server: %w", err)
	}
	return indication.Server, nil
}

// LOCPredictedOrbitsDataSource returns assistance download limits and server
// candidates.
func (c *Client) LOCPredictedOrbitsDataSource(ctx context.Context) (LOCPredictedOrbitsSource, error) {
	req := LOCGetPredictedOrbitsDataSourceRequest{Timeout: DefaultRequestTimeout}.Request()
	indicationTLVs, err := c.locSingleIndication(ctx, req)
	if err != nil {
		return LOCPredictedOrbitsSource{}, fmt.Errorf("querying QMI LOC predicted-orbits source: %w", err)
	}
	var indication LOCGetPredictedOrbitsDataSourceIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return LOCPredictedOrbitsSource{}, err
	}
	if err := indication.Result.Err(); err != nil {
		return LOCPredictedOrbitsSource{}, fmt.Errorf("querying QMI LOC predicted-orbits source: %w", err)
	}
	return indication.Source, nil
}

// LOCInjectPredictedOrbitsPart injects one predicted-orbits fragment and waits
// for its asynchronous acknowledgement.
func (c *Client) LOCInjectPredictedOrbitsPart(
	ctx context.Context,
	part LOCAssistanceDataPart,
	format *LOCPredictedOrbitsDataFormat,
) (LOCAssistanceDataPartResult, error) {
	req, err := (LOCInjectPredictedOrbitsDataRequest{
		Timeout: DefaultRequestTimeout,
		Part:    part,
		Format:  format,
	}).Request()
	if err != nil {
		return LOCAssistanceDataPartResult{}, fmt.Errorf("injecting QMI LOC predicted-orbits data: %w", err)
	}
	result, err := c.locInjectAssistancePart(ctx, req)
	if err != nil {
		return LOCAssistanceDataPartResult{}, fmt.Errorf("injecting QMI LOC predicted-orbits data: %w", err)
	}
	return result, nil
}

// LOCInjectXTRAPart injects one XTRA fragment and waits for its asynchronous
// acknowledgement.
func (c *Client) LOCInjectXTRAPart(ctx context.Context, part LOCAssistanceDataPart) (LOCAssistanceDataPartResult, error) {
	req, err := (LOCInjectXTRADataRequest{Timeout: DefaultRequestTimeout, Part: part}).Request()
	if err != nil {
		return LOCAssistanceDataPartResult{}, fmt.Errorf("injecting QMI LOC XTRA data: %w", err)
	}
	result, err := c.locInjectAssistancePart(ctx, req)
	if err != nil {
		return LOCAssistanceDataPartResult{}, fmt.Errorf("injecting QMI LOC XTRA data: %w", err)
	}
	return result, nil
}

func (c *Client) locInjectAssistancePart(ctx context.Context, req Request) (LOCAssistanceDataPartResult, error) {
	indicationTLVs, err := c.locSingleIndication(ctx, req)
	if err != nil {
		return LOCAssistanceDataPartResult{}, err
	}
	var indication LOCAssistanceDataPartIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return LOCAssistanceDataPartResult{}, err
	}
	if err := indication.Result.Err(); err != nil {
		return LOCAssistanceDataPartResult{}, err
	}
	return indication.Part, nil
}

func validateLOCServerType(serverType LOCServerType) error {
	if serverType > LOCServerCustomPDE {
		return fmt.Errorf("server type %d is out of range", serverType)
	}
	return nil
}

// MarshalBinary encodes a QMI LOC IPv4 server.
func (server LOCIPv4Server) MarshalBinary() ([]byte, error) {
	address := server.Address.Unmap()
	if !address.Is4() {
		return nil, fmt.Errorf("server address %q is not IPv4", server.Address)
	}
	addressBytes := address.As4()
	value := append([]byte(nil), addressBytes[:]...)
	value = binary.LittleEndian.AppendUint16(value, server.Port)
	return value, nil
}

// UnmarshalBinary decodes a QMI LOC IPv4 server.
func (server *LOCIPv4Server) UnmarshalBinary(value []byte) error {
	if len(value) != 6 {
		return fmt.Errorf("parsing QMI LOC server: IPv4 TLV length %d, want 6", len(value))
	}
	*server = LOCIPv4Server{
		Address: netip.AddrFrom4([4]byte(value[:4])),
		Port:    binary.LittleEndian.Uint16(value[4:6]),
	}
	return nil
}

// MarshalBinary encodes a QMI LOC IPv6 server.
func (server LOCIPv6Server) MarshalBinary() ([]byte, error) {
	address := server.Address.Unmap()
	if !address.Is6() {
		return nil, fmt.Errorf("server address %q is not IPv6", server.Address)
	}
	addressBytes := address.As16()
	value := append([]byte(nil), addressBytes[:]...)
	value = binary.LittleEndian.AppendUint32(value, server.Port)
	return value, nil
}

// UnmarshalBinary decodes a QMI LOC IPv6 server.
func (server *LOCIPv6Server) UnmarshalBinary(value []byte) error {
	if len(value) != 20 {
		return fmt.Errorf("parsing QMI LOC server: IPv6 TLV length %d, want 20", len(value))
	}
	*server = LOCIPv6Server{
		Address: netip.AddrFrom16([16]byte(value[:16])),
		Port:    binary.LittleEndian.Uint32(value[16:20]),
	}
	return nil
}

// MarshalTLVs encodes one LOC assistance-data fragment.
func (part LOCAssistanceDataPart) MarshalTLVs() (tlv.TLVs, error) {
	if len(part.Data) > locAssistanceDataPartMax {
		return nil, fmt.Errorf("part data length %d exceeds %d", len(part.Data), locAssistanceDataPartMax)
	}
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(part.Data)))
	value = append(value, part.Data...)
	return tlv.TLVs{
		tlv.Uint(0x01, part.TotalSize),
		tlv.Uint(0x02, part.TotalParts),
		tlv.Uint(0x03, part.PartNumber),
		tlv.Bytes(0x04, value),
	}, nil
}

func decodeLOCServerList(value []byte) ([]string, error) {
	servers, err := decodeLOCStringList(value)
	if err != nil {
		return nil, fmt.Errorf("parsing QMI LOC predicted-orbits source: %w", err)
	}
	return servers, nil
}

func decodeLOCStringList(value []byte) ([]string, error) {
	if len(value) < 1 {
		return nil, errors.New("server count is missing")
	}
	count := int(value[0])
	servers := make([]string, count)
	offset := 1
	for index := range count {
		if offset >= len(value) {
			return nil, fmt.Errorf("server %d length is missing", index)
		}
		length := int(value[offset])
		offset++
		if len(value)-offset < length {
			return nil, fmt.Errorf("server %d is truncated", index)
		}
		servers[index] = string(value[offset : offset+length])
		offset += length
	}
	if offset != len(value) {
		return nil, fmt.Errorf("%d trailing bytes", len(value)-offset)
	}
	return servers, nil
}
