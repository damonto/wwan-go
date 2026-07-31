package qcom

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// ServiceType represents QMI service types. QRTR uses a 32-bit service ID;
// QMUX transports accept only the legacy 8-bit subset.
type ServiceType uint32

const (
	ServiceControl ServiceType = 0x00        // Control service
	ServiceWDS     ServiceType = 0x01        // Wireless Data Service
	ServiceDMS     ServiceType = 0x02        // Device Management Service
	ServiceNAS     ServiceType = 0x03        // Network Access Service
	ServiceQOS     ServiceType = 0x04        // Quality of Service service
	ServiceWMS     ServiceType = 0x05        // Wireless Messaging Service
	ServicePDS     ServiceType = 0x06        // Position Determination Service
	ServiceAUTH    ServiceType = 0x07        // Authentication service
	ServiceEAP     ServiceType = ServiceAUTH // Deprecated: use ServiceAUTH.
	ServiceVoice   ServiceType = 0x09        // Voice service
	ServiceCAT2    ServiceType = 0x0A        // Card Application Toolkit service v2
	ServiceUIM     ServiceType = 0x0B        // UIM service
	ServiceSAR     ServiceType = 0x11        // Specific Absorption Rate service
	ServiceIMS     ServiceType = 0x12        // IMS Settings service
	ServiceIMSS    ServiceType = ServiceIMS  // Deprecated: use ServiceIMS.
	ServicePBM     ServiceType = 0x0C        // Phonebook Manager service
	ServiceLOC     ServiceType = 0x10        // Location service
	ServiceWDA     ServiceType = 0x1A        // Wireless Data Administrative service
	ServiceIMSP    ServiceType = 0x1F        // IMS Presence service
	ServicePDC     ServiceType = 0x24        // Persistent Device Configuration service
	ServiceDSD     ServiceType = 0x2A        // Data System Determination service
	ServiceDPM     ServiceType = 0x2F        // Data Port Mapper service
	ServiceIMSA    ServiceType = 0x21        // IMS Application service
	ServiceCAT     ServiceType = 0xE0        // Card Application Toolkit service v1
	ServiceOMA     ServiceType = 0xE2        // Open Mobile Alliance device management service
	ServiceSSC     ServiceType = 0x190       // Snapdragon Sensor Core service
	ServiceIMSDCM  ServiceType = 0x302       // IMS DCM service
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
	MessageCTLSetInstanceID     MessageID = 0x0020
	MessageGetVersionInfo       MessageID = 0x0021
	MessageAllocateClientID     MessageID = 0x0022
	MessageReleaseClientID      MessageID = 0x0023
	MessageCTLSetDataFormat     MessageID = 0x0026
	MessageCTLSync              MessageID = 0x0027
	MessageInternalProxyOpen    MessageID = 0xFF00
	MessageGetSupportedMessages MessageID = 0x001E
	MessageGetSupportedFields   MessageID = 0x001F

	// WDS service commands
	MessageWDSReset                          MessageID = 0x0000
	MessageWDSSetEventReport                 MessageID = 0x0001
	MessageWDSEventReport                    MessageID = MessageWDSSetEventReport
	MessageWDSAbort                          MessageID = 0x0002
	MessageWDSIndicationRegister             MessageID = 0x0003
	MessageWDSStartNetworkInterface          MessageID = 0x0020
	MessageWDSStopNetworkInterface           MessageID = 0x0021
	MessageWDSGetPacketServiceStatus         MessageID = 0x0022
	MessageWDSGetCurrentChannelRate          MessageID = 0x0023
	MessageWDSGetPacketStatistics            MessageID = 0x0024
	MessageWDSGoDormant                      MessageID = 0x0025
	MessageWDSGoActive                       MessageID = 0x0026
	MessageWDSCreateProfile                  MessageID = 0x0027
	MessageWDSModifyProfile                  MessageID = 0x0028
	MessageWDSDeleteProfile                  MessageID = 0x0029
	MessageWDSGetProfileList                 MessageID = 0x002A
	MessageWDSGetProfileSettings             MessageID = 0x002B
	MessageWDSGetDefaultSettings             MessageID = 0x002C
	MessageWDSGetRuntimeSettings             MessageID = 0x002D
	MessageWDSGetDormancyStatus              MessageID = 0x0030
	MessageWDSGetAutoconnectSettings         MessageID = 0x0034
	MessageWDSGetDataBearerTechnology        MessageID = 0x0037
	MessageWDSGetCurrentDataBearerTechnology MessageID = 0x0044
	MessageWDSGetDefaultProfile              MessageID = 0x0049
	MessageWDSSetDefaultProfile              MessageID = 0x004A
	MessageWDSSetClientIPFamily              MessageID = 0x004D
	MessageWDSSetAutoconnectSettings         MessageID = 0x0051
	MessageWDSGetPDNThrottleInfo             MessageID = 0x006C
	MessageWDSGetLTEAttachParameters         MessageID = 0x0085
	MessageWDSLegacyBindMuxDataPort          MessageID = 0x0089
	MessageWDSExtendedIPConfig               MessageID = 0x008C
	MessageWDSGetDataBearerTechnologyEx      MessageID = 0x0091
	MessageWDSGetMaxLTEAttachPDNNumber       MessageID = 0x0092
	MessageWDSSetLTEAttachPDNList            MessageID = 0x0093
	MessageWDSGetLTEAttachPDNList            MessageID = 0x0094
	MessageWDSBindMuxDataPort                MessageID = 0x00A2
	MessageWDSConfigureProfileEventList      MessageID = 0x00A7
	MessageWDSProfileChanged                 MessageID = 0x00A8
	MessageWDSBindSubscription               MessageID = 0x00AF
	MessageWDSGetBindSubscription            MessageID = 0x00B0

	// QOS service commands
	MessageQOSReset                       MessageID = 0x0000
	MessageQOSSetEventReport              MessageID = 0x0001
	MessageQOSRequest                     MessageID = 0x0020
	MessageQOSRelease                     MessageID = 0x0021
	MessageQOSSuspend                     MessageID = 0x0022
	MessageQOSResume                      MessageID = 0x0023
	MessageQOSGetGranted                  MessageID = 0x0025
	MessageQOSGetStatus                   MessageID = 0x0026
	MessageQOSStatus                      MessageID = 0x0026
	MessageQOSGetNetworkStatus            MessageID = 0x0027
	MessageQOSNetworkStatus               MessageID = 0x0027
	MessageQOSGetNetworkSupportedProfiles MessageID = 0x0028
	MessageQOSPrimaryEvent                MessageID = 0x0029
	MessageQOSSetClientIPFamily           MessageID = 0x002A
	MessageQOSBindDataPort                MessageID = 0x002B
	MessageQOSGetFilterParams             MessageID = 0x002C
	MessageQOSBindSubscription            MessageID = 0x002D
	MessageQOSGetBindSubscription         MessageID = 0x002E
	MessageQOSIndicationRegister          MessageID = 0x002F
	MessageQOSRequestEx                   MessageID = 0x0030
	MessageQOSGlobalFlow                  MessageID = 0x0031
	MessageQOSModifyEx                    MessageID = 0x0032
	MessageQOSGetInfo                     MessageID = 0x0033
	MessageQOSPerformFlowOperation        MessageID = 0xFFFE

	// AUTH service commands
	MessageAUTHIndicationRegister  MessageID = 0x0003
	MessageAUTHStartEAPSession     MessageID = 0x0020
	MessageAUTHSendEAPPacketLegacy MessageID = 0x0021
	MessageAUTHEAPSessionResult    MessageID = 0x0022
	MessageAUTHGetEAPSessionKeys   MessageID = 0x0023
	MessageAUTHEndEAPSession       MessageID = 0x0024
	MessageAUTHRunAKA              MessageID = 0x0025
	MessageAUTHAKAResult           MessageID = 0x0026
	MessageAUTHBindSubscription    MessageID = 0x0027
	MessageAUTHGetBindSubscription MessageID = 0x0028
	MessageAUTHEAPNotification     MessageID = 0x0029
	MessageAUTHEAPError            MessageID = 0x002A
	MessageAUTHEAPReject           MessageID = 0x002B
	MessageAUTHSendEAPPacket       MessageID = 0x002C
	MessageAUTHGetEAPCredentials   MessageID = 0x002D

	// WMS service commands
	MessageWMSReset                  MessageID = 0x0000
	MessageWMSSetEventReport         MessageID = 0x0001
	MessageWMSRawSend                MessageID = 0x0020
	MessageWMSRawWrite               MessageID = 0x0021
	MessageWMSRawRead                MessageID = 0x0022
	MessageWMSModifyTag              MessageID = 0x0023
	MessageWMSDelete                 MessageID = 0x0024
	MessageWMSGetMessageProtocol     MessageID = 0x0030
	MessageWMSListMessages           MessageID = 0x0031
	MessageWMSSetRoutes              MessageID = 0x0032
	MessageWMSGetRoutes              MessageID = 0x0033
	MessageWMSSendAck                MessageID = 0x0037
	MessageWMSGetSMSCAddress         MessageID = 0x0034
	MessageWMSSetSMSCAddress         MessageID = 0x0035
	MessageWMSGetStoreMaxSize        MessageID = 0x0036
	MessageWMSSetRetryPeriod         MessageID = 0x0038
	MessageWMSSetRetryInterval       MessageID = 0x0039
	MessageWMSSetDCAutoDisconnect    MessageID = 0x003A
	MessageWMSSetMemoryStatus        MessageID = 0x003B
	MessageWMSSetBroadcastActivation MessageID = 0x003C
	MessageWMSSetBroadcastConfig     MessageID = 0x003D
	MessageWMSGetBroadcastConfig     MessageID = 0x003E
	MessageWMSGetDomainPreference    MessageID = 0x0040
	MessageWMSSetDomainPreference    MessageID = 0x0041
	MessageWMSSendFromMemoryStore    MessageID = 0x0042
	MessageWMSGetMessageWaiting      MessageID = 0x0043
	MessageWMSMessageWaiting         MessageID = 0x0044
	MessageWMSSetPrimaryClient       MessageID = 0x0045
	MessageWMSSMSCAddress            MessageID = 0x0046
	MessageWMSIndicationRegister     MessageID = 0x0047
	MessageWMSGetTransportLayer      MessageID = 0x0048
	MessageWMSTransportLayer         MessageID = 0x0049
	MessageWMSGetTransportNetwork    MessageID = 0x004A
	MessageWMSTransportNetwork       MessageID = 0x004B
	MessageWMSBindSubscription       MessageID = 0x004C
	MessageWMSGetIndicationRegister  MessageID = 0x004D
	MessageWMSGetSMSParameters       MessageID = 0x004E
	MessageWMSSetSMSParameters       MessageID = 0x004F
	MessageWMSCallStatus             MessageID = 0x0050
	MessageWMSGetDomainPrefConfig    MessageID = 0x0051
	MessageWMSSetDomainPrefConfig    MessageID = 0x0052
	MessageWMSGetRetryPeriod         MessageID = 0x0053
	MessageWMSGetRetryInterval       MessageID = 0x0054
	MessageWMSGetDCAutoDisconnect    MessageID = 0x0055
	MessageWMSGetServiceReadyState   MessageID = 0x005C
	MessageWMSGetMemoryStatus        MessageID = 0x0056
	MessageWMSGetPrimaryClient       MessageID = 0x0057
	MessageWMSGetSubscription        MessageID = 0x0058
	MessageWMSAsyncRawSend           MessageID = 0x0059
	MessageWMSAsyncSendAck           MessageID = 0x005A
	MessageWMSAsyncSendFromStore     MessageID = 0x005B
	MessageWMSSetMessageWaiting      MessageID = 0x005F
	MessageWMSEventReport            MessageID = 0x0001
	MessageWMSServiceReady           MessageID = 0x005D
	MessageWMSBroadcastConfigChanged MessageID = 0x005E
	MessageWMSMemoryFull             MessageID = 0x003F
	MessageWMSTransportMWI           MessageID = 0x0060

	// PDS service commands
	MessagePDSReset                     MessageID = 0x0000
	MessagePDSSetEventReport            MessageID = 0x0001
	MessagePDSEventReport               MessageID = MessagePDSSetEventReport
	MessagePDSGetGPSServiceState        MessageID = 0x0020
	MessagePDSSetGPSServiceState        MessageID = 0x0021
	MessagePDSGetDefaultTrackingSession MessageID = 0x0029
	MessagePDSSetDefaultTrackingSession MessageID = 0x002A
	MessagePDSGetAGPSConfig             MessageID = 0x002E
	MessagePDSSetAGPSConfig             MessageID = 0x002F
	MessagePDSGetAutoTrackingState      MessageID = 0x0030
	MessagePDSSetAutoTrackingState      MessageID = 0x0031
	MessagePDSGPSReady                  MessageID = 0x0060

	// PBM service commands
	MessagePBMIndicationRegister           MessageID = 0x0001
	MessagePBMGetCapabilities              MessageID = 0x0002
	MessagePBMGetAllCapabilities           MessageID = 0x0003
	MessagePBMReadRecords                  MessageID = 0x0004
	MessagePBMRecordRead                   MessageID = MessagePBMReadRecords
	MessagePBMWriteRecord                  MessageID = 0x0005
	MessagePBMDeleteRecord                 MessageID = 0x0006
	MessagePBMDeleteAllRecords             MessageID = 0x0007
	MessagePBMSearchRecords                MessageID = 0x0008
	MessagePBMRecordUpdate                 MessageID = 0x0009
	MessagePBMRefresh                      MessageID = 0x000A
	MessagePBMPhonebookReady               MessageID = 0x000B
	MessagePBMEmergencyListChanged         MessageID = 0x000C
	MessagePBMAllPhonebooksReady           MessageID = 0x000D
	MessagePBMGetEmergencyList             MessageID = 0x000E
	MessagePBMGetAllGroups                 MessageID = 0x000F
	MessagePBMSetGroup                     MessageID = 0x0010
	MessagePBMGetPhonebookState            MessageID = 0x0011
	MessagePBMReadAllHiddenRecords         MessageID = 0x0012
	MessagePBMHiddenRecordStatus           MessageID = 0x0013
	MessagePBMGetNextEmptyRecord           MessageID = 0x0014
	MessagePBMGetNextRecord                MessageID = 0x0015
	MessagePBMGetAllAAS                    MessageID = 0x0016
	MessagePBMSetAAS                       MessageID = 0x0017
	MessagePBMAASUpdate                    MessageID = 0x0018
	MessagePBMGASUpdate                    MessageID = 0x0019
	MessagePBMBindSubscription             MessageID = 0x001A
	MessagePBMGetSubscription              MessageID = 0x001B
	MessagePBMReadPBSetCapabilities        MessageID = 0x001C
	MessagePBMPBSetCapabilityRead          MessageID = MessagePBMReadPBSetCapabilities
	MessagePBMReadRecordsExtended          MessageID = 0x001D
	MessagePBMRecordReadExtended           MessageID = MessagePBMReadRecordsExtended
	MessagePBMWriteRecordExtended          MessageID = 0x001E
	MessagePBMSearchRecordsExtended        MessageID = 0x001F
	MessagePBMReadAllHiddenRecordsExtended MessageID = 0x0020
	MessagePBMSIMReady                     MessageID = 0x0021
	MessagePBMReadRecordsExtendedUndecoded MessageID = 0x0022
	MessagePBMRecordReadExtendedUndecoded  MessageID = MessagePBMReadRecordsExtendedUndecoded
	MessagePBMSetConfiguration             MessageID = 0x0023
	MessagePBMGetConfiguration             MessageID = 0x0024

	// OMA service commands
	MessageOMAReset             MessageID = 0x0000
	MessageOMASetEventReport    MessageID = 0x0001
	MessageOMAEventReport       MessageID = MessageOMASetEventReport
	MessageOMAStartSession      MessageID = 0x0020
	MessageOMACancelSession     MessageID = 0x0021
	MessageOMAGetSessionInfo    MessageID = 0x0022
	MessageOMASendSelection     MessageID = 0x0023
	MessageOMAGetFeatureSetting MessageID = 0x0024
	MessageOMASetFeatureSetting MessageID = 0x0025

	// LOC service commands
	MessageLOCRegisterEvents               MessageID = 0x0021
	MessageLOCStart                        MessageID = 0x0022
	MessageLOCStop                         MessageID = 0x0023
	MessageLOCPositionReport               MessageID = 0x0024
	MessageLOCGNSSSatelliteInfo            MessageID = 0x0025
	MessageLOCNMEA                         MessageID = 0x0026
	MessageLOCInjectTimeRequest            MessageID = 0x0028
	MessageLOCInjectPredictedOrbitsRequest MessageID = 0x0029
	MessageLOCInjectPositionRequest        MessageID = 0x002A
	MessageLOCEngineState                  MessageID = 0x002B
	MessageLOCFixRecurrence                MessageID = 0x002C
	MessageLOCInjectPredictedOrbitsData    MessageID = 0x0035
	MessageLOCGetPredictedOrbitsDataSource MessageID = 0x0036
	MessageLOCGetPredictedOrbitsValidity   MessageID = 0x0037
	MessageLOCInjectUTCTime                MessageID = 0x0038
	MessageLOCInjectPosition               MessageID = 0x0039
	MessageLOCSetEngineLock                MessageID = 0x003A
	MessageLOCGetEngineLock                MessageID = 0x003B
	MessageLOCSetNMEATypes                 MessageID = 0x003E
	MessageLOCGetNMEATypes                 MessageID = 0x003F
	MessageLOCSetServer                    MessageID = 0x0042
	MessageLOCGetServer                    MessageID = 0x0043
	MessageLOCDeleteAssistanceData         MessageID = 0x0044
	MessageLOCSetOperationMode             MessageID = 0x004A
	MessageLOCGetOperationMode             MessageID = 0x004B
	MessageLOCInjectXTRAData               MessageID = 0x00A7

	// Voice service commands
	MessageVoiceIndicationRegister  MessageID = 0x0003
	MessageVoiceDialCall            MessageID = 0x0020
	MessageVoiceEndCall             MessageID = 0x0021
	MessageVoiceAnswerCall          MessageID = 0x0022
	MessageVoiceGetCallInfo         MessageID = 0x0024
	MessageVoiceSendFlash           MessageID = 0x0027
	MessageVoiceBurstDTMF           MessageID = 0x0028
	MessageVoiceStartContinuousDTMF MessageID = 0x0029
	MessageVoiceStopContinuousDTMF  MessageID = 0x002A
	MessageVoiceDTMF                MessageID = 0x002B
	MessageVoiceSetPreferredPrivacy MessageID = 0x002C
	MessageVoicePrivacy             MessageID = 0x002D
	MessageVoiceAllCallStatus       MessageID = 0x002E
	MessageVoiceGetAllCallInfo      MessageID = 0x002F
	MessageVoiceManageCalls         MessageID = 0x0031
	MessageVoiceSupplementaryNotify MessageID = 0x0032
	MessageVoiceSetSupplementary    MessageID = 0x0033
	MessageVoiceGetCallWaiting      MessageID = 0x0034
	MessageVoiceGetCallBarring      MessageID = 0x0035
	MessageVoiceGetCLIP             MessageID = 0x0036
	MessageVoiceGetCLIR             MessageID = 0x0037
	MessageVoiceGetCallForwarding   MessageID = 0x0038
	MessageVoiceSetBarringPassword  MessageID = 0x0039
	MessageVoiceOriginateUSSD       MessageID = 0x003A
	MessageVoiceAnswerUSSD          MessageID = 0x003B
	MessageVoiceCancelUSSD          MessageID = 0x003C
	MessageVoiceUSSDRelease         MessageID = 0x003D
	MessageVoiceUSSD                MessageID = 0x003E
	MessageVoiceSetConfig           MessageID = 0x0040
	MessageVoiceGetConfig           MessageID = 0x0041
	MessageVoiceSupplementaryResult MessageID = 0x0042
	MessageVoiceOriginateUSSDNoWait MessageID = 0x0043
	MessageVoiceBindSubscription    MessageID = 0x0044
	MessageVoiceSetALSLineSwitching MessageID = 0x0045
	MessageVoiceSelectALSLine       MessageID = 0x0046
	MessageVoiceGetCOLP             MessageID = 0x004B
	MessageVoiceGetCOLR             MessageID = 0x004C
	MessageVoiceGetCNAP             MessageID = 0x004D
	MessageVoiceManageIPCalls       MessageID = 0x004E
	MessageVoiceALSLineSwitching    MessageID = 0x004F
	MessageVoiceALSSelectedLine     MessageID = 0x0050
	MessageVoiceCallModified        MessageID = 0x0051
	MessageVoiceCallModifyRequest   MessageID = 0x0052
	MessageVoiceSpeechCodecInfo     MessageID = 0x0053
	MessageVoiceHandover            MessageID = 0x0054
	MessageVoiceSetupAnswer         MessageID = 0x005C
	MessageVoiceTTY                 MessageID = 0x005D
	MessageVoiceGetSpeechCodecInfo  MessageID = 0x006E
	MessageVoiceCancelIPOperation   MessageID = 0x006F

	// WDA service commands
	MessageWDASetDataFormat          MessageID = 0x0020
	MessageWDAGetDataFormat          MessageID = 0x0021
	MessageWDAPacketFilterEnable     MessageID = 0x0022
	MessageWDAPacketFilterDisable    MessageID = 0x0023
	MessageWDAPacketFilterGetState   MessageID = 0x0024
	MessageWDAPacketFilterAddRule    MessageID = 0x0025
	MessageWDAPacketFilterDeleteRule MessageID = 0x0026
	MessageWDAPacketFilterGetHandles MessageID = 0x0027
	MessageWDAPacketFilterGetRule    MessageID = 0x0028
	MessageWDASetLoopbackState       MessageID = 0x0029
	MessageWDAGetLoopbackState       MessageID = 0x002A
	MessageWDASetQMAPSettings        MessageID = 0x002B
	MessageWDAGetQMAPSettings        MessageID = 0x002C
	MessageWDASetPowersaveConfig     MessageID = 0x002D
	MessageWDASetPowersaveMode       MessageID = 0x002E
	MessageWDASetLoopbackConfig      MessageID = 0x002F
	MessageWDALoopbackConfigResult   MessageID = 0x002F
	MessageWDASetCapability          MessageID = 0x0030
	MessageWDAGetEthernetConfig      MessageID = 0x0033

	// DPM service commands
	MessageDPMOpenPort        MessageID = 0x0020
	MessageDPMClosePort       MessageID = 0x0021
	MessageDPMGetCapabilities MessageID = 0x0022

	// DSD service commands
	MessageDSDGetSystemStatus     MessageID = 0x0024
	MessageDSDSystemStatusChange  MessageID = 0x0025
	MessageDSDSystemStatus        MessageID = 0x0026
	MessageDSDBindSubscription    MessageID = 0x0027
	MessageDSDGetBindSubscription MessageID = 0x0028
	MessageDSDGetAPNInfo          MessageID = 0x0033
	MessageDSDIndicationRegister  MessageID = 0x0038
	MessageDSDSwitchDDS           MessageID = 0x004E
	MessageDSDGetCurrentDDS       MessageID = 0x004F
	MessageDSDCurrentDDS          MessageID = 0x0050
	MessageDSDSetAPNType          MessageID = 0x0051

	// PDC service commands
	MessagePDCReset                MessageID = 0x0000
	MessagePDCRegister             MessageID = 0x0020
	MessagePDCConfigChange         MessageID = 0x0021
	MessagePDCGetSelectedConfig    MessageID = 0x0022
	MessagePDCSetSelectedConfig    MessageID = 0x0023
	MessagePDCListConfigs          MessageID = 0x0024
	MessagePDCDeleteConfig         MessageID = 0x0025
	MessagePDCLoadConfig           MessageID = 0x0026
	MessagePDCActivateConfig       MessageID = 0x0027
	MessagePDCGetConfigInfo        MessageID = 0x0028
	MessagePDCGetConfigLimits      MessageID = 0x0029
	MessagePDCGetDefaultConfigInfo MessageID = 0x002A
	MessagePDCDeactivateConfig     MessageID = 0x002B
	MessagePDCValidateConfig       MessageID = 0x002C
	MessagePDCGetFeature           MessageID = 0x002D
	MessagePDCSetFeature           MessageID = 0x002E
	MessagePDCRefresh              MessageID = 0x002F
	MessagePDCGetConfig            MessageID = 0x0030
	MessagePDCNotification         MessageID = 0x0031

	// DMS service commands
	MessageDMSReset                     MessageID = 0x0000
	MessageDMSSetEventReport            MessageID = 0x0001
	MessageDMSIndicationRegister        MessageID = 0x0003
	MessageDMSUIMSetPINProtection       MessageID = 0x0027
	MessageDMSUIMVerifyPIN              MessageID = 0x0028
	MessageDMSUIMUnblockPIN             MessageID = 0x0029
	MessageDMSUIMChangePIN              MessageID = 0x002A
	MessageDMSUIMGetPINStatus           MessageID = 0x002B
	MessageDMSGetDeviceCapabilities     MessageID = 0x0020
	MessageDMSGetManufacturer           MessageID = 0x0021
	MessageDMSGetModelID                MessageID = 0x0022
	MessageDMSGetRevisionID             MessageID = 0x0023
	MessageDMSGetMSISDN                 MessageID = 0x0024
	MessageDMSGetSerialNumbers          MessageID = 0x0025
	MessageDMSGetPowerState             MessageID = 0x0026
	MessageDMSGetHardwareRevision       MessageID = 0x002C
	MessageDMSGetOperatingMode          MessageID = 0x002D
	MessageDMSSetOperatingMode          MessageID = 0x002E
	MessageDMSGetTime                   MessageID = 0x002F
	MessageDMSGetPRLVersion             MessageID = 0x0030
	MessageDMSGetActivationState        MessageID = 0x0031
	MessageDMSActivateAutomatic         MessageID = 0x0032
	MessageDMSActivateManual            MessageID = 0x0033
	MessageDMSGetUserLockState          MessageID = 0x0034
	MessageDMSSetUserLockState          MessageID = 0x0035
	MessageDMSSetUserLockCode           MessageID = 0x0036
	MessageDMSReadUserData              MessageID = 0x0037
	MessageDMSWriteUserData             MessageID = 0x0038
	MessageDMSReadERIFile               MessageID = 0x0039
	MessageDMSRestoreFactoryDefaults    MessageID = 0x003A
	MessageDMSValidateSPC               MessageID = 0x003B
	MessageDMSSetFirmwareID             MessageID = 0x003E
	MessageDMSUIMGetICCID               MessageID = 0x003C
	MessageDMSUIMGetCKStatus            MessageID = 0x0040
	MessageDMSUIMSetCKProtection        MessageID = 0x0041
	MessageDMSUIMUnblockCK              MessageID = 0x0042
	MessageDMSUIMGetIMSI                MessageID = 0x0043
	MessageDMSUIMGetState               MessageID = 0x0044
	MessageDMSGetBandCapabilities       MessageID = 0x0045
	MessageDMSGetFactorySKU             MessageID = 0x0046
	MessageDMSGetFirmwarePreference     MessageID = 0x0047
	MessageDMSSetFirmwarePreference     MessageID = 0x0048
	MessageDMSListStoredImages          MessageID = 0x0049
	MessageDMSDeleteStoredImage         MessageID = 0x004A
	MessageDMSSetTime                   MessageID = 0x004B
	MessageDMSGetStoredImageInfo        MessageID = 0x004C
	MessageDMSGetAltNetworkConfig       MessageID = 0x004D
	MessageDMSSetAltNetworkConfig       MessageID = 0x004E
	MessageDMSGetBootImageDownloadMode  MessageID = 0x004F
	MessageDMSSetBootImageDownloadMode  MessageID = 0x0050
	MessageDMSGetSoftwareVersion        MessageID = 0x0051
	MessageDMSSetSPC                    MessageID = 0x0052
	MessageDMSGetCurrentPRLInfo         MessageID = 0x0053
	MessageDMSBindSubscription          MessageID = 0x0054
	MessageDMSGetBindSubscription       MessageID = 0x0055
	MessageDMSSetAPSoftwareVersion      MessageID = 0x0056
	MessageDMSGetCDMALockMode           MessageID = 0x0057
	MessageDMSGetMACAddress             MessageID = 0x005C
	MessageDMSGetEncryptedSerialNumbers MessageID = 0x005D
	MessageDMSConfigureModemActivity    MessageID = 0x005E
	MessageDMSGetModemActivity          MessageID = 0x005F
	MessageDMSGetPSMConfig              MessageID = 0x0060
	MessageDMSEnterPSM                  MessageID = 0x0061
	MessageDMSPSMStatus                 MessageID = 0x0062
	MessageDMSGetUIStatus               MessageID = 0x0063
	MessageDMSSetUIStatus               MessageID = 0x0064
	MessageDMSSetDeviceCapabilityConfig MessageID = 0x0065
	MessageDMSSetPSMConfig              MessageID = 0x0066
	MessageDMSPSMConfigChanged          MessageID = 0x0067
	MessageDMSSetAPVersion              MessageID = 0x0069
	MessageDMSGetCapability             MessageID = 0x006A
	MessageDMSSetApplicationPriority    MessageID = 0x006B
	MessageDMSDevicePowerInfoRequest    MessageID = 0x0071
	MessageDMSInteractiveStateRequest   MessageID = 0x0072
	MessageDMSDevicePowerInfo           MessageID = 0x0073
	MessageDMSDeviceInteractiveState    MessageID = 0x0074

	// NAS service commands
	MessageNASReset                        MessageID = 0x0000
	MessageNASAbort                        MessageID = 0x0001
	MessageNASSetEventReport               MessageID = 0x0002
	MessageNASEventReport                  MessageID = MessageNASSetEventReport
	MessageNASIndicationRegister           MessageID = 0x0003
	MessageNASGetSignalStrength            MessageID = 0x0020
	MessageNASPerformNetworkScan           MessageID = 0x0021
	MessageNASInitiateNetworkRegister      MessageID = 0x0022
	MessageNASAttachDetach                 MessageID = 0x0023
	MessageNASGetServingSystem             MessageID = 0x0024
	MessageNASGetHomeNetwork               MessageID = 0x0025
	MessageNASGetPreferredNetworks         MessageID = 0x0026
	MessageNASSetPreferredNetworks         MessageID = 0x0027
	MessageNASGetForbiddenNetworks         MessageID = 0x0028
	MessageNASSetForbiddenNetworks         MessageID = 0x0029
	MessageNASSetTechnologyPreference      MessageID = 0x002A
	MessageNASGetTechnologyPreference      MessageID = 0x002B
	MessageNASGetAccessOverloadClass       MessageID = 0x002C
	MessageNASSetAccessOverloadClass       MessageID = 0x002D
	MessageNASGetNetworkSystemPreference   MessageID = 0x002E
	MessageNASGetRFBandInfo                MessageID = 0x0031
	MessageNASGetANAAAStatus               MessageID = 0x0032
	MessageNASSetSystemSelectionPreference MessageID = 0x0033
	MessageNASGetSystemSelectionPreference MessageID = 0x0034
	MessageNASGetOperatorName              MessageID = 0x0039
	MessageNASOperatorName                 MessageID = 0x003A
	MessageNASGetCellLocationInfo          MessageID = 0x0043
	MessageNASGetPLMNName                  MessageID = 0x0044
	MessageNASBindSubscription             MessageID = 0x0045
	MessageNASNetworkTime                  MessageID = 0x004C
	MessageNASGetSysInfo                   MessageID = 0x004D
	MessageNASSysInfo                      MessageID = 0x004E
	MessageNASGetSignalInfo                MessageID = 0x004F
	MessageNASConfigureSignalInfo          MessageID = 0x0050
	MessageNASSignalInfo                   MessageID = 0x0051
	MessageNASGetTxRxInfo                  MessageID = 0x005A
	MessageNASBlockLTEPLMN                 MessageID = 0x005E
	MessageNASUnblockLTEPLMN               MessageID = 0x005F
	MessageNASResetLTEPLMNBlocking         MessageID = 0x0060
	MessageNASCurrentPLMNName              MessageID = 0x0061
	MessageNASGetCDMAPositionInfo          MessageID = 0x0065
	MessageNASRFBandInfo                   MessageID = 0x0066
	MessageNASForceNetworkSearch           MessageID = 0x0067
	MessageNASNetworkReject                MessageID = 0x0068
	MessageNASConfigureSignalInfo2         MessageID = 0x006C
	MessageNASGetNetworkTime               MessageID = 0x007D
	MessageNASSetLTEBandPriority           MessageID = 0x0080
	MessageNASGetLTEBandPriority           MessageID = 0x0083
	MessageNASIncrementalNetworkScan       MessageID = 0x0085
	MessageNASSetDRX                       MessageID = 0x0088
	MessageNASGetDRX                       MessageID = 0x0089
	MessageNASSetDataRoaming               MessageID = 0x009A
	MessageNASGetDataRoaming               MessageID = 0x009B
	MessageNASGetLTECarrierAggregationInfo MessageID = 0x00AC
	MessageNASGetNegotiatedDRX             MessageID = 0x00AE
	MessageNASSetVoiceRoaming              MessageID = 0x00B7
	MessageNASGetVoiceRoaming              MessageID = 0x00B8
	MessageNASSetEDRX                      MessageID = 0x00BA
	MessageNASGetEDRX                      MessageID = 0x00BB
	MessageNASEDRXChangeInfo               MessageID = 0x00BF
	MessageNASSetEDRXParameters            MessageID = 0x00C0
	MessageNASGetEDRXParameters            MessageID = 0x00C1
	MessageNASAbortNetworkScan             MessageID = 0x00C2
	MessageNASBlockNR5GPLMN                MessageID = 0x00DE
	MessageNASUnblockNR5GPLMN              MessageID = 0x00DF
	MessageNASResetNR5GPLMNBlocking        MessageID = 0x00E0
	MessageNASSetENDCConfig                MessageID = 0x00E7
	MessageNASGetENDCConfig                MessageID = 0x00E8
	MessageNASSetNR5GBandPriority          MessageID = 0x00ED
	MessageNASGetNR5GBandPriority          MessageID = 0x00EE

	// IMSA service commands
	MessageIMSAGetRegistrationStatus MessageID = 0x0020
	MessageIMSAGetServiceStatus      MessageID = 0x0021
	MessageIMSARegisterIndications   MessageID = 0x0022
	MessageIMSARegistrationChanged   MessageID = 0x0023
	MessageIMSAServiceStatusChanged  MessageID = 0x0024
	MessageIMSABind                  MessageID = 0x0033
	MessageIMSAGetBind               MessageID = 0x0034

	// IMS service commands
	MessageIMSSSetRegistrationManagerConfig MessageID = 0x0021
	MessageIMSSGetRegistrationManagerConfig MessageID = 0x0026
	MessageIMSRegisterIndications           MessageID = 0x002A
	MessageIMSSetPolicyManagerSettings      MessageID = 0x0047
	MessageIMSGetPolicyManagerSettings      MessageID = 0x0048
	MessageIMSPolicyManagerSettings         MessageID = 0x0049
	MessageIMSSetServicesEnabled            MessageID = 0x008F
	MessageIMSGetServicesEnabled            MessageID = 0x0090
	MessageIMSServicesEnabled               MessageID = 0x0091
	MessageIMSBind                          MessageID = 0x0098

	// SAR service commands
	MessageSARRFSetState MessageID = 0x0001
	MessageSARRFGetState MessageID = 0x0002

	// IMSP service commands
	MessageIMSPGetEnablerState MessageID = 0x0024

	// IMS DCM service commands
	MessageIMSDCMPDPActivate   MessageID = 0x0020
	MessageIMSDCMPDPDeactivate MessageID = 0x0021

	// SSC service commands
	MessageSSCControl     MessageID = 0x0020
	MessageSSCReportSmall MessageID = 0x0021
	MessageSSCReportLarge MessageID = 0x0022

	// UIM service commands
	MessageReset                     MessageID = 0x0000
	MessageReadTransparent           MessageID = 0x0020
	MessageReadRecord                MessageID = 0x0021
	MessageWriteTransparent          MessageID = 0x0022
	MessageWriteRecord               MessageID = 0x0023
	MessageGetFileAttributes         MessageID = 0x0024
	MessageSetPINProtection          MessageID = 0x0025
	MessageVerifyPIN                 MessageID = 0x0026
	MessageUnblockPIN                MessageID = 0x0027
	MessageChangePIN                 MessageID = 0x0028
	MessageDepersonalization         MessageID = 0x0029
	MessageRefreshRegister           MessageID = 0x002A
	MessageRefreshComplete           MessageID = 0x002C
	MessageRegisterEvents            MessageID = 0x002E
	MessagePowerOffSIM               MessageID = 0x0030
	MessagePowerOnSIM                MessageID = 0x0031
	MessageCardStatus                MessageID = 0x0032
	MessageRefresh                   MessageID = 0x0033
	MessageChangeProvisioningSession MessageID = 0x0038
	MessageGetConfiguration          MessageID = 0x003A
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
	MessageRemoteUnlock              MessageID = 0x005D

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

// WDSIPFamily identifies the address family negotiated for an active WDS call.
type WDSIPFamily uint8

const (
	WDSIPFamilyIPv4 WDSIPFamily = 4
	WDSIPFamilyIPv6 WDSIPFamily = 6
)

// WDSIPPreference selects the address family requested when starting a WDS
// call. The zero value omits the optional QMI TLV and lets the modem use its
// default. QMI value 8 means unspecified; it is not an active dual-stack
// family value.
type WDSIPPreference uint8

const (
	WDSIPPreferenceDefault     WDSIPPreference = 0
	WDSIPPreferenceIPv4        WDSIPPreference = 4
	WDSIPPreferenceIPv6        WDSIPPreference = 6
	WDSIPPreferenceUnspecified WDSIPPreference = 8
)

func (p WDSIPPreference) String() string {
	switch p {
	case WDSIPPreferenceDefault:
		return "default"
	case WDSIPPreferenceIPv4:
		return "ipv4"
	case WDSIPPreferenceIPv6:
		return "ipv6"
	case WDSIPPreferenceUnspecified:
		return "unspecified"
	default:
		return fmt.Sprintf("preference-%d", p)
	}
}

// WDSCallType identifies the origin of a WDS packet-data call.
type WDSCallType uint8

const (
	WDSCallTypeLaptop WDSCallType = iota
	WDSCallTypeEmbedded
)

// WDSTechnologyPreference is the WDS technology preference bit mask.
type WDSTechnologyPreference uint8

const (
	WDSTechnologyPreference3GPP WDSTechnologyPreference = 1 << iota
	WDSTechnologyPreference3GPP2
)

// WDSExtendedTechnologyPreference selects one specific packet-data technology.
type WDSExtendedTechnologyPreference uint16

const (
	WDSExtendedTechnologyCDMA           WDSExtendedTechnologyPreference = 32769
	WDSExtendedTechnologyUMTS           WDSExtendedTechnologyPreference = 32772
	WDSExtendedTechnologyEPC            WDSExtendedTechnologyPreference = 34944
	WDSExtendedTechnologyEMBMS          WDSExtendedTechnologyPreference = 34946
	WDSExtendedTechnologyModemLinkLocal WDSExtendedTechnologyPreference = 34952
)

// WDSAuthenticationMask selects the authentication protocols offered for a
// packet-data call or stored profile. The zero value omits the optional field.
type WDSAuthenticationMask uint8

const (
	WDSAuthenticationPAP WDSAuthenticationMask = 1 << iota
	WDSAuthenticationCHAP
)

// WDSSIOPort identifies a legacy modem SIO data port.
type WDSSIOPort uint16

const (
	WDSSIOPortA2MuxRMNET0 WDSSIOPort = 0x0E04 + iota
	WDSSIOPortA2MuxRMNET1
	WDSSIOPortA2MuxRMNET2
	WDSSIOPortA2MuxRMNET3
	WDSSIOPortA2MuxRMNET4
	WDSSIOPortA2MuxRMNET5
	WDSSIOPortA2MuxRMNET6
	WDSSIOPortA2MuxRMNET7
)

// DataEndpointType identifies the physical data transport endpoint.
type DataEndpointType uint32

const (
	DataEndpointReserved DataEndpointType = iota
	DataEndpointHSIC
	DataEndpointHSUSB
	DataEndpointPCIe
	DataEndpointEmbedded
	DataEndpointBAMDMUX
)

// DataEndpoint identifies a physical data channel exposed by the modem.
type DataEndpoint struct {
	Type        DataEndpointType
	InterfaceID uint32
}

// WDSDataEndpointType is kept for source compatibility.
// Deprecated: use DataEndpointType.
type WDSDataEndpointType = DataEndpointType

const (
	WDSDataEndpointReserved = DataEndpointReserved
	WDSDataEndpointHSIC     = DataEndpointHSIC
	WDSDataEndpointHSUSB    = DataEndpointHSUSB
	WDSDataEndpointPCIe     = DataEndpointPCIe
	WDSDataEndpointEmbedded = DataEndpointEmbedded
	WDSDataEndpointBAMDMUX  = DataEndpointBAMDMUX
)

// WDSDataEndpoint is kept for source compatibility.
// Deprecated: use DataEndpoint.
type WDSDataEndpoint = DataEndpoint

// WDALinkLayerProtocol identifies the frames exchanged on the modem data port.
type WDALinkLayerProtocol uint32

const (
	WDALinkLayerEthernet WDALinkLayerProtocol = 0x01
	WDALinkLayerRawIP    WDALinkLayerProtocol = 0x02
)

// WDAAggregationProtocol identifies a modem data aggregation format.
type WDAAggregationProtocol uint32

const (
	WDAAggregationDisabled WDAAggregationProtocol = iota
	WDAAggregationTLP
	WDAAggregationQCNCM
	WDAAggregationMBIM
	WDAAggregationRNDIS
	WDAAggregationQMAP
	WDAAggregationQMAPv2
	WDAAggregationQMAPv3
	WDAAggregationQMAPv4
	WDAAggregationQMAPv5
)

// WDAQoSHeaderFormat identifies the optional uplink QoS header layout.
type WDAQoSHeaderFormat uint32

const (
	WDAQoSHeaderReserved WDAQoSHeaderFormat = iota
	WDAQoSHeader6Bytes
	WDAQoSHeader8Bytes
)

// WDAEthernetHardwareConfig identifies the modem's Ethernet PDU layout.
type WDAEthernetHardwareConfig uint32

const (
	WDAEthernetHardwareDefault WDAEthernetHardwareConfig = iota
	WDAEthernetHardwareVLANIP
	WDAEthernetHardwareNonVLANIP
)

// WDADataFormatConfig selects fields for WDA Set Data Format.
// Nil fields are omitted because every WDA data-format TLV is optional.
type WDADataFormatConfig struct {
	QoSEnabled                   *bool
	LinkLayerProtocol            *WDALinkLayerProtocol
	UplinkAggregation            *WDAAggregationProtocol
	DownlinkAggregation          *WDAAggregationProtocol
	NDPSignature                 *uint32
	DownlinkMaxDatagrams         *uint32
	DownlinkMaxSize              *uint32
	Endpoint                     *DataEndpoint
	QoSHeaderFormat              *WDAQoSHeaderFormat
	DownlinkMinimumPadding       *uint32
	TerminalEquipmentFlowControl *bool
}

// WDADataFormat contains data-format fields returned by the modem.
// A Known flag distinguishes an absent optional TLV from a zero value.
type WDADataFormat struct {
	QoSEnabled                        bool
	QoSEnabledKnown                   bool
	LinkLayerProtocol                 WDALinkLayerProtocol
	LinkLayerProtocolKnown            bool
	UplinkAggregation                 WDAAggregationProtocol
	UplinkAggregationKnown            bool
	DownlinkAggregation               WDAAggregationProtocol
	DownlinkAggregationKnown          bool
	NDPSignature                      uint32
	NDPSignatureKnown                 bool
	DownlinkMaxDatagrams              uint32
	DownlinkMaxDatagramsKnown         bool
	DownlinkMaxSize                   uint32
	DownlinkMaxSizeKnown              bool
	UplinkMaxDatagrams                uint32
	UplinkMaxDatagramsKnown           bool
	UplinkMaxSize                     uint32
	UplinkMaxSizeKnown                bool
	QoSHeaderFormat                   WDAQoSHeaderFormat
	QoSHeaderFormatKnown              bool
	DownlinkMinimumPadding            uint32
	DownlinkMinimumPaddingKnown       bool
	TerminalEquipmentFlowControl      bool
	TerminalEquipmentFlowControlKnown bool
}

// WDSMuxDataPort describes the logical data channel assigned to a WDS client.
type WDSMuxDataPort struct {
	Endpoint   *DataEndpoint
	MuxID      uint8
	Reversed   bool
	ClientType WDSClientType
}

type WDSClientType uint32

const (
	WDSClientTypeReserved WDSClientType = iota
	WDSClientTypeTethered
)

// WDSProfileType identifies a modem data-profile technology family.
type WDSProfileType uint8

const (
	WDSProfileType3GPP WDSProfileType = iota
	WDSProfileType3GPP2
	WDSProfileTypeEPC
)

// WDSProfileFamily identifies the profile family used by WDS default-profile operations.
type WDSProfileFamily uint8

const (
	WDSProfileFamilyEmbedded WDSProfileFamily = iota
	WDSProfileFamilyTethered
)

// WDSPDPType identifies the packet data protocol stored in a 3GPP profile.
type WDSPDPType uint8

const (
	WDSPDPTypeIPv4 WDSPDPType = iota
	WDSPDPTypePPP
	WDSPDPTypeIPv6
	WDSPDPTypeIPv4v6
	WDSPDPTypeNonIP
)

// WDSPDPHeaderCompression identifies the header-compression mode stored in a
// WDS profile.
type WDSPDPHeaderCompression uint8

const (
	WDSPDPHeaderCompressionOff WDSPDPHeaderCompression = iota
	WDSPDPHeaderCompressionManufacturerPreferred
	WDSPDPHeaderCompressionRFC1144
	WDSPDPHeaderCompressionRFC2507
	WDSPDPHeaderCompressionRFC3095
)

// WDSPDPDataCompression identifies the data-compression mode stored in a WDS
// profile.
type WDSPDPDataCompression uint8

const (
	WDSPDPDataCompressionOff WDSPDPDataCompression = iota
	WDSPDPDataCompressionManufacturerPreferred
	WDSPDPDataCompressionV42bis
	WDSPDPDataCompressionV44
)

// WDSPDPAccessControl identifies the access-control policy for a PDP context.
type WDSPDPAccessControl uint8

const (
	WDSPDPAccessControlNone WDSPDPAccessControl = iota
	WDSPDPAccessControlReject
	WDSPDPAccessControlPermission
)

// WDSAddressAllocationPreference identifies how the modem should obtain an IP
// address for a profile.
type WDSAddressAllocationPreference uint8

const (
	WDSAddressAllocationNAS WDSAddressAllocationPreference = iota
	WDSAddressAllocationDHCP
)

// WDSQoSClassIdentifier identifies an LTE QoS class.
type WDSQoSClassIdentifier uint8

const (
	WDSQoSClassNetworkAssigned WDSQoSClassIdentifier = iota
	WDSQoSClassGuaranteedBitrate1
	WDSQoSClassGuaranteedBitrate2
	WDSQoSClassGuaranteedBitrate3
	WDSQoSClassGuaranteedBitrate4
	WDSQoSClassNonGuaranteedBitrate5
	WDSQoSClassNonGuaranteedBitrate6
	WDSQoSClassNonGuaranteedBitrate7
	WDSQoSClassNonGuaranteedBitrate8
)

// WDSAPNTypeMask identifies the standardized uses assigned to an APN.
type WDSAPNTypeMask uint64

const (
	WDSAPNTypeDefault WDSAPNTypeMask = 1 << iota
	WDSAPNTypeIMS
	WDSAPNTypeMMS
	WDSAPNTypeDUN
	WDSAPNTypeSUPL
	WDSAPNTypeHIPRI
	WDSAPNTypeFOTA
	WDSAPNTypeCBS
	WDSAPNTypeIA
	WDSAPNTypeEmergency
	WDSAPNTypeUT
	WDSAPNTypeMCX
)

const wdsAPNTypeAll = WDSAPNTypeDefault |
	WDSAPNTypeIMS |
	WDSAPNTypeMMS |
	WDSAPNTypeDUN |
	WDSAPNTypeSUPL |
	WDSAPNTypeHIPRI |
	WDSAPNTypeFOTA |
	WDSAPNTypeCBS |
	WDSAPNTypeIA |
	WDSAPNTypeEmergency |
	WDSAPNTypeUT |
	WDSAPNTypeMCX

// WDSUMTSQoSWithSignaling combines UMTS QoS parameters with the signed
// signaling-indication value used by the QMI profile messages.
type WDSUMTSQoSWithSignaling struct {
	QoS                 WDSUMTSGrantedQoS
	SignalingIndication int8
}

// WDSLTEQoS contains LTE profile QoS parameters in bits per second.
type WDSLTEQoS struct {
	ClassIdentifier           WDSQoSClassIdentifier
	GuaranteedDownlinkBitrate uint32
	MaximumDownlinkBitrate    uint32
	GuaranteedUplinkBitrate   uint32
	MaximumUplinkBitrate      uint32
}

// WDSVLANRange is the inclusive VLAN ID range assigned to a profile.
type WDSVLANRange struct {
	Start uint16
	End   uint16
}

// WDSProfileID identifies a stored modem data profile.
type WDSProfileID struct {
	Type  WDSProfileType
	Index uint8
}

// WDSProfile is one entry returned by WDS Get Profile List.
type WDSProfile struct {
	ID   WDSProfileID
	Name string
}

// WDSProfileConfig contains fields used when creating a persistent packet-data
// profile. Invalid IP addresses and nil pointer fields are omitted.
type WDSProfileConfig struct {
	Type           WDSProfileType
	Name           string
	APN            string
	PDPType        WDSPDPType
	Username       string
	Password       string
	Authentication WDSAuthenticationMask

	HeaderCompression *WDSPDPHeaderCompression
	DataCompression   *WDSPDPDataCompression
	PrimaryIPv4DNS    netip.Addr
	SecondaryIPv4DNS  netip.Addr
	UMTSRequestedQoS  *WDSUMTSGrantedQoS
	UMTSMinimumQoS    *WDSUMTSGrantedQoS
	GPRSRequestedQoS  *WDSGPRSGrantedQoS
	GPRSMinimumQoS    *WDSGPRSGrantedQoS

	IPv4AddressPreference netip.Addr
	PCSCFUsingPCO         *bool
	PDPAccessControl      *WDSPDPAccessControl
	PCSCFUsingDHCP        *bool
	IMCN                  *bool
	PDPContextNumber      *uint8
	PDPContextSecondary   *bool
	PDPContextPrimaryID   *uint8
	IPv6AddressPreference netip.Addr

	UMTSRequestedQoSWithSignaling *WDSUMTSQoSWithSignaling
	UMTSMinimumQoSWithSignaling   *WDSUMTSQoSWithSignaling
	PrimaryIPv6DNS                netip.Addr
	SecondaryIPv6DNS              netip.Addr
	AddressAllocationPreference   *WDSAddressAllocationPreference
	LTEQoS                        *WDSLTEQoS
	APNDisabled                   *bool
	RoamingDisallowed             *bool
	VLAN                          *WDSVLANRange
	APNType                       *WDSAPNTypeMask
}

// WDSProfileUpdate contains optional changes for a stored profile. A nil
// field is left unchanged; a non-nil string may be empty to clear that field.
type WDSProfileUpdate struct {
	Name           *string
	APN            *string
	PDPType        *WDSPDPType
	Username       *string
	Password       *string
	Authentication *WDSAuthenticationMask

	HeaderCompression *WDSPDPHeaderCompression
	DataCompression   *WDSPDPDataCompression
	PrimaryIPv4DNS    *netip.Addr
	SecondaryIPv4DNS  *netip.Addr
	UMTSRequestedQoS  *WDSUMTSGrantedQoS
	UMTSMinimumQoS    *WDSUMTSGrantedQoS
	GPRSRequestedQoS  *WDSGPRSGrantedQoS
	GPRSMinimumQoS    *WDSGPRSGrantedQoS

	IPv4AddressPreference *netip.Addr
	PCSCFUsingPCO         *bool
	PDPAccessControl      *WDSPDPAccessControl
	PCSCFUsingDHCP        *bool
	IMCN                  *bool
	PDPContextNumber      *uint8
	PDPContextSecondary   *bool
	PDPContextPrimaryID   *uint8
	IPv6AddressPreference *netip.Addr

	UMTSRequestedQoSWithSignaling *WDSUMTSQoSWithSignaling
	UMTSMinimumQoSWithSignaling   *WDSUMTSQoSWithSignaling
	PrimaryIPv6DNS                *netip.Addr
	SecondaryIPv6DNS              *netip.Addr
	AddressAllocationPreference   *WDSAddressAllocationPreference
	LTEQoS                        *WDSLTEQoS
	APNDisabled                   *bool
	RoamingDisallowed             *bool
	VLAN                          *WDSVLANRange
	APNType                       *WDSAPNTypeMask
	CLATEnabled                   *bool
	IPv6PrefixDelegation          *bool
}

// WDSProfileSettings contains optional WDS profile fields. Each Known flag
// distinguishes an absent TLV from its type's zero value.
type WDSProfileSettings struct {
	ID WDSProfileID

	Name          string
	NameKnown     bool
	APN           string
	APNKnown      bool
	PDPType       WDSPDPType
	PDPKnown      bool
	Username      string
	UsernameKnown bool
	Password      string
	PasswordKnown bool

	Authentication      WDSAuthenticationMask
	AuthenticationKnown bool

	HeaderCompression      WDSPDPHeaderCompression
	HeaderCompressionKnown bool
	DataCompression        WDSPDPDataCompression
	DataCompressionKnown   bool
	PrimaryIPv4DNS         netip.Addr
	PrimaryIPv4DNSKnown    bool
	SecondaryIPv4DNS       netip.Addr
	SecondaryIPv4DNSKnown  bool
	UMTSRequestedQoS       WDSUMTSGrantedQoS
	UMTSRequestedQoSKnown  bool
	UMTSMinimumQoS         WDSUMTSGrantedQoS
	UMTSMinimumQoSKnown    bool
	GPRSRequestedQoS       WDSGPRSGrantedQoS
	GPRSRequestedQoSKnown  bool
	GPRSMinimumQoS         WDSGPRSGrantedQoS
	GPRSMinimumQoSKnown    bool

	IPv4AddressPreference      netip.Addr
	IPv4AddressPreferenceKnown bool
	PCSCFUsingPCO              bool
	PCSCFUsingPCOKnown         bool
	PDPAccessControl           WDSPDPAccessControl
	PDPAccessControlKnown      bool
	PCSCFUsingDHCP             bool
	PCSCFUsingDHCPKnown        bool
	IMCN                       bool
	IMCNKnown                  bool
	PDPContextNumber           uint8
	PDPContextNumberKnown      bool
	PDPContextSecondary        bool
	PDPContextSecondaryKnown   bool
	PDPContextPrimaryID        uint8
	PDPContextPrimaryIDKnown   bool
	IPv6AddressPreference      netip.Addr
	IPv6AddressPreferenceKnown bool

	UMTSRequestedQoSWithSignaling      WDSUMTSQoSWithSignaling
	UMTSRequestedQoSWithSignalingKnown bool
	UMTSMinimumQoSWithSignaling        WDSUMTSQoSWithSignaling
	UMTSMinimumQoSWithSignalingKnown   bool
	PrimaryIPv6DNS                     netip.Addr
	PrimaryIPv6DNSKnown                bool
	SecondaryIPv6DNS                   netip.Addr
	SecondaryIPv6DNSKnown              bool
	AddressAllocationPreference        WDSAddressAllocationPreference
	AddressAllocationPreferenceKnown   bool
	LTEQoS                             WDSLTEQoS
	LTEQoSKnown                        bool
	APNDisabled                        bool
	APNDisabledKnown                   bool
	RoamingDisallowed                  bool
	RoamingDisallowedKnown             bool
	VLAN                               WDSVLANRange
	VLANKnown                          bool
	APNType                            WDSAPNTypeMask
	APNTypeKnown                       bool
	CLATEnabled                        bool
	CLATEnabledKnown                   bool
	IPv6PrefixDelegation               bool
	IPv6PrefixDelegationKnown          bool
}

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

// Internal verbose call-end reasons returned in the WDS type-2 namespace.
const (
	WDSVerboseCallEndReasonInternalPDNIPv4CallDisallowed     int16 = 208
	WDSVerboseCallEndReasonInternalPDNIPv6CallDisallowed     int16 = 210
	WDSVerboseCallEndReasonInternalCallAlreadyPresent        int16 = 236
	WDSVerboseCallEndReasonInternalInterfaceInUse            int16 = 237
	WDSVerboseCallEndReasonInternalInterfaceInUseConfigMatch int16 = 241
)

// 3GPP verbose call-end reasons returned in the WDS type-6 namespace.
const (
	WDSVerboseCallEndReason3GPPOperatorDeterminedBarring    int16 = 8
	WDSVerboseCallEndReason3GPPLLCSNDCPFailure              int16 = 25
	WDSVerboseCallEndReason3GPPInsufficientResources        int16 = 26
	WDSVerboseCallEndReason3GPPUnknownAPN                   int16 = 27
	WDSVerboseCallEndReason3GPPUnknownPDP                   int16 = 28
	WDSVerboseCallEndReason3GPPAuthenticationFailed         int16 = 29
	WDSVerboseCallEndReason3GPPGGSNReject                   int16 = 30
	WDSVerboseCallEndReason3GPPActivationReject             int16 = 31
	WDSVerboseCallEndReason3GPPOptionNotSupported           int16 = 32
	WDSVerboseCallEndReason3GPPOptionUnsubscribed           int16 = 33
	WDSVerboseCallEndReason3GPPOptionTemporarilyOutOfOrder  int16 = 34
	WDSVerboseCallEndReason3GPPNSAPIAlreadyUsed             int16 = 35
	WDSVerboseCallEndReason3GPPRegularDeactivation          int16 = 36
	WDSVerboseCallEndReason3GPPQoSNotAccepted               int16 = 37
	WDSVerboseCallEndReason3GPPNetworkFailure               int16 = 38
	WDSVerboseCallEndReason3GPPUMTSReactivationRequested    int16 = 39
	WDSVerboseCallEndReason3GPPFeatureNotSupported          int16 = 40
	WDSVerboseCallEndReason3GPPTFTSemanticError             int16 = 41
	WDSVerboseCallEndReason3GPPTFTSyntaxError               int16 = 42
	WDSVerboseCallEndReason3GPPUnknownPDPContext            int16 = 43
	WDSVerboseCallEndReason3GPPFilterSemanticError          int16 = 44
	WDSVerboseCallEndReason3GPPFilterSyntaxError            int16 = 45
	WDSVerboseCallEndReason3GPPPDPWithoutActiveTFT          int16 = 46
	WDSVerboseCallEndReason3GPPIPv4OnlyAllowed              int16 = 50
	WDSVerboseCallEndReason3GPPIPv6OnlyAllowed              int16 = 51
	WDSVerboseCallEndReason3GPPSingleAddressBearerOnly      int16 = 52
	WDSVerboseCallEndReason3GPPInvalidTransactionID         int16 = 81
	WDSVerboseCallEndReason3GPPMessageIncorrectSemantic     int16 = 95
	WDSVerboseCallEndReason3GPPInvalidMandatoryInformation  int16 = 96
	WDSVerboseCallEndReason3GPPMessageTypeUnsupported       int16 = 97
	WDSVerboseCallEndReason3GPPMessageTypeIncompatibleState int16 = 98
	WDSVerboseCallEndReason3GPPUnknownInformationElement    int16 = 99
	WDSVerboseCallEndReason3GPPConditionalInformationError  int16 = 100
	WDSVerboseCallEndReason3GPPMessageProtocolStateMismatch int16 = 101
	WDSVerboseCallEndReason3GPPProtocolError                int16 = 111
	WDSVerboseCallEndReason3GPPAPNTypeConflict              int16 = 112
)

// WDSVerboseCallEndReason is the structured WDS call failure reason.
type WDSVerboseCallEndReason struct {
	Type   WDSVerboseCallEndReasonType
	Reason int16
}

// WDSRuntimeSettingsMask selects fields returned by WDS Get Runtime Settings.
type WDSRuntimeSettingsMask uint32

const (
	WDSRuntimeMaskProfileID WDSRuntimeSettingsMask = 1 << iota
	WDSRuntimeMaskProfileName
	WDSRuntimeMaskPDPType
	WDSRuntimeMaskAPN
	WDSRuntimeMaskDNSAddress
	WDSRuntimeMaskGrantedQoS
	WDSRuntimeMaskUsername
	WDSRuntimeMaskAuthentication
	WDSRuntimeMaskIPAddress
	WDSRuntimeMaskGatewayInfo
	WDSRuntimeMaskPCSCFUsingPCO
	WDSRuntimeMaskPCSCFServer
	WDSRuntimeMaskPCSCFDomainName
	WDSRuntimeMaskMTU
	WDSRuntimeMaskDomainName
	WDSRuntimeMaskIPFamily
	WDSRuntimeMaskIMCNFlag
	WDSRuntimeMaskExtendedTechnology
	WDSRuntimeMaskOperatorReservedPCO

	WDSRuntimeRequestedIMSSettings = WDSRuntimeMaskIPAddress |
		WDSRuntimeMaskPCSCFUsingPCO |
		WDSRuntimeMaskPCSCFServer |
		WDSRuntimeMaskIPFamily |
		WDSRuntimeMaskIMCNFlag

	WDSRuntimeRequestedNetworkSettings = WDSRuntimeMaskDNSAddress |
		WDSRuntimeMaskIPAddress |
		WDSRuntimeMaskGatewayInfo |
		WDSRuntimeMaskMTU |
		WDSRuntimeMaskIPFamily

	WDSRuntimeRequestedProfileSettings = WDSRuntimeMaskProfileID |
		WDSRuntimeMaskProfileName |
		WDSRuntimeMaskPDPType |
		WDSRuntimeMaskAPN |
		WDSRuntimeMaskUsername |
		WDSRuntimeMaskAuthentication |
		WDSRuntimeMaskExtendedTechnology
)

// WDSOperatorReservedPCO contains one operator-provided protocol configuration
// option returned for an active packet-data call.
type WDSOperatorReservedPCO struct {
	MCC                 uint16
	MNC                 uint16
	MNCIncludesPCSDigit bool
	AppSpecificInfo     []byte
	ContainerID         uint16
}

// WDSUMTSGrantedQoS contains the QoS values granted to an active UMTS call.
type WDSUMTSGrantedQoS struct {
	TrafficClass              uint8
	MaximumUplinkBitrate      uint32
	MaximumDownlinkBitrate    uint32
	GuaranteedUplinkBitrate   uint32
	GuaranteedDownlinkBitrate uint32
	DeliveryOrder             uint8
	MaximumSDUSize            uint32
	SDUErrorRatio             uint8
	ResidualBitErrorRatio     uint8
	ErroneousSDUDelivery      uint8
	TransferDelay             uint32
	TrafficHandlingPriority   uint32
}

// WDSGPRSGrantedQoS contains the QoS classes granted to an active GPRS call.
type WDSGPRSGrantedQoS struct {
	PrecedenceClass     uint32
	DelayClass          uint32
	ReliabilityClass    uint32
	PeakThroughputClass uint32
	MeanThroughputClass uint32
}

// WDSRuntimeSettings holds active packet-data profile and network settings.
type WDSRuntimeSettings struct {
	ProfileID           WDSProfileID
	ProfileIDKnown      bool
	ProfileName         string
	ProfileNameKnown    bool
	PDPType             WDSPDPType
	PDPTypeKnown        bool
	APN                 string
	APNKnown            bool
	Username            string
	UsernameKnown       bool
	Authentication      WDSAuthenticationMask
	AuthenticationKnown bool
	UMTSGrantedQoS      WDSUMTSGrantedQoS
	UMTSGrantedQoSKnown bool
	GPRSGrantedQoS      WDSGPRSGrantedQoS
	GPRSGrantedQoSKnown bool

	LocalIPv4                net.IP
	LocalIPv6                net.IP
	IPv4Gateway              net.IP
	IPv4SubnetMask           net.IP
	IPv6Gateway              net.IP
	IPv6PrefixLength         uint8
	DNS                      []net.IP
	MTU                      uint32
	PCSCFIPs                 []net.IP
	PCSCFUsingPCO            bool
	PCSCFUsingPCOKnown       bool
	PCSCFDomains             []string
	DomainNames              []string
	IPFamily                 WDSIPFamily
	IMCN                     bool
	ExtendedTechnology       WDSExtendedTechnologyPreference
	ExtendedTechnologyKnown  bool
	OperatorReservedPCO      WDSOperatorReservedPCO
	OperatorReservedPCOKnown bool
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

// NASRegistrationState is the network registration state reported by NAS.
type NASRegistrationState uint8

const (
	NASRegistrationNotRegistered NASRegistrationState = iota
	NASRegistrationRegistered
	NASRegistrationSearching
	NASRegistrationDenied
	NASRegistrationUnknown
)

// NASAttachState is a circuit-switched or packet-switched attach state.
type NASAttachState uint8

const (
	NASAttachUnknown NASAttachState = iota
	NASAttachAttached
	NASAttachDetached
)

// NASSelectedNetwork identifies the selected network family.
type NASSelectedNetwork uint8

const (
	NASSelectedNetworkUnknown NASSelectedNetwork = iota
	NASSelectedNetwork3GPP2
	NASSelectedNetwork3GPP
)

// NASRadioInterface identifies a radio interface currently in use.
type NASRadioInterface uint8

const (
	NASRadioInterfaceNoService NASRadioInterface = 0
	NASRadioInterfaceCDMA1X    NASRadioInterface = 1
	NASRadioInterfaceCDMAEVDO  NASRadioInterface = 2
	NASRadioInterfaceAMPS      NASRadioInterface = 3
	NASRadioInterfaceGSM       NASRadioInterface = 4
	NASRadioInterfaceUMTS      NASRadioInterface = 5
	NASRadioInterfaceWLAN      NASRadioInterface = 6
	NASRadioInterfaceGPS       NASRadioInterface = 7
	NASRadioInterfaceLTE       NASRadioInterface = 8
	NASRadioInterfaceTDSCDMA   NASRadioInterface = 9
	NASRadioInterfaceLTEM1     NASRadioInterface = 10
	NASRadioInterfaceLTENB1    NASRadioInterface = 11
	NASRadioInterfaceNR5G      NASRadioInterface = 12
	NASRadioInterfaceNoChange  NASRadioInterface = 0xFF
)

// NASServingSystem contains the fields from NAS Get Serving System.
type NASServingSystem struct {
	RegistrationState NASRegistrationState
	CSAttachState     NASAttachState
	PSAttachState     NASAttachState
	SelectedNetwork   NASSelectedNetwork
	RadioInterfaces   []NASRadioInterface

	RoamingIndicator      NASRoamingIndicator
	RoamingIndicatorKnown bool
	DataCapabilities      []NASDataCapability
	DataCapabilitiesKnown bool
	PLMN                  NASPLMN
	PLMNKnown             bool

	TimeZoneQuarterHours   int8
	TimeZoneKnown          bool
	DaylightSavingHours    uint8
	DaylightSavingKnown    bool
	LocationAreaCode       uint16
	LocationAreaKnown      bool
	CellID                 uint32
	CellIDKnown            bool
	TrackingAreaCode       uint16
	TrackingAreaKnown      bool
	NetworkNameSource      NASNetworkNameSource
	NetworkNameSourceKnown bool
}

// IMSRegistrationStatus is the QMI IMSA registration state.
type IMSRegistrationStatus uint32

const (
	IMSRegistrationStatusNotRegistered IMSRegistrationStatus = 0
	IMSRegistrationStatusRegistering   IMSRegistrationStatus = 1
	IMSRegistrationStatusRegistered    IMSRegistrationStatus = 2
	IMSRegistrationStatusLimited       IMSRegistrationStatus = 3
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
	RegistrationKnown             bool
	Registration                  IMSRegistrationStatus
	FailureCodeKnown              bool
	FailureCode                   uint16
	RegistrationErrorMessageKnown bool
	RegistrationErrorMessage      string
	RegistrationRATKnown          bool
	RegistrationRAT               IMSServiceRAT
	SMSServiceKnown               bool
	SMSService                    IMSServiceStatus
	SMSRATKnown                   bool
	SMSRAT                        IMSServiceRAT
	VoIPServiceKnown              bool
	VoIPService                   IMSServiceStatus
	VoIPRATKnown                  bool
	VoIPRAT                       IMSServiceRAT
	VTServiceKnown                bool
	VTService                     IMSServiceStatus
	VTRATKnown                    bool
	VTRAT                         IMSServiceRAT
	UTServiceKnown                bool
	UTService                     IMSServiceStatus
	UTRATKnown                    bool
	UTRAT                         IMSServiceRAT
	VSServiceKnown                bool
	VSService                     IMSServiceStatus
	VSRATKnown                    bool
	VSRAT                         IMSServiceRAT
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

// IndicationTransport extends Transport with indication delivery.
//
// Indications returns a channel for unsolicited messages matching service,
// clientID, and id. The channel is closed when ctx is done or the transport
// closes. Transports preserve indication order and do not silently drop
// messages while the subscription is active.
type IndicationTransport interface {
	Transport
	Indications(ctx context.Context, service ServiceType, clientID uint8, id MessageID) (<-chan Indication, error)
}

type FileStructure byte

const (
	FileStructureTransparent FileStructure = 0x41
	FileStructureLinearFixed FileStructure = 0x42
)

type FileType byte

const (
	FileTypeWorkingEF FileType = 0x21
	FileTypeDFOrADF   FileType = 0x38
)

type CardState byte

const (
	CardStateAbsent CardState = iota
	CardStatePresent
	CardStateError
)

type PhysicalCardState uint32

const (
	PhysicalCardStateUnknown PhysicalCardState = iota
	PhysicalCardStateAbsent
	PhysicalCardStatePresent
)

type SlotState uint32

const (
	SlotStateInactive SlotState = iota
	SlotStateActive
)

type CardProtocol uint32

const (
	CardProtocolUnknown CardProtocol = iota
	CardProtocolICC
	CardProtocolUICC
)

type QMIFileType byte

const (
	QMIFileTypeTransparent QMIFileType = iota
	QMIFileTypeCyclic
	QMIFileTypeLinearFixed
	QMIFileTypeDedicated
	QMIFileTypeMaster
)

type PINState byte

const (
	PINStateNotInitialized PINState = iota
	PINStateEnabledNotVerified
	PINStateEnabledVerified
	PINStateDisabled
	PINStateBlocked
	PINStatePermanentlyBlocked
)

type CardError byte

const (
	CardErrorUnknown CardError = iota
	CardErrorPowerDown
	CardErrorPoll
	CardErrorNoATRReceived
	CardErrorVoltageMismatch
	CardErrorParity
	CardErrorPossiblyRemoved
	CardErrorTechnical
	CardErrorNullBytes
	CardErrorSAPConnected
	CardErrorCommandTimeout
)

type ApplicationType byte

const (
	ApplicationTypeUnknown ApplicationType = iota
	ApplicationTypeSIM
	ApplicationTypeUSIM
	ApplicationTypeRUIM
	ApplicationTypeCSIM
	ApplicationTypeISIM
)

type ApplicationState byte

const (
	ApplicationStateUnknown ApplicationState = iota
	ApplicationStateDetected
	ApplicationStatePIN1OrUPINRequired
	ApplicationStatePUK1OrUPINRequired
	ApplicationStateCheckPersonalization
	ApplicationStatePIN1Blocked
	ApplicationStateIllegal
	ApplicationStateReady
)

type PersonalizationState byte

const (
	PersonalizationStateUnknown PersonalizationState = iota
	PersonalizationStateInProgress
	PersonalizationStateReady
	PersonalizationStateCodeRequired
	PersonalizationStatePUKCodeRequired
	PersonalizationStatePermanentlyBlocked
)

type PersonalizationFeature byte

const (
	PersonalizationFeatureGWNetwork PersonalizationFeature = iota
	PersonalizationFeatureGWNetworkSubset
	PersonalizationFeatureGWServiceProvider
	PersonalizationFeatureGWCorporate
	PersonalizationFeatureGWUIM
	PersonalizationFeatureOneXNetworkType1
	PersonalizationFeatureOneXNetworkType2
	PersonalizationFeatureOneXHRPD
	PersonalizationFeatureOneXServiceProvider
	PersonalizationFeatureOneXCorporate
	PersonalizationFeatureOneXRUIM
	PersonalizationFeatureUnknown
	PersonalizationFeatureGWServiceProviderName
	PersonalizationFeatureGWSPAndEHPLMN
	PersonalizationFeatureGWICCID
	PersonalizationFeatureGWIMPI
	PersonalizationFeatureGWNetworkSubsetServiceProvider
	PersonalizationFeatureGWCarrier
)

type CATConfigMode uint8

const (
	CATConfigDisabled      CATConfigMode = 0x00
	CATConfigGobi          CATConfigMode = 0x01
	CATConfigAndroid       CATConfigMode = 0x02
	CATConfigDecoded       CATConfigMode = 0x03
	CATConfigDecodedPull   CATConfigMode = 0x04
	CATConfigCustomRaw     CATConfigMode = 0x05
	CATConfigCustomDecoded CATConfigMode = 0x06
)

type Session uint8

const (
	SessionPrimaryGWProvisioning Session = 0

	SessionNonProvisioningSlot1 Session = 4
	SessionNonProvisioningSlot2 Session = 5
	SessionCardSlot1            Session = 6
	SessionCardSlot2            Session = 7

	SessionNonProvisioningSlot3 Session = 16
	SessionNonProvisioningSlot4 Session = 17
	SessionNonProvisioningSlot5 Session = 18
	SessionCardSlot3            Session = 19
	SessionCardSlot4            Session = 20
	SessionCardSlot5            Session = 21
)

type FileAttributes struct {
	FileStructure      FileStructure
	FileType           FileType
	FileID             uint16
	RecordSize         uint16
	RecordCount        uint16
	FileSize           uint16
	ReadSecurity       UIMFileSecurity
	WriteSecurity      UIMFileSecurity
	IncreaseSecurity   UIMFileSecurity
	DeactivateSecurity UIMFileSecurity
	ActivateSecurity   UIMFileSecurity
	Raw                []byte
}

type File struct {
	Session Session
	AID     []byte
	Path    []byte
}

type TransparentRead struct {
	File        File
	Offset      uint16
	Length      uint16
	EncryptData bool
}

type TransparentWrite struct {
	File   File
	Offset uint16
	Data   []byte
}

type RecordRead struct {
	File       File
	Record     uint16
	Length     uint16
	LastRecord uint16
}

type RecordWrite struct {
	File   File
	Record uint16
	Data   []byte
}

type AuthContext byte

const (
	AuthContext3G     AuthContext = 3
	AuthContextIMSAKA AuthContext = 11
)

type AuthenticateRequest struct {
	Session Session
	AID     []byte
	Context AuthContext
	Rand    []byte
	AUTN    []byte
}

type EnvelopeResponse struct {
	SW1  byte
	SW2  byte
	Data []byte
}

type PowerOnSIMRequest struct {
	Slot                uint8
	IgnoreHotSwapSwitch bool
}

type ChangeProvisioningSessionRequest struct {
	Session  Session
	Activate bool
	Slot     uint8
	AID      []byte
}

type OpenLogicalChannelRequest struct {
	AID []byte
}

type OpenLogicalChannelResponse struct {
	Channel        uint8
	SelectResponse []byte
}

type CloseLogicalChannelRequest struct {
	Channel uint8
}

type CloseLogicalChannelResponse struct{}

type SendAPDURequest struct {
	Command []byte
}

type SendAPDUResponse struct {
	Response []byte
}

type RefreshStage uint8

const (
	RefreshStageWaitForOK RefreshStage = iota
	RefreshStageStart
	RefreshStageEndWithSuccess
	RefreshStageEndWithFailure
)

type RefreshMode uint8

const (
	RefreshModeReset RefreshMode = iota
	RefreshModeInit
	RefreshModeInitFCN
	RefreshModeFCN
	RefreshModeInitFullFCN
	RefreshModeApplicationReset
	RefreshMode3GReset
)

type RefreshFile struct {
	FileID uint16
	Path   []byte
}

type RefreshEvent struct {
	Stage   RefreshStage
	Mode    RefreshMode
	Session Session
	AID     []byte
	Files   []RefreshFile
}

type RefreshRegisterRequest struct {
	Session     Session
	AID         []byte
	VoteForInit bool
	Files       []RefreshFile
}
