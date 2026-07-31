package qcom

import (
	"encoding/binary"
	"net/netip"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestIMSDCMPDPActivateRequest(t *testing.T) {
	sequence := uint32(0x01020304)
	subscription := uint32(2)
	slot := uint32(1)
	instance := IMSDCMInstanceNone
	tests := []struct {
		name      string
		request   IMSDCMPDPActivateRequest
		wantError bool
	}{
		{
			name: "all fields",
			request: IMSDCMPDPActivateRequest{
				ClientID:      7,
				TransactionID: 9,
				Connection: IMSDCMConnection{
					APN: "ims", APNType: IMSDCMAPNIMS, RAT: IMSDCMRATLTE,
					IPFamily: IMSDCMIPv6, WDSProfileNum: 3,
				},
				SequenceNumber: &sequence,
				SubscriptionID: &subscription,
				SlotID:         &slot,
				Instance:       &instance,
			},
		},
		{name: "missing APN", request: IMSDCMPDPActivateRequest{}, wantError: true},
		{name: "invalid RAT", request: IMSDCMPDPActivateRequest{Connection: IMSDCMConnection{APN: "ims", RAT: IMSDCMRATWLAN + 1}}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.request.Request()
			if (err != nil) != tt.wantError {
				t.Fatalf("Request() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if req.Service != ServiceIMSDCM || req.ClientID != 7 || req.TransactionID != 9 || req.MessageID != MessageIMSDCMPDPActivate {
				t.Fatalf("Request() = %+v, want IMS DCM PDP Activate", req)
			}
			connection := []byte{3, 'i', 'm', 's'}
			connection = binary.LittleEndian.AppendUint32(connection, uint32(IMSDCMAPNIMS))
			connection = binary.LittleEndian.AppendUint32(connection, uint32(IMSDCMRATLTE))
			connection = binary.LittleEndian.AppendUint32(connection, uint32(IMSDCMIPv6))
			connection = binary.LittleEndian.AppendUint32(connection, 3)
			assertTLV(t, req.TLVs, 0x01, connection)
			assertTLV(t, req.TLVs, 0x10, binary.LittleEndian.AppendUint32(nil, sequence))
			assertTLV(t, req.TLVs, 0x13, []byte{0xff, 0xff, 0xff, 0xff})
		})
	}
}

func TestIMSDCMPDPActivateResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		wantError bool
	}{
		{name: "all fields", tlvs: tlv.TLVs{tlv.Uint(0x10, uint8(3)), tlv.Uint(0x11, uint32(7)), tlv.Uint(0x12, uint32(IMSDCMInstance2))}},
		{name: "optional fields absent"},
		{name: "PDP ID trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{3, 0})}, wantError: true},
		{name: "sequence trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x11, make([]byte, 5))}, wantError: true},
		{name: "instance truncated", tlvs: tlv.TLVs{tlv.Bytes(0x12, make([]byte, 3))}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSDCMPDPActivateResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.name == "all fields" && (!got.PDPIDKnown || got.PDPID != 3 || !got.SequenceNumberKnown || got.SequenceNumber != 7 || !got.InstanceKnown || got.Instance != IMSDCMInstance2) {
				t.Fatalf("UnmarshalTLVs() = %+v, want all fields", got)
			}
		})
	}
}

func TestIMSDCMPDPActivationUnmarshalTLVs(t *testing.T) {
	address := binary.LittleEndian.AppendUint32(nil, uint32(IMSDCMIPv4))
	address = append(address, byte(len("192.0.2.1")))
	address = append(address, "192.0.2.1"...)
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		wantError bool
	}{
		{
			name: "success",
			tlvs: tlv.TLVs{
				tlv.Bytes(qmiTLVResult, []byte{0, 0, 0, 0}),
				tlv.Uint(0x01, uint8(4)),
				tlv.Bytes(0x11, address),
			},
		},
		{name: "result missing", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(4))}, wantError: true},
		{name: "result trailing byte", tlvs: tlv.TLVs{tlv.Bytes(qmiTLVResult, make([]byte, 5)), tlv.Uint(0x01, uint8(4))}, wantError: true},
		{name: "PDP ID missing", tlvs: tlv.TLVs{tlv.Bytes(qmiTLVResult, make([]byte, 4))}, wantError: true},
		{name: "address count mismatch", tlvs: tlv.TLVs{tlv.Bytes(qmiTLVResult, make([]byte, 4)), tlv.Uint(0x01, uint8(4)), tlv.Bytes(0x11, append(address, 0))}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSDCMPDPActivation
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.name == "success" && (got.PDPID != 4 || !got.AddressKnown || got.Address != netip.MustParseAddr("192.0.2.1")) {
				t.Fatalf("UnmarshalTLVs() = %+v, want decoded activation", got)
			}
		})
	}
}
