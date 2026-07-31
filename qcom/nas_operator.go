package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// NASPLMNNameRequest selects the network and optional lookup policy.
type NASPLMNNameRequest struct {
	PLMN               NASPLMN
	SuppressSIMError   *bool
	AlwaysSend         *bool
	UseStaticTableOnly *bool
	CSGID              *uint32
	RadioInterface     *NASRadioInterface
	SendAllInformation *bool
}

// NASNetworkDescriptionEncoding identifies bytes in an encoded network name.
type NASNetworkDescriptionEncoding uint8

const (
	NASNetworkDescriptionGSM7 NASNetworkDescriptionEncoding = iota
	NASNetworkDescriptionUCS2LE
)

// NASCountryInitials controls whether country initials prefix a PLMN name.
type NASCountryInitials uint8

const (
	NASCountryInitialsDoNotAdd NASCountryInitials = iota
	NASCountryInitialsAdd
	NASCountryInitialsUnspecified NASCountryInitials = 0xFF
)

// NASNameSpareBits is the number-of-spare-bits code for GSM packed names.
type NASNameSpareBits uint8

// NASEncodedNetworkName preserves the modem-provided name bytes and encoding.
type NASEncodedNetworkName struct {
	Encoding        NASNetworkDescriptionEncoding
	CountryInitials NASCountryInitials
	SpareBits       NASNameSpareBits
	Data            []byte
}

// NASTriState is Qualcomm's false, true, or unknown state.
type NASTriState uint32

const (
	NASTriFalse NASTriState = iota
	NASTriTrue
	NASTriUnknown
)

// NASPLMNLanguage identifies the language of one UCS-2 operator name.
type NASPLMNLanguage uint32

const (
	NASPLMNLanguageUnknown NASPLMNLanguage = iota
	NASPLMNLanguageChineseTraditional
	NASPLMNLanguageChineseSimplified
)

// NASLocalizedPLMNName contains one language-specific UCS-2 name pair.
type NASLocalizedPLMNName struct {
	LongName  []uint16
	ShortName []uint16
	Language  NASPLMNLanguage
}

// NASPLMNName contains operator names and modem display guidance.
type NASPLMNName struct {
	ServiceProvider        NASEncodedNetworkName
	Short                  NASEncodedNetworkName
	Long                   NASEncodedNetworkName
	NamesKnown             bool
	DisplayServiceProvider NASTriState
	DisplayPLMN            NASTriState
	DisplayBitsKnown       bool
	HomeNetwork            NASTriState
	HomeNetworkKnown       bool
	Localized              []NASLocalizedPLMNName
	LocalizedKnown         bool
	AdditionalInfo         []uint16
	AdditionalInfoKnown    bool
	Source                 NASNetworkNameSource
	SourceKnown            bool
}

// NASCurrentPLMNName contains the operator-name data carried by the Current
// PLMN Name indication. Its TLVs differ from Get PLMN Name and are decoded
// independently.
type NASCurrentPLMNName struct {
	PLMN                      NASPLMN
	PLMNKnown                 bool
	ServiceProvider           NASEncodedNetworkName
	ServiceProviderKnown      bool
	Short                     NASEncodedNetworkName
	ShortKnown                bool
	Long                      NASEncodedNetworkName
	LongKnown                 bool
	CSGID                     uint32
	CSGIDKnown                bool
	DisplayServiceProvider    NASTriState
	DisplayPLMN               NASTriState
	DisplayBitsKnown          bool
	HomeNetwork               NASTriState
	HomeNetworkKnown          bool
	RadioInterface            NASRadioInterface
	RadioInterfaceKnown       bool
	Localized                 []NASLocalizedPLMNName
	LocalizedKnown            bool
	AdditionalInfo            []uint16
	AdditionalInfoKnown       bool
	Source                    NASNetworkNameSource
	SourceKnown               bool
	ServiceProviderExtended   []uint16
	ServiceProviderExtKnown   bool
	NR5GTrackingAreaCode      [3]byte
	NR5GTrackingAreaCodeKnown bool
}

// NASNetworkServiceDomain identifies the rejected registration domain.
type NASNetworkServiceDomain uint8

const (
	NASNetworkServiceNone NASNetworkServiceDomain = iota
	NASNetworkServiceCS
	NASNetworkServicePS
	NASNetworkServiceCSPS
	NASNetworkServiceCamped
)

// NASRejectCause is a 3GPP mobility-management reject cause.
type NASRejectCause uint8

const (
	NASRejectNone                   NASRejectCause = 0x00
	NASRejectIMSIUnknown            NASRejectCause = 0x02
	NASRejectIllegalUE              NASRejectCause = 0x03
	NASRejectIMEINotAccepted        NASRejectCause = 0x05
	NASRejectIllegalME              NASRejectCause = 0x06
	NASRejectPSServicesNotAllowed   NASRejectCause = 0x07
	NASRejectPLMNNotAllowed         NASRejectCause = 0x0B
	NASRejectLocationAreaNotAllowed NASRejectCause = 0x0C
	NASRejectRoamingNotAllowed      NASRejectCause = 0x0D
	NASRejectNoSuitableCells        NASRejectCause = 0x0F
	NASRejectNetworkFailure         NASRejectCause = 0x11
	NASRejectCongestion             NASRejectCause = 0x16
)

// NASNetworkReject describes one rejected network registration attempt.
type NASNetworkReject struct {
	RadioInterface   NASRadioInterface
	ServiceDomain    NASNetworkServiceDomain
	Cause            NASRejectCause
	PLMN             NASPLMN
	PLMNKnown        bool
	CSGID            uint32
	CSGIDKnown       bool
	CIoTLTEMode      uint32
	CIoTLTEModeKnown bool
}

// PLMNName queries the modem's operator-name sources for one PLMN.
func (c *Client) PLMNName(ctx context.Context, req NASPLMNNameRequest) (NASPLMNName, error) {
	if req.PLMN.MCC > 999 || req.PLMN.MNC > 999 {
		return NASPLMNName{}, fmt.Errorf("querying QMI NAS PLMN name: PLMN %d/%d is out of range", req.PLMN.MCC, req.PLMN.MNC)
	}
	request := nasEmptyRequest(0, 0, DefaultRequestTimeout, MessageNASGetPLMNName)
	request.TLVs = encodeNASPLMNNameRequest(req)
	var result NASPLMNName
	if err := c.nasReadRequest(ctx, request, &result); err != nil {
		return NASPLMNName{}, fmt.Errorf("querying QMI NAS PLMN name: %w", err)
	}
	return result, nil
}

// UnmarshalTLVs parses QMI NAS Get PLMN Name response TLVs.
func (n *NASPLMNName) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*n = NASPLMNName{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if err := n.decodeNames(value); err != nil {
			return err
		}
		n.NamesKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI NAS PLMN name display bits: TLV length %d, want 8", len(value))
		}
		n.DisplayServiceProvider = NASTriState(binary.LittleEndian.Uint32(value[:4]))
		n.DisplayPLMN = NASTriState(binary.LittleEndian.Uint32(value[4:]))
		n.DisplayBitsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS PLMN home-network state: TLV length %d, want 4", len(value))
		}
		n.HomeNetwork = NASTriState(binary.LittleEndian.Uint32(value))
		n.HomeNetworkKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		localized, err := decodeNASLocalizedPLMNNames(value)
		if err != nil {
			return err
		}
		n.Localized = localized
		n.LocalizedKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		additional, err := decodeNASUint16Array(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS PLMN additional information: %w", err)
		}
		n.AdditionalInfo = additional
		n.AdditionalInfoKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS PLMN name source: TLV length %d, want 4", len(value))
		}
		n.Source = NASNetworkNameSource(binary.LittleEndian.Uint32(value))
		n.SourceKnown = true
	}
	return nil
}

// UnmarshalTLVs parses QMI NAS Current PLMN Name indication TLVs.
func (n *NASCurrentPLMNName) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*n = NASCurrentPLMNName{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 5 {
			return fmt.Errorf("parsing QMI NAS current PLMN ID: TLV length %d, want 5", len(value))
		}
		n.PLMN = NASPLMN{
			MCC:                 binary.LittleEndian.Uint16(value[0:2]),
			MNC:                 binary.LittleEndian.Uint16(value[2:4]),
			MNCThreeDigits:      value[4] != 0,
			MNCThreeDigitsKnown: true,
		}
		n.PLMNKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		name, err := decodeNASCurrentPLMNEncodedName(value, false)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS current PLMN service-provider name: %w", err)
		}
		n.ServiceProvider = name
		n.ServiceProviderKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		name, err := decodeNASCurrentPLMNEncodedName(value, true)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS current PLMN short name: %w", err)
		}
		n.Short = name
		n.ShortKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		name, err := decodeNASCurrentPLMNEncodedName(value, true)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS current PLMN long name: %w", err)
		}
		n.Long = name
		n.LongKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS current PLMN CSG ID: TLV length %d, want 4", len(value))
		}
		n.CSGID = binary.LittleEndian.Uint32(value)
		n.CSGIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI NAS current PLMN display bits: TLV length %d, want 8", len(value))
		}
		n.DisplayServiceProvider = NASTriState(binary.LittleEndian.Uint32(value[0:4]))
		n.DisplayPLMN = NASTriState(binary.LittleEndian.Uint32(value[4:8]))
		n.DisplayBitsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS current PLMN home-network state: TLV length %d, want 4", len(value))
		}
		n.HomeNetwork = NASTriState(binary.LittleEndian.Uint32(value))
		n.HomeNetworkKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS current PLMN radio interface: TLV length %d, want 1", len(value))
		}
		n.RadioInterface = NASRadioInterface(value[0])
		n.RadioInterfaceKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x18); ok {
		localized, err := decodeNASLocalizedPLMNNames(value)
		if err != nil {
			return err
		}
		n.Localized = localized
		n.LocalizedKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x19); ok {
		additional, err := decodeNASUint16Array(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS current PLMN additional information: %w", err)
		}
		n.AdditionalInfo = additional
		n.AdditionalInfoKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x1A); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS current PLMN name source: TLV length %d, want 4", len(value))
		}
		n.Source = NASNetworkNameSource(binary.LittleEndian.Uint32(value))
		n.SourceKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x1B); ok {
		name, err := decodeNASUTF16String(value, 64)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS current PLMN extended service-provider name: %w", err)
		}
		n.ServiceProviderExtended = name
		n.ServiceProviderExtKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x1C); ok {
		if len(value) != len(n.NR5GTrackingAreaCode) {
			return fmt.Errorf("parsing QMI NAS current PLMN NR5G tracking area code: TLV length %d, want %d", len(value), len(n.NR5GTrackingAreaCode))
		}
		copy(n.NR5GTrackingAreaCode[:], value)
		n.NR5GTrackingAreaCodeKnown = true
	}
	return nil
}

func decodeNASCurrentPLMNEncodedName(value []byte, metadata bool) (NASEncodedNetworkName, error) {
	name, offset, err := decodeNASEncodedName(value, 0, metadata)
	if err != nil {
		return NASEncodedNetworkName{}, err
	}
	if offset != len(value) {
		return NASEncodedNetworkName{}, fmt.Errorf("%d trailing bytes", len(value)-offset)
	}
	return name, nil
}

// UnmarshalTLVs parses QMI NAS Network Reject indication TLVs.
func (r *NASNetworkReject) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASNetworkReject{}
	radio, err := requiredNASByte(tlvs, 0x01)
	if err != nil {
		return fmt.Errorf("parsing QMI NAS network reject radio interface: %w", err)
	}
	domain, err := requiredNASByte(tlvs, 0x02)
	if err != nil {
		return fmt.Errorf("parsing QMI NAS network reject service domain: %w", err)
	}
	cause, err := requiredNASByte(tlvs, 0x03)
	if err != nil {
		return fmt.Errorf("parsing QMI NAS network reject cause: %w", err)
	}
	r.RadioInterface = NASRadioInterface(radio)
	r.ServiceDomain = NASNetworkServiceDomain(domain)
	r.Cause = NASRejectCause(cause)
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 5 {
			return fmt.Errorf("parsing QMI NAS network reject PLMN: TLV length %d, want 5", len(value))
		}
		r.PLMN = NASPLMN{
			MCC:                 binary.LittleEndian.Uint16(value[:2]),
			MNC:                 binary.LittleEndian.Uint16(value[2:4]),
			MNCThreeDigits:      value[4] != 0,
			MNCThreeDigitsKnown: true,
		}
		r.PLMNKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS network reject CSG ID: TLV length %d, want 4", len(value))
		}
		r.CSGID = binary.LittleEndian.Uint32(value)
		r.CSGIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS network reject CIoT LTE mode: TLV length %d, want 4", len(value))
		}
		r.CIoTLTEMode = binary.LittleEndian.Uint32(value)
		r.CIoTLTEModeKnown = true
	}
	return nil
}

func encodeNASPLMNNameRequest(req NASPLMNNameRequest) tlv.TLVs {
	plmn := binary.LittleEndian.AppendUint16(nil, req.PLMN.MCC)
	plmn = binary.LittleEndian.AppendUint16(plmn, req.PLMN.MNC)
	tlvs := tlv.TLVs{tlv.Bytes(0x01, plmn)}
	if req.SuppressSIMError != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*req.SuppressSIMError)))
	}
	if req.PLMN.MNCThreeDigitsKnown {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(req.PLMN.MNCThreeDigits)))
	}
	if req.AlwaysSend != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*req.AlwaysSend)))
	}
	if req.UseStaticTableOnly != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, boolByte(*req.UseStaticTableOnly)))
	}
	if req.CSGID != nil {
		tlvs = append(tlvs, tlv.Uint(0x14, *req.CSGID))
	}
	if req.RadioInterface != nil {
		tlvs = append(tlvs, tlv.Uint(0x15, uint8(*req.RadioInterface)))
	}
	if req.SendAllInformation != nil {
		tlvs = append(tlvs, tlv.Uint(0x16, boolByte(*req.SendAllInformation)))
	}
	return tlvs
}

func (n *NASPLMNName) decodeNames(value []byte) error {
	offset := 0
	spn, next, err := decodeNASEncodedName(value, offset, false)
	if err != nil {
		return fmt.Errorf("parsing QMI NAS PLMN service-provider name: %w", err)
	}
	offset = next
	short, next, err := decodeNASEncodedName(value, offset, true)
	if err != nil {
		return fmt.Errorf("parsing QMI NAS PLMN short name: %w", err)
	}
	offset = next
	long, offset, err := decodeNASEncodedName(value, offset, true)
	if err != nil {
		return fmt.Errorf("parsing QMI NAS PLMN long name: %w", err)
	}
	if offset != len(value) {
		return fmt.Errorf("parsing QMI NAS PLMN names: %d trailing bytes", len(value)-offset)
	}
	n.ServiceProvider = spn
	n.Short = short
	n.Long = long
	return nil
}

func decodeNASEncodedName(value []byte, offset int, metadata bool) (NASEncodedNetworkName, int, error) {
	metadataLength := 0
	if metadata {
		metadataLength = 2
	}
	if len(value)-offset < 2+metadataLength {
		return NASEncodedNetworkName{}, offset, errors.New("metadata or length is truncated")
	}
	name := NASEncodedNetworkName{Encoding: NASNetworkDescriptionEncoding(value[offset])}
	offset++
	if metadata {
		name.CountryInitials = NASCountryInitials(value[offset])
		name.SpareBits = NASNameSpareBits(value[offset+1])
		offset += 2
	}
	length := int(value[offset])
	offset++
	if len(value)-offset < length {
		return NASEncodedNetworkName{}, offset, errors.New("data is truncated")
	}
	name.Data = slices.Clone(value[offset : offset+length])
	return name, offset + length, nil
}

func decodeNASLocalizedPLMNNames(value []byte) ([]NASLocalizedPLMNName, error) {
	if len(value) == 0 {
		return nil, errors.New("parsing QMI NAS localized PLMN names: count is missing")
	}
	count := int(value[0])
	offset := 1
	names := make([]NASLocalizedPLMNName, count)
	for i := range count {
		longName, next, err := decodeNASUint16ArrayAt(value, offset)
		if err != nil {
			return nil, fmt.Errorf("parsing QMI NAS localized PLMN name %d long name: %w", i, err)
		}
		offset = next
		shortName, next, err := decodeNASUint16ArrayAt(value, offset)
		if err != nil {
			return nil, fmt.Errorf("parsing QMI NAS localized PLMN name %d short name: %w", i, err)
		}
		offset = next
		if len(value)-offset < 4 {
			return nil, fmt.Errorf("parsing QMI NAS localized PLMN name %d: language is truncated", i)
		}
		names[i] = NASLocalizedPLMNName{
			LongName:  longName,
			ShortName: shortName,
			Language:  NASPLMNLanguage(binary.LittleEndian.Uint32(value[offset : offset+4])),
		}
		offset += 4
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI NAS localized PLMN names: %d trailing bytes", len(value)-offset)
	}
	return names, nil
}

func decodeNASUint16Array(value []byte) ([]uint16, error) {
	decoded, offset, err := decodeNASUint16ArrayAt(value, 0)
	if err != nil {
		return nil, err
	}
	if offset != len(value) {
		return nil, fmt.Errorf("%d trailing bytes", len(value)-offset)
	}
	return decoded, nil
}

func decodeNASUint16ArrayAt(value []byte, offset int) ([]uint16, int, error) {
	if offset >= len(value) {
		return nil, offset, errors.New("count is missing")
	}
	count := int(value[offset])
	offset++
	if len(value)-offset < count*2 {
		return nil, offset, errors.New("data is truncated")
	}
	decoded := make([]uint16, count)
	for i := range count {
		decoded[i] = binary.LittleEndian.Uint16(value[offset+i*2 : offset+i*2+2])
	}
	return decoded, offset + count*2, nil
}

func decodeNASUTF16String(value []byte, maximum int) ([]uint16, error) {
	if len(value)%2 != 0 {
		return nil, fmt.Errorf("UTF-16 byte length %d is odd", len(value))
	}
	count := len(value) / 2
	if count > maximum {
		return nil, fmt.Errorf("UTF-16 length %d exceeds %d", count, maximum)
	}
	decoded := make([]uint16, count)
	for i := range count {
		decoded[i] = binary.LittleEndian.Uint16(value[i*2 : i*2+2])
	}
	if len(decoded) > 0 && decoded[len(decoded)-1] == 0 {
		decoded = decoded[:len(decoded)-1]
	}
	return decoded, nil
}

func requiredNASByte(tlvs tlv.TLVs, typ byte) (byte, error) {
	value, ok := tlv.Value(tlvs, typ)
	if !ok {
		return 0, fmt.Errorf("TLV 0x%02x missing", typ)
	}
	if len(value) != 1 {
		return 0, fmt.Errorf("TLV 0x%02x length %d, want 1", typ, len(value))
	}
	return value[0], nil
}
