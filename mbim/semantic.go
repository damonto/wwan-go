package mbim

const (
	simClassMask = SIMClassLogical | SIMClassRemovable
	smsCapsMask  = SMSCapsPDUReceive | SMSCapsPDUSend | SMSCapsTextReceive | SMSCapsTextSend

	dataClassV1Mask = DataClassGPRS | DataClassEDGE | DataClassUMTS | DataClassHSDPA |
		DataClassHSUPA | DataClassLTE | DataClass1XRTT | DataClass1XEVDO |
		DataClass1XEVDORevA | DataClass1XEVDV | DataClass3XRTT |
		DataClass1XEVDORevB | DataClassUMB | DataClassCustom
	dataClassV2Mask = dataClassV1Mask | DataClass5GNSA | DataClass5GSA
	dataClassV3Mask = dataClassV1Mask | DataClass5G

	dataSubclassMask = DataSubclass5GENDC | DataSubclass5GNR | DataSubclass5GNEDC |
		DataSubclass5GELTE | DataSubclass5GNGENDC
	dataSubclass5GCoreMask = DataSubclass5GNR | DataSubclass5GNEDC |
		DataSubclass5GELTE | DataSubclass5GNGENDC

	controlCapsV1Mask = ControlCapsManualRegistration | ControlCapsHardwareRadioSwitch |
		ControlCapsCDMAMobileIP | ControlCapsCDMASimpleIP | ControlCapsMultiCarrier
	controlCapsV3Mask = controlCapsV1Mask | ControlCapsESIM |
		ControlCapsUEPolicyRouteSelection | ControlCapsSIMHotSwap
	controlCapsV4Mask = controlCapsV1Mask | ControlCapsESIM | ControlCapsSIMHotSwap |
		ControlCapsUseURSPRuleOnEPC
)

func validDataClass(version uint16, class DataClass) bool {
	return class&^dataClassMask(version) == 0
}

func dataClassMask(version uint16) DataClass {
	switch {
	case version >= mbimExVersion30:
		return dataClassV3Mask
	case version >= mbimExVersion20:
		return dataClassV2Mask
	default:
		return dataClassV1Mask
	}
}

func dataClassHas5G(version uint16, class DataClass) bool {
	if version >= mbimExVersion30 {
		return class&DataClass5G != 0
	}
	if version >= mbimExVersion20 {
		return class&(DataClass5GNSA|DataClass5GSA) != 0
	}
	return false
}

func validDataSubclass(subclass DataSubclass) bool {
	return subclass&^dataSubclassMask == 0
}

func dataSubclassUses5GCore(subclass DataSubclass) bool {
	return subclass&dataSubclass5GCoreMask != 0
}

func validControlCaps(version uint16, caps ControlCaps) bool {
	var mask ControlCaps
	switch {
	case version >= mbimExVersion40:
		mask = controlCapsV4Mask
	case version >= mbimExVersion30:
		mask = controlCapsV3Mask
	default:
		mask = controlCapsV1Mask
	}
	return caps&^mask == 0
}

func validCompression(compression Compression) bool {
	return compression <= CompressionEnable
}

func validAuthProtocol(protocol AuthProtocol) bool {
	return protocol <= AuthProtocolMSCHAPV2
}

func validContextIPType(ipType ContextIPType) bool {
	return ipType <= ContextIPTypeIPv4AndIPv6
}

func validContextSource(source ContextSource) bool {
	return source <= ContextSourceDevice
}
