package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// PBMEventRegistrationMask selects Phonebook Management indications.
type PBMEventRegistrationMask uint32

const (
	PBMEventRecordUpdate PBMEventRegistrationMask = 1 << iota
	PBMEventPhonebookReady
	PBMEventEmergencyNumberList
	PBMEventHiddenRecordStatus
	PBMEventAASUpdate
	PBMEventGASUpdate
)

const pbmKnownEventRegistrationMask = PBMEventRecordUpdate |
	PBMEventPhonebookReady |
	PBMEventEmergencyNumberList |
	PBMEventHiddenRecordStatus |
	PBMEventAASUpdate |
	PBMEventGASUpdate

// PBMSessionType identifies a phonebook session.
type PBMSessionType uint8

const (
	PBMSessionGWPrimary PBMSessionType = iota
	PBMSession1XPrimary
	PBMSessionGWSecondary
	PBMSession1XSecondary
	PBMSessionNonProvisioningSlot1
	PBMSessionNonProvisioningSlot2
	PBMSessionGlobalPhonebookSlot1
	PBMSessionGlobalPhonebookSlot2
	PBMSessionGWTertiary
	PBMSession1XTertiary
	PBMSessionGlobalPhonebookSlot3
)

// PBMPhonebookType is a bitmask of phonebook types.
type PBMPhonebookType uint16

const (
	PBMPhonebookADN PBMPhonebookType = 1 << iota
	PBMPhonebookFDN
	PBMPhonebookMSISDN
	PBMPhonebookMBDN
	PBMPhonebookSDN
	PBMPhonebookBDN
	PBMPhonebookLND
	PBMPhonebookMBN
	PBMPhonebookGAS
	PBMPhonebookAAS
)

// PBMEmergencyNumberFlags describes the service categories of an emergency
// number.
type PBMEmergencyNumberFlags uint8

const (
	PBMEmergencyPolice PBMEmergencyNumberFlags = 1 << iota
	PBMEmergencyAmbulance
	PBMEmergencyFireBrigade
	PBMEmergencyMarineGuard
	PBMEmergencyMountainRescue
	PBMEmergencyManualECall
	PBMEmergencyAutomaticECall
	PBMEmergencySpare
)

// PBMPhonebookCapability contains the record limits for one phonebook.
type PBMPhonebookCapability struct {
	Phonebook           PBMPhonebookType
	UsedRecords         uint16
	MaximumRecords      uint16
	MaximumNumberLength uint8
	MaximumNameLength   uint8
}

// PBMBasicCapabilities groups phonebook record limits by session.
type PBMBasicCapabilities struct {
	Session    PBMSessionType
	Phonebooks []PBMPhonebookCapability
}

// PBMGroupCapability contains group limits for a phonebook session.
type PBMGroupCapability struct {
	MaximumGroups         uint8
	MaximumGroupTagLength uint8
}

// PBMAdditionalNumberCapability contains additional-number limits.
type PBMAdditionalNumberCapability struct {
	MaximumNumbers         uint8
	MaximumNumberLength    uint8
	MaximumNumberTagLength uint8
}

// PBMEmailCapability contains email-field limits.
type PBMEmailCapability struct {
	MaximumEmails        uint8
	MaximumAddressLength uint8
}

// PBMSecondNameCapability contains the second-name length limit.
type PBMSecondNameCapability struct {
	MaximumLength uint8
}

// PBMHiddenRecordsCapability reports whether hidden records are supported.
type PBMHiddenRecordsCapability struct {
	Supported bool
}

// PBMAlphaStringCapability contains GAS or AAS record limits.
type PBMAlphaStringCapability struct {
	MaximumRecords      uint8
	UsedRecords         uint8
	MaximumStringLength uint8
}

// PBMSessionGroupCapability associates group limits with a session.
type PBMSessionGroupCapability struct {
	PBMGroupCapability
	Session PBMSessionType
}

// PBMSessionAdditionalNumberCapability associates additional-number limits
// with a session.
type PBMSessionAdditionalNumberCapability struct {
	PBMAdditionalNumberCapability
	Session PBMSessionType
}

// PBMSessionEmailCapability associates email limits with a session.
type PBMSessionEmailCapability struct {
	PBMEmailCapability
	Session PBMSessionType
}

// PBMSessionSecondNameCapability associates second-name limits with a session.
type PBMSessionSecondNameCapability struct {
	PBMSecondNameCapability
	Session PBMSessionType
}

// PBMSessionHiddenRecordsCapability associates hidden-record support with a
// session.
type PBMSessionHiddenRecordsCapability struct {
	PBMHiddenRecordsCapability
	Session PBMSessionType
}

// PBMSessionAlphaStringCapability associates GAS or AAS limits with a session.
type PBMSessionAlphaStringCapability struct {
	PBMAlphaStringCapability
	Session PBMSessionType
}

// PBMCapabilities contains the optional capability TLVs returned for one
// requested phonebook.
type PBMCapabilities struct {
	Basic                 PBMBasicCapabilities
	BasicKnown            bool
	Group                 PBMGroupCapability
	GroupKnown            bool
	AdditionalNumber      PBMAdditionalNumberCapability
	AdditionalNumberKnown bool
	Email                 PBMEmailCapability
	EmailKnown            bool
	SecondName            PBMSecondNameCapability
	SecondNameKnown       bool
	HiddenRecords         PBMHiddenRecordsCapability
	HiddenRecordsKnown    bool
	GAS                   PBMAlphaStringCapability
	GASKnown              bool
	AAS                   PBMAlphaStringCapability
	AASKnown              bool
}

// PBMAllCapabilities contains capability arrays for every available session.
type PBMAllCapabilities struct {
	Basic             []PBMBasicCapabilities
	Groups            []PBMSessionGroupCapability
	AdditionalNumbers []PBMSessionAdditionalNumberCapability
	Emails            []PBMSessionEmailCapability
	SecondNames       []PBMSessionSecondNameCapability
	HiddenRecords     []PBMSessionHiddenRecordsCapability
	GAS               []PBMSessionAlphaStringCapability
	AAS               []PBMSessionAlphaStringCapability
}

// PBMEmergencyNumber is a categorized card or network emergency number.
type PBMEmergencyNumber struct {
	Flags  PBMEmergencyNumberFlags
	Number string
}

// PBMSessionEmergencyNumbers groups emergency numbers by phonebook session.
type PBMSessionEmergencyNumbers struct {
	Session PBMSessionType
	Numbers []PBMEmergencyNumber
}

// PBMEmergencyList is the consolidated emergency-number list known by the
// modem.
type PBMEmergencyList struct {
	Hardcoded      []string
	HardcodedFlags []PBMEmergencyNumberFlags
	NV             []string
	NVFlags        []PBMEmergencyNumberFlags
	Card           []PBMSessionEmergencyNumbers
	Network        []PBMSessionEmergencyNumbers
}

// PBMIndicationRegisterRequest encodes QMI PBM Indication Register.
type PBMIndicationRegisterRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Mask          PBMEventRegistrationMask
}

// Request converts the event mask into a QMI PBM request.
func (r PBMIndicationRegisterRequest) Request() Request {
	return Request{
		Service:       ServicePBM,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePBMIndicationRegister,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Mask))},
	}
}

// PBMGetCapabilitiesRequest encodes QMI PBM Get Capabilities.
type PBMGetCapabilitiesRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Session       PBMSessionType
	Phonebook     PBMPhonebookType
}

// Request validates and converts the capability query into a QMI PBM request.
func (r PBMGetCapabilitiesRequest) Request() (Request, error) {
	if err := validatePBMSessionType(r.Session); err != nil {
		return Request{}, fmt.Errorf("encoding QMI PBM capability query: %w", err)
	}
	if r.Phonebook == 0 {
		return Request{}, errors.New("encoding QMI PBM capability query: phonebook mask is empty")
	}
	value := []byte{byte(r.Session)}
	value = binary.LittleEndian.AppendUint16(value, uint16(r.Phonebook))
	return Request{
		Service:       ServicePBM,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePBMGetCapabilities,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, value)},
	}, nil
}

// PBMGetAllCapabilitiesRequest encodes QMI PBM Get All Capabilities.
type PBMGetAllCapabilitiesRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI PBM request.
func (r PBMGetAllCapabilitiesRequest) Request() Request {
	return pbmEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessagePBMGetAllCapabilities)
}

// PBMGetEmergencyListRequest encodes QMI PBM Get Emergency List.
type PBMGetEmergencyListRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI PBM request.
func (r PBMGetEmergencyListRequest) Request() Request {
	return pbmEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessagePBMGetEmergencyList)
}

// PBMIndicationRegisterResponse contains the effective event mask when the
// modem returns it.
type PBMIndicationRegisterResponse struct {
	Mask      PBMEventRegistrationMask
	MaskKnown bool
}

// UnmarshalTLVs parses QMI PBM Indication Register output.
func (r *PBMIndicationRegisterResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = PBMIndicationRegisterResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI PBM indication registration: event mask TLV length %d, want 4", len(value))
	}
	r.Mask = PBMEventRegistrationMask(binary.LittleEndian.Uint32(value))
	r.MaskKnown = true
	return nil
}

// UnmarshalTLVs parses QMI PBM Get Capabilities output.
func (c *PBMCapabilities) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*c = PBMCapabilities{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		basic, err := decodePBMSingleBasicCapabilities(value)
		if err != nil {
			return err
		}
		c.Basic = basic
		c.BasicKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI PBM capabilities: group capability TLV length %d, want 2", len(value))
		}
		c.Group = PBMGroupCapability{MaximumGroups: value[0], MaximumGroupTagLength: value[1]}
		c.GroupKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 3 {
			return fmt.Errorf("parsing QMI PBM capabilities: additional-number capability TLV length %d, want 3", len(value))
		}
		c.AdditionalNumber = PBMAdditionalNumberCapability{
			MaximumNumbers:         value[0],
			MaximumNumberLength:    value[1],
			MaximumNumberTagLength: value[2],
		}
		c.AdditionalNumberKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI PBM capabilities: email capability TLV length %d, want 2", len(value))
		}
		c.Email = PBMEmailCapability{MaximumEmails: value[0], MaximumAddressLength: value[1]}
		c.EmailKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI PBM capabilities: second-name capability TLV length %d, want 1", len(value))
		}
		c.SecondName = PBMSecondNameCapability{MaximumLength: value[0]}
		c.SecondNameKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI PBM capabilities: hidden-record capability TLV length %d, want 1", len(value))
		}
		c.HiddenRecords = PBMHiddenRecordsCapability{Supported: value[0] != 0}
		c.HiddenRecordsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if err := c.GAS.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI PBM GAS capability: %w", err)
		}
		c.GASKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		if err := c.AAS.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI PBM AAS capability: %w", err)
		}
		c.AASKnown = true
	}
	return nil
}

// UnmarshalTLVs parses QMI PBM Get All Capabilities output.
func (c *PBMAllCapabilities) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*c = PBMAllCapabilities{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		basic, err := decodePBMBasicCapabilityArray(value)
		if err != nil {
			return err
		}
		c.Basic = basic
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		groups, err := decodePBMArray(value, func(r *pbmValueReader) PBMSessionGroupCapability {
			return PBMSessionGroupCapability{
				Session: PBMSessionType(r.uint8()),
				PBMGroupCapability: PBMGroupCapability{
					MaximumGroups:         r.uint8(),
					MaximumGroupTagLength: r.uint8(),
				},
			}
		})
		if err != nil {
			return fmt.Errorf("parsing QMI PBM group capability array: %w", err)
		}
		c.Groups = groups
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		additional, err := decodePBMArray(value, func(r *pbmValueReader) PBMSessionAdditionalNumberCapability {
			return PBMSessionAdditionalNumberCapability{
				Session: PBMSessionType(r.uint8()),
				PBMAdditionalNumberCapability: PBMAdditionalNumberCapability{
					MaximumNumbers:         r.uint8(),
					MaximumNumberLength:    r.uint8(),
					MaximumNumberTagLength: r.uint8(),
				},
			}
		})
		if err != nil {
			return fmt.Errorf("parsing QMI PBM additional-number capability array: %w", err)
		}
		c.AdditionalNumbers = additional
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		emails, err := decodePBMArray(value, func(r *pbmValueReader) PBMSessionEmailCapability {
			return PBMSessionEmailCapability{
				Session: PBMSessionType(r.uint8()),
				PBMEmailCapability: PBMEmailCapability{
					MaximumEmails:        r.uint8(),
					MaximumAddressLength: r.uint8(),
				},
			}
		})
		if err != nil {
			return fmt.Errorf("parsing QMI PBM email capability array: %w", err)
		}
		c.Emails = emails
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		secondNames, err := decodePBMArray(value, func(r *pbmValueReader) PBMSessionSecondNameCapability {
			return PBMSessionSecondNameCapability{
				Session: PBMSessionType(r.uint8()),
				PBMSecondNameCapability: PBMSecondNameCapability{
					MaximumLength: r.uint8(),
				},
			}
		})
		if err != nil {
			return fmt.Errorf("parsing QMI PBM second-name capability array: %w", err)
		}
		c.SecondNames = secondNames
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		hidden, err := decodePBMArray(value, func(r *pbmValueReader) PBMSessionHiddenRecordsCapability {
			return PBMSessionHiddenRecordsCapability{
				Session: PBMSessionType(r.uint8()),
				PBMHiddenRecordsCapability: PBMHiddenRecordsCapability{
					Supported: r.uint8() != 0,
				},
			}
		})
		if err != nil {
			return fmt.Errorf("parsing QMI PBM hidden-record capability array: %w", err)
		}
		c.HiddenRecords = hidden
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		gas, err := decodePBMSessionAlphaStringArray(value)
		if err != nil {
			return fmt.Errorf("parsing QMI PBM GAS capability array: %w", err)
		}
		c.GAS = gas
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		aas, err := decodePBMSessionAlphaStringArray(value)
		if err != nil {
			return fmt.Errorf("parsing QMI PBM AAS capability array: %w", err)
		}
		c.AAS = aas
	}
	return nil
}

// UnmarshalTLVs parses QMI PBM Get Emergency List output.
func (l *PBMEmergencyList) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*l = PBMEmergencyList{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		hardcoded, err := decodePBMStringArray(value)
		if err != nil {
			return fmt.Errorf("parsing QMI PBM hardcoded emergency numbers: %w", err)
		}
		l.Hardcoded = hardcoded
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		nv, err := decodePBMStringArray(value)
		if err != nil {
			return fmt.Errorf("parsing QMI PBM NV emergency numbers: %w", err)
		}
		l.NV = nv
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		card, err := decodePBMSessionEmergencyNumbers(value)
		if err != nil {
			return fmt.Errorf("parsing QMI PBM card emergency numbers: %w", err)
		}
		l.Card = card
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		network, err := decodePBMSessionEmergencyNumbers(value)
		if err != nil {
			return fmt.Errorf("parsing QMI PBM network emergency numbers: %w", err)
		}
		l.Network = network
	}
	return nil
}

// PBMSetIndicationRegistration updates the PBM indication event mask.
func (c *Client) PBMSetIndicationRegistration(ctx context.Context, mask PBMEventRegistrationMask) error {
	if mask&^pbmKnownEventRegistrationMask != 0 {
		return fmt.Errorf("configuring QMI PBM indications: event mask 0x%08X contains unknown bits", mask)
	}
	req := (PBMIndicationRegisterRequest{Timeout: DefaultRequestTimeout, Mask: mask}).Request()
	var response PBMIndicationRegisterResponse
	if err := c.pbmRequest(ctx, req, &response); err != nil {
		return fmt.Errorf("configuring QMI PBM indications: %w", err)
	}
	return nil
}

// PBMCapabilities reads capabilities for the selected session and phonebook
// mask.
func (c *Client) PBMCapabilities(ctx context.Context, session PBMSessionType, phonebook PBMPhonebookType) (PBMCapabilities, error) {
	req, err := (PBMGetCapabilitiesRequest{
		Timeout:   DefaultRequestTimeout,
		Session:   session,
		Phonebook: phonebook,
	}).Request()
	if err != nil {
		return PBMCapabilities{}, err
	}
	var capabilities PBMCapabilities
	if err := c.pbmRequest(ctx, req, &capabilities); err != nil {
		return PBMCapabilities{}, fmt.Errorf("reading QMI PBM capabilities: %w", err)
	}
	return capabilities, nil
}

// PBMAllCapabilities reads capabilities for every available PBM session.
func (c *Client) PBMAllCapabilities(ctx context.Context) (PBMAllCapabilities, error) {
	req := (PBMGetAllCapabilitiesRequest{Timeout: DefaultRequestTimeout}).Request()
	var capabilities PBMAllCapabilities
	if err := c.pbmRequest(ctx, req, &capabilities); err != nil {
		return PBMAllCapabilities{}, fmt.Errorf("reading all QMI PBM capabilities: %w", err)
	}
	return capabilities, nil
}

// PBMEmergencyList reads the modem's consolidated emergency-number list.
func (c *Client) PBMEmergencyList(ctx context.Context) (PBMEmergencyList, error) {
	req := (PBMGetEmergencyListRequest{Timeout: DefaultRequestTimeout}).Request()
	var list PBMEmergencyList
	if err := c.pbmRequest(ctx, req, &list); err != nil {
		return PBMEmergencyList{}, fmt.Errorf("reading QMI PBM emergency list: %w", err)
	}
	return list, nil
}

func (c *Client) pbmRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	c.pbmMu.Lock()
	defer c.pbmMu.Unlock()

	return c.withServiceClient(ctx, ServicePBM, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServicePBM, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return dst.UnmarshalTLVs(resp.TLVs)
	})
}

func validatePBMSessionType(session PBMSessionType) error {
	if session > PBMSessionGlobalPhonebookSlot3 {
		return fmt.Errorf("session type %d is out of range", session)
	}
	return nil
}

func decodePBMSingleBasicCapabilities(value []byte) (PBMBasicCapabilities, error) {
	if len(value) != 9 {
		return PBMBasicCapabilities{}, fmt.Errorf("parsing QMI PBM capabilities: basic capability TLV length %d, want 9", len(value))
	}
	r := newPBMValueReader(value)
	basic := PBMBasicCapabilities{
		Session:    PBMSessionType(r.uint8()),
		Phonebooks: []PBMPhonebookCapability{decodePBMPhonebookCapability(r)},
	}
	if err := r.finish(); err != nil {
		return PBMBasicCapabilities{}, fmt.Errorf("parsing QMI PBM basic capability: %w", err)
	}
	return basic, nil
}

func decodePBMBasicCapabilityArray(value []byte) ([]PBMBasicCapabilities, error) {
	r := newPBMValueReader(value)
	count := int(r.uint8())
	capabilities := make([]PBMBasicCapabilities, 0, count)
	for range count {
		basic := PBMBasicCapabilities{Session: PBMSessionType(r.uint8())}
		phonebookCount := int(r.uint8())
		basic.Phonebooks = make([]PBMPhonebookCapability, 0, phonebookCount)
		for range phonebookCount {
			basic.Phonebooks = append(basic.Phonebooks, decodePBMPhonebookCapability(r))
		}
		capabilities = append(capabilities, basic)
	}
	if err := r.finish(); err != nil {
		return nil, fmt.Errorf("parsing QMI PBM basic capability array: %w", err)
	}
	return capabilities, nil
}

func decodePBMPhonebookCapability(r *pbmValueReader) PBMPhonebookCapability {
	return PBMPhonebookCapability{
		Phonebook:           PBMPhonebookType(r.uint16()),
		UsedRecords:         r.uint16(),
		MaximumRecords:      r.uint16(),
		MaximumNumberLength: r.uint8(),
		MaximumNameLength:   r.uint8(),
	}
}

func (capability PBMAlphaStringCapability) MarshalBinary() ([]byte, error) {
	return []byte{capability.MaximumRecords, capability.UsedRecords, capability.MaximumStringLength}, nil
}

func (capability *PBMAlphaStringCapability) UnmarshalBinary(value []byte) error {
	if len(value) != 3 {
		return fmt.Errorf("alpha string capability length %d, want 3", len(value))
	}
	*capability = PBMAlphaStringCapability{
		MaximumRecords:      value[0],
		UsedRecords:         value[1],
		MaximumStringLength: value[2],
	}
	return nil
}

func decodePBMSessionAlphaStringArray(value []byte) ([]PBMSessionAlphaStringCapability, error) {
	return decodePBMArray(value, func(r *pbmValueReader) PBMSessionAlphaStringCapability {
		return PBMSessionAlphaStringCapability{
			Session: PBMSessionType(r.uint8()),
			PBMAlphaStringCapability: PBMAlphaStringCapability{
				MaximumRecords:      r.uint8(),
				UsedRecords:         r.uint8(),
				MaximumStringLength: r.uint8(),
			},
		}
	})
}

func decodePBMStringArray(value []byte) ([]string, error) {
	return decodePBMArray(value, func(r *pbmValueReader) string {
		return r.string8()
	})
}

func decodePBMSessionEmergencyNumbers(value []byte) ([]PBMSessionEmergencyNumbers, error) {
	return decodePBMArray(value, func(r *pbmValueReader) PBMSessionEmergencyNumbers {
		session := PBMSessionEmergencyNumbers{Session: PBMSessionType(r.uint8())}
		count := int(r.uint8())
		session.Numbers = make([]PBMEmergencyNumber, 0, count)
		for range count {
			session.Numbers = append(session.Numbers, PBMEmergencyNumber{
				Flags:  PBMEmergencyNumberFlags(r.uint8()),
				Number: r.string8(),
			})
		}
		return session
	})
}

func decodePBMArray[T any](value []byte, decode func(*pbmValueReader) T) ([]T, error) {
	r := newPBMValueReader(value)
	count := int(r.uint8())
	items := make([]T, 0, count)
	for range count {
		items = append(items, decode(r))
	}
	if err := r.finish(); err != nil {
		return nil, err
	}
	return items, nil
}

type pbmValueReader struct {
	value  []byte
	offset int
	err    error
}

func newPBMValueReader(value []byte) *pbmValueReader {
	return &pbmValueReader{value: value}
}

func (r *pbmValueReader) uint8() uint8 {
	value := r.bytes(1)
	if r.err != nil {
		return 0
	}
	return value[0]
}

func (r *pbmValueReader) uint16() uint16 {
	value := r.bytes(2)
	if r.err != nil {
		return 0
	}
	return binary.LittleEndian.Uint16(value)
}

func (r *pbmValueReader) string8() string {
	return string(r.bytes(int(r.uint8())))
}

func (r *pbmValueReader) bytes(length int) []byte {
	if r.err != nil {
		return nil
	}
	if len(r.value)-r.offset < length {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	value := r.value[r.offset : r.offset+length]
	r.offset += length
	return value
}

func (r *pbmValueReader) finish() error {
	if r.err != nil {
		return r.err
	}
	if r.offset != len(r.value) {
		return fmt.Errorf("%d trailing bytes", len(r.value)-r.offset)
	}
	return nil
}

func pbmEmptyRequest(clientID uint8, transactionID uint16, timeout time.Duration, message MessageID) Request {
	return Request{
		Service:       ServicePBM,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     message,
		Timeout:       timeout,
	}
}
