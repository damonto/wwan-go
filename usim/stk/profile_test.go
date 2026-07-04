package stk

import (
	"slices"
	"testing"
)

func TestProfileProactiveCommandTypes(t *testing.T) {
	tests := []struct {
		name string
		in   Profile
		want []CommandType
	}{
		{
			name: "explicit commands",
			in: Profile{
				Data:     []byte{0x00},
				Commands: []CommandType{CommandDisplayText, CommandSetupMenu},
			},
			want: []CommandType{CommandDisplayText, CommandSetupMenu},
		},
		{
			name: "derived from terminal profile",
			in: Profile{
				Data: []byte{
					0x00,
					0x00,
					0b00000111,
					0b01100010,
					0b00001001,
					0x00,
					0b10001000,
					0x00,
					0x00,
					0x00,
					0x00,
					0b00011111,
					0x00,
					0x00,
					0x00,
					0x00,
					0x01,
				},
			},
			want: []CommandType{
				CommandDisplayText,
				CommandGetInkey,
				CommandGetInput,
				CommandSendSMS,
				CommandSetupMenu,
				CommandProvideLocalInfo,
				CommandSetupIdleModeText,
				CommandLanguageNotify,
				CommandLaunchBrowser,
				CommandSendDTMF,
				CommandOpenChannel,
				CommandCloseChannel,
				CommandReceiveData,
				CommandSendData,
				CommandGetChannelStatus,
				CommandActivate,
			},
		},
		{
			name: "empty profile",
			in:   Profile{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.ProactiveCommandTypes()
			if !slices.Equal(got, tt.want) {
				t.Fatalf("ProactiveCommandTypes() = %v, want %v", got, tt.want)
			}
			if len(got) > 0 {
				got[0] = 0xff
				again := tt.in.ProactiveCommandTypes()
				if slices.Equal(got, again) {
					t.Fatal("ProactiveCommandTypes() returned mutable internal storage")
				}
			}
		})
	}
}

func TestProfileBIPBits(t *testing.T) {
	profile := NewProfile(CapabilityBIP)
	got := profile.Bytes()
	if len(got) < 13 {
		t.Fatalf("profile length = %d, want at least 13", len(got))
	}
	if got[5]&0x0C != 0x0C {
		t.Fatalf("byte 6 = %08b, want data available/channel status event bits", got[5])
	}
	if got[11]&0x1F != 0x1F {
		t.Fatalf("byte 12 = %08b, want BIP command bits", got[11])
	}
	if got[12] != 0x07 {
		t.Fatalf("byte 13 = 0x%02X, want seven supported channels", got[12])
	}
	if got[7]&0x1F != 0 {
		t.Fatalf("byte 8 = %08b, want no BIP command bits", got[7])
	}
}
