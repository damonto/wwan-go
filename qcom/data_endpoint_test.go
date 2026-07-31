package qcom

import (
	"bytes"
	"encoding"
	"io"
	"testing"
)

var (
	_ encoding.BinaryMarshaler   = DataEndpoint{}
	_ encoding.BinaryAppender    = DataEndpoint{}
	_ encoding.BinaryUnmarshaler = (*DataEndpoint)(nil)
	_ io.WriterTo                = DataEndpoint{}
	_ io.ReaderFrom              = (*DataEndpoint)(nil)
)

func TestDataEndpointBinaryEncoding(t *testing.T) {
	tests := []struct {
		name     string
		endpoint DataEndpoint
		prefix   []byte
		want     []byte
	}{
		{
			name:     "BAM DMUX interface",
			endpoint: DataEndpoint{Type: DataEndpointBAMDMUX, InterfaceID: 1},
			want:     []byte{0x05, 0, 0, 0, 0x01, 0, 0, 0},
		},
		{
			name:     "HSUSB interface with prefix",
			endpoint: DataEndpoint{Type: DataEndpointHSUSB, InterfaceID: 4},
			prefix:   []byte{0xAA, 0xBB},
			want:     []byte{0xAA, 0xBB, 0x02, 0, 0, 0, 0x04, 0, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.endpoint.AppendBinary(bytes.Clone(tt.prefix))
			if err != nil {
				t.Fatalf("AppendBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("AppendBinary() = % X, want % X", got, tt.want)
			}

			marshaled, err := tt.endpoint.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			wantMarshaled := tt.want[len(tt.prefix):]
			if !bytes.Equal(marshaled, wantMarshaled) {
				t.Fatalf("MarshalBinary() = % X, want % X", marshaled, wantMarshaled)
			}

			var decoded DataEndpoint
			if err := decoded.UnmarshalBinary(wantMarshaled); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if decoded != tt.endpoint {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", decoded, tt.endpoint)
			}

			var written bytes.Buffer
			n, err := tt.endpoint.WriteTo(&written)
			if err != nil {
				t.Fatalf("WriteTo() error = %v", err)
			}
			if n != int64(len(wantMarshaled)) || !bytes.Equal(written.Bytes(), wantMarshaled) {
				t.Fatalf("WriteTo() = (%d, % X), want (%d, % X)", n, written.Bytes(), len(wantMarshaled), wantMarshaled)
			}

			var read DataEndpoint
			n, err = read.ReadFrom(bytes.NewReader(wantMarshaled))
			if err != nil {
				t.Fatalf("ReadFrom() error = %v", err)
			}
			if n != int64(len(wantMarshaled)) || read != tt.endpoint {
				t.Fatalf("ReadFrom() = (%d, %+v), want (%d, %+v)", n, read, len(wantMarshaled), tt.endpoint)
			}
		})
	}
}

func TestWDSDataEndpointCompatibilityAliases(t *testing.T) {
	tests := []struct {
		name     string
		endpoint WDSDataEndpoint
		wantType WDSDataEndpointType
	}{
		{
			name:     "BAM DMUX",
			endpoint: WDSDataEndpoint{Type: WDSDataEndpointBAMDMUX, InterfaceID: 1},
			wantType: WDSDataEndpointBAMDMUX,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.endpoint.Type != tt.wantType {
				t.Fatalf("Type = %d, want %d", tt.endpoint.Type, tt.wantType)
			}
		})
	}
}
