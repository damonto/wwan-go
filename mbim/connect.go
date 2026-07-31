package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"
)

const mbimConnectSetResponseTimeout = 198 * time.Second

type ActivationCommand uint32

const (
	ActivationCommandDeactivate ActivationCommand = iota
	ActivationCommandActivate
)

type ActivationOption uint32

const (
	ActivationOptionDefault ActivationOption = iota
	ActivationOptionPerNonDefaultURSPRules
	ActivationOptionPerDefaultURSPRule
	ActivationOptionPerURSPRules
)

type ActivationState uint32

const (
	ActivationStateUnknown ActivationState = iota
	ActivationStateActivated
	ActivationStateActivating
	ActivationStateDeactivated
	ActivationStateDeactivating
)

type VoiceCallState uint32

const (
	VoiceCallStateNone VoiceCallState = iota
	VoiceCallStateInProgress
	VoiceCallStateHangUp
)

type AccessMediaType uint32

const (
	AccessMediaTypeNone AccessMediaType = iota
	AccessMediaType3GPP
	AccessMediaType3GPPPreferred
)

type ConnectRequest struct {
	TransactionID     uint32
	MBIMExVersion     uint16
	Timeout           time.Duration
	SessionID         SessionID
	ActivationCommand ActivationCommand
	AccessString      string
	UserName          string
	Password          string
	Compression       Compression
	AuthProtocol      AuthProtocol
	IPType            ContextIPType
	ContextType       ContextType
	MediaPreference   AccessMediaType
	ActivationOption  ActivationOption
	SNSSAI            *SNSSAI
	TLVs              TLVs
	Response          *ConnectInfo
}

func (r *ConnectRequest) Request() *Request {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = mbimConnectSetResponseTimeout
	}
	data, err := r.marshalCommandData()

	r.Response = &ConnectInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       timeout,
		Command: commandWithError(
			ServiceBasicConnect,
			CIDConnect,
			CommandTypeSet,
			data,
			err,
		),
		Response: r.Response,
	}
}

func (r *ConnectRequest) marshalCommandData() ([]byte, error) {
	cfg := ConnectConfig{
		Timeout:           r.Timeout,
		SessionID:         r.SessionID,
		ActivationCommand: r.ActivationCommand,
		AccessString:      r.AccessString,
		UserName:          r.UserName,
		Password:          r.Password,
		Compression:       r.Compression,
		AuthProtocol:      r.AuthProtocol,
		IPType:            r.IPType,
		ContextType:       r.ContextType,
		MediaPreference:   r.MediaPreference,
		ActivationOption:  r.ActivationOption,
		SNSSAI:            r.SNSSAI,
		TLVs:              r.TLVs,
	}
	if err := validateConnectConfig(cfg, r.MBIMExVersion); err != nil {
		return nil, err
	}
	return r.connectSetData(), nil
}

type ConnectQueryRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SessionID     SessionID
	Response      *ConnectInfo
}

func (r *ConnectQueryRequest) Request() *Request {
	data := make([]byte, 36)
	if r.MBIMExVersion >= mbimExVersion30 {
		data = make([]byte, 4)
	}
	binary.LittleEndian.PutUint32(data[:4], uint32(r.SessionID))

	r.Response = &ConnectInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDConnect,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type ConnectInfo struct {
	MBIMExVersion     uint16
	SessionID         SessionID
	ActivationState   ActivationState
	VoiceCallState    VoiceCallState
	IPType            ContextIPType
	ContextType       ContextType
	NwError           uint32
	AccessMedia       AccessMediaType
	AccessString      string
	SNSSAI            *SNSSAI
	TrafficParameters *TrafficParameters
	MatchingSessionID *SessionID
	TLVs              TLVs
	PCO               []ProtocolConfigurationOptions
	PCSCFIPs          []net.IP
	DNSIPs            []net.IP
	IPv4LinkMTU       uint16
	IPv4LinkMTUKnown  bool
}

type ConnectConfig struct {
	Timeout            time.Duration
	PDUActivationCount uint32
	SessionID          SessionID
	ActivationCommand  ActivationCommand
	AccessString       string
	UserName           string
	Password           string
	Compression        Compression
	AuthProtocol       AuthProtocol
	IPType             ContextIPType
	ContextType        ContextType
	MediaPreference    AccessMediaType
	ActivationOption   ActivationOption
	SNSSAI             *SNSSAI
	TLVs               TLVs
}

func (r *ConnectInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 36 {
		return errors.New("parsing MBIM connect info: payload is truncated")
	}

	version := r.MBIMExVersion
	if version == 0 && len(data) > 36 {
		// Older callers constructed ConnectInfo directly, before it carried the
		// negotiated version. Preserve their ability to decode MBIMEx 3 payloads.
		version = mbimExVersion30
	}
	result := ConnectInfo{
		MBIMExVersion:   version,
		SessionID:       SessionID(binary.LittleEndian.Uint32(data[:4])),
		ActivationState: ActivationState(binary.LittleEndian.Uint32(data[4:8])),
		VoiceCallState:  VoiceCallState(binary.LittleEndian.Uint32(data[8:12])),
		IPType:          ContextIPType(binary.LittleEndian.Uint32(data[12:16])),
		NwError:         binary.LittleEndian.Uint32(data[32:36]),
	}
	if result.ActivationState > ActivationStateDeactivating {
		return fmt.Errorf("parsing MBIM connect info: activation state %d is outside 0..%d", result.ActivationState, ActivationStateDeactivating)
	}
	if result.VoiceCallState > VoiceCallStateHangUp {
		return fmt.Errorf("parsing MBIM connect info: voice call state %d is outside 0..%d", result.VoiceCallState, VoiceCallStateHangUp)
	}
	if result.IPType > ContextIPTypeIPv4AndIPv6 {
		return fmt.Errorf("parsing MBIM connect info: IP type %d is outside 0..%d", result.IPType, ContextIPTypeIPv4AndIPv6)
	}
	copy(result.ContextType[:], data[16:32])
	if version < mbimExVersion30 {
		if len(data) != 36 {
			return errors.New("parsing MBIM connect info: MBIM 1 payload has trailing data")
		}
		*r = result
		return nil
	}
	if len(data) < 40 {
		return errors.New("parsing MBIM connect info: EX payload is truncated")
	}

	result.AccessMedia = AccessMediaType(binary.LittleEndian.Uint32(data[36:40]))
	if result.AccessMedia > AccessMediaType3GPPPreferred {
		return fmt.Errorf("parsing MBIM connect info: access media type %d is outside 0..%d", result.AccessMedia, AccessMediaType3GPPPreferred)
	}
	remaining := data[40:]
	accessStringTLV, consumed, err := unmarshalTLVPrefix(remaining)
	if err != nil {
		return fmt.Errorf("parsing MBIM connect access string TLV: %w", err)
	}
	if accessStringTLV.Type != TLVTypeWCharString {
		return fmt.Errorf("parsing MBIM connect access string: TLV type is %d, want %d", accessStringTLV.Type, TLVTypeWCharString)
	}
	if len(accessStringTLV.Data) > accessStringMaximumSize {
		return fmt.Errorf("parsing MBIM connect access string: size %d exceeds %d bytes", len(accessStringTLV.Data), accessStringMaximumSize)
	}
	result.AccessString, err = utf16RawString(accessStringTLV.Data)
	if err != nil {
		return fmt.Errorf("parsing MBIM connect access string: %w", err)
	}
	remaining = remaining[consumed:]

	if version >= mbimExVersion40 {
		snssaiTLV, consumed, err := unmarshalTLVPrefix(remaining)
		if err != nil {
			return fmt.Errorf("parsing MBIM connect S-NSSAI TLV: %w", err)
		}
		var snssai OptionalSNSSAI
		if err := snssai.UnmarshalTLV(snssaiTLV); err != nil {
			return fmt.Errorf("parsing MBIM connect S-NSSAI: %w", err)
		}
		result.SNSSAI = snssai.Value
		remaining = remaining[consumed:]
	}

	var tlvs TLVs
	if err := tlvs.UnmarshalBinary(remaining); err != nil {
		return fmt.Errorf("parsing MBIM connect unnamed TLVs: %w", err)
	}
	result.TLVs = tlvs
	for _, tlv := range tlvs {
		switch tlv.Type {
		case TLVTypePCO:
			var pco ProtocolConfigurationOptions
			if err := pco.UnmarshalBinary(tlv.Data); err != nil {
				return fmt.Errorf("parsing MBIM connect PCO: %w", err)
			}
			result.PCO = append(result.PCO, pco)
			result.PCSCFIPs = uniqueIPs(append(result.PCSCFIPs, pco.PCSCFIPs...))
			result.DNSIPs = uniqueIPs(append(result.DNSIPs, pco.DNSIPs...))
			if pco.IPv4LinkMTUKnown && !result.IPv4LinkMTUKnown {
				result.IPv4LinkMTU = pco.IPv4LinkMTU
				result.IPv4LinkMTUKnown = true
			}
		case TLVTypeTrafficParameters:
			if result.TrafficParameters != nil {
				return errors.New("parsing MBIM connect info: more than one traffic parameters TLV")
			}
			var trafficParameters TrafficParameters
			if err := trafficParameters.UnmarshalTLV(tlv); err != nil {
				return fmt.Errorf("parsing MBIM connect traffic parameters: %w", err)
			}
			result.TrafficParameters = &trafficParameters
		case TLVTypeSessionID:
			if version < mbimExVersion40 {
				continue
			}
			if result.MatchingSessionID != nil {
				return errors.New("parsing MBIM connect info: more than one session ID TLV")
			}
			var sessionID SessionID
			if err := sessionID.UnmarshalTLV(tlv); err != nil {
				return fmt.Errorf("parsing MBIM connect matching session ID: %w", err)
			}
			result.MatchingSessionID = &sessionID
		}
	}
	*r = result
	return nil
}

func (c *Client) QueryConnect(ctx context.Context, sessionID SessionID) (ConnectInfo, error) {
	request := ConnectQueryRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		SessionID:     sessionID,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return cloneConnectInfo(*request.Response), fmt.Errorf("reading MBIM connection: %w", err)
	}
	return cloneConnectInfo(*request.Response), nil
}

func (c *Client) SetConnect(ctx context.Context, cfg ConnectConfig) (ConnectInfo, error) {
	if err := validateConnectConfig(cfg, c.mbimExVersion); err != nil {
		return ConnectInfo{}, err
	}
	request := ConnectRequest{
		TransactionID:     c.nextTransactionID(),
		MBIMExVersion:     c.mbimExVersion,
		Timeout:           connectTimeout(cfg.Timeout, cfg.PDUActivationCount),
		SessionID:         cfg.SessionID,
		ActivationCommand: cfg.ActivationCommand,
		AccessString:      cfg.AccessString,
		UserName:          cfg.UserName,
		Password:          cfg.Password,
		Compression:       cfg.Compression,
		AuthProtocol:      cfg.AuthProtocol,
		IPType:            cfg.IPType,
		ContextType:       cfg.ContextType,
		MediaPreference:   cfg.MediaPreference,
		ActivationOption:  cfg.ActivationOption,
		SNSSAI:            cfg.SNSSAI,
		TLVs:              cfg.TLVs,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		if !errors.Is(err, StatusMatchingPDUSessionFound) {
			return cloneConnectInfo(*request.Response), fmt.Errorf("setting MBIM connection: %w", err)
		}
		response := cloneConnectInfo(*request.Response)
		if response.ActivationState != ActivationStateDeactivated {
			return ConnectInfo{}, fmt.Errorf(
				"setting MBIM connection: matching PDU session response activation state is %d, want %d: %w",
				response.ActivationState,
				ActivationStateDeactivated,
				err,
			)
		}
		if response.MatchingSessionID == nil {
			return ConnectInfo{}, fmt.Errorf("setting MBIM connection: matching PDU session response is missing session ID TLV: %w", err)
		}
		return response, fmt.Errorf("setting MBIM connection: %w", err)
	}
	return cloneConnectInfo(*request.Response), nil
}

func connectTimeout(timeout time.Duration, pduActivationCount uint32) time.Duration {
	if timeout > 0 {
		return timeout
	}
	if pduActivationCount == 0 {
		pduActivationCount = 1
	}
	return time.Duration(pduActivationCount) * mbimConnectSetResponseTimeout
}

func validateConnectConfig(cfg ConnectConfig, version uint16) error {
	if cfg.Timeout <= 0 && uint64(cfg.PDUActivationCount) > uint64((time.Duration(1<<63-1)/mbimConnectSetResponseTimeout)) {
		return fmt.Errorf("setting MBIM connection: PDU activation count %d overflows the response timeout", cfg.PDUActivationCount)
	}
	fields := []struct {
		name    string
		value   string
		maximum int
	}{
		{name: "access string", value: cfg.AccessString, maximum: accessStringMaximumSize},
		{name: "user name", value: cfg.UserName, maximum: userNameMaximumSize},
		{name: "password", value: cfg.Password, maximum: passwordMaximumSize},
	}
	for _, field := range fields {
		if size := len(utf16Bytes(field.value)); size > field.maximum {
			return fmt.Errorf("setting MBIM connection: %s size %d exceeds %d bytes", field.name, size, field.maximum)
		}
	}
	if cfg.ActivationCommand > ActivationCommandActivate {
		return fmt.Errorf("setting MBIM connection: activation command %d is reserved", cfg.ActivationCommand)
	}
	if cfg.Compression > CompressionEnable {
		return fmt.Errorf("setting MBIM connection: compression %d is outside 0..%d", cfg.Compression, CompressionEnable)
	}
	if cfg.AuthProtocol > AuthProtocolMSCHAPV2 {
		return fmt.Errorf("setting MBIM connection: authentication protocol %d is outside 0..%d", cfg.AuthProtocol, AuthProtocolMSCHAPV2)
	}
	if cfg.IPType > ContextIPTypeIPv4AndIPv6 {
		return fmt.Errorf("setting MBIM connection: IP type %d is outside 0..%d", cfg.IPType, ContextIPTypeIPv4AndIPv6)
	}
	if cfg.MediaPreference > AccessMediaType3GPPPreferred {
		return fmt.Errorf("setting MBIM connection: media preference %d is outside 0..%d", cfg.MediaPreference, AccessMediaType3GPPPreferred)
	}
	if cfg.ActivationOption > ActivationOptionPerURSPRules {
		return fmt.Errorf("setting MBIM connection: activation option %d is reserved", cfg.ActivationOption)
	}
	if cfg.ActivationCommand == ActivationCommandDeactivate && cfg.ActivationOption != ActivationOptionDefault {
		return errors.New("setting MBIM connection: deactivation requires the default activation option")
	}
	if version < mbimExVersion30 {
		if cfg.MediaPreference != AccessMediaTypeNone {
			return errors.New("setting MBIM connection: media preference requires MBIMEx 3.0")
		}
		if len(cfg.TLVs) != 0 {
			return errors.New("setting MBIM connection: unnamed TLVs require MBIMEx 3.0")
		}
	}
	if version < mbimExVersion40 {
		if cfg.ActivationOption != ActivationOptionDefault {
			return errors.New("setting MBIM connection: activation option requires MBIMEx 4.0")
		}
		if cfg.SNSSAI != nil {
			return errors.New("setting MBIM connection: S-NSSAI requires MBIMEx 4.0")
		}
	}
	if cfg.SNSSAI != nil {
		if _, err := cfg.SNSSAI.MarshalBinary(); err != nil {
			return fmt.Errorf("setting MBIM connection S-NSSAI: %w", err)
		}
	}
	if _, err := cfg.TLVs.MarshalBinary(); err != nil {
		return fmt.Errorf("setting MBIM connection TLVs: %w", err)
	}
	trafficParameters := 0
	for index, tlv := range cfg.TLVs {
		if tlv.Type != TLVTypeTrafficParameters {
			continue
		}
		trafficParameters++
		if trafficParameters > 1 {
			return errors.New("setting MBIM connection: more than one traffic parameters TLV")
		}
		var parameters TrafficParameters
		if err := parameters.UnmarshalTLV(tlv); err != nil {
			return fmt.Errorf("setting MBIM connection traffic parameters TLV %d: %w", index, err)
		}
	}
	if cfg.ActivationCommand == ActivationCommandActivate &&
		(cfg.ActivationOption == ActivationOptionPerNonDefaultURSPRules || cfg.ActivationOption == ActivationOptionPerURSPRules) {
		if trafficParameters != 1 {
			return fmt.Errorf("setting MBIM connection: activation option %d requires exactly one traffic parameters TLV", cfg.ActivationOption)
		}
	}
	return nil
}

func cloneConnectInfo(info ConnectInfo) ConnectInfo {
	cloned := info
	if info.SNSSAI != nil {
		snssai := *info.SNSSAI
		cloned.SNSSAI = &snssai
	}
	if info.TrafficParameters != nil {
		trafficParameters := *info.TrafficParameters
		trafficParameters.TrafficDescriptor = slices.Clone(info.TrafficParameters.TrafficDescriptor)
		cloned.TrafficParameters = &trafficParameters
	}
	if info.MatchingSessionID != nil {
		sessionID := *info.MatchingSessionID
		cloned.MatchingSessionID = &sessionID
	}
	cloned.TLVs = slices.Clone(info.TLVs)
	for i, tlv := range info.TLVs {
		cloned.TLVs[i] = TLV{Type: tlv.Type, Data: slices.Clone(tlv.Data)}
	}
	cloned.PCO = slices.Clone(info.PCO)
	for i, pco := range info.PCO {
		cloned.PCO[i].Options = slices.Clone(pco.Options)
		for j, option := range pco.Options {
			cloned.PCO[i].Options[j] = PCOOption{ID: option.ID, Data: slices.Clone(option.Data)}
		}
		cloned.PCO[i].PCSCFIPs = cloneIPs(pco.PCSCFIPs)
		cloned.PCO[i].DNSIPs = cloneIPs(pco.DNSIPs)
	}
	cloned.PCSCFIPs = cloneIPs(info.PCSCFIPs)
	cloned.DNSIPs = cloneIPs(info.DNSIPs)
	return cloned
}

func writeConnectStringRefs(data []byte, baseOffset int, values ...[]byte) {
	offset := baseOffset
	for i, value := range values {
		fieldOffset := 8 + i*8
		if len(value) != 0 {
			binary.LittleEndian.PutUint32(data[fieldOffset:fieldOffset+4], uint32(offset))
			binary.LittleEndian.PutUint32(data[fieldOffset+4:fieldOffset+8], uint32(len(value)))
		}
		offset = align4(offset + len(value))
	}
}

func (r *ConnectRequest) connectSetData() []byte {
	switch {
	case r.MBIMExVersion >= mbimExVersion40:
		return r.connectSetDataEx4()
	case r.MBIMExVersion >= mbimExVersion30:
		return r.connectSetDataEx3()
	default:
		return r.connectSetDataV1()
	}
}

func (r *ConnectRequest) connectSetDataV1() []byte {
	accessString := utf16Bytes(r.AccessString)
	userName := utf16Bytes(r.UserName)
	password := utf16Bytes(r.Password)

	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[0:4], uint32(r.SessionID))
	binary.LittleEndian.PutUint32(data[4:8], uint32(r.ActivationCommand))
	writeConnectStringRefs(data, 60, accessString, userName, password)
	binary.LittleEndian.PutUint32(data[32:36], uint32(r.Compression))
	binary.LittleEndian.PutUint32(data[36:40], uint32(r.AuthProtocol))
	binary.LittleEndian.PutUint32(data[40:44], uint32(r.IPType))
	copy(data[44:60], r.ContextType[:])
	return appendConnectStrings(data, accessString, userName, password)
}

func (r *ConnectRequest) connectSetDataEx3() []byte {
	data := make([]byte, 40)
	binary.LittleEndian.PutUint32(data[0:4], uint32(r.SessionID))
	binary.LittleEndian.PutUint32(data[4:8], uint32(r.ActivationCommand))
	binary.LittleEndian.PutUint32(data[8:12], uint32(r.Compression))
	binary.LittleEndian.PutUint32(data[12:16], uint32(r.AuthProtocol))
	binary.LittleEndian.PutUint32(data[16:20], uint32(r.IPType))
	copy(data[20:36], r.ContextType[:])
	binary.LittleEndian.PutUint32(data[36:40], uint32(r.MediaPreference))
	data = appendConnectStringTLVs(data, r.AccessString, r.UserName, r.Password)
	return append(data, marshalTLVsUnchecked(r.TLVs)...)
}

func (r *ConnectRequest) connectSetDataEx4() []byte {
	data := make([]byte, 44)
	binary.LittleEndian.PutUint32(data[0:4], uint32(r.SessionID))
	binary.LittleEndian.PutUint32(data[4:8], uint32(r.ActivationCommand))
	binary.LittleEndian.PutUint32(data[8:12], uint32(r.activationOption()))
	binary.LittleEndian.PutUint32(data[12:16], uint32(r.Compression))
	binary.LittleEndian.PutUint32(data[16:20], uint32(r.AuthProtocol))
	binary.LittleEndian.PutUint32(data[20:24], uint32(r.IPType))
	copy(data[24:40], r.ContextType[:])
	binary.LittleEndian.PutUint32(data[40:44], uint32(r.MediaPreference))
	data = appendConnectStringTLVs(data, r.AccessString, r.UserName, r.Password)
	var snssai []byte
	if r.SNSSAI != nil {
		snssai = r.SNSSAI.marshalBinaryUnchecked()
	}
	data = append(data, marshalTLV(TLVTypeSingleNSSAI, snssai)...)
	return append(data, marshalTLVsUnchecked(r.TLVs)...)
}

func (r *ConnectRequest) activationOption() ActivationOption {
	if r.ActivationCommand == ActivationCommandDeactivate {
		return ActivationOptionDefault
	}
	return r.ActivationOption
}

func appendConnectStringTLVs(data []byte, values ...string) []byte {
	for _, value := range values {
		data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(value))...)
	}
	return data
}

func appendConnectStrings(data []byte, values ...[]byte) []byte {
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		data = append(data, value...)
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
	}
	return data
}
