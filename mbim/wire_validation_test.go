package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestOffsetSizeRefsValidateDataBuffer(t *testing.T) {
	tests := []struct {
		name        string
		offset      uint32
		size        uint32
		length      int
		paddingByte byte
		wantErr     bool
	}{
		{name: "valid", offset: 12, size: 4, length: 16},
		{name: "final padding", offset: 12, size: 2, length: 16},
		{name: "nonzero final padding", offset: 12, size: 2, length: 16, paddingByte: 0xff},
		{name: "missing final padding", offset: 12, size: 2, length: 14, wantErr: true},
		{name: "trailing data", offset: 12, size: 4, length: 20, wantErr: true},
		{name: "reference table", offset: 4, size: 4, wantErr: true},
		{name: "zero offset with data", offset: 0, size: 4, wantErr: true},
		{name: "nonzero offset without data", offset: 12, length: 12},
		{name: "unaligned offset", offset: 14, size: 2, length: 16, wantErr: true},
		{name: "sparse value", offset: 16, size: 4, length: 20, wantErr: true},
		{name: "truncated value", offset: 12, size: 8, length: 16, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			length := tt.length
			if length == 0 {
				length = 16
			}
			data := make([]byte, length)
			binary.LittleEndian.PutUint32(data[4:8], tt.offset)
			binary.LittleEndian.PutUint32(data[8:12], tt.size)
			if tt.paddingByte != 0 {
				for i := int(tt.offset + tt.size); i < len(data); i++ {
					data[i] = tt.paddingByte
				}
			}
			_, err := offsetSizeRefs(data, 4, 1)
			if (err != nil) != tt.wantErr {
				t.Fatalf("offsetSizeRefs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOffsetSizeRefsValidateOrdering(t *testing.T) {
	tests := []struct {
		name         string
		firstSize    uint32
		secondOffset uint32
		length       int
		wantErr      bool
	}{
		{name: "ordered", firstSize: 4, secondOffset: 24, length: 28},
		{name: "alignment padding", firstSize: 2, secondOffset: 24, length: 28},
		{name: "overlap", firstSize: 4, secondOffset: 20, length: 28, wantErr: true},
		{name: "out of order", firstSize: 4, secondOffset: 16, length: 28, wantErr: true},
		{name: "sparse", firstSize: 4, secondOffset: 28, length: 32, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.length)
			binary.LittleEndian.PutUint32(data[4:8], 20)
			binary.LittleEndian.PutUint32(data[8:12], tt.firstSize)
			binary.LittleEndian.PutUint32(data[12:16], tt.secondOffset)
			binary.LittleEndian.PutUint32(data[16:20], 4)
			_, err := offsetSizeRefs(data, 4, 2)
			if (err != nil) != tt.wantErr {
				t.Fatalf("offsetSizeRefs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCountedPayloadLengthValidation(t *testing.T) {
	signalState := make([]byte, 28)
	rsrpSNR := make([]byte, 24)
	signalState = appendRefValue(signalState, 20, rsrpSNR)

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "device service CID list has trailing data",
			run: func() error {
				data := make([]byte, 36)
				binary.LittleEndian.PutUint32(data[24:28], 1)
				binary.LittleEndian.PutUint32(data[28:32], CIDConnect)
				return new(DeviceService).UnmarshalBinary(data)
			},
		},
		{
			name: "subscription CID list has trailing data",
			run: func() error {
				data := make([]byte, 28)
				binary.LittleEndian.PutUint32(data[16:20], 1)
				binary.LittleEndian.PutUint32(data[20:24], CIDConnect)
				return new(DeviceServiceSubscribeEntry).UnmarshalBinary(data)
			},
		},
		{
			name: "RSRP SNR array has trailing data",
			run: func() error {
				return new(SignalStateInfo).UnmarshalBinary(signalState)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestMessageInformationLengthValidation(t *testing.T) {
	commandDone := append(mbimCommandDone(1, ServiceBasicConnect, CIDConnect, []byte{1, 2, 3, 4}), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(commandDone[4:8], uint32(len(commandDone)))
	indication := append(mbimIndication(ServiceBasicConnect, CIDConnect, []byte{1, 2, 3, 4}), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(indication[4:8], uint32(len(indication)))

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "command response leaves trailing data",
			run: func() error {
				return new(CommandResponse).UnmarshalBinary(commandDone)
			},
		},
		{
			name: "indication leaves trailing data",
			run: func() error {
				return new(Indication).UnmarshalBinary(indication)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestUICCAndSTKDataBufferReferenceValidation(t *testing.T) {
	applicationList := applicationListPayloadForValidation()
	applicationOffset := binary.LittleEndian.Uint32(applicationList[16:20])
	terminalCapabilities := terminalCapabilityData([][]byte{{0x01}})

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "slot mapping points into reference table",
			run: func() error {
				data := mutateBytes(slotMappingsPayload(1), func(data []byte) {
					binary.LittleEndian.PutUint32(data[4:8], 4)
				})
				return new(DeviceSlotMappingsResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "application points into reference table",
			run: func() error {
				data := mutateBytes(applicationList, func(data []byte) {
					binary.LittleEndian.PutUint32(data[16:20], 16)
				})
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "application ID points into fixed fields",
			run: func() error {
				data := mutateBytes(applicationList, func(data []byte) {
					binary.LittleEndian.PutUint32(data[applicationOffset+4:applicationOffset+8], 4)
				})
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "application name overlaps application ID",
			run: func() error {
				data := mutateBytes(applicationList, func(data []byte) {
					aidOffset := binary.LittleEndian.Uint32(data[applicationOffset+4 : applicationOffset+8])
					binary.LittleEndian.PutUint32(data[applicationOffset+12:applicationOffset+16], aidOffset)
				})
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "PIN references point into fixed fields",
			run: func() error {
				data := mutateBytes(applicationList, func(data []byte) {
					binary.LittleEndian.PutUint32(data[applicationOffset+24:applicationOffset+28], 24)
				})
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "read binary points into fixed fields",
			run: func() error {
				data := offsetSizePayloadForValidation(20, 12, []byte{0x01})
				binary.LittleEndian.PutUint32(data[12:16], 12)
				return new(ReadBinaryResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "read record points into fixed fields",
			run: func() error {
				data := offsetSizePayloadForValidation(20, 12, []byte{0x01})
				binary.LittleEndian.PutUint32(data[12:16], 12)
				return new(ReadRecordResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "ATR points into fixed fields",
			run: func() error {
				data := sizeOffsetPayloadForValidation(8, 0, []byte{0x3b})
				binary.LittleEndian.PutUint32(data[4:8], 4)
				return new(UiccATRResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "open channel points into fixed fields",
			run: func() error {
				data := sizeOffsetPayloadForValidation(16, 8, []byte{0x90, 0x00})
				binary.LittleEndian.PutUint32(data[12:16], 8)
				return new(OpenChannelResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "APDU points into fixed fields",
			run: func() error {
				data := sizeOffsetPayloadForValidation(12, 4, []byte{0x90, 0x00})
				binary.LittleEndian.PutUint32(data[8:12], 4)
				return new(APDUResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "terminal capability points into reference table",
			run: func() error {
				data := mutateBytes(terminalCapabilities, func(data []byte) {
					binary.LittleEndian.PutUint32(data[4:8], 4)
				})
				return new(UiccTerminalCapabilityResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "STK terminal response points into fixed fields",
			run: func() error {
				data := offsetSizePayloadForValidation(12, 0, []byte{0x90, 0x00})
				binary.LittleEndian.PutUint32(data[:4], 8)
				return new(STKTerminalResponseInfo).UnmarshalBinary(data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestUICCResponseSemanticValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "application list version",
			run: func() error {
				data := applicationListPayloadForValidation()
				binary.LittleEndian.PutUint32(data[:4], 2)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "active application index",
			run: func() error {
				data := applicationListPayloadForValidation()
				binary.LittleEndian.PutUint32(data[8:12], 1)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "application list size",
			run: func() error {
				data := applicationListPayloadForValidation()
				binary.LittleEndian.PutUint32(data[12:16], binary.LittleEndian.Uint32(data[12:16])+1)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "application ID too long",
			run: func() error {
				data := applicationListPayloadWithValues(
					make([]byte, uiccApplicationIDMaximumSize+1),
					[]byte("app"),
					[]byte{0x01},
					1,
				)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "application type",
			run: func() error {
				data := applicationListPayloadForValidation()
				applicationOffset := binary.LittleEndian.Uint32(data[16:20])
				binary.LittleEndian.PutUint32(data[applicationOffset:applicationOffset+4], uint32(UiccApplicationTypeISIM)+1)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "MF application ID",
			run: func() error {
				data := applicationListPayloadForValidation()
				applicationOffset := binary.LittleEndian.Uint32(data[16:20])
				binary.LittleEndian.PutUint32(data[applicationOffset:applicationOffset+4], uint32(UiccApplicationTypeMF))
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "application name too long",
			run: func() error {
				data := applicationListPayloadWithValues(
					[]byte{0xa0},
					make([]byte, uiccApplicationNameMaximumSize+1),
					[]byte{0x01},
					1,
				)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "application name invalid UTF-8",
			run: func() error {
				data := applicationListPayloadWithValues([]byte{0xa0}, []byte{0xff}, []byte{0x01}, 1)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "PIN key reference count too high",
			run: func() error {
				data := applicationListPayloadWithValues(
					[]byte{0xa0},
					[]byte("app"),
					make([]byte, uiccPinKeyReferenceMaximumCount+1),
					uiccPinKeyReferenceMaximumCount+1,
				)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "PIN key reference count exceeds data",
			run: func() error {
				data := applicationListPayloadWithValues([]byte{0xa0}, []byte("app"), []byte{0x01}, 2)
				return new(ApplicationListResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "ATR too long",
			run: func() error {
				data := sizeOffsetPayloadForValidation(8, 0, make([]byte, uiccATRMaximumSize+1))
				return new(UiccATRResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "open channel response too long",
			run: func() error {
				data := sizeOffsetPayloadForValidation(16, 8, make([]byte, uiccOpenChannelResponseMaximumSize+1))
				return new(OpenChannelResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "open channel status exceeds two bytes",
			run: func() error {
				data := sizeOffsetPayloadForValidation(16, 8, nil)
				binary.LittleEndian.PutUint32(data[:4], 0x10000)
				return new(OpenChannelResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "open channel number",
			run: func() error {
				data := sizeOffsetPayloadForValidation(16, 8, nil)
				binary.LittleEndian.PutUint32(data[4:8], uiccLogicalChannelMaximum+1)
				return new(OpenChannelResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "close channel status exceeds two bytes",
			run: func() error {
				return new(CloseChannelResponse).UnmarshalBinary(binary.LittleEndian.AppendUint32(nil, 0x10000))
			},
		},
		{
			name: "APDU status exceeds two bytes",
			run: func() error {
				data := sizeOffsetPayloadForValidation(12, 4, nil)
				binary.LittleEndian.PutUint32(data[:4], 0x10000)
				return new(APDUResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "file status version",
			run: func() error {
				data := uiccFileStatusPayloadForValidation()
				binary.LittleEndian.PutUint32(data[:4], 2)
				return new(FileStatusResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "file accessibility",
			run: func() error {
				data := uiccFileStatusPayloadForValidation()
				binary.LittleEndian.PutUint32(data[12:16], uint32(UiccFileAccessibilityShareable)+1)
				return new(FileStatusResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "file type",
			run: func() error {
				data := uiccFileStatusPayloadForValidation()
				binary.LittleEndian.PutUint32(data[16:20], uint32(UiccFileTypeDFOrADF)+1)
				return new(FileStatusResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "file structure",
			run: func() error {
				data := uiccFileStatusPayloadForValidation()
				binary.LittleEndian.PutUint32(data[20:24], uint32(UiccFileStructureBERTLV)+1)
				return new(FileStatusResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "transparent file item count",
			run: func() error {
				data := uiccFileStatusPayloadForValidation()
				binary.LittleEndian.PutUint32(data[20:24], uint32(UiccFileStructureTransparent))
				binary.LittleEndian.PutUint32(data[24:28], 2)
				return new(FileStatusResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "BER-TLV file item count",
			run: func() error {
				data := uiccFileStatusPayloadForValidation()
				binary.LittleEndian.PutUint32(data[20:24], uint32(UiccFileStructureBERTLV))
				binary.LittleEndian.PutUint32(data[24:28], 0)
				return new(FileStatusResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "file access condition",
			run: func() error {
				data := uiccFileStatusPayloadForValidation()
				binary.LittleEndian.PutUint32(data[32:36], uint32(PinTypeADM)+1)
				return new(FileStatusResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "read binary version",
			run: func() error {
				data := uiccFileResponsePayloadForValidation(nil)
				binary.LittleEndian.PutUint32(data[:4], 2)
				return new(ReadBinaryResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "read binary response too long",
			run: func() error {
				return new(ReadBinaryResponse).UnmarshalBinary(
					uiccFileResponsePayloadForValidation(make([]byte, uiccFileResponseMaximumSize+1)),
				)
			},
		},
		{
			name: "read record version",
			run: func() error {
				data := uiccFileResponsePayloadForValidation(nil)
				binary.LittleEndian.PutUint32(data[:4], 2)
				return new(ReadRecordResponse).UnmarshalBinary(data)
			},
		},
		{
			name: "read record response too long",
			run: func() error {
				return new(ReadRecordResponse).UnmarshalBinary(
					uiccFileResponsePayloadForValidation(make([]byte, uiccFileResponseMaximumSize+1)),
				)
			},
		},
		{
			name: "reset status",
			run: func() error {
				data := binary.LittleEndian.AppendUint32(nil, 2)
				return new(UiccResetResponse).UnmarshalBinary(data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestUICCApplicationLabel(t *testing.T) {
	tests := []struct {
		name  string
		label []byte
		want  string
	}{
		{name: "without terminator", label: []byte("app"), want: "app"},
		{name: "with terminator", label: []byte{'a', 'p', 'p', 0}, want: "app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := applicationListPayloadWithValues([]byte{0xa0}, tt.label, []byte{0x01}, 1)
			var response ApplicationListResponse
			if err := response.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got := response.Applications[0].Label; got != tt.want {
				t.Fatalf("Label = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIPConfigurationDataBufferReferenceValidation(t *testing.T) {
	tests := []struct {
		name        string
		countOffset int
		valueOffset int
		wantErr     bool
	}{
		{name: "empty", countOffset: -1, valueOffset: -1},
		{name: "IPv4 addresses point into fixed fields", countOffset: 12, valueOffset: 16, wantErr: true},
		{name: "IPv6 addresses point into fixed fields", countOffset: 20, valueOffset: 24, wantErr: true},
		{name: "IPv4 gateway points into fixed fields", countOffset: -1, valueOffset: 28, wantErr: true},
		{name: "IPv6 gateway points into fixed fields", countOffset: -1, valueOffset: 32, wantErr: true},
		{name: "IPv4 DNS points into fixed fields", countOffset: 36, valueOffset: 40, wantErr: true},
		{name: "IPv6 DNS points into fixed fields", countOffset: 44, valueOffset: 48, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, ipConfigurationInfoFixedSize)
			if tt.countOffset >= 0 {
				binary.LittleEndian.PutUint32(data[tt.countOffset:tt.countOffset+4], 1)
			}
			if tt.valueOffset >= 0 {
				binary.LittleEndian.PutUint32(data[tt.valueOffset:tt.valueOffset+4], 4)
			}
			var got IPConfigurationInfo
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIPConfigurationDataBufferOrdering(t *testing.T) {
	tests := []struct {
		name              string
		ipv6AddressOffset uint32
		wantErr           bool
	}{
		{name: "ordered", ipv6AddressOffset: 68},
		{name: "overlap", ipv6AddressOffset: 60, wantErr: true},
		{name: "out of order", ipv6AddressOffset: 56, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 88)
			binary.LittleEndian.PutUint32(data[4:8], uint32(ipConfigurationBasicMask))
			binary.LittleEndian.PutUint32(data[8:12], uint32(ipConfigurationBasicMask))
			binary.LittleEndian.PutUint32(data[12:16], 1)
			binary.LittleEndian.PutUint32(data[16:20], 60)
			binary.LittleEndian.PutUint32(data[20:24], 1)
			binary.LittleEndian.PutUint32(data[24:28], tt.ipv6AddressOffset)

			var got IPConfigurationInfo
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConnectInfoAccessStringProtocolLimit(t *testing.T) {
	tests := []struct {
		name         string
		accessString string
		wantErr      bool
	}{
		{name: "maximum length", accessString: strings.Repeat("a", 100)},
		{name: "too long", accessString: strings.Repeat("a", 101), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 40)
			data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(tt.accessString))...)
			got := ConnectInfo{MBIMExVersion: mbimExVersion30}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOpenIMSPDNValidatesAccessString(t *testing.T) {
	tests := []struct {
		name string
		apn  string
	}{
		{name: "ASCII", apn: strings.Repeat("a", 101)},
		{name: "UTF-16 surrogate pairs", apn: strings.Repeat("😀", 51)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := new(Client).OpenIMSPDN(ctx, IMSPDNConfig{APN: tt.apn})
			if !errors.Is(err, StatusInvalidAccessString) {
				t.Fatalf("OpenIMSPDN() error = %v, want StatusInvalidAccessString", err)
			}
		})
	}
}

func TestAuthSIMResponseCountValidation(t *testing.T) {
	tests := []struct {
		name    string
		count   uint32
		wantErr bool
	}{
		{name: "two responses", count: 2},
		{name: "three responses", count: 3},
		{name: "zero responses", wantErr: true},
		{name: "four responses", count: 4, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 40)
			binary.LittleEndian.PutUint32(data[36:40], tt.count)
			var got AuthSIMResponse
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.N != tt.count {
				t.Fatalf("N = %d, want %d", got.N, tt.count)
			}
		})
	}
}

func TestAuthSIMResponseRejectsTrailingData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "one trailing byte", data: make([]byte, 41)},
		{name: "aligned trailing data", data: make([]byte, 44)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary.LittleEndian.PutUint32(tt.data[36:40], 2)
			if err := new(AuthSIMResponse).UnmarshalBinary(tt.data); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestFixedSizeResponseLengthValidation(t *testing.T) {
	tests := []struct {
		name string
		size int
		run  func([]byte) error
	}{
		{name: "version", size: 4, run: func(data []byte) error { return new(VersionInfo).UnmarshalBinary(data) }},
		{name: "radio state", size: 8, run: func(data []byte) error { return new(RadioStateInfo).UnmarshalBinary(data) }},
		{name: "PIN info", size: 12, run: func(data []byte) error { return new(PinInfo).UnmarshalBinary(data) }},
		{name: "PIN list", size: 160, run: func(data []byte) error { return new(PinListInfo).UnmarshalBinary(data) }},
		{name: "packet statistics", size: 48, run: func(data []byte) error { return new(PacketStatisticsInfo).UnmarshalBinary(data) }},
		{name: "network idle hint", size: 4, run: func(data []byte) error { return new(NetworkIdleHint).UnmarshalBinary(data) }},
		{name: "emergency mode", size: 4, run: func(data []byte) error { return new(EmergencyMode).UnmarshalBinary(data) }},
		{name: "SMS send", size: 4, run: func(data []byte) error { return new(SMSSendInfo).UnmarshalBinary(data) }},
		{name: "SMS store status", size: 8, run: func(data []byte) error { return new(SMSStoreStatusInfo).UnmarshalBinary(data) }},
		{name: "phonebook configuration", size: 20, run: func(data []byte) error { return new(PhonebookConfigurationInfo).UnmarshalBinary(data) }},
		{name: "AKA authentication", size: 66, run: func(data []byte) error { return new(AuthAKAResponse).UnmarshalBinary(data) }},
		{
			name: "UICC file status",
			size: 48,
			run: func(data []byte) error {
				if len(data) >= 4 {
					binary.LittleEndian.PutUint32(data[:4], 1)
				}
				return new(FileStatusResponse).UnmarshalBinary(data)
			},
		},
		{name: "UICC close channel", size: 4, run: func(data []byte) error { return new(CloseChannelResponse).UnmarshalBinary(data) }},
		{name: "UICC reset", size: 4, run: func(data []byte) error { return new(UiccResetResponse).UnmarshalBinary(data) }},
		{name: "STK PAC info", size: stkPACSupportLength, run: func(data []byte) error { return new(STKPACInfo).UnmarshalBinary(data) }},
		{name: "STK envelope info", size: stkEnvelopeSupportLength, run: func(data []byte) error { return new(STKEnvelopeInfo).UnmarshalBinary(data) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lengths := []struct {
				name    string
				length  int
				wantErr bool
			}{
				{name: "exact", length: tt.size},
				{name: "truncated", length: tt.size - 1, wantErr: true},
				{name: "trailing data", length: tt.size + 1, wantErr: true},
			}
			for _, length := range lengths {
				t.Run(length.name, func(t *testing.T) {
					err := tt.run(make([]byte, length.length))
					if (err != nil) != length.wantErr {
						t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, length.wantErr)
					}
				})
			}
		})
	}
}

func TestUICCClientInputLimits(t *testing.T) {
	tests := []struct {
		name    string
		run     func(context.Context) error
		wantErr error
	}{
		{
			name: "application ID too long",
			run: func(ctx context.Context) error {
				_, err := new(Client).OpenChannel(ctx, make([]byte, uiccApplicationIDMaximumSize+1))
				return err
			},
			wantErr: StatusParameterTooLong,
		},
		{
			name: "APDU basic channel",
			run: func(ctx context.Context) error {
				_, _, err := new(Client).TransmitAPDU(ctx, 0, nil)
				return err
			},
			wantErr: StatusInvalidParameters,
		},
		{
			name: "APDU channel too high",
			run: func(ctx context.Context) error {
				_, _, err := new(Client).TransmitAPDU(ctx, uiccLogicalChannelMaximum+1, nil)
				return err
			},
			wantErr: StatusInvalidParameters,
		},
		{
			name: "APDU command too long",
			run: func(ctx context.Context) error {
				_, _, err := new(Client).TransmitAPDU(ctx, 1, make([]byte, uiccAPDUCommandMaximumSize+1))
				return err
			},
			wantErr: StatusParameterTooLong,
		},
		{
			name: "close channel too high",
			run: func(ctx context.Context) error {
				return new(Client).CloseChannel(ctx, uiccLogicalChannelMaximum+1)
			},
			wantErr: StatusInvalidParameters,
		},
		{
			name: "reset action",
			run: func(ctx context.Context) error {
				_, err := new(Client).SetUiccReset(ctx, UiccPassThroughActionEnable+1)
				return err
			},
			wantErr: StatusInvalidParameters,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := tt.run(ctx); !errors.Is(err, tt.wantErr) {
				t.Fatalf("client call error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestProviderMarshalBinaryValidation(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		wantErr  bool
	}{
		{name: "valid", provider: Provider{ID: "310260", Name: strings.Repeat("n", 20)}},
		{name: "provider ID too long", provider: Provider{ID: "3102600"}, wantErr: true},
		{name: "provider name too long", provider: Provider{Name: strings.Repeat("n", 21)}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.provider.MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProviderUnmarshalBinaryValidation(t *testing.T) {
	valid := Provider{ID: "310260", Name: "Carrier"}.marshalBinary()
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{
			name: "provider ID points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], 8)
			}),
			wantErr: true,
		},
		{
			name:    "provider ID too long",
			data:    Provider{ID: "3102600", Name: "Carrier"}.marshalBinary(),
			wantErr: true,
		},
		{
			name: "provider name points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[12:16], 20)
			}),
			wantErr: true,
		},
		{
			name:    "provider name too long",
			data:    Provider{ID: "310260", Name: strings.Repeat("n", 21)}.marshalBinary(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Provider
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProviderSettersValidateFields(t *testing.T) {
	provider := Provider{ID: "3102600"}
	tests := []struct {
		name string
		run  func(*Client) error
	}{
		{
			name: "home provider",
			run: func(client *Client) error {
				_, err := client.SetHomeProvider(context.Background(), provider)
				return err
			},
		},
		{
			name: "preferred providers",
			run: func(client *Client) error {
				_, err := client.SetPreferredProviders(context.Background(), []Provider{provider})
				return err
			},
		},
		{
			name: "multicarrier providers",
			run: func(client *Client) error {
				_, err := client.SetMulticarrierProviders(context.Background(), []Provider{provider})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(new(Client)); err == nil {
				t.Fatal("setter error = nil, want non-nil")
			}
		})
	}
}

func TestIPPacketFiltersValidation(t *testing.T) {
	tests := []struct {
		name    string
		info    IPPacketFiltersInfo
		wantErr bool
	}{
		{
			name: "legacy duplicate IDs ignored",
			info: IPPacketFiltersInfo{Filters: []PacketFilter{{
				Filter: []byte{0x45, 0x00},
				Mask:   []byte{0xff, 0xff},
				ID:     7,
			}, {
				Filter: []byte{0x60, 0x00},
				Mask:   []byte{0xff, 0xff},
				ID:     7,
			}}},
		},
		{
			name: "MBIMEx 3 unique IDs",
			info: IPPacketFiltersInfo{
				MBIMExVersion: mbimExVersion30,
				Filters: []PacketFilter{{
					Filter: []byte{0x45, 0x00},
					Mask:   []byte{0xff, 0xff},
					ID:     7,
				}, {
					Filter: []byte{0x60, 0x00},
					Mask:   []byte{0xff, 0xff},
					ID:     8,
				}},
			},
		},
		{
			name: "MBIMEx 3 duplicate IDs",
			info: IPPacketFiltersInfo{
				MBIMExVersion: mbimExVersion30,
				Filters: []PacketFilter{{
					Filter: []byte{0x45, 0x00},
					Mask:   []byte{0xff, 0xff},
					ID:     7,
				}, {
					Filter: []byte{0x60, 0x00},
					Mask:   []byte{0xff, 0xff},
					ID:     7,
				}},
			},
			wantErr: true,
		},
		{
			name: "mask length mismatch",
			info: IPPacketFiltersInfo{Filters: []PacketFilter{{
				Filter: []byte{0x45, 0x00},
				Mask:   []byte{0xff},
			}}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.info.MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIPPacketFiltersUnmarshalBinaryValidation(t *testing.T) {
	valid := (IPPacketFiltersInfo{
		SessionID: 1,
		Filters: []PacketFilter{{
			Filter: []byte{0x45, 0x00},
			Mask:   []byte{0xff, 0xff},
		}},
	}).marshalBinary(0)
	recordOffset := binary.LittleEndian.Uint32(valid[8:12])
	filterOffset := binary.LittleEndian.Uint32(valid[recordOffset+4 : recordOffset+8])
	validV3 := (IPPacketFiltersInfo{
		Filters: []PacketFilter{{
			Filter: []byte{0x45},
			Mask:   []byte{0xff},
			ID:     7,
		}, {
			Filter: []byte{0x60},
			Mask:   []byte{0xff},
			ID:     8,
		}},
	}).marshalBinary(mbimExVersion30)
	duplicateIDsV3 := (IPPacketFiltersInfo{
		Filters: []PacketFilter{{
			Filter: []byte{0x45},
			Mask:   []byte{0xff},
			ID:     7,
		}, {
			Filter: []byte{0x60},
			Mask:   []byte{0xff},
			ID:     7,
		}},
	}).marshalBinary(mbimExVersion30)

	tests := []struct {
		name    string
		version uint16
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{name: "MBIMEx 3 valid", version: mbimExVersion30, data: validV3},
		{name: "MBIMEx 3 duplicate IDs", version: mbimExVersion30, data: duplicateIDsV3, wantErr: true},
		{
			name: "record points into reference table",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], 8)
			}),
			wantErr: true,
		},
		{
			name: "filter points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[recordOffset+4:recordOffset+8], 4)
			}),
			wantErr: true,
		},
		{
			name: "mask overlaps filter",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[recordOffset+8:recordOffset+12], filterOffset)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IPPacketFiltersInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetIPPacketFiltersRejectsDuplicateIDs(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		wantErr bool
	}{
		{name: "legacy", version: 0},
		{name: "MBIMEx 3", version: mbimExVersion30, wantErr: true},
	}

	filters := IPPacketFiltersInfo{Filters: []PacketFilter{{ID: 7}, {ID: 7}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{mbimExVersion: tt.version}
			_, err := client.SetIPPacketFilters(context.Background(), filters)
			if tt.wantErr {
				if err == nil {
					t.Fatal("SetIPPacketFilters() error = nil, want non-nil")
				}
				return
			}
			if err == nil {
				t.Fatal("SetIPPacketFilters() error = nil, want transport error")
			}
			if strings.Contains(err.Error(), "duplicates") {
				t.Fatalf("SetIPPacketFilters() error = %v, want transport error", err)
			}
		})
	}
}

func TestDeviceCapsInfoProtocolLimits(t *testing.T) {
	v1 := deviceCapsPayload(1)
	v1LongCustom := deviceCapsPayload(1)
	v1LongCustom = appendRefValue(v1LongCustom, 32, utf16Bytes(strings.Repeat("c", 12)))
	v2LongDeviceID := deviceCapsPayloadV2ForTest(1, "", strings.Repeat("d", 19), "", "")
	v3LongHardware := deviceCapsPayloadV3WithValuesForTest(1, nil, nil, "", "", "", strings.Repeat("h", 31))
	v3TooManySessions := deviceCapsPayloadV3WithValuesForTest(257, nil, nil, "", "", "", "")

	tests := []struct {
		name    string
		version uint16
		data    []byte
		wantErr bool
	}{
		{name: "MBIM 1 valid fixed fields", data: v1},
		{name: "MBIM 1 old 32-byte payload", data: v1[:32], wantErr: true},
		{
			name: "MBIM 1 string points into fixed fields",
			data: mutateBytes(v1, func(data []byte) {
				binary.LittleEndian.PutUint32(data[32:36], 32)
				binary.LittleEndian.PutUint32(data[36:40], 2)
			}),
			wantErr: true,
		},
		{name: "MBIM 1 custom data class too long", data: v1LongCustom, wantErr: true},
		{
			name: "MBIM 1 too many sessions",
			data: mutateBytes(v1, func(data []byte) {
				binary.LittleEndian.PutUint32(data[28:32], 257)
			}),
			wantErr: true,
		},
		{name: "MBIMEx 2 device ID too long", version: mbimExVersion20, data: v2LongDeviceID, wantErr: true},
		{name: "MBIMEx 3 hardware info too long", version: mbimExVersion30, data: v3LongHardware, wantErr: true},
		{name: "MBIMEx 3 too many sessions", version: mbimExVersion30, data: v3TooManySessions, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeviceCapsInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubscriberReadyStatusProtocolLimits(t *testing.T) {
	v1 := subscriberReadyPayload(t, SubscriberReadyStateInitialized, "00101", "8901", ReadyInfoNone, "+1555")
	ex3NoProfile := subscriberReadyPayloadEx3(t, SubscriberReadyStateNoESIMProfile, SubscriberReadyStatusFlagESIM, "", "8901", ReadyInfoNone)
	ex4 := subscriberReadyPayloadEx4(t, SubscriberReadyStateInitialized, SubscriberReadyStatusFlagSIMSlotActive, 0, "00101", "8901", ReadyInfoNone, "+1555")
	tests := []struct {
		name    string
		version uint16
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: v1},
		{name: "MBIM 1 device locked", data: subscriberReadyPayload(t, SubscriberReadyStateDeviceLocked, "00101", "8901", ReadyInfoNone)},
		{name: "MBIM 1 no eSIM profile", data: subscriberReadyPayload(t, SubscriberReadyStateNoESIMProfile, "", "8901", ReadyInfoNone), wantErr: true},
		{name: "MBIMEx 2 no eSIM profile", version: mbimExVersion20, data: subscriberReadyPayload(t, SubscriberReadyStateNoESIMProfile, "", "8901", ReadyInfoNone), wantErr: true},
		{name: "MBIMEx 3 no eSIM profile", version: mbimExVersion30, data: ex3NoProfile},
		{name: "MBIMEx 3 reserved ready state", version: mbimExVersion30, data: subscriberReadyPayloadEx3(t, SubscriberReadyStateNoESIMProfile+1, 0, "", "", ReadyInfoNone), wantErr: true},
		{name: "MBIMEx 3 reserved flags", version: mbimExVersion30, data: subscriberReadyPayloadEx3(t, SubscriberReadyStateInitialized, SubscriberReadyStatusFlagSIMSlotActive, "00101", "8901", ReadyInfoNone), wantErr: true},
		{name: "MBIMEx 4 reserved flags", version: mbimExVersion40, data: subscriberReadyPayloadEx4(t, SubscriberReadyStateInitialized, 1<<4, 0, "00101", "8901", ReadyInfoNone), wantErr: true},
		{name: "SIM removable without known", version: mbimExVersion30, data: subscriberReadyPayloadEx3(t, SubscriberReadyStateInitialized, SubscriberReadyStatusFlagSIMRemovable, "00101", "8901", ReadyInfoNone), wantErr: true},
		{name: "eSIM flag in locked state", version: mbimExVersion30, data: subscriberReadyPayloadEx3(t, SubscriberReadyStateDeviceLocked, SubscriberReadyStatusFlagESIM, "00101", "8901", ReadyInfoNone), wantErr: true},
		{
			name:    "SIM removable flag in bad SIM state",
			version: mbimExVersion30,
			data: subscriberReadyPayloadEx3(
				t,
				SubscriberReadyStateBadSIM,
				SubscriberReadyStatusFlagSIMRemovabilityKnown|SubscriberReadyStatusFlagSIMRemovable,
				"00101",
				"8901",
				ReadyInfoNone,
			),
			wantErr: true,
		},
		{name: "reserved ready info", data: subscriberReadyPayload(t, SubscriberReadyStateInitialized, "00101", "8901", ReadyInfo(1<<1)), wantErr: true},
		{name: "telephone number before initialized", data: subscriberReadyPayload(t, SubscriberReadyStateDeviceLocked, "00101", "8901", ReadyInfoNone, "+1555"), wantErr: true},
		{name: "MBIMEx 4 slot 1", version: mbimExVersion40, data: subscriberReadyPayloadEx4(t, SubscriberReadyStateInitialized, SubscriberReadyStatusFlagSIMSlotActive, 1, "00101", "8901", ReadyInfoNone)},
		{name: "MBIMEx 4 reserved slot", version: mbimExVersion40, data: subscriberReadyPayloadEx4(t, SubscriberReadyStateInitialized, 0, 2, "00101", "8901", ReadyInfoNone), wantErr: true},
		{name: "MBIMEx 4 active-slot sentinel in response", version: mbimExVersion40, data: subscriberReadyPayloadEx4(t, SubscriberReadyStateInitialized, SubscriberReadyStatusFlagSIMSlotActive, activeSubscriberSlot, "00101", "8901", ReadyInfoNone), wantErr: true},
		{
			name: "subscriber ID points into telephone table",
			data: mutateBytes(v1, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 28)
			}),
			wantErr: true,
		},
		{name: "subscriber ID too long", data: subscriberReadyPayload(t, SubscriberReadyStateInitialized, strings.Repeat("1", 16), "8901", ReadyInfoNone), wantErr: true},
		{name: "SIM ICCID too long", data: subscriberReadyPayload(t, SubscriberReadyStateInitialized, "00101", strings.Repeat("8", 21), ReadyInfoNone), wantErr: true},
		{name: "telephone number too long", data: subscriberReadyPayload(t, SubscriberReadyStateInitialized, "00101", "8901", ReadyInfoNone, strings.Repeat("1", 23)), wantErr: true},
		{
			name:    "MBIMEx 4 subscriber ID points into telephone table",
			version: mbimExVersion40,
			data: mutateBytes(ex4, func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], 36)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubscriberReadyStatusResponse{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistrationStateProtocolLimits(t *testing.T) {
	v1 := registrationStatePayloadForValidation(0, "310260", "Carrier", "Roaming")
	v2 := registrationStatePayloadForValidation(mbimExVersion20, "310260", "Carrier", "Roaming")
	tests := []struct {
		name    string
		version uint16
		data    []byte
		wantErr bool
	}{
		{name: "MBIM 1 valid", data: v1},
		{name: "MBIMEx 2 valid", version: mbimExVersion20, data: v2},
		{
			name: "provider ID points into fixed fields",
			data: mutateBytes(v1, func(data []byte) {
				binary.LittleEndian.PutUint32(data[20:24], 20)
			}),
			wantErr: true,
		},
		{name: "provider ID too long", data: registrationStatePayloadForValidation(0, "3102600", "", ""), wantErr: true},
		{name: "provider name too long", data: registrationStatePayloadForValidation(0, "", strings.Repeat("n", 21), ""), wantErr: true},
		{name: "roaming text too long", data: registrationStatePayloadForValidation(0, "", "", strings.Repeat("r", 64)), wantErr: true},
		{
			name:    "MBIMEx 2 string points before extended data buffer",
			version: mbimExVersion20,
			data: mutateBytes(v2, func(data []byte) {
				binary.LittleEndian.PutUint32(data[20:24], 48)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegistrationStateInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSignalStateExtendedReferenceValidation(t *testing.T) {
	valid := make([]byte, 28)
	valid = appendRefValue(valid, 20, make([]byte, 4))
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{
			name: "RSRP SNR points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[20:24], 20)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SignalStateInfo{MBIMExVersion: mbimExVersion20}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSignalStateInfoVersionValidation(t *testing.T) {
	extended := make([]byte, 28)
	tests := []struct {
		name    string
		version uint16
		data    []byte
		wantErr bool
	}{
		{name: "MBIM 1", version: mbimExVersion10, data: make([]byte, 20)},
		{name: "MBIM 1 truncated", version: mbimExVersion10, data: make([]byte, 19), wantErr: true},
		{name: "MBIM 1 trailing data", version: mbimExVersion10, data: extended, wantErr: true},
		{name: "MBIMEx 2 empty RSRP SNR", version: mbimExVersion20, data: extended},
		{name: "MBIMEx 2 truncated", version: mbimExVersion20, data: make([]byte, 27), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SignalStateInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.MBIMExVersion != tt.version {
				t.Fatalf("version = %#x, want %#x", got.MBIMExVersion, tt.version)
			}
		})
	}
}

func TestSignalStateRequestsCarryVersion(t *testing.T) {
	tests := []struct {
		name    string
		request *Request
	}{
		{
			name: "query",
			request: (&SignalStateRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
			}).Request(),
		},
		{
			name: "set",
			request: (&SignalStateSetRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
			}).Request(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := tt.request.Response.(*SignalStateInfo)
			if response.MBIMExVersion != mbimExVersion40 {
				t.Fatalf("response version = %#x, want %#x", response.MBIMExVersion, mbimExVersion40)
			}
		})
	}
}

func TestSMSConfigurationInfoProtocolLimits(t *testing.T) {
	valid := smsConfigurationPayloadForValidation("+123")
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{
			name: "address points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[16:20], 16)
			}),
			wantErr: true,
		},
		{name: "address too long", data: smsConfigurationPayloadForValidation(strings.Repeat("1", 21)), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SMSConfigurationInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMSPDURecordProtocolLimits(t *testing.T) {
	valid := smsPDURecordForValidation([]byte{0x01, 0x02})
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{
			name: "PDU points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], 8)
			}),
			wantErr: true,
		},
		{name: "PDU too long", data: smsPDURecordForValidation(make([]byte, 256)), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SMSPDURecord
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMSCDMARecordProtocolLimits(t *testing.T) {
	valid := smsCDMARecordForValidation("+123", "24/01/01,00:00:00+00", []byte{0x01})
	addressOffset := binary.LittleEndian.Uint32(valid[8:12])
	tests := []struct {
		name          string
		data          []byte
		wantTimestamp string
		wantErr       bool
	}{
		{name: "valid", data: valid, wantTimestamp: "24/01/01,00:00:00+00"},
		{
			name:          "valid NUL-terminated",
			data:          smsCDMARecordForValidation("", "24/01/01,00:00:00+00\x00", nil),
			wantTimestamp: "24/01/01,00:00:00+00",
		},
		{name: "empty timestamp", data: smsCDMARecordForValidation("", "", nil)},
		{
			name: "address points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], 8)
			}),
			wantErr: true,
		},
		{name: "address too long", data: smsCDMARecordForValidation(strings.Repeat("1", 21), "", nil), wantErr: true},
		{name: "timestamp too long", data: smsCDMARecordForValidation("", strings.Repeat("0", 22), nil), wantErr: true},
		{name: "timestamp too short", data: smsCDMARecordForValidation("", "24/01/01", nil), wantErr: true},
		{name: "timestamp bad terminator", data: smsCDMARecordForValidation("", "24/01/01,00:00:00+00X", nil), wantErr: true},
		{name: "timestamp bad separator", data: smsCDMARecordForValidation("", "24-01/01,00:00:00+00", nil), wantErr: true},
		{name: "timestamp non-decimal year", data: smsCDMARecordForValidation("", "2A/01/01,00:00:00+00", nil), wantErr: true},
		{name: "timestamp month zero", data: smsCDMARecordForValidation("", "24/00/01,00:00:00+00", nil), wantErr: true},
		{name: "timestamp month 13", data: smsCDMARecordForValidation("", "24/13/01,00:00:00+00", nil), wantErr: true},
		{name: "timestamp day zero", data: smsCDMARecordForValidation("", "24/01/00,00:00:00+00", nil), wantErr: true},
		{name: "timestamp day 32", data: smsCDMARecordForValidation("", "24/01/32,00:00:00+00", nil), wantErr: true},
		{name: "timestamp hour 24", data: smsCDMARecordForValidation("", "24/01/01,24:00:00+00", nil), wantErr: true},
		{name: "timestamp minute 60", data: smsCDMARecordForValidation("", "24/01/01,00:60:00+00", nil), wantErr: true},
		{name: "timestamp second 60", data: smsCDMARecordForValidation("", "24/01/01,00:00:60+00", nil), wantErr: true},
		{name: "timestamp bad sign", data: smsCDMARecordForValidation("", "24/01/01,00:00:00*00", nil), wantErr: true},
		{name: "timestamp positive zone 14", data: smsCDMARecordForValidation("", "24/01/01,00:00:00+14", nil), wantErr: true},
		{name: "timestamp negative zone 13", data: smsCDMARecordForValidation("", "24/01/01,00:00:00-13", nil), wantErr: true},
		{name: "encoded message too long", data: smsCDMARecordForValidation("", "", make([]byte, 161)), wantErr: true},
		{
			name: "timestamp overlaps address",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[16:20], addressOffset)
			}),
			wantErr: true,
		},
		{name: "timestamp is not ASCII", data: smsCDMARecordForValidation("", string([]byte{0xff}), nil), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SMSCDMARecord
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.Timestamp != tt.wantTimestamp {
				t.Fatalf("Timestamp = %q, want %q", got.Timestamp, tt.wantTimestamp)
			}
		})
	}
}

func TestUSSDInfoProtocolLimits(t *testing.T) {
	valid := ussdPayloadForValidation([]byte{0x01})
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{
			name: "payload points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[12:16], 12)
			}),
			wantErr: true,
		},
		{name: "payload too long", data: ussdPayloadForValidation(make([]byte, 161)), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got USSDInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPhonebookEntryReferenceValidation(t *testing.T) {
	valid := phonebookEntryPayloadForValidation("+123", "Alice")
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{
			name: "number points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 4)
			}),
			wantErr: true,
		},
		{
			name: "name points into fixed fields",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[12:16], 12)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PhonebookEntry
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContextStringDecodersEnforceMaximumSizes(t *testing.T) {
	lteConfiguration := LTEAttachConfiguration{}.marshalBinary(utf16Bytes(strings.Repeat("a", 101)), nil, nil)
	lteInfo := lteAttachInfoPayloadForValidation("", strings.Repeat("u", 256), "")
	provisioned := make([]byte, 52)
	provisioned = appendRefValue(provisioned, 36, utf16Bytes(strings.Repeat("p", 256)))
	provisionedV2 := marshalProvisionedContextV2(1, ProvisionedContextV2{AccessString: strings.Repeat("a", 101)}, mbimExVersion30)

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "LTE attach configuration access string",
			run: func() error {
				var got LTEAttachConfiguration
				return got.UnmarshalBinary(lteConfiguration)
			},
		},
		{
			name: "LTE attach info user name",
			run: func() error {
				got := LTEAttachInfo{MBIMExVersion: mbimExVersion20}
				return got.UnmarshalBinary(lteInfo)
			},
		},
		{
			name: "provisioned context password",
			run: func() error {
				var got ProvisionedContext
				return got.unmarshalBinary(provisioned)
			},
		},
		{
			name: "provisioned context V2 access string",
			run: func() error {
				got := ProvisionedContextV2{MBIMExVersion: mbimExVersion30}
				return got.UnmarshalBinary(provisionedV2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("decoder error = nil, want non-nil")
			}
		})
	}
}

func TestDeviceServicesResponseProtocolLimits(t *testing.T) {
	valid := deviceServicesPayload(DeviceService{ServiceID: ServiceBasicConnect, CIDs: []uint32{CIDConnect}})
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{
			name: "too many DSS sessions",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 257)
			}),
			wantErr: true,
		},
		{
			name: "service points into reference table",
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], 8)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DeviceServicesResponse
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetDSSLinkStateRejectsReservedSessionBits(t *testing.T) {
	tests := []struct {
		name      string
		sessionID SessionID
		wantErr   bool
	}{
		{name: "reserved high bit", sessionID: 256, wantErr: true},
		{name: "all reserved high bits", sessionID: ^SessionID(0), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := new(Client).SetDSSLinkState(context.Background(), ServiceBasicConnect, tt.sessionID, DSSLinkStateActivate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetDSSLinkState() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWakePayloadProtocolLimits(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "connect payload has wrong length",
			run: func() error {
				_, err := (WakeCommand{ServiceID: ServiceBasicConnect, CID: CIDConnect}).MarshalBinary()
				return err
			},
		},
		{
			name: "connect payload has reserved value",
			run: func() error {
				_, err := (WakeCommand{
					ServiceID: ServiceBasicConnect,
					CID:       CIDConnect,
					Payload:   binary.LittleEndian.AppendUint32(nil, 2),
				}).MarshalBinary()
				return err
			},
		},
		{
			name: "saved packet exceeds original packet",
			run: func() error {
				_, err := (WakePacket{OriginalPacketSize: 1, SavedPacket: []byte{1, 2}}).MarshalBinary()
				return err
			},
		},
		{
			name: "decode invalid connect payload",
			run: func() error {
				data := make([]byte, 28)
				copy(data[:16], ServiceBasicConnect[:])
				binary.LittleEndian.PutUint32(data[16:20], CIDConnect)
				data = appendRefValue(data, 20, binary.LittleEndian.AppendUint32(nil, 2))
				var got WakeCommand
				return got.UnmarshalBinary(data)
			},
		},
		{
			name: "decode saved packet larger than original",
			run: func() error {
				data := make([]byte, 16)
				binary.LittleEndian.PutUint32(data[4:8], 1)
				data = appendRefValue(data, 8, []byte{1, 2})
				var got WakePacket
				return got.UnmarshalBinary(data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("validation error = nil, want non-nil")
			}
		})
	}
}

func TestWakeDataBuffersIncludeFinalPadding(t *testing.T) {
	tests := []struct {
		name       string
		marshal    func() ([]byte, error)
		unmarshal  func([]byte) ([]byte, error)
		wantLength int
		want       []byte
	}{
		{
			name: "wake command",
			marshal: func() ([]byte, error) {
				return (WakeCommand{ServiceID: ServiceSMS, CID: CIDSMSRead, Payload: []byte{1, 2, 3}}).MarshalBinary()
			},
			unmarshal: func(data []byte) ([]byte, error) {
				var value WakeCommand
				if err := value.UnmarshalBinary(data); err != nil {
					return nil, err
				}
				return value.Payload, nil
			},
			wantLength: 32,
			want:       []byte{1, 2, 3},
		},
		{
			name: "wake packet",
			marshal: func() ([]byte, error) {
				return (WakePacket{OriginalPacketSize: 3, SavedPacket: []byte{1, 2, 3}}).MarshalBinary()
			},
			unmarshal: func(data []byte) ([]byte, error) {
				var value WakePacket
				if err := value.UnmarshalBinary(data); err != nil {
					return nil, err
				}
				return value.SavedPacket, nil
			},
			wantLength: 20,
			want:       []byte{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.marshal()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if len(data) != tt.wantLength {
				t.Fatalf("MarshalBinary() length = %d, want %d", len(data), tt.wantLength)
			}
			data[len(data)-1] = 0xff
			got, err := tt.unmarshal(data)
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("payload = %x, want %x", got, tt.want)
			}
		})
	}
}

func registrationStatePayloadForValidation(version uint16, providerID, providerName, roamingText string) []byte {
	fixedLength := 48
	if version >= mbimExVersion20 {
		fixedLength = 52
	}
	data := make([]byte, fixedLength)
	data = appendRefValue(data, 20, utf16Bytes(providerID))
	data = appendRefValue(data, 28, utf16Bytes(providerName))
	return appendRefValue(data, 36, utf16Bytes(roamingText))
}

func smsConfigurationPayloadForValidation(address string) []byte {
	data := make([]byte, 24)
	binary.LittleEndian.PutUint32(data[0:4], uint32(SMSStorageStateInitialized))
	binary.LittleEndian.PutUint32(data[4:8], uint32(SMSFormatPDU))
	return appendRefValue(data, 16, utf16Bytes(address))
}

func smsPDURecordForValidation(pdu []byte) []byte {
	data := make([]byte, 16)
	return appendRefValue(data, 8, pdu)
}

func smsCDMARecordForValidation(address, timestamp string, message []byte) []byte {
	data := make([]byte, 44)
	data = appendRefValue(data, 8, utf16Bytes(address))
	data = appendRefValue(data, 16, []byte(timestamp))
	return appendRefValue(data, 32, message)
}

func ussdPayloadForValidation(payload []byte) []byte {
	data := make([]byte, 20)
	return appendRefValue(data, 12, payload)
}

func phonebookEntryPayloadForValidation(number, name string) []byte {
	data := make([]byte, 20)
	data = appendRefValue(data, 4, utf16Bytes(number))
	return appendRefValue(data, 12, utf16Bytes(name))
}

func applicationListPayloadForValidation() []byte {
	return applicationListPayloadWithValues([]byte{0xa0}, []byte("app"), []byte{0x01}, 1)
}

func uiccFileStatusPayloadForValidation() []byte {
	data := make([]byte, 48)
	binary.LittleEndian.PutUint32(data[:4], 1)
	return data
}

func uiccFileResponsePayloadForValidation(value []byte) []byte {
	data := offsetSizePayloadForValidation(20, 12, value)
	binary.LittleEndian.PutUint32(data[:4], 1)
	return data
}

func applicationListPayloadWithValues(aid, label, pinKeyReferences []byte, pinKeyReferenceCount uint32) []byte {
	application := make([]byte, 32)
	binary.LittleEndian.PutUint32(application[:4], uint32(UiccApplicationTypeUSIM))
	binary.LittleEndian.PutUint32(application[20:24], pinKeyReferenceCount)
	application = appendRefValue(application, 4, aid)
	application = appendRefValue(application, 12, label)
	application = appendRefValue(application, 24, pinKeyReferences)

	header := make([]byte, 16)
	binary.LittleEndian.PutUint32(header[:4], 1)
	binary.LittleEndian.PutUint32(header[4:8], 1)
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(application)))
	return appendOffsetSizeElements(header, [][]byte{application})
}

func offsetSizePayloadForValidation(fixedSize, fieldOffset int, value []byte) []byte {
	data := make([]byte, fixedSize)
	return appendRefValue(data, fieldOffset, value)
}

func sizeOffsetPayloadForValidation(fixedSize, fieldOffset int, value []byte) []byte {
	data := make([]byte, fixedSize)
	binary.LittleEndian.PutUint32(data[fieldOffset:fieldOffset+4], uint32(len(value)))
	if len(value) != 0 {
		binary.LittleEndian.PutUint32(data[fieldOffset+4:fieldOffset+8], uint32(fixedSize))
	}
	return padTo4Bytes(append(data, value...))
}

func lteAttachInfoPayloadForValidation(accessString, userName, password string) []byte {
	data := make([]byte, 40)
	data = appendRefValue(data, 8, utf16Bytes(accessString))
	data = appendRefValue(data, 16, utf16Bytes(userName))
	return appendRefValue(data, 24, utf16Bytes(password))
}
