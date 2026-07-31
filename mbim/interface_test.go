package mbim

import (
	"encoding"
	"testing"
)

func TestProtocolTypesImplementStandardInterfaces(t *testing.T) {
	var _ encoding.BinaryMarshaler = (*Request)(nil)
	var _ encoding.BinaryMarshaler = (*Command)(nil)
	var _ encoding.BinaryMarshaler = (*OpenDeviceRequest)(nil)
	var _ encoding.BinaryMarshaler = (*CloseRequest)(nil)
	var _ encoding.BinaryMarshaler = (*HostErrorRequest)(nil)
	var _ encoding.BinaryMarshaler = RouteSelectionDescriptor{}
	var _ encoding.BinaryMarshaler = VisibleProvidersAction(0)
	var _ encoding.BinaryMarshaler = NetworkIdleHint(0)
	var _ encoding.BinaryMarshaler = SARConfigState{}
	var _ encoding.BinaryMarshaler = SARConfig{}
	var _ encoding.BinaryMarshaler = URSPTrafficDescriptor{}
	var _ encoding.BinaryMarshaler = RejectedSNSSAI{}
	var _ encoding.BinaryMarshaler = PreconfiguredDefaultNSSAI{}

	var _ encoding.BinaryUnmarshaler = (*CommandResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*Indication)(nil)
	var _ encoding.BinaryUnmarshaler = (*VersionInfo)(nil)
	var _ encoding.BinaryUnmarshaler = (*ProxyConfigResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*OpenDeviceResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*CloseResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*RouteSelectionDescriptor)(nil)
	var _ encoding.BinaryUnmarshaler = (*FirmwareID)(nil)
	var _ encoding.BinaryUnmarshaler = (*HostShutdownResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*SARConfigState)(nil)
	var _ encoding.BinaryUnmarshaler = (*URSPTrafficDescriptor)(nil)
	var _ encoding.BinaryUnmarshaler = (*RejectedSNSSAI)(nil)
	var _ encoding.BinaryUnmarshaler = (*PreconfiguredDefaultNSSAI)(nil)
	var _ encoding.BinaryUnmarshaler = (*NetworkIdleHint)(nil)
	var _ encoding.BinaryUnmarshaler = (*SARConfigInfo)(nil)
	var _ encoding.BinaryUnmarshaler = (*TransmissionStatusInfo)(nil)
	var _ encoding.BinaryUnmarshaler = (*NITZInfo)(nil)
	var _ encoding.BinaryUnmarshaler = (*DeviceSlotMappingsResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*SubscriberReadyStatusResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*ApplicationListResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*UICCApplication)(nil)
	var _ encoding.BinaryUnmarshaler = (*FileStatusResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*ReadBinaryResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*ReadRecordResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*AuthAKAResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*STKPACInfo)(nil)
	var _ encoding.BinaryUnmarshaler = (*STKPAC)(nil)
	var _ encoding.BinaryUnmarshaler = (*STKTerminalResponseInfo)(nil)
	var _ encoding.BinaryUnmarshaler = (*STKEnvelopeInfo)(nil)
	var _ encoding.BinaryUnmarshaler = (*UiccATRResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*OpenChannelResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*CloseChannelResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*APDUResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*STKEnvelopeResponse)(nil)
}
