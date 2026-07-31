package qcom

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASGetSignalStrengthRequest(t *testing.T) {
	tests := []struct {
		name string
		mask NASSignalStrengthRequestMask
		want []byte
	}{
		{name: "default mask"},
		{
			name: "selected measurements",
			mask: NASSignalStrengthRequestRSSI | NASSignalStrengthRequestLTERSRP,
			want: []byte{0x81, 0x00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (NASGetSignalStrengthRequest{
				ClientID: 7, TransactionID: 9, Timeout: 2 * time.Second, Mask: tt.mask,
			}).Request()
			if got.Service != ServiceNAS || got.ClientID != 7 || got.TransactionID != 9 ||
				got.MessageID != MessageNASGetSignalStrength || got.Timeout != 2*time.Second {
				t.Fatalf("Request() = %+v", got)
			}
			if len(tt.want) == 0 {
				if len(got.TLVs) != 0 {
					t.Fatalf("TLVs = %+v, want empty", got.TLVs)
				}
				return
			}
			assertTLV(t, got.TLVs, 0x10, tt.want)
		})
	}
}

func TestNASSignalStrengthResultUnmarshalTLVs(t *testing.T) {
	current := []byte{nasLegacySignedByte(-80), byte(NASRadioInterfaceLTE)}
	strengths := []byte{
		2, 0,
		nasLegacySignedByte(-90), byte(NASRadioInterfaceGSM),
		nasLegacySignedByte(-95), byte(NASRadioInterfaceUMTS),
	}
	rssi := []byte{2, 0, 70, byte(NASRadioInterfaceLTE), 80, byte(NASRadioInterfaceGSM)}
	ecio := []byte{1, 0, 14, byte(NASRadioInterfaceUMTS)}
	errorRates := []byte{2, 0, 0x2C, 0x01, byte(NASRadioInterfaceGSM), 0xFF, 0xFF, byte(NASRadioInterfaceLTE)}
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, current),
		tlv.Bytes(0x10, strengths),
		tlv.Bytes(0x11, rssi),
		tlv.Bytes(0x12, ecio),
		tlv.Bytes(0x13, nasLegacyInt32(-9000)),
		tlv.Bytes(0x14, []byte{5}),
		tlv.Bytes(0x15, errorRates),
		tlv.Bytes(0x16, []byte{nasLegacySignedByte(-12), byte(NASRadioInterfaceLTE)}),
		tlv.Bytes(0x17, nasTestInt16s(-160)),
		tlv.Bytes(0x18, nasTestInt16s(-100)),
	}

	var got NASSignalStrengthResult
	if err := got.UnmarshalTLVs(tlvs); err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	if got.Current != (NASSignalStrength{Strength: -80, RadioInterface: NASRadioInterfaceLTE}) {
		t.Fatalf("Current = %+v", got.Current)
	}
	if len(got.Strengths) != 2 || got.Strengths[0].Strength != -90 || got.Strengths[1].RadioInterface != NASRadioInterfaceUMTS {
		t.Fatalf("Strengths = %+v", got.Strengths)
	}
	if len(got.RSSI) != 2 || got.RSSI[0].RSSI != 70 || !got.RSSIKnown {
		t.Fatalf("RSSI = %+v, known=%v", got.RSSI, got.RSSIKnown)
	}
	if len(got.ECIO) != 1 || got.ECIO[0].ECIO != 14 || !got.ECIOKnown {
		t.Fatalf("ECIO = %+v, known=%v", got.ECIO, got.ECIOKnown)
	}
	if got.IO != -9000 || !got.IOKnown || got.SINRLevel != 5 || !got.SINRKnown {
		t.Fatalf("scalar measurements = %+v", got)
	}
	if len(got.ErrorRates) != 2 || got.ErrorRates[0].Rate != 300 || got.ErrorRates[1].Rate != 0xFFFF {
		t.Fatalf("ErrorRates = %+v", got.ErrorRates)
	}
	if got.RSRQ.RSRQ != -12 || !got.RSRQKnown || got.LTESNR != -160 || !got.LTESNRKnown || got.LTERSRP != -100 || !got.LTERSRPKnown {
		t.Fatalf("LTE measurements = %+v", got)
	}
}

func TestNASSignalStrengthResultRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want string
	}{
		{name: "missing current", want: "current signal"},
		{name: "current length", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1})}, want: "current signal"},
		{
			name: "strength count",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x10, []byte{3, 0})},
			want: "signal strength list: count",
		},
		{
			name: "strength length",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x10, []byte{1, 0, 1})},
			want: "signal strength list: TLV length",
		},
		{
			name: "rssi count",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x11, []byte{1})},
			want: "RSSI list: count",
		},
		{
			name: "ecio length",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x12, []byte{1, 0, 1})},
			want: "ECIO list: TLV length",
		},
		{
			name: "io length",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x13, []byte{1, 2})},
			want: "IO TLV length",
		},
		{
			name: "sinr length",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x14, []byte{})},
			want: "SINR TLV length",
		},
		{
			name: "error rate length",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x15, []byte{1, 0, 1})},
			want: "error rate list: TLV length",
		},
		{
			name: "rsrq length",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x16, []byte{1})},
			want: "RSRQ TLV length",
		},
		{
			name: "lte snr length",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x17, []byte{1})},
			want: "LTE SNR TLV length",
		},
		{
			name: "lte rsrp length",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8}), tlv.Bytes(0x18, []byte{1})},
			want: "LTE RSRP TLV length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASSignalStrengthResult
			err := got.UnmarshalTLVs(tt.tlvs)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestClientSignalStrength(t *testing.T) {
	transport := &fakeTransport{t: t, calls: []transportCall{{
		check: func(req Request) {
			if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != MessageNASGetSignalStrength {
				t.Fatalf("request = %+v", req)
			}
			assertTLV(t, req.TLVs, 0x10, []byte{0x81, 0})
		},
		resp: successResponse(MessageNASGetSignalStrength, tlv.Bytes(0x01, []byte{nasLegacySignedByte(-75), byte(NASRadioInterfaceLTE)})),
	}}}
	client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
	got, err := client.SignalStrength(context.Background(), NASSignalStrengthRequestRSSI|NASSignalStrengthRequestLTERSRP)
	if err != nil {
		t.Fatalf("SignalStrength() error = %v", err)
	}
	if got.Current.Strength != -75 || got.Current.RadioInterface != NASRadioInterfaceLTE {
		t.Fatalf("Current = %+v", got.Current)
	}
}

func nasLegacySignedByte(value int8) byte {
	return byte(value)
}

func nasLegacyInt32(value int32) []byte {
	return binary.LittleEndian.AppendUint32(nil, uint32(value))
}
