package mbim

const (
	CIDDeviceCaps                 = 0x00000001
	CIDRadioState                 = 0x00000003
	CIDSubscriberReadyStatus      = 0x00000002
	CIDPin                        = 0x00000004
	CIDPinList                    = 0x00000005
	CIDHomeProvider               = 0x00000006
	CIDPreferredProviders         = 0x00000007
	CIDVisibleProviders           = 0x00000008
	CIDRegisterState              = 0x00000009
	CIDPacketService              = 0x0000000A
	CIDSignalState                = 0x0000000B
	CIDConnect                    = 0x0000000C
	CIDProvisionedContexts        = 0x0000000D
	CIDServiceActivation          = 0x0000000E
	CIDIPConfiguration            = 0x0000000F
	CIDDeviceServices             = 0x00000010
	CIDDeviceServiceSubscribeList = 0x00000013
	CIDPacketStatistics           = 0x00000014
	CIDNetworkIdleHint            = 0x00000015
	CIDEmergencyMode              = 0x00000016
	CIDIPPacketFilters            = 0x00000017
	CIDMulticarrierProviders      = 0x00000018

	CIDSMSConfiguration      = 0x00000001
	CIDSMSRead               = 0x00000002
	CIDSMSSend               = 0x00000003
	CIDSMSDelete             = 0x00000004
	CIDSMSMessageStoreStatus = 0x00000005

	CIDUSSD = 0x00000001

	CIDPhonebookConfiguration = 0x00000001
	CIDPhonebookRead          = 0x00000002
	CIDPhonebookDelete        = 0x00000003
	CIDPhonebookWrite         = 0x00000004

	CIDAuthAKA  = 0x00000001
	CIDAuthAKAP = 0x00000002
	CIDAuthSIM  = 0x00000003

	CIDDSSConnect              = 0x00000001
	CIDMsFirmwareIDGet         = 0x00000001
	CIDMsHostShutdownNotify    = 0x00000001
	CIDMsSARConfig             = 0x00000001
	CIDMsSARTransmissionStatus = 0x00000002
	CIDMsVoiceExtensionsNITZ   = 0x0000000A

	CIDSTKPAC              = 0x00000001
	CIDSTKTerminalResponse = 0x00000002
	CIDSTKEnvelope         = 0x00000003

	CIDUiccATR                = 0x00000001
	CIDUiccOpenChannel        = 0x00000002
	CIDUiccCloseChannel       = 0x00000003
	CIDUiccAPDU               = 0x00000004
	CIDUiccTerminalCapability = 0x00000005
	CIDUiccReset              = 0x00000006
	CIDUiccApplicationList    = 0x00000007
	CIDUiccFileStatus         = 0x00000008
	CIDUiccReadBinary         = 0x00000009
	CIDUiccReadRecord         = 0x0000000A

	CIDProxyControlConfiguration = 0x00000001
	CIDProxyControlVersion       = 0x00000002
	CIDDeviceSlotMappings        = 0x00000007
	CIDVersion                   = 0x0000000F
	CIDMsProvisionedContexts     = 0x00000001
	CIDMsLteAttachConfiguration  = 0x00000003
	CIDMsLteAttachInfo           = 0x00000004
	CIDMsSystemCapabilities      = 0x00000005
	CIDMsDeviceCapsV2            = 0x00000006
	CIDMsSlotInfoStatus          = 0x00000008
	CIDMsPCO                     = 0x00000009
	CIDMsDeviceReset             = 0x0000000A
	CIDMsBaseStationsInfo        = 0x0000000B
	CIDMsLocationInfoStatus      = 0x0000000C
	CIDMsModemConfiguration      = 0x00000010
	CIDMsRegistrationParameters  = 0x00000011
	CIDMsNetworkParameters       = 0x00000012
	CIDMsWakeReason              = 0x00000013
	CIDMsUEPolicy                = 0x00000014
)
