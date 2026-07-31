package mbim

import "testing"

func TestMicrosoftServiceIdentifiers(t *testing.T) {
	tests := []struct {
		name     string
		gotUUID  [16]byte
		wantUUID [16]byte
		gotCID   uint32
		wantCID  uint32
	}{
		{
			name:     "SAR configuration",
			gotUUID:  ServiceMsSAR,
			wantUUID: [16]byte{0x68, 0x22, 0x3D, 0x04, 0x9F, 0x6C, 0x4E, 0x0F, 0x82, 0x2D, 0x28, 0x44, 0x1F, 0xB7, 0x23, 0x40},
			gotCID:   CIDMsSARConfig,
			wantCID:  1,
		},
		{
			name:     "SAR transmission status",
			gotUUID:  ServiceMsSAR,
			wantUUID: [16]byte{0x68, 0x22, 0x3D, 0x04, 0x9F, 0x6C, 0x4E, 0x0F, 0x82, 0x2D, 0x28, 0x44, 0x1F, 0xB7, 0x23, 0x40},
			gotCID:   CIDMsSARTransmissionStatus,
			wantCID:  2,
		},
		{
			name:     "voice extensions NITZ",
			gotUUID:  ServiceMsVoiceExtensions,
			wantUUID: [16]byte{0x8D, 0x8B, 0x9E, 0xBA, 0x37, 0xBE, 0x44, 0x9B, 0x8F, 0x1E, 0x61, 0xCB, 0x03, 0x4A, 0x70, 0x2E},
			gotCID:   CIDMsVoiceExtensionsNITZ,
			wantCID:  10,
		},
		{
			name:     "proxy version",
			gotUUID:  ServiceMbimProxyControl,
			wantUUID: [16]byte{0x83, 0x8C, 0xF7, 0xFB, 0x8D, 0x0D, 0x4D, 0x7F, 0x87, 0x1E, 0xD7, 0x1D, 0xBE, 0xFB, 0xB3, 0x9B},
			gotCID:   CIDProxyControlVersion,
			wantCID:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.gotUUID != tt.wantUUID {
				t.Fatalf("service UUID = %x, want %x", tt.gotUUID, tt.wantUUID)
			}
			if tt.gotCID != tt.wantCID {
				t.Fatalf("CID = %d, want %d", tt.gotCID, tt.wantCID)
			}
		})
	}
}
