package mbim

import (
	"encoding/binary"
	"testing"
)

func TestPacketServiceRequestCarriesVersion(t *testing.T) {
	tests := []struct {
		name string
		set  bool
	}{
		{name: "query"},
		{name: "set", set: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request *Request
			if tt.set {
				request = (&PacketServiceSetRequest{
					TransactionID: 1,
					MBIMExVersion: mbimExVersion40,
					Action:        PacketServiceActionAttach,
				}).Request()
			} else {
				request = (&PacketServiceRequest{
					TransactionID: 1,
					MBIMExVersion: mbimExVersion40,
				}).Request()
			}
			response := request.Response.(*PacketServiceInfo)
			if response.MBIMExVersion != mbimExVersion40 {
				t.Fatalf("response version = %#x, want %#x", response.MBIMExVersion, mbimExVersion40)
			}
		})
	}
}

func TestPacketServiceSetResponseState(t *testing.T) {
	tests := []struct {
		name    string
		set     bool
		action  PacketServiceAction
		state   PacketServiceState
		wantErr bool
	}{
		{name: "attach completes attached", set: true, action: PacketServiceActionAttach, state: PacketServiceStateAttached},
		{name: "attach cannot complete detached", set: true, action: PacketServiceActionAttach, state: PacketServiceStateDetached, wantErr: true},
		{name: "attach cannot complete attaching", set: true, action: PacketServiceActionAttach, state: PacketServiceStateAttaching, wantErr: true},
		{name: "detach completes detached", set: true, action: PacketServiceActionDetach, state: PacketServiceStateDetached},
		{name: "detach cannot complete attached", set: true, action: PacketServiceActionDetach, state: PacketServiceStateAttached, wantErr: true},
		{name: "detach cannot complete detaching", set: true, action: PacketServiceActionDetach, state: PacketServiceStateDetaching, wantErr: true},
		{name: "query may report attaching", state: PacketServiceStateAttaching},
		{name: "query may report detaching", state: PacketServiceStateDetaching},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response *PacketServiceInfo
			if tt.set {
				request := (&PacketServiceSetRequest{Action: tt.action}).Request()
				response = request.Response.(*PacketServiceInfo)
			} else {
				request := (&PacketServiceRequest{}).Request()
				response = request.Response.(*PacketServiceInfo)
			}

			err := response.UnmarshalBinary(packetServicePayloadForTest(tt.state))
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPacketServiceInfoUnmarshalBinaryVersions(t *testing.T) {
	v3 := packetServicePayloadV3ForTest()
	v4 := append(
		packetServicePayloadV3ForTest(),
		mbimTLV(TLVType3GPPReleaseVersion, binary.LittleEndian.AppendUint32(nil, 16))...,
	)

	tests := []struct {
		name    string
		version uint16
		data    []byte
		want    PacketServiceInfo
		wantErr bool
	}{
		{
			name:    "MBIM 1",
			version: mbimExVersion10,
			data:    packetServicePayloadForVersionForTest(mbimExVersion10, PacketServiceStateAttached),
		},
		{
			name:    "MBIMEx 2",
			version: mbimExVersion20,
			data: func() []byte {
				data := packetServicePayloadForVersionForTest(mbimExVersion20, PacketServiceStateAttached)
				binary.LittleEndian.PutUint32(data[8:12], uint32(DataClass5GSA))
				binary.LittleEndian.PutUint32(data[28:32], uint32(FrequencyRange1|FrequencyRange2))
				return data
			}(),
			want: PacketServiceInfo{FrequencyRange: FrequencyRange1 | FrequencyRange2},
		},
		{
			name:    "MBIMEx 3",
			version: mbimExVersion30,
			data:    v3,
			want: PacketServiceInfo{
				FrequencyRange:       FrequencyRange1,
				CurrentDataSubclass:  DataSubclass5GNR,
				TrackingAreaIdentity: TrackingAreaIdentity{PLMN: PLMN{MCC: 0x0213, MNC: 0x8001}, TAC: 0x010203},
			},
		},
		{
			name:    "MBIMEx 4 release",
			version: mbimExVersion40,
			data:    v4,
			want: PacketServiceInfo{
				FrequencyRange:         FrequencyRange1,
				CurrentDataSubclass:    DataSubclass5GNR,
				TrackingAreaIdentity:   TrackingAreaIdentity{PLMN: PLMN{MCC: 0x0213, MNC: 0x8001}, TAC: 0x010203},
				ThreeGPPReleaseVersion: ThreeGPPReleaseVersion16,
				Has3GPPReleaseVersion:  true,
			},
		},
		{name: "MBIM 1 trailing data", version: mbimExVersion10, data: append(packetServicePayloadForVersionForTest(mbimExVersion10, PacketServiceStateAttached), 0), wantErr: true},
		{name: "MBIMEx 2 truncated", version: mbimExVersion20, data: make([]byte, 31), wantErr: true},
		{name: "MBIMEx 2 trailing data", version: mbimExVersion20, data: append(packetServicePayloadForVersionForTest(mbimExVersion20, PacketServiceStateAttached), 0), wantErr: true},
		{name: "MBIMEx 3 truncated", version: mbimExVersion30, data: make([]byte, 43), wantErr: true},
		{name: "MBIMEx 3 trailing data", version: mbimExVersion30, data: append(packetServicePayloadForVersionForTest(mbimExVersion30, PacketServiceStateAttached), 0), wantErr: true},
		{
			name:    "MBIMEx 4 malformed TLV",
			version: mbimExVersion40,
			data:    append(packetServicePayloadV3ForTest(), []byte{1}...),
			wantErr: true,
		},
		{
			name:    "MBIMEx 4 invalid release length",
			version: mbimExVersion40,
			data: append(
				packetServicePayloadV3ForTest(),
				mbimTLV(TLVType3GPPReleaseVersion, []byte{16})...,
			),
			wantErr: true,
		},
		{
			name:    "MBIMEx 4 duplicate release",
			version: mbimExVersion40,
			data: append(
				append(
					packetServicePayloadV3ForTest(),
					mbimTLV(TLVType3GPPReleaseVersion, binary.LittleEndian.AppendUint32(nil, 15))...,
				),
				mbimTLV(TLVType3GPPReleaseVersion, binary.LittleEndian.AppendUint32(nil, 16))...,
			),
			wantErr: true,
		},
		{
			name:    "MBIMEx 4 reserved release",
			version: mbimExVersion40,
			data: append(
				packetServicePayloadV3ForTest(),
				mbimTLV(TLVType3GPPReleaseVersion, binary.LittleEndian.AppendUint32(nil, 17))...,
			),
			wantErr: true,
		},
		{
			name:    "MBIMEx 4 unknown TLV",
			version: mbimExVersion40,
			data: append(
				packetServicePayloadV3ForTest(),
				mbimTLV(TLVTypePCO, nil)...,
			),
			want: PacketServiceInfo{
				FrequencyRange:       FrequencyRange1,
				CurrentDataSubclass:  DataSubclass5GNR,
				TrackingAreaIdentity: TrackingAreaIdentity{PLMN: PLMN{MCC: 0x0213, MNC: 0x8001}, TAC: 0x010203},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PacketServiceInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.MBIMExVersion != tt.version ||
				got.FrequencyRange != tt.want.FrequencyRange ||
				got.CurrentDataSubclass != tt.want.CurrentDataSubclass ||
				got.TrackingAreaIdentity != tt.want.TrackingAreaIdentity ||
				got.ThreeGPPReleaseVersion != tt.want.ThreeGPPReleaseVersion ||
				got.Has3GPPReleaseVersion != tt.want.Has3GPPReleaseVersion {
				t.Fatalf("UnmarshalBinary() = %+v, want selected fields %+v", got, tt.want)
			}
		})
	}
}

func TestPacketServiceTrackingAreaIdentityValidation(t *testing.T) {
	tests := []struct {
		name     string
		subclass DataSubclass
		mcc      uint16
		mnc      uint16
		tac      uint32
		wantErr  bool
	}{
		{name: "valid 5GC TAI", subclass: DataSubclass5GNR, mcc: 0x0213, mnc: 0x8001, tac: 0x010203},
		{name: "unknown 5GC TAI", subclass: DataSubclass5GNR, mnc: 0xffff, tac: 0xffffffff},
		{name: "non-decimal MCC", subclass: DataSubclass5GNR, mcc: 0x0a13, mnc: 0x8001, tac: 1, wantErr: true},
		{name: "non-decimal MNC", subclass: DataSubclass5GNR, mcc: 0x0213, mnc: 0x800a, tac: 1, wantErr: true},
		{name: "MNC unused bits", subclass: DataSubclass5GNR, mcc: 0x0213, mnc: 0x8101, tac: 1, wantErr: true},
		{name: "TAC exceeds 24 bits", subclass: DataSubclass5GNR, mcc: 0x0213, mnc: 0x8001, tac: 1 << 24, wantErr: true},
		{name: "EPC TAI is ignored", subclass: DataSubclass5GENDC, mcc: 0x0a13, mnc: 0xffff, tac: 0xffffffff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := packetServicePayloadV3ForTest()
			binary.LittleEndian.PutUint32(data[32:36], uint32(tt.subclass))
			binary.LittleEndian.PutUint16(data[36:38], tt.mcc)
			binary.LittleEndian.PutUint16(data[38:40], tt.mnc)
			binary.LittleEndian.PutUint32(data[40:44], tt.tac)

			got := PacketServiceInfo{MBIMExVersion: mbimExVersion30}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPacketServiceReleaseTLVApplicability(t *testing.T) {
	releaseTLV := mbimTLV(TLVType3GPPReleaseVersion, binary.LittleEndian.AppendUint32(nil, 16))
	tests := []struct {
		name      string
		state     PacketServiceState
		dataClass DataClass
		wantErr   bool
	}{
		{name: "attached 5G", state: PacketServiceStateAttached, dataClass: DataClass5G},
		{name: "detached", state: PacketServiceStateDetached, wantErr: true},
		{name: "attached LTE", state: PacketServiceStateAttached, dataClass: DataClassLTE, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := packetServicePayloadForVersionForTest(mbimExVersion40, tt.state)
			binary.LittleEndian.PutUint32(data[8:12], uint32(tt.dataClass))
			if tt.dataClass == DataClass5G {
				binary.LittleEndian.PutUint32(data[32:36], uint32(DataSubclass5GNR))
			}
			data = append(data, releaseTLV...)

			got := PacketServiceInfo{MBIMExVersion: mbimExVersion40}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func packetServicePayloadV3ForTest() []byte {
	data := packetServicePayloadForVersionForTest(mbimExVersion30, PacketServiceStateAttached)
	binary.LittleEndian.PutUint32(data[8:12], uint32(DataClass5G))
	binary.LittleEndian.PutUint32(data[28:32], uint32(FrequencyRange1))
	binary.LittleEndian.PutUint32(data[32:36], uint32(DataSubclass5GNR))
	binary.LittleEndian.PutUint16(data[36:38], 0x0213)
	binary.LittleEndian.PutUint16(data[38:40], 0x8001)
	binary.LittleEndian.PutUint32(data[40:44], 0x010203)
	return data
}
