package qcom

import (
	"bytes"
	"testing"
)

func TestPutSessionValueAIDLimit(t *testing.T) {
	tests := []struct {
		name    string
		aid     []byte
		wantErr bool
	}{
		{
			name: "maximum AID",
			aid:  bytes.Repeat([]byte{0xA0}, uimAIDMaxLength),
		},
		{
			name:    "AID too long",
			aid:     bytes.Repeat([]byte{0xA0}, uimAIDMaxLength+1),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := putSessionValue(SessionCardSlot1, tt.aid)
			if (err != nil) != tt.wantErr {
				t.Fatalf("putSessionValue() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(got) != 2+len(tt.aid) || got[0] != byte(SessionCardSlot1) || got[1] != byte(len(tt.aid)) {
				t.Fatalf("putSessionValue() = % X", got)
			}
		})
	}
}

func TestPutFileValuePathLimit(t *testing.T) {
	tests := []struct {
		name           string
		directoryBytes int
		wantErr        bool
	}{
		{
			name:           "maximum path",
			directoryBytes: uimPathMaxLength,
		},
		{
			name:           "path too long",
			directoryBytes: uimPathMaxLength + 2,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := bytes.Repeat([]byte{0x3F, 0x00}, tt.directoryBytes/2)
			path = append(path, 0x6F, 0x07)
			got, err := putFileValue(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("putFileValue() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(got) != 3+tt.directoryBytes || got[2] != byte(tt.directoryBytes) {
				t.Fatalf("putFileValue() = % X", got)
			}
		})
	}
}
