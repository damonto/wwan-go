package mbim

import (
	"testing"

	mbimproto "github.com/damonto/wwan-go/mbim"
)

func TestUSSDCodec(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "GSM7", text: "*123#"},
		{name: "GSM7 extension", text: "{EUR}"},
		{name: "UCS2", text: "余额"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dcs, payload, err := encodeMBIMUSSD(tt.text)
			if err != nil {
				t.Fatalf("encodeMBIMUSSD() error = %v", err)
			}
			decoded, err := mbimUSSDMessage(mbimproto.USSDInfo{DataCodingScheme: dcs, Payload: payload})
			if err != nil {
				t.Fatalf("mbimUSSDMessage() error = %v", err)
			}
			if decoded.Text != tt.text {
				t.Errorf("USSD round trip = %q, want %q", decoded.Text, tt.text)
			}
		})
	}
}

func TestUSSDDecodeValidation(t *testing.T) {
	tests := []struct {
		name string
		info mbimproto.USSDInfo
	}{
		{name: "odd UCS2", info: mbimproto.USSDInfo{DataCodingScheme: ussdDCSUCS2, Payload: []byte{0}}},
		{name: "unknown nonempty DCS", info: mbimproto.USSDInfo{DataCodingScheme: 1, Payload: []byte{1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mbimUSSDMessage(tt.info); err == nil {
				t.Fatal("mbimUSSDMessage() error = nil, want non-nil")
			}
		})
	}
}
