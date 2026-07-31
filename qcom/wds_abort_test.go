package qcom

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestAbortRequests(t *testing.T) {
	tests := []struct {
		name   string
		got    Request
		want   ServiceType
		id     MessageID
		target uint16
	}{
		{
			name: "NAS",
			got: (NASAbortRequest{
				ClientID: 7, TransactionID: 9, Timeout: time.Second, TargetTransactionID: 42,
			}).Request(),
			want:   ServiceNAS,
			id:     MessageNASAbort,
			target: 42,
		},
		{
			name: "WDS",
			got: (WDSAbortRequest{
				ClientID: 8, TransactionID: 10, Timeout: 2 * time.Second, TargetTransactionID: 43,
			}).Request(),
			want:   ServiceWDS,
			id:     MessageWDSAbort,
			target: 43,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Service != tt.want || tt.got.MessageID != tt.id || tt.got.Timeout <= 0 {
				t.Fatalf("Request() = %+v", tt.got)
			}
			assertTLV(t, tt.got.TLVs, 0x01, binary.LittleEndian.AppendUint16(nil, tt.target))
		})
	}
}
