package mbim

import (
	"context"
	"encoding/binary"
	"testing"
)

func TestPINInfoEnumValidation(t *testing.T) {
	tests := []struct {
		name    string
		pinType PinType
		state   PinState
		wantErr bool
	}{
		{name: "maximum values", pinType: PinTypeCorporatePUK, state: PinStateLocked},
		{name: "UICC-only PIN type", pinType: PinTypeNEV, wantErr: true},
		{name: "reserved state", state: PinStateLocked + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 12)
			binary.LittleEndian.PutUint32(data[0:4], uint32(tt.pinType))
			binary.LittleEndian.PutUint32(data[4:8], uint32(tt.state))
			var got PinInfo
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPINListDescriptorValidation(t *testing.T) {
	tests := []struct {
		name    string
		mode    PinMode
		format  PinFormat
		min     uint32
		max     uint32
		wantErr bool
	}{
		{name: "known lengths", mode: PinModeEnabled, format: PinFormatNumeric, min: 4, max: 8},
		{name: "unknown lengths", min: pinLengthUnknown, max: pinLengthUnknown},
		{name: "reserved mode", mode: PinModeDisabled + 1, wantErr: true},
		{name: "reserved format", format: PinFormatAlphanumeric + 1, wantErr: true},
		{name: "minimum too large", min: 17, max: 17, wantErr: true},
		{name: "maximum too large", max: 17, wantErr: true},
		{name: "inverted range", min: 8, max: 4, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 160)
			binary.LittleEndian.PutUint32(data[0:4], uint32(tt.mode))
			binary.LittleEndian.PutUint32(data[4:8], uint32(tt.format))
			binary.LittleEndian.PutUint32(data[8:12], tt.min)
			binary.LittleEndian.PutUint32(data[12:16], tt.max)
			var got PinListInfo
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetPINInputValidation(t *testing.T) {
	tests := []struct {
		name      string
		pinType   PinType
		operation PinOperation
		newPIN    string
	}{
		{name: "UICC-only PIN type", pinType: PinTypeADM},
		{name: "reserved operation", pinType: PinTypePIN1, operation: PinOperationChange + 1},
		{name: "new PIN on enable", pinType: PinTypePIN1, operation: PinOperationEnable, newPIN: "1234"},
		{name: "new PIN on non-PUK enter", pinType: PinTypePIN1, operation: PinOperationEnter, newPIN: "1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := new(Client).SetPIN(ctx, tt.pinType, tt.operation, "0000", tt.newPIN)
			if err == nil {
				t.Fatal("SetPIN() error = nil, want non-nil")
			}
		})
	}
}

func TestSMSConfigurationSemanticValidation(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantState SMSStorageState
		wantErr   bool
	}{
		{name: "initialized", data: smsConfigurationPayloadForValidation("+123"), wantState: SMSStorageStateInitialized},
		{
			name: "uninitialized ignores other fields",
			data: func() []byte {
				data := make([]byte, 24)
				binary.LittleEndian.PutUint32(data[4:8], ^uint32(0))
				binary.LittleEndian.PutUint32(data[16:20], ^uint32(0))
				binary.LittleEndian.PutUint32(data[20:24], ^uint32(0))
				return data
			}(),
			wantState: SMSStorageStateNotInitialized,
		},
		{
			name: "reserved storage state",
			data: func() []byte {
				data := make([]byte, 24)
				binary.LittleEndian.PutUint32(data[0:4], uint32(SMSStorageStateInitialized)+1)
				return data
			}(),
			wantErr: true,
		},
		{
			name: "reserved format",
			data: func() []byte {
				data := smsConfigurationPayloadForValidation("")
				binary.LittleEndian.PutUint32(data[4:8], uint32(SMSFormatCDMA)+1)
				return data
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SMSConfigurationInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.StorageState != tt.wantState {
				t.Fatalf("StorageState = %d, want %d", got.StorageState, tt.wantState)
			}
		})
	}
}

func TestSMSRecordEnumValidation(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr bool
	}{
		{
			name: "PDU status",
			run: func() error {
				data := smsPDURecordForValidation(nil)
				binary.LittleEndian.PutUint32(data[4:8], uint32(SMSStatusSent)+1)
				return new(SMSPDURecord).UnmarshalBinary(data)
			},
			wantErr: true,
		},
		{
			name: "CDMA status",
			run: func() error {
				data := smsCDMARecordForValidation("", "", nil)
				binary.LittleEndian.PutUint32(data[4:8], uint32(SMSStatusSent)+1)
				return new(SMSCDMARecord).UnmarshalBinary(data)
			},
			wantErr: true,
		},
		{
			name: "CDMA encoding",
			run: func() error {
				data := smsCDMARecordForValidation("", "", nil)
				binary.LittleEndian.PutUint32(data[24:28], uint32(SMSCDMAEncodingGSM7Bit)+1)
				return new(SMSCDMARecord).UnmarshalBinary(data)
			},
			wantErr: true,
		},
		{
			name: "CDMA language",
			run: func() error {
				data := smsCDMARecordForValidation("", "", nil)
				binary.LittleEndian.PutUint32(data[28:32], uint32(SMSCDMALanguageHebrew)+1)
				return new(SMSCDMARecord).UnmarshalBinary(data)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); (err != nil) != tt.wantErr {
				t.Fatalf("decoder error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMSFixedFieldSemanticValidation(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr bool
	}{
		{
			name: "maximum message reference",
			run: func() error {
				return new(SMSSendInfo).UnmarshalBinary(binary.LittleEndian.AppendUint32(nil, 0xffff))
			},
		},
		{
			name: "message reference too large",
			run: func() error {
				return new(SMSSendInfo).UnmarshalBinary(binary.LittleEndian.AppendUint32(nil, 0x10000))
			},
			wantErr: true,
		},
		{
			name: "all store flags",
			run: func() error {
				data := binary.LittleEndian.AppendUint32(nil, 3)
				data = binary.LittleEndian.AppendUint32(data, 0)
				return new(SMSStoreStatusInfo).UnmarshalBinary(data)
			},
		},
		{
			name: "reserved store flag",
			run: func() error {
				data := binary.LittleEndian.AppendUint32(nil, 4)
				data = binary.LittleEndian.AppendUint32(data, 0)
				return new(SMSStoreStatusInfo).UnmarshalBinary(data)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); (err != nil) != tt.wantErr {
				t.Fatalf("decoder error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMSClientInputValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *Client) error
	}{
		{
			name: "configuration format",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.SetSMSConfiguration(ctx, SMSFormatCDMA+1, "")
				return err
			},
		},
		{
			name: "read format",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.ReadSMS(ctx, SMSFormatCDMA+1, SMSReadFlagAll, 0)
				return err
			},
		},
		{
			name: "read flag",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.ReadSMS(ctx, SMSFormatPDU, SMSReadFlagDraft+1, 0)
				return err
			},
		},
		{
			name: "read missing index",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.ReadSMS(ctx, SMSFormatPDU, SMSReadFlagIndex, 0)
				return err
			},
		},
		{
			name: "read unexpected index",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.ReadSMS(ctx, SMSFormatPDU, SMSReadFlagAll, 1)
				return err
			},
		},
		{
			name: "send encoding",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.SendSMSCDMA(ctx, SMSSendCDMA{Encoding: SMSCDMAEncodingGSM7Bit + 1})
				return err
			},
		},
		{
			name: "send language",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.SendSMSCDMA(ctx, SMSSendCDMA{Language: SMSCDMALanguageHebrew + 1})
				return err
			},
		},
		{
			name: "delete missing index",
			run: func(ctx context.Context, client *Client) error {
				return client.DeleteSMS(ctx, SMSReadFlagIndex, 0)
			},
		},
		{
			name: "delete unexpected index",
			run: func(ctx context.Context, client *Client) error {
				return client.DeleteSMS(ctx, SMSReadFlagOld, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := tt.run(ctx, new(Client)); err == nil {
				t.Fatal("client call error = nil, want non-nil")
			}
		})
	}
}

func TestUSSDSemanticValidation(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "payload response", data: ussdPayloadForValidation([]byte{1})},
		{
			name: "reserved response",
			data: mutateBytes(ussdPayloadForValidation([]byte{1}), func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], uint32(USSDResponseNetworkTimeout)+1)
			}),
			wantErr: true,
		},
		{
			name: "reserved session state",
			data: mutateBytes(ussdPayloadForValidation([]byte{1}), func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(USSDSessionStateExisting)+1)
			}),
			wantErr: true,
		},
		{name: "payload response is empty", data: ussdPayloadForValidation(nil), wantErr: true},
		{
			name: "terminated without payload",
			data: mutateBytes(ussdPayloadForValidation(nil), func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], uint32(USSDResponseTerminatedByNetwork))
			}),
		},
		{
			name: "terminated with payload",
			data: mutateBytes(ussdPayloadForValidation([]byte{1}), func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], uint32(USSDResponseTerminatedByNetwork))
			}),
			wantErr: true,
		},
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

func TestUSSDClientInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		action  USSDAction
		payload []byte
	}{
		{name: "reserved action", action: USSDActionCancel + 1, payload: []byte{1}},
		{name: "empty initiate", action: USSDActionInitiate},
		{name: "empty continue", action: USSDActionContinue},
		{name: "cancel payload", action: USSDActionCancel, payload: []byte{1}},
		{name: "payload too long", action: USSDActionInitiate, payload: make([]byte, 161)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := new(Client).USSD(ctx, tt.action, 0, tt.payload)
			if err == nil {
				t.Fatal("USSD() error = nil, want non-nil")
			}
		})
	}
}

func TestPhonebookConfigurationSemanticValidation(t *testing.T) {
	tests := []struct {
		name    string
		state   PhonebookState
		total   uint32
		used    uint32
		wantErr bool
	}{
		{name: "initialized", state: PhonebookStateInitialized, total: 10, used: 5},
		{name: "reserved state", state: PhonebookStateInitialized + 1, wantErr: true},
		{name: "used exceeds total", state: PhonebookStateInitialized, total: 4, used: 5, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 20)
			binary.LittleEndian.PutUint32(data[0:4], uint32(tt.state))
			binary.LittleEndian.PutUint32(data[4:8], tt.total)
			binary.LittleEndian.PutUint32(data[8:12], tt.used)
			var got PhonebookConfigurationInfo
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPhonebookClientInputValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *Client) error
	}{
		{
			name: "read flag",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.ReadPhonebook(ctx, PhonebookFlagIndex+1, 0)
				return err
			},
		},
		{
			name: "read missing index",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.ReadPhonebook(ctx, PhonebookFlagIndex, 0)
				return err
			},
		},
		{
			name: "read unexpected index",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.ReadPhonebook(ctx, PhonebookFlagAll, 1)
				return err
			},
		},
		{
			name: "delete missing index",
			run: func(ctx context.Context, client *Client) error {
				return client.DeletePhonebook(ctx, PhonebookFlagIndex, 0)
			},
		},
		{
			name: "write flag",
			run: func(ctx context.Context, client *Client) error {
				return client.WritePhonebook(ctx, PhonebookWriteFlagIndex+1, 0, "", "")
			},
		},
		{
			name: "write missing index",
			run: func(ctx context.Context, client *Client) error {
				return client.WritePhonebook(ctx, PhonebookWriteFlagIndex, 0, "", "")
			},
		},
		{
			name: "write unexpected index",
			run: func(ctx context.Context, client *Client) error {
				return client.WritePhonebook(ctx, PhonebookWriteFlagUnused, 1, "", "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := tt.run(ctx, new(Client)); err == nil {
				t.Fatal("client call error = nil, want non-nil")
			}
		})
	}
}

func TestBasicEnumSemanticValidation(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr bool
	}{
		{
			name: "network idle enabled",
			run: func() error {
				return new(NetworkIdleHint).UnmarshalBinary(binary.LittleEndian.AppendUint32(nil, 1))
			},
		},
		{
			name: "network idle reserved",
			run: func() error {
				return new(NetworkIdleHint).UnmarshalBinary(binary.LittleEndian.AppendUint32(nil, 2))
			},
			wantErr: true,
		},
		{
			name: "emergency mode on",
			run: func() error {
				return new(EmergencyMode).UnmarshalBinary(binary.LittleEndian.AppendUint32(nil, 1))
			},
		},
		{
			name: "emergency mode reserved",
			run: func() error {
				return new(EmergencyMode).UnmarshalBinary(binary.LittleEndian.AppendUint32(nil, 2))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); (err != nil) != tt.wantErr {
				t.Fatalf("decoder error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBasicClientEnumValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *Client) error
	}{
		{
			name: "visible providers action",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.VisibleProviders(ctx, VisibleProvidersActionRestrictedScan+1)
				return err
			},
		},
		{
			name: "registration action",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.SetRegistrationState(ctx, "", RegisterActionManual+1, DataClassNone)
				return err
			},
		},
		{
			name: "registration provider ID",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.SetRegistrationState(ctx, "310x60", RegisterActionManual, DataClassNone)
				return err
			},
		},
		{
			name: "manual registration without provider ID",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.SetRegistrationState(ctx, "", RegisterActionManual, DataClassNone)
				return err
			},
		},
		{
			name: "network idle hint",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.SetNetworkIdleHint(ctx, NetworkIdleHintEnabled+1)
				return err
			},
		},
		{
			name: "DSS link state",
			run: func(ctx context.Context, client *Client) error {
				return client.SetDSSLinkState(ctx, ServiceBasicConnect, 0, DSSLinkStateActivate+1)
			},
		},
		{
			name: "registration data class",
			run: func(ctx context.Context, client *Client) error {
				client.mbimExVersion = mbimExVersion30
				_, err := client.SetRegistrationState(ctx, "", RegisterActionAutomatic, DataClassUnused)
				return err
			},
		},
		{
			name: "packet service action",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.SetPacketService(ctx, PacketServiceActionDetach+1)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := tt.run(ctx, new(Client)); err == nil {
				t.Fatal("client call error = nil, want non-nil")
			}
		})
	}
}

func TestProviderIDSemanticValidation(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr bool
	}{
		{
			name: "numeric provider",
			run: func() error {
				_, err := (Provider{ID: "310260"}).MarshalBinary()
				return err
			},
		},
		{
			name: "non-numeric provider",
			run: func() error {
				_, err := (Provider{ID: "310x60"}).MarshalBinary()
				return err
			},
			wantErr: true,
		},
		{
			name: "non-numeric registration response",
			run: func() error {
				return new(RegistrationStateInfo).UnmarshalBinary(registrationStatePayloadForValidation(0, "310x60", "", ""))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); (err != nil) != tt.wantErr {
				t.Fatalf("operation error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProviderWireValueValidation(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		wantErr  bool
	}{
		{name: "known maximums", provider: Provider{State: providerStateMask, CellularClass: cellularClassMask, RSSI: 31, ErrorRate: 7}},
		{name: "unknown signal values", provider: Provider{RSSI: 99, ErrorRate: 99}},
		{name: "reserved provider state", provider: Provider{State: providerStateMask | 1<<6}, wantErr: true},
		{name: "reserved cellular class", provider: Provider{CellularClass: cellularClassMask | 1<<2}, wantErr: true},
		{name: "reserved RSSI", provider: Provider{RSSI: 32}, wantErr: true},
		{name: "reserved error rate", provider: Provider{ErrorRate: 8}, wantErr: true},
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

func TestProviderDecoderWireValueValidation(t *testing.T) {
	tests := []struct {
		name    string
		offset  int
		value   uint32
		wantErr bool
	}{
		{name: "known provider state", offset: 8, value: uint32(providerStateMask)},
		{name: "reserved provider state", offset: 8, value: uint32(providerStateMask) | 1<<6, wantErr: true},
		{name: "reserved cellular class", offset: 20, value: 1 << 2, wantErr: true},
		{name: "reserved RSSI", offset: 24, value: 32, wantErr: true},
		{name: "reserved error rate", offset: 28, value: 8, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := Provider{}.marshalBinary()
			binary.LittleEndian.PutUint32(data[tt.offset:tt.offset+4], tt.value)
			var got Provider
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistrationStateWireValueValidation(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		mutate  func([]byte)
		wantErr bool
	}{
		{name: "default values"},
		{
			name: "MBIM 1 LTE data class",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(RegisterStateHome))
				binary.LittleEndian.PutUint32(data[12:16], uint32(DataClassLTE))
			},
		},
		{
			name: "MBIM 1 reserved data class",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(RegisterStateHome))
				binary.LittleEndian.PutUint32(data[12:16], uint32(DataClass5GNSA))
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 2 5G data classes",
			version: mbimExVersion20,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(RegisterStateHome))
				binary.LittleEndian.PutUint32(data[12:16], uint32(DataClass5GSA))
				binary.LittleEndian.PutUint32(data[48:52], uint32(DataClass5GNSA))
			},
		},
		{
			name:    "MBIMEx 3 unused data class",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[48:52], uint32(DataClassUnused))
			},
			wantErr: true,
		},
		{
			name: "reserved state",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(RegisterStateDenied)+1)
			},
			wantErr: true,
		},
		{
			name: "reserved mode",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], uint32(RegisterModeManual)+1)
			},
			wantErr: true,
		},
		{
			name: "data classes while deregistered",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(RegisterStateDeregistered))
				binary.LittleEndian.PutUint32(data[12:16], uint32(DataClassLTE))
			},
			wantErr: true,
		},
		{
			name: "reserved cellular class",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[16:20], 1<<2)
			},
			wantErr: true,
		},
		{
			name: "reserved flags",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[44:48], 1<<2)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := registrationStatePayloadForValidation(tt.version, "", "", "")
			if tt.mutate != nil {
				tt.mutate(data)
			}
			got := RegistrationStateInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeviceCapsWireValueValidation(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		mutate  func([]byte)
		wantErr bool
	}{
		{
			name: "MBIM 1 known values",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], uint32(DeviceTypeRemote))
				binary.LittleEndian.PutUint32(data[4:8], uint32(cellularClassMask))
				binary.LittleEndian.PutUint32(data[8:12], uint32(VoiceClassSimultaneousVoiceData))
				binary.LittleEndian.PutUint32(data[12:16], uint32(simClassMask))
				binary.LittleEndian.PutUint32(data[16:20], uint32(dataClassV1Mask))
				binary.LittleEndian.PutUint32(data[20:24], uint32(smsCapsMask))
				binary.LittleEndian.PutUint32(data[24:28], uint32(controlCapsV1Mask))
			},
		},
		{
			name: "reserved device type",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], uint32(DeviceTypeRemote)+1)
			},
			wantErr: true,
		},
		{
			name: "reserved cellular class",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 1<<2)
			},
			wantErr: true,
		},
		{
			name: "reserved voice class",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], uint32(VoiceClassSimultaneousVoiceData)+1)
			},
			wantErr: true,
		},
		{
			name: "reserved SIM class",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[12:16], 1<<2)
			},
			wantErr: true,
		},
		{
			name: "MBIM 1 reserved data class",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[16:20], uint32(DataClass5GNSA))
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 2 5G SA",
			version: mbimExVersion20,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[16:20], uint32(DataClass5GSA))
			},
		},
		{
			name: "reserved SMS capability",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[20:24], 1<<4)
			},
			wantErr: true,
		},
		{
			name: "MBIM 1 extended control capability",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[24:28], uint32(ControlCapsESIM))
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 control capabilities",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[24:28], uint32(controlCapsV3Mask))
			},
		},
		{
			name:    "MBIMEx 3 reserved control capability",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[24:28], uint32(ControlCapsUseURSPRuleOnEPC))
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 4 URSP on EPC capability",
			version: mbimExVersion40,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[24:28], uint32(ControlCapsUseURSPRuleOnEPC))
			},
		},
		{
			name:    "MBIMEx 4 UE policy capability must be zero",
			version: mbimExVersion40,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[24:28], uint32(ControlCapsUEPolicyRouteSelection))
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 unused data class",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[16:20], uint32(DataClass5G|DataClassUnused))
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 reserved data subclass",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint64(data[28:36], uint64(dataSubclassMask|1<<5))
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 subclass without 5G",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[16:20], uint32(DataClassLTE))
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 5G without subclass",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint64(data[28:36], 0)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := deviceCapsPayloadForSemanticValidation(tt.version)
			if tt.mutate != nil {
				tt.mutate(data)
			}
			got := DeviceCapsInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConnectWireValueValidation(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		mutate  func([]byte)
		wantErr bool
	}{
		{
			name: "known maximums",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(ActivationStateDeactivating))
				binary.LittleEndian.PutUint32(data[8:12], uint32(VoiceCallStateHangUp))
				binary.LittleEndian.PutUint32(data[12:16], uint32(ContextIPTypeIPv4AndIPv6))
			},
		},
		{
			name: "reserved activation state",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(ActivationStateDeactivating)+1)
			},
			wantErr: true,
		},
		{
			name: "reserved voice call state",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], uint32(VoiceCallStateHangUp)+1)
			},
			wantErr: true,
		},
		{
			name: "reserved IP type",
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[12:16], uint32(ContextIPTypeIPv4AndIPv6)+1)
			},
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 access media",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[36:40], uint32(AccessMediaType3GPPPreferred))
			},
		},
		{
			name:    "MBIMEx 3 reserved access media",
			version: mbimExVersion30,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint32(data[36:40], uint32(AccessMediaType3GPPPreferred)+1)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := connectPayloadForSemanticValidation(tt.version)
			if tt.mutate != nil {
				tt.mutate(data)
			}
			got := ConnectInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConnectConfigEnumValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ConnectConfig
		wantErr bool
	}{
		{
			name: "known maximums",
			cfg: ConnectConfig{
				ActivationCommand: ActivationCommandActivate,
				Compression:       CompressionEnable,
				AuthProtocol:      AuthProtocolMSCHAPV2,
				IPType:            ContextIPTypeIPv4AndIPv6,
				MediaPreference:   AccessMediaType3GPPPreferred,
			},
		},
		{name: "reserved compression", cfg: ConnectConfig{Compression: CompressionEnable + 1}, wantErr: true},
		{name: "reserved authentication", cfg: ConnectConfig{AuthProtocol: AuthProtocolMSCHAPV2 + 1}, wantErr: true},
		{name: "reserved IP type", cfg: ConnectConfig{IPType: ContextIPTypeIPv4AndIPv6 + 1}, wantErr: true},
		{name: "reserved media preference", cfg: ConnectConfig{MediaPreference: AccessMediaType3GPPPreferred + 1}, wantErr: true},
		{
			name: "deactivation with non-default option",
			cfg: ConnectConfig{
				ActivationCommand: ActivationCommandDeactivate,
				ActivationOption:  ActivationOptionPerDefaultURSPRule,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnectConfig(tt.cfg, mbimExVersion40)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateConnectConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIPConfigurationAvailabilityValidation(t *testing.T) {
	tests := []struct {
		name    string
		ipv4    IPConfigurationAvailable
		ipv6    IPConfigurationAvailable
		wantErr bool
	}{
		{name: "no information"},
		{name: "all information", ipv4: ipConfigurationAvailableMask, ipv6: ipConfigurationAvailableMask},
		{name: "IPv4 reserved bit", ipv4: 1 << 4, wantErr: true},
		{name: "IPv6 reserved bit", ipv6: 1 << 4, wantErr: true},
		{name: "IPv4 partial basic group", ipv4: IPConfigurationAvailableAddress, wantErr: true},
		{name: "IPv6 partial basic group", ipv6: IPConfigurationAvailableGateway | IPConfigurationAvailableMTU, wantErr: true},
		{name: "DNS only", ipv4: IPConfigurationAvailableDNSServer, ipv6: IPConfigurationAvailableDNSServer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, ipConfigurationInfoFixedSize)
			binary.LittleEndian.PutUint32(data[4:8], uint32(tt.ipv4))
			binary.LittleEndian.PutUint32(data[8:12], uint32(tt.ipv6))
			var got IPConfigurationInfo
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIPConfigurationUnavailableFieldsAreZero(t *testing.T) {
	tests := []struct {
		name   string
		offset int
	}{
		{name: "IPv4 address count", offset: 12},
		{name: "IPv4 address offset", offset: 16},
		{name: "IPv6 address count", offset: 20},
		{name: "IPv6 address offset", offset: 24},
		{name: "IPv4 gateway offset", offset: 28},
		{name: "IPv6 gateway offset", offset: 32},
		{name: "IPv4 DNS count", offset: 36},
		{name: "IPv4 DNS offset", offset: 40},
		{name: "IPv6 DNS count", offset: 44},
		{name: "IPv6 DNS offset", offset: 48},
		{name: "IPv4 MTU", offset: 52},
		{name: "IPv6 MTU", offset: 56},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, ipConfigurationInfoFixedSize)
			binary.LittleEndian.PutUint32(data[tt.offset:tt.offset+4], 1)
			if err := new(IPConfigurationInfo).UnmarshalBinary(data); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestPacketServiceWireValueValidation(t *testing.T) {
	tests := []struct {
		name         string
		version      uint16
		state        PacketServiceState
		dataClass    DataClass
		frequency    FrequencyRange
		dataSubclass DataSubclass
		wantErr      bool
	}{
		{name: "MBIM 1 LTE", state: PacketServiceStateAttached, dataClass: DataClassLTE},
		{name: "reserved state", state: PacketServiceStateDetached + 1, wantErr: true},
		{name: "data class while detached", state: PacketServiceStateDetached, dataClass: DataClassLTE, wantErr: true},
		{name: "MBIM 1 reserved 5G", state: PacketServiceStateAttached, dataClass: DataClass5GNSA, wantErr: true},
		{
			name:      "MBIMEx 2 5G",
			version:   mbimExVersion20,
			state:     PacketServiceStateAttached,
			dataClass: DataClass5GSA,
			frequency: FrequencyRange1 | FrequencyRange2,
		},
		{
			name:      "MBIMEx 2 reserved frequency range",
			version:   mbimExVersion20,
			state:     PacketServiceStateAttached,
			dataClass: DataClass5GNSA,
			frequency: FrequencyRange(1 << 2),
			wantErr:   true,
		},
		{
			name:      "MBIMEx 2 frequency without 5G",
			version:   mbimExVersion20,
			state:     PacketServiceStateAttached,
			dataClass: DataClassLTE,
			frequency: FrequencyRange1,
			wantErr:   true,
		},
		{
			name:         "MBIMEx 3 5G",
			version:      mbimExVersion30,
			state:        PacketServiceStateAttached,
			dataClass:    DataClass5G,
			frequency:    FrequencyRange1,
			dataSubclass: dataSubclassMask,
		},
		{
			name:         "MBIMEx 3 reserved data subclass",
			version:      mbimExVersion30,
			state:        PacketServiceStateAttached,
			dataClass:    DataClass5G,
			dataSubclass: dataSubclassMask | 1<<5,
			wantErr:      true,
		},
		{
			name:         "MBIMEx 3 subclass without 5G",
			version:      mbimExVersion30,
			state:        PacketServiceStateAttached,
			dataClass:    DataClassLTE,
			dataSubclass: DataSubclass5GNR,
			wantErr:      true,
		},
		{
			name:      "MBIMEx 3 unused data class",
			version:   mbimExVersion30,
			state:     PacketServiceStateAttached,
			dataClass: DataClassUnused,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := packetServicePayloadForVersionForTest(tt.version, tt.state)
			binary.LittleEndian.PutUint32(data[8:12], uint32(tt.dataClass))
			if tt.version >= mbimExVersion20 {
				binary.LittleEndian.PutUint32(data[28:32], uint32(tt.frequency))
			}
			if tt.version >= mbimExVersion30 {
				binary.LittleEndian.PutUint32(data[32:36], uint32(tt.dataSubclass))
			}
			got := PacketServiceInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func deviceCapsPayloadForSemanticValidation(version uint16) []byte {
	switch {
	case version >= mbimExVersion30:
		return deviceCapsPayloadV3WithValuesForTest(1, nil, nil, "", "", "", "")
	case version >= mbimExVersion20:
		return deviceCapsPayloadV2ForTest(1, "", "", "", "")
	default:
		return deviceCapsPayload(1)
	}
}

func connectPayloadForSemanticValidation(version uint16) []byte {
	if version >= mbimExVersion30 {
		return connectInfoPayloadEx3ForTest(
			1,
			ActivationStateActivated,
			ContextIPTypeIPv4,
			ContextTypeInternet,
			"",
			nil,
		)
	}
	return connectInfoPayloadForTest(
		1,
		ActivationStateActivated,
		ContextIPTypeIPv4,
		ContextTypeInternet,
	)
}

func TestSignalStateWireValueValidation(t *testing.T) {
	tests := []struct {
		name      string
		rssi      uint32
		errorRate uint32
		wantErr   bool
	}{
		{name: "maximum coded values", rssi: 31, errorRate: 7},
		{name: "unknown values", rssi: 99, errorRate: 99},
		{name: "reserved RSSI", rssi: 32, wantErr: true},
		{name: "reserved error rate", errorRate: 8, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 20)
			binary.LittleEndian.PutUint32(data[0:4], tt.rssi)
			binary.LittleEndian.PutUint32(data[4:8], tt.errorRate)
			var got SignalStateInfo
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSignalStateRSRPSNRSemanticValidation(t *testing.T) {
	tests := []struct {
		name       string
		version    uint16
		rssi       uint32
		rsrp       uint32
		snr        uint32
		systemType DataClass
		wantErr    bool
	}{
		{name: "LTE", version: mbimExVersion20, rssi: 99, rsrp: 127, snr: 128, systemType: DataClassLTE},
		{name: "MBIMEx 2 5G SA", version: mbimExVersion20, rssi: 99, systemType: DataClass5GSA},
		{name: "MBIMEx 3 5G", version: mbimExVersion30, rssi: 99, systemType: DataClass5G},
		{name: "RSSI must be unknown", version: mbimExVersion20, rssi: 31, systemType: DataClassLTE, wantErr: true},
		{name: "RSRP reserved", version: mbimExVersion20, rssi: 99, rsrp: 128, systemType: DataClassLTE, wantErr: true},
		{name: "SNR reserved", version: mbimExVersion20, rssi: 99, snr: 129, systemType: DataClassLTE, wantErr: true},
		{name: "unsupported system type", version: mbimExVersion20, rssi: 99, systemType: DataClassGPRS, wantErr: true},
		{name: "MBIMEx 3 rejects old SA bit", version: mbimExVersion30, rssi: 99, systemType: DataClass5GSA, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsrpSNR := make([]byte, 24)
			binary.LittleEndian.PutUint32(rsrpSNR[0:4], 1)
			binary.LittleEndian.PutUint32(rsrpSNR[4:8], tt.rsrp)
			binary.LittleEndian.PutUint32(rsrpSNR[8:12], tt.snr)
			binary.LittleEndian.PutUint32(rsrpSNR[20:24], uint32(tt.systemType))
			data := make([]byte, 28)
			binary.LittleEndian.PutUint32(data[0:4], tt.rssi)
			data = appendRefValue(data, 20, rsrpSNR)
			got := SignalStateInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
