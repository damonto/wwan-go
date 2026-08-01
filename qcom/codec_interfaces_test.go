package qcom

import (
	"bytes"
	"encoding"
	"fmt"
	"testing"
)

var (
	_ encoding.BinaryMarshaler   = qmiLength8Bytes{}
	_ encoding.BinaryUnmarshaler = (*qmiLength8Bytes)(nil)
	_ encoding.BinaryMarshaler   = qmiLength16Bytes{}
	_ encoding.BinaryUnmarshaler = (*qmiLength16Bytes)(nil)
	_ encoding.BinaryMarshaler   = dmsFirmwareImages{}
	_ encoding.BinaryUnmarshaler = (*dmsFirmwareImages)(nil)
	_ encoding.BinaryMarshaler   = dsdSystems{}
	_ encoding.BinaryUnmarshaler = (*dsdSystems)(nil)
	_ encoding.BinaryMarshaler   = nasAcquisitionOrder{}
	_ encoding.BinaryUnmarshaler = (*nasAcquisitionOrder)(nil)
	_ encoding.BinaryMarshaler   = wdsAttachPDNList{}
	_ encoding.BinaryUnmarshaler = (*wdsAttachPDNList)(nil)
	_ encoding.BinaryMarshaler   = wmsRouteList{}
	_ encoding.BinaryUnmarshaler = (*wmsRouteList)(nil)
	_ encoding.BinaryMarshaler   = wmsAckInfo{}
	_ encoding.BinaryUnmarshaler = (*wmsAckInfo)(nil)
	_ encoding.BinaryMarshaler   = DMSPINState{}
	_ encoding.BinaryUnmarshaler = (*DMSPINState)(nil)
	_ encoding.BinaryMarshaler   = DMSSubscriptionVoiceData{}
	_ encoding.BinaryUnmarshaler = (*DMSSubscriptionVoiceData)(nil)
	_ encoding.BinaryMarshaler   = DMSIMSCapability{}
	_ encoding.BinaryUnmarshaler = (*DMSIMSCapability)(nil)
	_ encoding.BinaryMarshaler   = DSDSystem{}
	_ encoding.BinaryUnmarshaler = (*DSDSystem)(nil)
	_ encoding.BinaryMarshaler   = UIMFileSecurity{}
	_ encoding.BinaryUnmarshaler = (*UIMFileSecurity)(nil)
	_ encoding.BinaryMarshaler   = RawFileAttributes{}
	_ encoding.BinaryUnmarshaler = (*RawFileAttributes)(nil)
	_ encoding.BinaryMarshaler   = IMSDCMInstance(0)
	_ encoding.BinaryUnmarshaler = (*IMSDCMInstance)(nil)
	_ encoding.BinaryMarshaler   = IMSDCMConnection{}
	_ encoding.BinaryUnmarshaler = (*IMSDCMConnection)(nil)
	_ encoding.BinaryMarshaler   = LOCVector3{}
	_ encoding.BinaryUnmarshaler = (*LOCVector3)(nil)
	_ encoding.BinaryMarshaler   = NASCommonSignalInfo{}
	_ encoding.BinaryUnmarshaler = (*NASCommonSignalInfo)(nil)
	_ encoding.BinaryMarshaler   = NASRFDedicatedBand{}
	_ encoding.BinaryUnmarshaler = (*NASRFDedicatedBand)(nil)
	_ encoding.BinaryMarshaler   = NASRFBandwidth{}
	_ encoding.BinaryUnmarshaler = (*NASRFBandwidth)(nil)
	_ encoding.BinaryMarshaler   = NASSignalStrength{}
	_ encoding.BinaryUnmarshaler = (*NASSignalStrength)(nil)
	_ encoding.BinaryMarshaler   = OMANetworkInitiatedAlert{}
	_ encoding.BinaryUnmarshaler = (*OMANetworkInitiatedAlert)(nil)
	_ encoding.BinaryMarshaler   = PBMAlphaStringCapability{}
	_ encoding.BinaryUnmarshaler = (*PBMAlphaStringCapability)(nil)
	_ encoding.BinaryMarshaler   = VoiceAlphaIdentifier{}
	_ encoding.BinaryUnmarshaler = (*VoiceAlphaIdentifier)(nil)
	_ encoding.BinaryMarshaler   = WDSOperatorReservedPCO{}
	_ encoding.BinaryUnmarshaler = (*WDSOperatorReservedPCO)(nil)
	_ encoding.BinaryMarshaler   = WDSCurrentBearerTechnology{}
	_ encoding.BinaryUnmarshaler = (*WDSCurrentBearerTechnology)(nil)
	_ encoding.BinaryMarshaler   = WDSBearerTechnology{}
	_ encoding.BinaryUnmarshaler = (*WDSBearerTechnology)(nil)
	_ encoding.BinaryMarshaler   = WDSDataSystems{}
	_ encoding.BinaryUnmarshaler = (*WDSDataSystems)(nil)
	_ encoding.BinaryMarshaler   = WMSGSMCauseInfo{}
	_ encoding.BinaryUnmarshaler = (*WMSGSMCauseInfo)(nil)
	_ encoding.BinaryMarshaler   = WMSCDMAForceOnDC{}
	_ encoding.BinaryUnmarshaler = (*WMSCDMAForceOnDC)(nil)
	_ encoding.BinaryMarshaler   = WMSAck3GPP2Failure{}
	_ encoding.BinaryUnmarshaler = (*WMSAck3GPP2Failure)(nil)
	_ encoding.BinaryMarshaler   = WMSAck3GPPFailure{}
	_ encoding.BinaryUnmarshaler = (*WMSAck3GPPFailure)(nil)
	_ encoding.BinaryMarshaler   = WMSMessageReference{}
	_ encoding.BinaryUnmarshaler = (*WMSMessageReference)(nil)
	_ encoding.BinaryMarshaler   = WMSRejectCause{}
	_ encoding.BinaryUnmarshaler = (*WMSRejectCause)(nil)
	_ encoding.BinaryMarshaler   = WMSSMSCAddress{}
	_ encoding.BinaryUnmarshaler = (*WMSSMSCAddress)(nil)
	_ encoding.BinaryMarshaler   = WMSRoute{}
	_ encoding.BinaryUnmarshaler = (*WMSRoute)(nil)
	_ encoding.BinaryMarshaler   = WMS3GPPBroadcastChannel{}
	_ encoding.BinaryUnmarshaler = (*WMS3GPPBroadcastChannel)(nil)
	_ encoding.BinaryMarshaler   = WMS3GPP2BroadcastChannel{}
	_ encoding.BinaryUnmarshaler = (*WMS3GPP2BroadcastChannel)(nil)

	_ encoding.BinaryUnmarshaler = (*NASGERANCellLocation)(nil)
	_ encoding.BinaryUnmarshaler = (*NASUMTSCellLocation)(nil)
	_ encoding.BinaryUnmarshaler = (*NASCDMACellLocation)(nil)
	_ encoding.BinaryUnmarshaler = (*NASLTEIntraFrequency)(nil)
	_ encoding.BinaryUnmarshaler = (*NASLTEInterFrequency)(nil)
	_ encoding.BinaryUnmarshaler = (*NASLTEGERANNeighbors)(nil)
	_ encoding.BinaryUnmarshaler = (*NASLTEWCDMANeighbors)(nil)
	_ encoding.BinaryUnmarshaler = (*NASUMTSLTENeighbors)(nil)
	_ encoding.BinaryUnmarshaler = (*NASNR5GCellLocation)(nil)

	_ encoding.TextMarshaler     = NASPLMN{}
	_ encoding.TextUnmarshaler   = (*NASPLMN)(nil)
	_ fmt.Stringer               = NASPLMN{}
	_ encoding.BinaryMarshaler   = pdcUTF16("")
	_ encoding.BinaryUnmarshaler = (*pdcUTF16)(nil)
	_ encoding.TextMarshaler     = pdcUTF16("")
	_ encoding.TextUnmarshaler   = (*pdcUTF16)(nil)
	_ fmt.Stringer               = pdcUTF16("")
)

type binaryCodec interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

func TestBinaryCodecRoundTrip(t *testing.T) {
	instance := IMSDCMInstance1
	firmwareImages := dmsFirmwareImages{{
		Type: DMSFirmwareImageModem, UniqueID: [dmsFirmwareUniqueIDLength]byte{0xAA}, BuildID: "x",
	}}
	tests := []struct {
		name    string
		value   binaryCodec
		decoded func() binaryCodec
		want    []byte
	}{
		{name: "QMI length-8 bytes", value: &qmiLength8Bytes{'a', 'b'}, decoded: func() binaryCodec { return new(qmiLength8Bytes) }, want: []byte{2, 'a', 'b'}},
		{name: "QMI length-16 bytes", value: &qmiLength16Bytes{'a', 'b'}, decoded: func() binaryCodec { return new(qmiLength16Bytes) }, want: []byte{2, 0, 'a', 'b'}},
		{
			name:    "DMS firmware image list",
			value:   &firmwareImages,
			decoded: func() binaryCodec { return new(dmsFirmwareImages) },
			want:    []byte{1, 0, 0xAA, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 'x'},
		},
		{
			name:    "DSD system list",
			value:   &dsdSystems{{Network: 1, RAT: 2, ServiceOptions: 3}},
			decoded: func() binaryCodec { return new(dsdSystems) },
			want:    []byte{1, 1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0},
		},
		{name: "NAS acquisition order", value: &nasAcquisitionOrder{1, 8}, decoded: func() binaryCodec { return new(nasAcquisitionOrder) }, want: []byte{2, 1, 8}},
		{name: "WDS attach PDN list", value: &wdsAttachPDNList{0x1234, 0x5678}, decoded: func() binaryCodec { return new(wdsAttachPDNList) }, want: []byte{2, 0x34, 0x12, 0x78, 0x56}},
		{name: "WMS route list", value: &wmsRouteList{{MessageType: 0, MessageClass: 2, Storage: 1, Action: 3}}, decoded: func() binaryCodec { return new(wmsRouteList) }, want: []byte{1, 0, 0, 2, 1, 3}},
		{name: "WMS ACK info", value: &wmsAckInfo{TransactionID: 0x01020304, Protocol: WMSMessageProtocolWCDMA, Success: true}, decoded: func() binaryCodec { return new(wmsAckInfo) }, want: []byte{4, 3, 2, 1, 1, 1}},
		{name: "DMS PIN state", value: &DMSPINState{Status: 2, VerifyRetries: 3, UnblockRetries: 4}, decoded: func() binaryCodec { return new(DMSPINState) }, want: []byte{2, 3, 4}},
		{name: "DMS subscription voice/data", value: &DMSSubscriptionVoiceData{Capability: 0x01020304, Concurrent: true}, decoded: func() binaryCodec { return new(DMSSubscriptionVoiceData) }, want: []byte{4, 3, 2, 1, 1}},
		{name: "DMS IMS capability", value: &DMSIMSCapability{Subscription: DMSSubscriptionSecondary, Enabled: true}, decoded: func() binaryCodec { return new(DMSIMSCapability) }, want: []byte{2, 0, 0, 0, 1}},
		{name: "DSD system", value: &DSDSystem{Network: 1, RAT: 2, ServiceOptions: 3}, decoded: func() binaryCodec { return new(DSDSystem) }, want: []byte{1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0}},
		{name: "UIM file security", value: &UIMFileSecurity{Logic: 1, Attributes: 0x0203}, decoded: func() binaryCodec { return new(UIMFileSecurity) }, want: []byte{1, 3, 2}},
		{
			name: "raw UIM file attributes",
			value: &RawFileAttributes{
				FileSize: 0x1234, FileID: 0x5678, FileType: 2, RecordSize: 0x009A, RecordCount: 3,
				Raw: []byte{0xAA, 0xBB},
			},
			decoded: func() binaryCodec { return new(RawFileAttributes) },
			want: []byte{
				0x34, 0x12, 0x78, 0x56, 2, 0x9A, 0, 3, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				2, 0, 0xAA, 0xBB,
			},
		},
		{name: "IMS DCM instance", value: &instance, decoded: func() binaryCodec { return new(IMSDCMInstance) }, want: []byte{1, 0, 0, 0}},
		{
			name:    "IMS DCM connection",
			value:   &IMSDCMConnection{APN: "ims", APNType: IMSDCMAPNIMS, RAT: IMSDCMRATLTE, IPFamily: IMSDCMIPv4, WDSProfileNum: 5},
			decoded: func() binaryCodec { return new(IMSDCMConnection) },
			want:    []byte{3, 'i', 'm', 's', 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0},
		},
		{
			name:    "LOC vector",
			value:   &LOCVector3{East: 1.5, North: -2.25, Up: 0.75},
			decoded: func() binaryCodec { return new(LOCVector3) },
			want:    []byte{0, 0, 0xC0, 0x3F, 0, 0, 0x10, 0xC0, 0, 0, 0x40, 0x3F},
		},
		{name: "NAS common signal", value: &NASCommonSignalInfo{RSSI: -42, ECIO: -12}, decoded: func() binaryCodec { return new(NASCommonSignalInfo) }, want: []byte{0xD6, 0xF4, 0xFF}},
		{name: "NAS dedicated RF band", value: &NASRFDedicatedBand{RadioInterface: 8, Band: 0x1234}, decoded: func() binaryCodec { return new(NASRFDedicatedBand) }, want: []byte{8, 0x34, 0x12}},
		{name: "NAS RF bandwidth", value: &NASRFBandwidth{RadioInterface: 8, Bandwidth: 0x01020304}, decoded: func() binaryCodec { return new(NASRFBandwidth) }, want: []byte{8, 4, 3, 2, 1}},
		{name: "NAS signal strength", value: &NASSignalStrength{Strength: -70, RadioInterface: 8}, decoded: func() binaryCodec { return new(NASSignalStrength) }, want: []byte{0xBA, 8}},
		{name: "OMA alert", value: &OMANetworkInitiatedAlert{SessionType: 2, SessionID: 0x1234}, decoded: func() binaryCodec { return new(OMANetworkInitiatedAlert) }, want: []byte{2, 0x34, 0x12}},
		{name: "PBM alpha capability", value: &PBMAlphaStringCapability{MaximumRecords: 10, UsedRecords: 4, MaximumStringLength: 20}, decoded: func() binaryCodec { return new(PBMAlphaStringCapability) }, want: []byte{10, 4, 20}},
		{name: "Voice alpha identifier", value: &VoiceAlphaIdentifier{Encoding: VoiceAlphaEncodingGSM, Data: []byte("hi")}, decoded: func() binaryCodec { return new(VoiceAlphaIdentifier) }, want: []byte{1, 2, 'h', 'i'}},
		{name: "WDS operator PCO", value: &WDSOperatorReservedPCO{MCC: 460, MNC: 1, MNCIncludesPCSDigit: true, AppSpecificInfo: []byte{0xAA, 0xBB}, ContainerID: 0x1234}, decoded: func() binaryCodec { return new(WDSOperatorReservedPCO) }, want: []byte{0xCC, 0x01, 1, 0, 1, 2, 0xAA, 0xBB, 0x34, 0x12}},
		{name: "WDS current bearer", value: &WDSCurrentBearerTechnology{Network: 1, RATMask: 0x02030405, ServiceOptionMask: 0x06070809}, decoded: func() binaryCodec { return new(WDSCurrentBearerTechnology) }, want: []byte{1, 5, 4, 3, 2, 9, 8, 7, 6}},
		{name: "WDS extended bearer", value: &WDSBearerTechnology{Network: 1, RAT: 2, ServiceOptions: 3}, decoded: func() binaryCodec { return new(WDSBearerTechnology) }, want: []byte{1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0}},
		{name: "WDS data systems", value: &WDSDataSystems{Preferred: 1, Networks: []WDSDataSystemNetwork{{Type: 2, RATMask: 0x03040506, ServiceOptionMask: 0x0708090A}}}, decoded: func() binaryCodec { return new(WDSDataSystems) }, want: []byte{1, 1, 2, 6, 5, 4, 3, 0x0A, 9, 8, 7}},
		{name: "WMS CDMA force-on-DC", value: &WMSCDMAForceOnDC{Force: true, ServiceOption: WMSCDMAServiceOption14}, decoded: func() binaryCodec { return new(WMSCDMAForceOnDC) }, want: []byte{1, 0x0E}},
		{name: "WMS 3GPP2 ACK failure", value: &WMSAck3GPP2Failure{ErrorClass: 2, CauseCode: 0x60}, decoded: func() binaryCodec { return new(WMSAck3GPP2Failure) }, want: []byte{2, 0x60}},
		{name: "WMS 3GPP ACK failure", value: &WMSAck3GPPFailure{RPCause: 0x21, TPCause: 0xD3}, decoded: func() binaryCodec { return new(WMSAck3GPPFailure) }, want: []byte{0x21, 0xD3}},
		{name: "WMS message reference", value: &WMSMessageReference{Storage: WMSStorageNV, Index: 0x01020304}, decoded: func() binaryCodec { return new(WMSMessageReference) }, want: []byte{1, 4, 3, 2, 1}},
		{name: "WMS GSM cause", value: &WMSGSMCauseInfo{RPCause: 0x1234, TPCause: 0x56}, decoded: func() binaryCodec { return new(WMSGSMCauseInfo) }, want: []byte{0x34, 0x12, 0x56}},
		{name: "WMS reject cause", value: &WMSRejectCause{Type: 0x01020304, Value: 5}, decoded: func() binaryCodec { return new(WMSRejectCause) }, want: []byte{4, 3, 2, 1, 5}},
		{name: "WMS SMSC address", value: &WMSSMSCAddress{Type: "145", Digits: "+86"}, decoded: func() binaryCodec { return new(WMSSMSCAddress) }, want: []byte{'1', '4', '5', 3, '+', '8', '6'}},
		{name: "WMS route", value: &WMSRoute{MessageType: 0, MessageClass: 2, Storage: 1, Action: 3}, decoded: func() binaryCodec { return new(WMSRoute) }, want: []byte{0, 2, 1, 3}},
		{name: "WMS 3GPP broadcast channel", value: &WMS3GPPBroadcastChannel{Start: 0x1234, End: 0x5678, Selected: true}, decoded: func() binaryCodec { return new(WMS3GPPBroadcastChannel) }, want: []byte{0x34, 0x12, 0x78, 0x56, 1}},
		{name: "WMS 3GPP2 broadcast channel", value: &WMS3GPP2BroadcastChannel{ServiceCategory: 0x1234, Language: WMSLanguageEnglish}, decoded: func() binaryCodec { return new(WMS3GPP2BroadcastChannel) }, want: []byte{0x34, 0x12, 1, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := tt.value.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(encoded, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", encoded, tt.want)
			}
			original := bytes.Clone(encoded)
			decoded := tt.decoded()
			if err := decoded.UnmarshalBinary(encoded); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			encoded[len(encoded)-1] ^= 0xFF
			got, err := decoded.MarshalBinary()
			if err != nil {
				t.Fatalf("round-trip MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("round-trip MarshalBinary() = % X, want % X", got, original)
			}
		})
	}
}

func TestQMILengthCodecErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "length-8 marshal overflow", run: func() error {
			_, err := qmiLength8Bytes(make([]byte, 256)).MarshalBinary()
			return err
		}},
		{name: "length-8 missing prefix", run: func() error {
			return new(qmiLength8Bytes).UnmarshalBinary(nil)
		}},
		{name: "length-8 truncated value", run: func() error {
			return new(qmiLength8Bytes).UnmarshalBinary([]byte{2, 1})
		}},
		{name: "length-8 trailing value", run: func() error {
			return new(qmiLength8Bytes).UnmarshalBinary([]byte{1, 1, 2})
		}},
		{name: "length-16 marshal overflow", run: func() error {
			_, err := qmiLength16Bytes(make([]byte, 65536)).MarshalBinary()
			return err
		}},
		{name: "length-16 missing prefix", run: func() error {
			return new(qmiLength16Bytes).UnmarshalBinary([]byte{1})
		}},
		{name: "length-16 truncated value", run: func() error {
			return new(qmiLength16Bytes).UnmarshalBinary([]byte{2, 0, 1})
		}},
		{name: "length-16 trailing value", run: func() error {
			return new(qmiLength16Bytes).UnmarshalBinary([]byte{1, 0, 1, 2})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("codec error = nil, want non-nil")
			}
		})
	}
}

func TestPDCUTF16BinaryAndTextCodec(t *testing.T) {
	tests := []struct {
		name      string
		wire      []byte
		want      string
		canonical []byte
	}{
		{
			name:      "trailing terminator",
			wire:      []byte{'/', 0, 'm', 0, 0, 0},
			want:      "/m",
			canonical: []byte{'/', 0, 'm', 0},
		},
		{
			name:      "surrogate pair",
			wire:      []byte{0x3D, 0xD8, 0x00, 0xDE},
			want:      "😀",
			canonical: []byte{0x3D, 0xD8, 0x00, 0xDE},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := bytes.Clone(tt.wire)
			var got pdcUTF16
			if err := got.UnmarshalBinary(input); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			input[0] ^= 0xff
			if got.String() != tt.want {
				t.Fatalf("String() = %q, want %q", got.String(), tt.want)
			}
			encoded, err := got.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(encoded, tt.canonical) {
				t.Fatalf("MarshalBinary() = % X, want % X", encoded, tt.canonical)
			}

			text, err := got.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			if string(text) != tt.want {
				t.Fatalf("MarshalText() = %q, want %q", text, tt.want)
			}
			if err := got.UnmarshalText(text); err != nil {
				t.Fatalf("UnmarshalText() error = %v", err)
			}
		})
	}
}

func TestPDCUTF16TextErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "odd byte length", run: func() error {
			return new(pdcUTF16).UnmarshalBinary([]byte{1})
		}},
		{name: "unmarshal too long", run: func() error {
			return new(pdcUTF16).UnmarshalBinary(make([]byte, (pdcConfigPathMax+1)*2))
		}},
		{name: "unmarshal embedded NUL", run: func() error {
			return new(pdcUTF16).UnmarshalBinary([]byte{'a', 0, 0, 0, 'b', 0})
		}},
		{name: "unpaired high surrogate", run: func() error {
			return new(pdcUTF16).UnmarshalBinary([]byte{0x00, 0xd8})
		}},
		{name: "unpaired low surrogate", run: func() error {
			return new(pdcUTF16).UnmarshalBinary([]byte{0x00, 0xdc})
		}},
		{name: "invalid UTF-8 text", run: func() error {
			return new(pdcUTF16).UnmarshalText([]byte{0xff})
		}},
		{name: "marshal too long", run: func() error {
			_, err := pdcUTF16(string(bytes.Repeat([]byte{'x'}, pdcConfigPathMax+1))).MarshalText()
			return err
		}},
		{name: "marshal NUL", run: func() error {
			_, err := pdcUTF16("a\x00b").MarshalText()
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("codec error = nil, want non-nil")
			}
		})
	}
}

func TestNASPLMNTextRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		text string
		want NASPLMN
	}{
		{name: "two-digit MNC", text: "46001", want: NASPLMN{MCC: 460, MNC: 1, MNCThreeDigitsKnown: true}},
		{name: "three-digit MNC", text: "310260", want: NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true, MNCThreeDigitsKnown: true}},
		{name: "leading zeros", text: "00101", want: NASPLMN{MCC: 1, MNC: 1, MNCThreeDigitsKnown: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASPLMN
			if err := got.UnmarshalText([]byte(tt.text)); err != nil {
				t.Fatalf("UnmarshalText() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalText() = %+v, want %+v", got, tt.want)
			}
			encoded, err := got.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			if string(encoded) != tt.text || got.String() != tt.text {
				t.Fatalf("text = %q, String() = %q, want %q", encoded, got.String(), tt.text)
			}
		})
	}
}

func TestNASPLMNTextErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "short", run: func() error { return new(NASPLMN).UnmarshalText([]byte("4601")) }},
		{name: "non-decimal", run: func() error { return new(NASPLMN).UnmarshalText([]byte("460A1")) }},
		{name: "MCC out of range", run: func() error { _, err := (NASPLMN{MCC: 1000, MNC: 1}).MarshalText(); return err }},
		{name: "two-digit MNC out of range", run: func() error {
			_, err := (NASPLMN{MCC: 460, MNC: 260, MNCThreeDigitsKnown: true}).MarshalText()
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("codec error = nil, want non-nil")
			}
		})
	}
}
