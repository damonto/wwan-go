package qcom

import (
	"context"
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSRevisionInfoUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSRevisionInfo
		wantErr bool
	}{
		{
			name: "revision only",
			tlvs: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte("MPSS.HI.2.0"))},
			want: DMSRevisionInfo{Revision: "MPSS.HI.2.0"},
		},
		{
			name: "all revisions",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, []byte("MPSS.HI.2.0")),
				tlv.Bytes(dmsTLVRevisionBootCode, []byte("BOOT.1.0")),
				tlv.Bytes(dmsTLVRevisionPRI, []byte("PRI.42")),
			},
			want: DMSRevisionInfo{
				Revision:      "MPSS.HI.2.0",
				BootCode:      "BOOT.1.0",
				BootCodeKnown: true,
				PRI:           "PRI.42",
				PRIKnown:      true,
			},
		},
		{name: "missing revision", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSRevisionInfo
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSSerialNumbersUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want DMSSerialNumbers
	}{
		{name: "no identifiers"},
		{
			name: "all identifiers",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVSerialESN, []byte("80ABCDEF")),
				tlv.Bytes(dmsTLVSerialIMEI, []byte("123456789012345")),
				tlv.Bytes(dmsTLVSerialMEID, []byte("A1000012345678")),
				tlv.Bytes(dmsTLVSerialIMEISV, []byte("01")),
			},
			want: DMSSerialNumbers{
				ESN:         "80ABCDEF",
				ESNKnown:    true,
				IMEI:        "123456789012345",
				IMEIKnown:   true,
				MEID:        "A1000012345678",
				MEIDKnown:   true,
				IMEISV:      "01",
				IMEISVKnown: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSSerialNumbers
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSEncryptedSerialNumbersUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSEncryptedSerialNumbers
		wantErr bool
	}{
		{name: "no identifiers"},
		{
			name: "all identifiers",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVSerialESN, []byte{0x00, 0xFF}),
				tlv.Bytes(dmsTLVSerialIMEI, []byte{0x10, 0x20}),
				tlv.Bytes(dmsTLVSerialMEID, []byte{0x30, 0x40}),
				tlv.Bytes(dmsTLVSerialIMEISV, []byte{0x50, 0x60}),
			},
			want: DMSEncryptedSerialNumbers{
				ESN: []byte{0x00, 0xFF}, ESNKnown: true,
				IMEI: []byte{0x10, 0x20}, IMEIKnown: true,
				MEID: []byte{0x30, 0x40}, MEIDKnown: true,
				IMEISV: []byte{0x50, 0x60}, IMEISVKnown: true,
			},
		},
		{
			name:    "identifier too long",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVSerialIMEI, make([]byte, dmsMaxEncryptedSerialNumber+1))},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSEncryptedSerialNumbers
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalTLVs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.ESNKnown != tt.want.ESNKnown || !slices.Equal(got.ESN, tt.want.ESN) ||
				got.IMEIKnown != tt.want.IMEIKnown || !slices.Equal(got.IMEI, tt.want.IMEI) ||
				got.MEIDKnown != tt.want.MEIDKnown || !slices.Equal(got.MEID, tt.want.MEID) ||
				got.IMEISVKnown != tt.want.IMEISVKnown || !slices.Equal(got.IMEISV, tt.want.IMEISV) {
				t.Fatalf("UnmarshalTLVs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDMSPowerStateUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSPowerState
		wantErr bool
	}{
		{
			name: "external power charging",
			tlvs: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte{
				byte(DMSPowerExternalSource | DMSPowerBatteryConnected | DMSPowerBatteryCharging),
				87,
			})},
			want: DMSPowerState{
				Status:       DMSPowerExternalSource | DMSPowerBatteryConnected | DMSPowerBatteryCharging,
				BatteryLevel: 87,
			},
		},
		{name: "missing", wantErr: true},
		{name: "truncated", tlvs: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte{1})}, wantErr: true},
		{name: "trailing byte", tlvs: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte{1, 2, 3})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSPowerState
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSDeviceTimeUnmarshalTLVs(t *testing.T) {
	deviceTime := []byte{1, 2, 3, 4, 5, 6}
	deviceTime = binary.LittleEndian.AppendUint16(deviceTime, uint16(DMSTimeSourceCDMA))
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSDeviceTime
		wantErr bool
	}{
		{
			name: "all timestamps",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, deviceTime),
				dmsUint64TLV(dmsTLVSystemTime, 1234),
				dmsUint64TLV(dmsTLVUserTime, 5678),
			},
			want: DMSDeviceTime{
				TimeCount:               0x060504030201,
				Source:                  DMSTimeSourceCDMA,
				SystemMilliseconds:      1234,
				SystemMillisecondsKnown: true,
				UserMilliseconds:        5678,
				UserMillisecondsKnown:   true,
			},
		},
		{name: "missing device time", wantErr: true},
		{name: "truncated device time", tlvs: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, deviceTime[:7])}, wantErr: true},
		{
			name: "truncated system time",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, deviceTime),
				tlv.Bytes(dmsTLVSystemTime, make([]byte, 7)),
			},
			wantErr: true,
		},
		{
			name: "oversized user time",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, deviceTime),
				tlv.Bytes(dmsTLVUserTime, make([]byte, 9)),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSDeviceTime
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSBandCapabilitiesUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSBandCapabilities
		wantErr bool
	}{
		{
			name: "all capabilities",
			tlvs: tlv.TLVs{
				dmsUint64TLV(dmsTLVPrimaryValue, 0x0102030405060708),
				dmsUint64TLV(dmsTLVDeprecatedLTEBandMask, 0x11),
				dmsUint64TLV(dmsTLVTDSBandMask, 0x22),
				tlv.Bytes(dmsTLVSupportedLTEBands, encodeDMSBandList(1, 3, 41)),
				tlv.Bytes(dmsTLVSupportedNR5GBands, encodeDMSBandList(1, 78, 257)),
			},
			want: DMSBandCapabilities{
				BandMask:         0x0102030405060708,
				LTEBandMask:      0x11,
				LTEBandMaskKnown: true,
				TDSBandMask:      0x22,
				TDSBandMaskKnown: true,
				LTEBands:         []uint16{1, 3, 41},
				LTEBandsKnown:    true,
				NR5GBands:        []uint16{1, 78, 257},
				NR5GBandsKnown:   true,
			},
		},
		{name: "missing band mask", wantErr: true},
		{name: "truncated band mask", tlvs: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, make([]byte, 7))}, wantErr: true},
		{
			name: "truncated LTE count",
			tlvs: tlv.TLVs{
				dmsUint64TLV(dmsTLVPrimaryValue, 1),
				tlv.Bytes(dmsTLVSupportedLTEBands, []byte{1}),
			},
			wantErr: true,
		},
		{
			name: "LTE count exceeds maximum",
			tlvs: tlv.TLVs{
				dmsUint64TLV(dmsTLVPrimaryValue, 1),
				tlv.Bytes(dmsTLVSupportedLTEBands, binary.LittleEndian.AppendUint16(nil, dmsMaxLTEBands+1)),
			},
			wantErr: true,
		},
		{
			name: "NR5G length mismatch",
			tlvs: tlv.TLVs{
				dmsUint64TLV(dmsTLVPrimaryValue, 1),
				tlv.Bytes(dmsTLVSupportedNR5GBands, []byte{2, 0, 1, 0}),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSBandCapabilities
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.BandMask != tt.want.BandMask ||
				got.LTEBandMask != tt.want.LTEBandMask || got.LTEBandMaskKnown != tt.want.LTEBandMaskKnown ||
				got.TDSBandMask != tt.want.TDSBandMask || got.TDSBandMaskKnown != tt.want.TDSBandMaskKnown ||
				got.LTEBandsKnown != tt.want.LTEBandsKnown || !slices.Equal(got.LTEBands, tt.want.LTEBands) ||
				got.NR5GBandsKnown != tt.want.NR5GBandsKnown || !slices.Equal(got.NR5GBands, tt.want.NR5GBands) {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSSoftwareVersionUnmarshalTLVs(t *testing.T) {
	images := encodeDMSImageVersions(
		DMSImageVersion{Type: DMSImageMPSS, Version: "MPSS.HI.2.0"},
		DMSImageVersion{Type: DMSImageADSP, Version: "ADSP.1.0"},
	)
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSSoftwareVersion
		wantErr bool
	}{
		{
			name: "all versions",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, []byte("01.02.03")),
				tlv.Bytes(dmsTLVImageVersions, images),
				tlv.Bytes(dmsTLVExtendedSoftwareVersion, []byte("01.02.03-extended")),
				tlv.Bytes(dmsTLVSecondarySoftwareVersion, []byte("secondary")),
			},
			want: DMSSoftwareVersion{
				Version:        "01.02.03",
				Images:         []DMSImageVersion{{Type: DMSImageMPSS, Version: "MPSS.HI.2.0"}, {Type: DMSImageADSP, Version: "ADSP.1.0"}},
				ImagesKnown:    true,
				Extended:       "01.02.03-extended",
				ExtendedKnown:  true,
				Secondary:      "secondary",
				SecondaryKnown: true,
			},
		},
		{name: "missing software version", wantErr: true},
		{
			name: "missing image count",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, []byte("01.02.03")),
				tlv.Bytes(dmsTLVImageVersions, nil),
			},
			wantErr: true,
		},
		{
			name: "too many images",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, []byte("01.02.03")),
				tlv.Bytes(dmsTLVImageVersions, []byte{dmsMaxImageVersions + 1}),
			},
			wantErr: true,
		},
		{
			name: "truncated image header",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, []byte("01.02.03")),
				tlv.Bytes(dmsTLVImageVersions, []byte{1, 1, 2, 3}),
			},
			wantErr: true,
		},
		{
			name: "truncated image version",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, []byte("01.02.03")),
				tlv.Bytes(dmsTLVImageVersions, []byte{1, 9, 0, 0, 0, 2, 'x'}),
			},
			wantErr: true,
		},
		{
			name: "trailing image bytes",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, []byte("01.02.03")),
				tlv.Bytes(dmsTLVImageVersions, []byte{0, 1}),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSSoftwareVersion
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Version != tt.want.Version || got.ImagesKnown != tt.want.ImagesKnown ||
				!slices.Equal(got.Images, tt.want.Images) || got.Extended != tt.want.Extended ||
				got.ExtendedKnown != tt.want.ExtendedKnown || got.Secondary != tt.want.Secondary ||
				got.SecondaryKnown != tt.want.SecondaryKnown {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSSubscriptionWire(t *testing.T) {
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLV     []byte
		wantErr     bool
	}{
		{
			name: "bind secondary",
			request: func() (Request, error) {
				return (DMSBindSubscriptionRequest{
					ClientID:      7,
					TransactionID: 9,
					Timeout:       2 * time.Second,
					Subscription:  DMSSubscriptionSecondary,
				}).Request()
			},
			wantMessage: MessageDMSBindSubscription,
			wantTLV:     binary.LittleEndian.AppendUint32(nil, uint32(DMSSubscriptionSecondary)),
		},
		{
			name: "get binding",
			request: func() (Request, error) {
				return (DMSGetBindSubscriptionRequest{
					ClientID:      7,
					TransactionID: 9,
					Timeout:       2 * time.Second,
				}).Request(), nil
			},
			wantMessage: MessageDMSGetBindSubscription,
		},
		{
			name: "invalid subscription",
			request: func() (Request, error) {
				return (DMSBindSubscriptionRequest{Subscription: 4}).Request()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Request() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceDMS || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = service 0x%02X message 0x%04X", got.Service, got.MessageID)
			}
			if tt.wantTLV == nil {
				if len(got.TLVs) != 0 {
					t.Fatalf("TLVs len = %d, want 0", len(got.TLVs))
				}
				return
			}
			assertTLV(t, got.TLVs, dmsTLVPrimaryValue, tt.wantTLV)
		})
	}
}

func TestDMSGetBindSubscriptionResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSGetBindSubscriptionResponse
		wantErr bool
	}{
		{name: "not reported"},
		{
			name: "secondary",
			tlvs: tlv.TLVs{tlv.Uint(dmsTLVBoundSubscription, uint32(DMSSubscriptionSecondary))},
			want: DMSGetBindSubscriptionResponse{Subscription: DMSSubscriptionSecondary, SubscriptionKnown: true},
		},
		{name: "truncated", tlvs: tlv.TLVs{tlv.Bytes(dmsTLVBoundSubscription, make([]byte, 3))}, wantErr: true},
		{name: "oversized", tlvs: tlv.TLVs{tlv.Bytes(dmsTLVBoundSubscription, make([]byte, 5))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSGetBindSubscriptionResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSClientQueries(t *testing.T) {
	tests := []struct {
		name        string
		message     MessageID
		responseTLV tlv.TLVs
		wantTLV     []byte
		run         func(context.Context, *Client) error
	}{
		{
			name:    "device capabilities",
			message: MessageDMSGetDeviceCapabilities,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, encodeDMSDeviceCapabilitiesCore(
				1,
				2,
				DMSDataServicePSOnly,
				DMSSIMSupported,
				DMSRadioInterfaceLTE,
			))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.DeviceCapabilities(ctx)
				return err
			},
		},
		{
			name:        "manufacturer",
			message:     MessageDMSGetManufacturer,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte("Qualcomm"))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.Manufacturer(ctx)
				return err
			},
		},
		{
			name:        "model ID",
			message:     MessageDMSGetModelID,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte("SDX75"))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.ModelID(ctx)
				return err
			},
		},
		{
			name:        "revision information",
			message:     MessageDMSGetRevisionID,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte("MPSS.1"))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.RevisionInfo(ctx)
				return err
			},
		},
		{
			name:    "serial numbers",
			message: MessageDMSGetSerialNumbers,
			run: func(ctx context.Context, c *Client) error {
				_, err := c.SerialNumbers(ctx)
				return err
			},
		},
		{
			name:    "encrypted serial numbers",
			message: MessageDMSGetEncryptedSerialNumbers,
			run: func(ctx context.Context, c *Client) error {
				_, err := c.EncryptedSerialNumbers(ctx)
				return err
			},
		},
		{
			name:        "power state",
			message:     MessageDMSGetPowerState,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte{0, 50})},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.PowerState(ctx)
				return err
			},
		},
		{
			name:        "hardware revision",
			message:     MessageDMSGetHardwareRevision,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte("1.0"))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.HardwareRevision(ctx)
				return err
			},
		},
		{
			name:        "device time",
			message:     MessageDMSGetTime,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, make([]byte, 8))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.DeviceTime(ctx)
				return err
			},
		},
		{
			name:        "band capabilities",
			message:     MessageDMSGetBandCapabilities,
			responseTLV: tlv.TLVs{dmsUint64TLV(dmsTLVPrimaryValue, 1)},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.BandCapabilities(ctx)
				return err
			},
		},
		{
			name:        "factory SKU",
			message:     MessageDMSGetFactorySKU,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte("SKU-1"))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.FactorySKU(ctx)
				return err
			},
		},
		{
			name:        "software version",
			message:     MessageDMSGetSoftwareVersion,
			responseTLV: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, []byte("01.02.03"))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.SoftwareVersion(ctx)
				return err
			},
		},
		{
			name:    "bind subscription",
			message: MessageDMSBindSubscription,
			wantTLV: binary.LittleEndian.AppendUint32(nil, uint32(DMSSubscriptionTertiary)),
			run: func(ctx context.Context, c *Client) error {
				return c.DMSBindSubscription(ctx, DMSSubscriptionTertiary)
			},
		},
		{
			name:        "get bound subscription",
			message:     MessageDMSGetBindSubscription,
			responseTLV: tlv.TLVs{tlv.Uint(dmsTLVBoundSubscription, uint32(DMSSubscriptionSecondary))},
			run: func(ctx context.Context, c *Client) error {
				_, err := c.DMSBoundSubscription(ctx)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &serviceBoundFakeTransport{
				fakeTransport: fakeTransport{
					t: t,
					calls: []transportCall{{
						check: func(req Request) {
							if req.Service != ServiceDMS || req.ClientID != 0 || req.MessageID != tt.message {
								t.Fatalf("unexpected DMS request: %+v", req)
							}
							if tt.wantTLV == nil {
								if len(req.TLVs) != 0 {
									t.Fatalf("TLVs len = %d, want 0", len(req.TLVs))
								}
								return
							}
							assertTLV(t, req.TLVs, dmsTLVPrimaryValue, tt.wantTLV)
						},
						resp: successResponse(tt.message, tt.responseTLV...),
					}},
				},
				service: ServiceDMS,
			}
			client, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if err := tt.run(context.Background(), client); err != nil {
				t.Fatalf("query error = %v", err)
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls = %d, want 1", got)
			}
		})
	}
}

func encodeDMSBandList(bands ...uint16) []byte {
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(bands)))
	for _, band := range bands {
		value = binary.LittleEndian.AppendUint16(value, band)
	}
	return value
}

func encodeDMSImageVersions(images ...DMSImageVersion) []byte {
	value := []byte{byte(len(images))}
	for _, image := range images {
		value = binary.LittleEndian.AppendUint32(value, uint32(image.Type))
		value = append(value, byte(len(image.Version)))
		value = append(value, image.Version...)
	}
	return value
}

func dmsUint64TLV(typ byte, value uint64) tlv.TLV {
	return tlv.Bytes(typ, binary.LittleEndian.AppendUint64(nil, value))
}
