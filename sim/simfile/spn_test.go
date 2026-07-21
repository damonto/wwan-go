package simfile

import "testing"

func TestServiceProviderNameUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want ServiceProviderName
	}{
		{name: "GSM text", data: []byte{0x00, 'g', 'i', 'f', 'f', 'g', 'a', 'f', 'f', 0xFF}, want: "giffgaff"},
		{name: "UCS2", data: []byte{0x00, 0x80, 0x00, 'O', 0x00, '2', 0xFF}, want: "O2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ServiceProviderName
			if err := got.UnmarshalBinary(tt.data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalBinary() = %q, want %q", got, tt.want)
			}
		})
	}
}
