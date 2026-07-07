package qcom

import (
	"context"
	"net"
	"time"

	"github.com/damonto/uicc-go/qcom/tlv"
)

// ServiceType represents QMI service types.
type ServiceType uint8

const (
	ServiceControl ServiceType = 0x00 // Control service
	ServiceWDS     ServiceType = 0x01 // Wireless Data Service
	ServiceDMS     ServiceType = 0x02 // Device Management Service
	ServiceNAS     ServiceType = 0x03 // Network Access Service
	ServiceCAT2    ServiceType = 0x0A // Card Application Toolkit service v2
	ServiceUIM     ServiceType = 0x0B // UIM service
	ServiceIMSA    ServiceType = 0x21 // IMS Application service
	ServiceCAT     ServiceType = 0xE0 // Card Application Toolkit service v1
)

// MessageType represents QMI message types.
type MessageType uint8

const (
	MessageTypeRequest    MessageType = 0x00
	MessageTypeResponse   MessageType = 0x02
	MessageTypeIndication MessageType = 0x04
)

// MessageID represents QMI command message IDs.
type MessageID uint16

const (
	// CTL service commands
	MessageGetVersionInfo    MessageID = 0x0021
	MessageAllocateClientID  MessageID = 0x0022
	MessageReleaseClientID   MessageID = 0x0023
	MessageInternalProxyOpen MessageID = 0xFF00

	// WDS service commands
	MessageWDSStartNetworkInterface MessageID = 0x0020
	MessageWDSStopNetworkInterface  MessageID = 0x0021
	MessageWDSGetRuntimeSettings    MessageID = 0x002D

	// DMS service commands
	MessageDMSSetEventReport   MessageID = 0x0001
	MessageDMSGetOperatingMode MessageID = 0x002D
	MessageDMSSetOperatingMode MessageID = 0x002E

	// NAS service commands
	MessageNASGetSysInfo MessageID = 0x004D

	// IMSA service commands
	MessageIMSAGetRegistrationStatus MessageID = 0x0020
	MessageIMSAGetServiceStatus      MessageID = 0x0021

	// UIM service commands
	MessageReset                     MessageID = 0x0000
	MessageReadTransparent           MessageID = 0x0020
	MessageReadRecord                MessageID = 0x0021
	MessageGetFileAttributes         MessageID = 0x0024
	MessageRefreshRegister           MessageID = 0x002A
	MessageRefreshComplete           MessageID = 0x002C
	MessageRegisterEvents            MessageID = 0x002E
	MessagePowerOffSIM               MessageID = 0x0030
	MessagePowerOnSIM                MessageID = 0x0031
	MessageRefresh                   MessageID = 0x0033
	MessageChangeProvisioningSession MessageID = 0x0038
	MessageSendAPDU                  MessageID = 0x003B
	MessageOpenLogicalChannel        MessageID = 0x0042
	MessageCloseLogicalChannel       MessageID = 0x003F
	MessageGetATR                    MessageID = 0x0041
	MessageRefreshRegisterAll        MessageID = 0x0044
	MessageSwitchSlot                MessageID = 0x0046
	MessageGetSlotStatus             MessageID = 0x0047
	MessageSlotStatus                MessageID = 0x0048
	MessageGetCardStatus             MessageID = 0x002F
	MessageAuthenticate              MessageID = 0x0034

	// CAT/CAT2 service commands
	MessageCATSetEventReport       MessageID = 0x0001
	MessageCATEventReport          MessageID = 0x0001
	MessageCATGetServiceState      MessageID = 0x0020
	MessageCATSendTerminalResponse MessageID = 0x0021
	MessageSendEnvelope            MessageID = 0x0022
	MessageCATSendEnvelope         MessageID = 0x0022
	MessageCATEventConfirmation    MessageID = 0x0026
	MessageCATGetTerminalProfile   MessageID = 0x002C
	MessageCATSetConfiguration     MessageID = 0x002D
	MessageCATGetConfiguration     MessageID = 0x002E
)

// QMIResult represents the result code in QMI responses.
type QMIResult uint16

const (
	QMIResultSuccess QMIResult = 0x0000 // Success
	QMIResultFailure QMIResult = 0x0001 // Failure
)

// WDSIPFamily is the QMI WDS IP family preference value.
type WDSIPFamily uint8

const (
	WDSIPFamilyIPv4        WDSIPFamily = 4
	WDSIPFamilyIPv6        WDSIPFamily = 6
	WDSIPFamilyUnspecified WDSIPFamily = 8

	WDSIPFamilyIPv4v6 = WDSIPFamilyUnspecified
)

// WDSTechnologyPreference is the WDS technology preference bit mask.
type WDSTechnologyPreference uint8

const (
	WDSTechnologyPreference3GPP WDSTechnologyPreference = 1
)

// WDSCallEndReason is the basic WDS call end reason returned by start-network.
type WDSCallEndReason uint16

const (
	WDSCallEndReasonGenericUnspecified WDSCallEndReason = 1
)

// WDSVerboseCallEndReasonType selects the namespace for a verbose WDS call end reason.
type WDSVerboseCallEndReasonType uint16

const (
	WDSVerboseCallEndReasonTypeMIP      WDSVerboseCallEndReasonType = 1
	WDSVerboseCallEndReasonTypeInternal WDSVerboseCallEndReasonType = 2
	WDSVerboseCallEndReasonTypeCM       WDSVerboseCallEndReasonType = 3
	WDSVerboseCallEndReasonType3GPP     WDSVerboseCallEndReasonType = 6
	WDSVerboseCallEndReasonTypePPP      WDSVerboseCallEndReasonType = 7
	WDSVerboseCallEndReasonTypeEHRPD    WDSVerboseCallEndReasonType = 8
	WDSVerboseCallEndReasonTypeIPv6     WDSVerboseCallEndReasonType = 9
)

// WDSVerboseCallEndReason is the structured WDS call failure reason.
type WDSVerboseCallEndReason struct {
	Type   WDSVerboseCallEndReasonType
	Reason int16
}

// WDSRuntimeSettingsMask selects fields returned by WDS Get Runtime Settings.
type WDSRuntimeSettingsMask uint32

const (
	WDSRuntimeMaskIPAddress     WDSRuntimeSettingsMask = 0x00000100
	WDSRuntimeMaskPCSCFUsingPCO WDSRuntimeSettingsMask = 0x00000400
	WDSRuntimeMaskPCSCFServer   WDSRuntimeSettingsMask = 0x00000800
	WDSRuntimeMaskIPFamily      WDSRuntimeSettingsMask = 0x00008000
	WDSRuntimeMaskIMCNFlag      WDSRuntimeSettingsMask = 0x00010000

	WDSRuntimeRequestedIMSSettings = WDSRuntimeMaskIPAddress |
		WDSRuntimeMaskPCSCFUsingPCO |
		WDSRuntimeMaskPCSCFServer |
		WDSRuntimeMaskIPFamily |
		WDSRuntimeMaskIMCNFlag
)

// WDSRuntimeSettings holds IMS PDN addressing and P-CSCF data from WDS.
type WDSRuntimeSettings struct {
	LocalIPv4 net.IP
	LocalIPv6 net.IP
	PCSCFIPs  []net.IP
	IPFamily  WDSIPFamily
	IMCN      bool
}

// DMSOperatingMode is the QMI DMS modem operating mode.
type DMSOperatingMode uint8

const (
	DMSOperatingModeOnline             DMSOperatingMode = 0x00
	DMSOperatingModeLowPower           DMSOperatingMode = 0x01
	DMSOperatingModeFactoryTest        DMSOperatingMode = 0x02
	DMSOperatingModeOffline            DMSOperatingMode = 0x03
	DMSOperatingModeResetting          DMSOperatingMode = 0x04
	DMSOperatingModeShuttingDown       DMSOperatingMode = 0x05
	DMSOperatingModePersistentLowPower DMSOperatingMode = 0x06
	DMSOperatingModeModeOnlyLowPower   DMSOperatingMode = 0x07
	DMSOperatingModeNetworkTestGW      DMSOperatingMode = 0x08
)

// NASSysInfo is the NAS system information used by IMS access selection.
type NASSysInfo struct {
	VoPSKnown     bool
	VoPSSupported bool
}

// IMSRegistrationStatus is the QMI IMSA registration state.
type IMSRegistrationStatus uint32

const (
	IMSRegistrationStatusNotRegistered IMSRegistrationStatus = 0
	IMSRegistrationStatusRegistering   IMSRegistrationStatus = 1
	IMSRegistrationStatusRegistered    IMSRegistrationStatus = 2
)

// IMSServiceStatus is the QMI IMSA per-service availability state.
type IMSServiceStatus uint32

const (
	IMSServiceStatusNoService      IMSServiceStatus = 0
	IMSServiceStatusLimitedService IMSServiceStatus = 1
	IMSServiceStatusFullService    IMSServiceStatus = 2
)

// IMSServiceRAT is the access technology used by an IMS service.
type IMSServiceRAT uint32

const (
	IMSServiceRATWLAN  IMSServiceRAT = 0
	IMSServiceRATWWAN  IMSServiceRAT = 1
	IMSServiceRATIWLAN IMSServiceRAT = 2
)

// IMSAStatus contains IMS registration and VoIP service information from QMI IMSA.
type IMSAStatus struct {
	RegistrationKnown bool
	Registration      IMSRegistrationStatus
	FailureCodeKnown  bool
	FailureCode       uint16
	VoIPServiceKnown  bool
	VoIPService       IMSServiceStatus
	VoIPRATKnown      bool
	VoIPRAT           IMSServiceRAT
}

// IMSRegistered reports whether the modem is registered on IMS.
func (s IMSAStatus) IMSRegistered() bool {
	return s.RegistrationKnown && s.Registration == IMSRegistrationStatusRegistered
}

// VoLTERegistered reports whether IMS VoIP service is registered over WWAN.
func (s IMSAStatus) VoLTERegistered() bool {
	return s.IMSRegistered() &&
		s.VoIPServiceKnown && s.VoIPService == IMSServiceStatusFullService &&
		s.VoIPRATKnown && s.VoIPRAT == IMSServiceRATWWAN
}

type Request struct {
	Service       ServiceType
	ClientID      uint8
	TransactionID uint16
	MessageID     MessageID
	Timeout       time.Duration
	TLVs          tlv.TLVs
}

type Response struct {
	Service       ServiceType
	ClientID      uint8
	TransactionID uint16
	MessageID     MessageID
	TLVs          tlv.TLVs
}

// Indication is an unsolicited QMI message delivered outside a request/response
// transaction.
type Indication struct {
	Service       ServiceType
	ClientID      uint8
	TransactionID uint16
	MessageID     MessageID
	TLVs          tlv.TLVs
}

type Transport interface {
	Do(ctx context.Context, req Request) (Response, error)
	Close() error
}

// IndicationTransport extends Transport with best-effort indication delivery.
//
// Indications returns a channel for unsolicited messages matching service,
// clientID, and id. The channel is closed when ctx is done or the transport
// closes. Delivery is lossy: a slow subscriber may miss indications.
type IndicationTransport interface {
	Transport
	Indications(ctx context.Context, service ServiceType, clientID uint8, id MessageID) (<-chan Indication, error)
}

func RequestDeadline(ctx context.Context, timeout time.Duration) (time.Time, bool) {
	if deadline, ok := ctx.Deadline(); ok {
		if timeout <= 0 {
			return deadline, true
		}

		timeoutDeadline := time.Now().Add(timeout)
		if deadline.Before(timeoutDeadline) {
			return deadline, true
		}
		return timeoutDeadline, true
	}
	if timeout <= 0 {
		return time.Time{}, false
	}
	return time.Now().Add(timeout), true
}
