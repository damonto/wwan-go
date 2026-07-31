package qmi

import (
	"testing"

	"github.com/damonto/wwan-go/qcom"
)

func TestUSSDCodec(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "ASCII", text: "*123#"},
		{name: "UCS2", text: "余额"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := encodeQMIUSSD(tt.text)
			if err != nil {
				t.Fatalf("encodeQMIUSSD() error = %v", err)
			}
			decoded, err := qmiUSSDData(encoded)
			if err != nil {
				t.Fatalf("qmiUSSDData() error = %v", err)
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
		data qcom.VoiceUSSDData
	}{
		{name: "odd UCS2", data: qcom.VoiceUSSDData{Encoding: qcom.VoiceUSSDEncodingUCS2, Data: []byte{0}}},
		{name: "unknown encoding", data: qcom.VoiceUSSDData{Encoding: 99}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := qmiUSSDData(tt.data); err == nil {
				t.Fatal("qmiUSSDData() error = nil, want non-nil")
			}
		})
	}
}
