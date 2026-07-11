package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/damonto/uicc-go/apdu"
)

func TestRequestMarshalBinary(t *testing.T) {
	tests := []struct {
		name string
		req  *Request
		want []byte
	}{
		{
			name: "open",
			req:  (&OpenDeviceRequest{TransactionID: 1}).Request(),
			want: []byte{
				0x01, 0x00, 0x00, 0x00,
				0x10, 0x00, 0x00, 0x00,
				0x01, 0x00, 0x00, 0x00,
				0x00, 0x10, 0x00, 0x00,
			},
		},
		{
			name: "subscriber ready status",
			req:  (&SubscriberReadyStatusRequest{TransactionID: 6}).Request(),
			want: mustDecodeHex(t, "0300000030000000060000000100000000000000A289CC33BCBB8B4FB6B0133EC2AAE6DF020000000000000000000000"),
		},
		{
			name: "auth aka",
			req: (&AuthAKARequest{
				TransactionID: 7,
				Rand:          []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F},
				AUTN:          []byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8, 0xF7, 0xF6, 0xF5, 0xF4, 0xF3, 0xF2, 0xF1, 0xF0},
			}).Request(),
			want: mustDecodeHex(t, "03000000500000000700000001000000000000001D2B5FF70AA148B2AA5250F15767174E010000000000000020000000000102030405060708090A0B0C0D0E0FFFFEFDFCFBFAF9F8F7F6F5F4F3F2F1F0"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.req.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = %X, want %X", got, tt.want)
			}
		})
	}
}

func TestCommandMarshalBinaryPadsInformationBuffer(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"already aligned", []byte{0xAA, 0xBB, 0xCC, 0xDD}},
		{"needs padding", []byte{0xAA, 0xBB, 0xCC}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{
				FragmentTotal:   1,
				FragmentCurrent: 0,
				Data:            tt.data,
			}
			req := &Request{
				MessageType:   MessageTypeCommand,
				TransactionID: 1,
				Command:       cmd,
			}

			got, err := req.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}

			wantMessageLength := uint32(12 + 36 + align4(len(tt.data)))
			if gotMessageLength := binary.LittleEndian.Uint32(got[4:8]); gotMessageLength != wantMessageLength {
				t.Fatalf("MessageLength = %d, want %d", gotMessageLength, wantMessageLength)
			}
			if gotInformationBufferLength := binary.LittleEndian.Uint32(got[44:48]); gotInformationBufferLength != uint32(len(tt.data)) {
				t.Fatalf("InformationBufferLength = %d, want %d", gotInformationBufferLength, len(tt.data))
			}
			if gotData := got[48 : 48+len(tt.data)]; !bytes.Equal(gotData, tt.data) {
				t.Fatalf("InformationBuffer = %X, want %X", gotData, tt.data)
			}
			for _, b := range got[48+len(tt.data):] {
				if b != 0 {
					t.Fatalf("padding contains %#x, want zero", b)
				}
			}
		})
	}
}

func TestVersionRequestData(t *testing.T) {
	tests := []struct {
		name        string
		req         *Request
		serviceID   [16]byte
		commandID   uint32
		commandType CommandType
		want        []byte
	}{
		{
			name:        "device capabilities",
			req:         (&DeviceCapsRequest{TransactionID: 1}).Request(),
			serviceID:   ServiceBasicConnect,
			commandID:   CIDDeviceCaps,
			commandType: CommandTypeQuery,
			want:        nil,
		},
		{
			name:        "device services",
			req:         (&DeviceServicesRequest{TransactionID: 1}).Request(),
			serviceID:   ServiceBasicConnect,
			commandID:   CIDDeviceServices,
			commandType: CommandTypeQuery,
			want:        nil,
		},
		{
			name: "version",
			req: (&VersionRequest{
				TransactionID: 1,
				MBIMVersion:   mbimVersion10,
				MBIMExVersion: hostMBIMExVersion,
			}).Request(),
			serviceID:   ServiceMsBasicConnectExtensions,
			commandID:   CIDVersion,
			commandType: CommandTypeQuery,
			want:        mustDecodeHex(t, "00010004"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.req.Command.(*Command)
			if command.ServiceID != tt.serviceID {
				t.Fatalf("ServiceID = % X, want % X", command.ServiceID, tt.serviceID)
			}
			if command.CommandID != tt.commandID || command.CommandType != tt.commandType {
				t.Fatalf("command = cid %d type %d, want cid %d type %d", command.CommandID, command.CommandType, tt.commandID, tt.commandType)
			}
			if !bytes.Equal(command.Data, tt.want) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.want)
			}
		})
	}
}

func TestDeviceCapsInfoUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantSessions uint32
		wantErr      bool
	}{
		{name: "multiple sessions", data: deviceCapsPayload(3), wantSessions: 3},
		{name: "single session", data: deviceCapsPayload(1), wantSessions: 1},
		{name: "truncated payload", data: make([]byte, 31), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DeviceCapsInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.MaxSessions != tt.wantSessions {
				t.Fatalf("MaxSessions = %d, want %d", got.MaxSessions, tt.wantSessions)
			}
		})
	}
}

func TestCommandRequestTimeouts(t *testing.T) {
	tests := []struct {
		name string
		req  *Request
		want time.Duration
	}{
		{
			name: "radio state query",
			req:  (&RadioStateRequest{TransactionID: 1}).Request(),
			want: mbimCIDResponseTimeout,
		},
		{
			name: "radio state set",
			req: (&RadioStateSetRequest{
				TransactionID: 1,
				State:         RadioSwitchStateOn,
			}).Request(),
			want: mbimCIDLongResponseTimeout,
		},
		{
			name: "subscriber ready status",
			req:  (&SubscriberReadyStatusRequest{TransactionID: 1}).Request(),
			want: mbimCIDResponseTimeout,
		},
		{
			name: "UICC ATR",
			req:  (&UiccATRQueryRequest{TransactionID: 1}).Request(),
			want: mbimCIDLongResponseTimeout,
		},
		{
			name: "open channel",
			req: (&OpenChannelRequest{
				TransactionID: 1,
				ChannelGroup:  uiccChannelGroupDefault,
			}).Request(),
			want: mbimCIDResponseTimeout,
		},
		{
			name: "close channel",
			req: (&CloseChannelRequest{
				TransactionID: 1,
				ChannelGroup:  uiccChannelGroupDefault,
			}).Request(),
			want: mbimCIDResponseTimeout,
		},
		{
			name: "APDU",
			req: (&APDURequest{
				TransactionID:   1,
				SecureMessaging: UiccSecureMessagingNone,
				ClassByteType:   UiccClassByteTypeInterIndustry,
			}).Request(),
			want: mbimCIDLongResponseTimeout,
		},
		{
			name: "terminal capability set",
			req:  (&UiccTerminalCapabilitySetRequest{TransactionID: 1}).Request(),
			want: mbimCIDResponseTimeout,
		},
		{
			name: "terminal capability query",
			req:  (&UiccTerminalCapabilityQueryRequest{TransactionID: 1}).Request(),
			want: mbimCIDResponseTimeout,
		},
		{
			name: "UICC reset set",
			req: (&UiccResetSetRequest{
				TransactionID: 1,
				Action:        UiccPassThroughActionEnable,
			}).Request(),
			want: mbimCIDLongResponseTimeout,
		},
		{
			name: "UICC reset query",
			req:  (&UiccResetQueryRequest{TransactionID: 1}).Request(),
			want: mbimCIDLongResponseTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Timeout != tt.want {
				t.Fatalf("Timeout = %v, want %v", tt.req.Timeout, tt.want)
			}
		})
	}
}

func TestReaderNegotiatesMBIMExVersion(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMCommandWithService(server, 1, ServiceBasicConnect, CIDDeviceServices, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		services := deviceServicesPayload(DeviceService{
			ServiceID: ServiceMsBasicConnectExtensions,
			CIDs:      []uint32{CIDVersion},
		})
		if _, err := server.Write(mbimCommandDone(1, ServiceBasicConnect, CIDDeviceServices, services)); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithService(server, 2, ServiceMsBasicConnectExtensions, CIDVersion, CommandTypeQuery, mustDecodeHex(t, "00010004")); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(2, ServiceMsBasicConnectExtensions, CIDVersion, mustDecodeHex(t, "00010004"))); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := reader.negotiateVersion(ctx); err != nil {
		t.Fatalf("negotiateVersion() error = %v", err)
	}
	if reader.mbimExVersion != mbimExVersion40 {
		t.Fatalf("mbimExVersion = %#x, want %#x", reader.mbimExVersion, mbimExVersion40)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderConnectSkipsSlotActivationWithMBIMEx4(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMOpen(server, 1); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimOpenDone(1)); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithService(server, 2, ServiceBasicConnect, CIDDeviceServices, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		services := deviceServicesPayload(DeviceService{
			ServiceID: ServiceMsBasicConnectExtensions,
			CIDs:      []uint32{CIDVersion},
		})
		if _, err := server.Write(mbimCommandDone(2, ServiceBasicConnect, CIDDeviceServices, services)); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithService(server, 3, ServiceMsBasicConnectExtensions, CIDVersion, CommandTypeQuery, mustDecodeHex(t, "00010004")); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(3, ServiceMsBasicConnectExtensions, CIDVersion, mustDecodeHex(t, "00010004"))); err != nil {
			errc <- err
			return
		}
		errc <- expectNoMBIMCommand(server, 50*time.Millisecond)
	}()

	reader := &Reader{conn: client, slot: 1}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := reader.connect(ctx, ""); err != nil {
		t.Fatalf("connect() error = %v", err)
	}
	if reader.mbimExVersion != mbimExVersion40 {
		t.Fatalf("mbimExVersion = %#x, want %#x", reader.mbimExVersion, mbimExVersion40)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderConnectActivatesSlotBeforeMBIMEx4(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMOpen(server, 1); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimOpenDone(1)); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithService(server, 2, ServiceBasicConnect, CIDDeviceServices, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(2, ServiceBasicConnect, CIDDeviceServices, deviceServicesPayload())); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithService(server, 3, ServiceMsBasicConnectExtensions, CIDDeviceSlotMappings, CommandTypeQuery, mustDecodeHex(t, "00000000")); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(3, ServiceMsBasicConnectExtensions, CIDDeviceSlotMappings, slotMappingsPayload(1))); err != nil {
			errc <- err
			return
		}
		errc <- expectNoMBIMCommand(server, 50*time.Millisecond)
	}()

	reader := &Reader{conn: client, slot: 1}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := reader.connect(ctx, ""); err != nil {
		t.Fatalf("connect() error = %v", err)
	}
	if reader.mbimExVersion != mbimExVersion10 {
		t.Fatalf("mbimExVersion = %#x, want %#x", reader.mbimExVersion, mbimExVersion10)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestSTKRequestData(t *testing.T) {
	envelope := []byte{0xD1, 0x09, 0x82, 0x02, 0x83, 0x81, 0x8B, 0x03, 0x00, 0x7F, 0xF6}
	terminalResponse := []byte{0x81, 0x03, 0x01, 0x21, 0x00}
	pacHostControl := bytes.Repeat([]byte{0xA5}, stkPACHostControlLength)

	tests := []struct {
		name        string
		req         *Request
		commandID   uint32
		commandType CommandType
		want        []byte
	}{
		{
			name:        "PAC query",
			req:         (&STKPACQueryRequest{TransactionID: 7}).Request(),
			commandID:   CIDSTKPAC,
			commandType: CommandTypeQuery,
			want:        nil,
		},
		{
			name: "PAC set",
			req: (&STKPACSetRequest{
				TransactionID:  7,
				PacHostControl: pacHostControl,
			}).Request(),
			commandID:   CIDSTKPAC,
			commandType: CommandTypeSet,
			want:        pacHostControl,
		},
		{
			name: "terminal response set",
			req: (&STKTerminalResponseRequest{
				TransactionID: 7,
				Data:          terminalResponse,
			}).Request(),
			commandID:   CIDSTKTerminalResponse,
			commandType: CommandTypeSet,
			want:        append(binary.LittleEndian.AppendUint32(nil, uint32(len(terminalResponse))), terminalResponse...),
		},
		{
			name:        "envelope query",
			req:         (&STKEnvelopeQueryRequest{TransactionID: 7}).Request(),
			commandID:   CIDSTKEnvelope,
			commandType: CommandTypeQuery,
			want:        nil,
		},
		{
			name: "envelope set",
			req: (&STKEnvelopeRequest{
				TransactionID: 7,
				Data:          envelope,
			}).Request(),
			commandID:   CIDSTKEnvelope,
			commandType: CommandTypeSet,
			want:        envelope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := tt.req.Command.(*Command)
			if !ok {
				t.Fatalf("Command type = %T, want *Command", tt.req.Command)
			}
			if cmd.ServiceID != ServiceSTK {
				t.Fatalf("ServiceID = % X, want STK", cmd.ServiceID)
			}
			if cmd.CommandID != tt.commandID || cmd.CommandType != tt.commandType {
				t.Fatalf("command = cid %d type %d, want cid %d type %d", cmd.CommandID, cmd.CommandType, tt.commandID, tt.commandType)
			}
			if !bytes.Equal(cmd.Data, tt.want) {
				t.Fatalf("Data = % X, want % X", cmd.Data, tt.want)
			}
		})
	}
}

func TestSTKResponseUnmarshalBinary(t *testing.T) {
	envelopeSupport := make([]byte, stkEnvelopeSupportLength)
	setBit(envelopeSupport, 0xD1)
	pacSupport := make([]byte, stkPACSupportLength)
	pacSupport[0x21] = byte(STKPACHandledByHostFunctionAbleToHandle)
	resultData := []byte{0xAA, 0xBB}
	terminalResponseInfo := binary.LittleEndian.AppendUint32(nil, 12)
	terminalResponseInfo = binary.LittleEndian.AppendUint32(terminalResponseInfo, uint32(len(resultData)))
	terminalResponseInfo = binary.LittleEndian.AppendUint32(terminalResponseInfo, 0x9000)
	terminalResponseInfo = append(terminalResponseInfo, resultData...)

	tests := []struct {
		name    string
		run     func([]byte) error
		data    []byte
		wantErr bool
	}{
		{
			name: "PAC info",
			data: pacSupport,
			run: func(data []byte) error {
				var got STKPACInfo
				if err := got.UnmarshalBinary(data); err != nil {
					return err
				}
				if got.PacSupport[0x21] != STKPACHandledByHostFunctionAbleToHandle {
					return fmt.Errorf("PacSupport[0x21] = %d", got.PacSupport[0x21])
				}
				return nil
			},
		},
		{
			name:    "PAC info truncated",
			data:    pacSupport[:stkPACSupportLength-1],
			run:     func(data []byte) error { return new(STKPACInfo).UnmarshalBinary(data) },
			wantErr: true,
		},
		{
			name: "PAC notification",
			data: append(binary.LittleEndian.AppendUint32(nil, uint32(STKPACTypeNotification)), envelopeSupport[:3]...),
			run: func(data []byte) error {
				var got STKPAC
				if err := got.UnmarshalBinary(data); err != nil {
					return err
				}
				if got.Type != STKPACTypeNotification {
					return fmt.Errorf("Type = %d", got.Type)
				}
				if !bytes.Equal(got.Command, envelopeSupport[:3]) {
					return fmt.Errorf("Command = %X", got.Command)
				}
				return nil
			},
		},
		{
			name:    "PAC notification truncated",
			data:    []byte{0x01, 0x00, 0x00},
			run:     func(data []byte) error { return new(STKPAC).UnmarshalBinary(data) },
			wantErr: true,
		},
		{
			name: "terminal response info",
			data: terminalResponseInfo,
			run: func(data []byte) error {
				var got STKTerminalResponseInfo
				if err := got.UnmarshalBinary(data); err != nil {
					return err
				}
				if got.StatusWords != 0x9000 {
					return fmt.Errorf("StatusWords = %#x", got.StatusWords)
				}
				if !bytes.Equal(got.ResultData, resultData) {
					return fmt.Errorf("ResultData = %X", got.ResultData)
				}
				return nil
			},
		},
		{
			name:    "terminal response info truncated",
			data:    terminalResponseInfo[:11],
			run:     func(data []byte) error { return new(STKTerminalResponseInfo).UnmarshalBinary(data) },
			wantErr: true,
		},
		{
			name: "envelope info",
			data: envelopeSupport,
			run: func(data []byte) error {
				var got STKEnvelopeInfo
				if err := got.UnmarshalBinary(data); err != nil {
					return err
				}
				if !got.Supports(0xD1) {
					return errors.New("D1 envelope not supported")
				}
				if got.Supports(0xD2) {
					return errors.New("D2 envelope unexpectedly supported")
				}
				return nil
			},
		},
		{
			name:    "envelope info truncated",
			data:    envelopeSupport[:stkEnvelopeSupportLength-1],
			run:     func(data []byte) error { return new(STKEnvelopeInfo).UnmarshalBinary(data) },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUiccATRRequestData(t *testing.T) {
	tests := []struct {
		name string
		req  *Request
		want []byte
	}{
		{
			name: "query",
			req:  (&UiccATRQueryRequest{TransactionID: 1}).Request(),
			want: nil,
		},
		{
			name: "query EX4",
			req: (&UiccATRQueryRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				SlotID:        1,
			}).Request(),
			want: mustDecodeHex(t, "01000000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.req.Command.(*Command)
			if command.ServiceID != ServiceMsUiccLowLevelAccess {
				t.Fatalf("ServiceID = % X, want MS UICC low level access", command.ServiceID)
			}
			if command.CommandID != CIDUiccATR || command.CommandType != CommandTypeQuery {
				t.Fatalf("command = cid %d type %d, want cid %d query", command.CommandID, command.CommandType, CIDUiccATR)
			}
			if !bytes.Equal(command.Data, tt.want) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.want)
			}
		})
	}
}

func TestUICCChannelRequestData(t *testing.T) {
	tests := []struct {
		name      string
		req       *Request
		commandID uint32
		want      []byte
	}{
		{
			name: "open channel",
			req: (&OpenChannelRequest{
				TransactionID: 1,
				ApplicationID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04},
				ChannelGroup:  uiccChannelGroupDefault,
			}).Request(),
			commandID: CIDUiccOpenChannel,
			want:      mustDecodeHex(t, "07000000100000000000000001000000A0000000871004"),
		},
		{
			name: "open channel EX4",
			req: (&OpenChannelRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				SlotID:        1,
				ApplicationID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04},
				ChannelGroup:  uiccChannelGroupDefault,
			}).Request(),
			commandID: CIDUiccOpenChannel,
			want:      mustDecodeHex(t, "0700000014000000000000000100000001000000A0000000871004"),
		},
		{
			name: "apdu",
			req: (&APDURequest{
				TransactionID:   1,
				Channel:         3,
				SecureMessaging: UiccSecureMessagingNone,
				ClassByteType:   UiccClassByteTypeInterIndustry,
				Command:         []byte{0x00, 0x88, 0x00, 0x81},
			}).Request(),
			commandID: CIDUiccAPDU,
			want:      mustDecodeHex(t, "030000000000000000000000040000001400000000880081"),
		},
		{
			name: "apdu EX4",
			req: (&APDURequest{
				TransactionID:   1,
				MBIMExVersion:   mbimExVersion40,
				SlotID:          1,
				Channel:         3,
				SecureMessaging: UiccSecureMessagingNone,
				ClassByteType:   UiccClassByteTypeInterIndustry,
				Command:         []byte{0x00, 0x88, 0x00, 0x81},
			}).Request(),
			commandID: CIDUiccAPDU,
			want:      mustDecodeHex(t, "03000000000000000000000004000000180000000100000000880081"),
		},
		{
			name: "close channel",
			req: (&CloseChannelRequest{
				TransactionID: 1,
				Channel:       3,
				ChannelGroup:  uiccChannelGroupDefault,
			}).Request(),
			commandID: CIDUiccCloseChannel,
			want:      mustDecodeHex(t, "0300000001000000"),
		},
		{
			name: "close channel EX4",
			req: (&CloseChannelRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				SlotID:        1,
				Channel:       3,
				ChannelGroup:  uiccChannelGroupDefault,
			}).Request(),
			commandID: CIDUiccCloseChannel,
			want:      mustDecodeHex(t, "030000000100000001000000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.req.Command.(*Command)
			if command.ServiceID != ServiceMsUiccLowLevelAccess {
				t.Fatalf("ServiceID = % X, want MS UICC low level access", command.ServiceID)
			}
			if command.CommandID != tt.commandID || command.CommandType != CommandTypeSet {
				t.Fatalf("command = cid %d type %d, want cid %d set", command.CommandID, command.CommandType, tt.commandID)
			}
			if !bytes.Equal(command.Data, tt.want) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.want)
			}
		})
	}
}

func TestUiccResetRequestData(t *testing.T) {
	tests := []struct {
		name        string
		req         *Request
		commandType CommandType
		want        []byte
	}{
		{
			name: "set",
			req: (&UiccResetSetRequest{
				TransactionID: 1,
				Action:        UiccPassThroughActionEnable,
			}).Request(),
			commandType: CommandTypeSet,
			want:        mustDecodeHex(t, "01000000"),
		},
		{
			name: "set EX4",
			req: (&UiccResetSetRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				SlotID:        1,
				Action:        UiccPassThroughActionEnable,
			}).Request(),
			commandType: CommandTypeSet,
			want:        mustDecodeHex(t, "0100000001000000"),
		},
		{
			name:        "query",
			req:         (&UiccResetQueryRequest{TransactionID: 1}).Request(),
			commandType: CommandTypeQuery,
			want:        nil,
		},
		{
			name: "query EX4",
			req: (&UiccResetQueryRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				SlotID:        1,
			}).Request(),
			commandType: CommandTypeQuery,
			want:        mustDecodeHex(t, "01000000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.req.Command.(*Command)
			if command.ServiceID != ServiceMsUiccLowLevelAccess {
				t.Fatalf("ServiceID = % X, want MS UICC low level access", command.ServiceID)
			}
			if command.CommandID != CIDUiccReset || command.CommandType != tt.commandType {
				t.Fatalf("command = cid %d type %d, want cid %d type %d", command.CommandID, command.CommandType, CIDUiccReset, tt.commandType)
			}
			if !bytes.Equal(command.Data, tt.want) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.want)
			}
		})
	}
}

func TestUiccATRResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []byte
		wantErr bool
	}{
		{
			name: "atr",
			data: mustDecodeHex(t, "03000000080000003B9F96"),
			want: []byte{0x3B, 0x9F, 0x96},
		},
		{
			name:    "truncated reference",
			data:    mustDecodeHex(t, "03000000"),
			wantErr: true,
		},
		{
			name:    "truncated value",
			data:    mustDecodeHex(t, "03000000080000003B9F"),
			wantErr: true,
		},
		{
			name:    "zero offset with nonzero size",
			data:    mustDecodeHex(t, "03000000000000003B9F96"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got UiccATRResponse
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !bytes.Equal(got.ATR, tt.want) {
				t.Fatalf("ATR = %X, want %X", got.ATR, tt.want)
			}
		})
	}
}

func TestUiccResetResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    UiccPassThroughStatus
		wantErr bool
	}{
		{
			name: "disabled",
			data: mustDecodeHex(t, "00000000"),
			want: UiccPassThroughStatusDisabled,
		},
		{
			name: "enabled",
			data: mustDecodeHex(t, "01000000"),
			want: UiccPassThroughStatusEnabled,
		},
		{
			name:    "truncated",
			data:    []byte{0x01},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got UiccResetResponse
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.PassThroughStatus != tt.want {
				t.Fatalf("PassThroughStatus = %d, want %d", got.PassThroughStatus, tt.want)
			}
		})
	}
}

func TestUiccTerminalCapabilityRequestData(t *testing.T) {
	terminalCapabilities := [][]byte{
		{0x0A, 0x0B, 0x0C, 0x0D, 0x0A},
		{0xA0, 0xB0, 0xC0},
	}
	terminalCapabilityPayload := mustDecodeHex(t, "0200000014000000050000001C000000030000000A0B0C0D0A000000A0B0C000")
	terminalCapabilityPayloadEx4 := mustDecodeHex(t, "0100000002000000180000000500000020000000030000000A0B0C0D0A000000A0B0C000")
	tests := []struct {
		name        string
		req         *Request
		commandType CommandType
		want        []byte
	}{
		{
			name: "set",
			req: (&UiccTerminalCapabilitySetRequest{
				TransactionID: 1,
				Capabilities:  terminalCapabilities,
			}).Request(),
			commandType: CommandTypeSet,
			want:        terminalCapabilityPayload,
		},
		{
			name: "set EX4",
			req: (&UiccTerminalCapabilitySetRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				SlotID:        1,
				Capabilities:  terminalCapabilities,
			}).Request(),
			commandType: CommandTypeSet,
			want:        terminalCapabilityPayloadEx4,
		},
		{
			name: "set empty",
			req: (&UiccTerminalCapabilitySetRequest{
				TransactionID: 1,
			}).Request(),
			commandType: CommandTypeSet,
			want:        mustDecodeHex(t, "00000000"),
		},
		{
			name: "set empty EX4",
			req: (&UiccTerminalCapabilitySetRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				SlotID:        1,
			}).Request(),
			commandType: CommandTypeSet,
			want:        mustDecodeHex(t, "0100000000000000"),
		},
		{
			name:        "query",
			req:         (&UiccTerminalCapabilityQueryRequest{TransactionID: 1}).Request(),
			commandType: CommandTypeQuery,
			want:        nil,
		},
		{
			name: "query EX4",
			req: (&UiccTerminalCapabilityQueryRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				SlotID:        1,
			}).Request(),
			commandType: CommandTypeQuery,
			want:        mustDecodeHex(t, "01000000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.req.Command.(*Command)
			if command.ServiceID != ServiceMsUiccLowLevelAccess {
				t.Fatalf("ServiceID = % X, want MS UICC low level access", command.ServiceID)
			}
			if command.CommandID != CIDUiccTerminalCapability || command.CommandType != tt.commandType {
				t.Fatalf("command = cid %d type %d, want cid %d type %d", command.CommandID, command.CommandType, CIDUiccTerminalCapability, tt.commandType)
			}
			if !bytes.Equal(command.Data, tt.want) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.want)
			}
		})
	}
}

func TestUiccTerminalCapabilityResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    [][]byte
		wantErr bool
	}{
		{
			name: "empty",
			data: mustDecodeHex(t, "00000000"),
			want: nil,
		},
		{
			name: "multiple",
			data: mustDecodeHex(t, "0200000014000000050000001C000000030000000A0B0C0D0A000000A0B0C000"),
			want: [][]byte{
				{0x0A, 0x0B, 0x0C, 0x0D, 0x0A},
				{0xA0, 0xB0, 0xC0},
			},
		},
		{
			name:    "truncated table",
			data:    mustDecodeHex(t, "01000000"),
			wantErr: true,
		},
		{
			name:    "truncated capability",
			data:    mustDecodeHex(t, "010000000C00000002000000AA"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got UiccTerminalCapabilityResponse
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got.Capabilities) != len(tt.want) {
				t.Fatalf("Capabilities length = %d, want %d", len(got.Capabilities), len(tt.want))
			}
			for i := range tt.want {
				if !slices.Equal(got.Capabilities[i], tt.want[i]) {
					t.Fatalf("Capabilities[%d] = %X, want %X", i, got.Capabilities[i], tt.want[i])
				}
			}
		})
	}
}

func TestUICCChannelResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		parse   func([]byte) ([]byte, uint32, error)
		data    []byte
		want    []byte
		status  uint32
		wantErr bool
	}{
		{
			name: "open channel",
			parse: func(data []byte) ([]byte, uint32, error) {
				var got OpenChannelResponse
				err := got.UnmarshalBinary(data)
				if got.Channel != 3 {
					return nil, 0, errors.New("channel mismatch")
				}
				return got.Response, got.Status, err
			},
			data:   mustDecodeHex(t, "900000000300000002000000100000009000"),
			want:   []byte{0x90, 0x00},
			status: 0x90,
		},
		{
			name: "apdu",
			parse: func(data []byte) ([]byte, uint32, error) {
				var got APDUResponse
				err := got.UnmarshalBinary(data)
				return got.Response, got.Status, err
			},
			data:   mustDecodeHex(t, "90000000020000000C000000DB00"),
			want:   []byte{0xDB, 0x00},
			status: 0x90,
		},
		{
			name: "close channel",
			parse: func(data []byte) ([]byte, uint32, error) {
				var got CloseChannelResponse
				err := got.UnmarshalBinary(data)
				return nil, got.Status, err
			},
			data:   mustDecodeHex(t, "90000000"),
			status: 0x90,
		},
		{
			name: "truncated apdu",
			parse: func(data []byte) ([]byte, uint32, error) {
				var got APDUResponse
				err := got.UnmarshalBinary(data)
				return got.Response, got.Status, err
			},
			data:    []byte{0x00},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, status, err := tt.parse(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if status != tt.status {
				t.Fatalf("Status = %d, want %d", status, tt.status)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("Response = %X, want %X", got, tt.want)
			}
		})
	}
}

func TestReaderQueryUiccATR(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []byte
	}{
		{
			name: "query",
			data: mustDecodeHex(t, "03000000080000003B9F96"),
			want: []byte{0x3B, 0x9F, 0x96},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			errc := make(chan error, 1)
			go func() {
				defer close(errc)
				defer server.Close()

				if err := expectMBIMCommandWithType(server, 1, CIDUiccATR, CommandTypeQuery, nil); err != nil {
					errc <- err
					return
				}
				if _, err := server.Write(mbimCommandDone(1, ServiceMsUiccLowLevelAccess, CIDUiccATR, tt.data)); err != nil {
					errc <- err
					return
				}
			}()

			reader := &Reader{conn: client}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			got, err := reader.QueryUiccATR(ctx)
			if err != nil {
				t.Fatalf("QueryUiccATR() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("QueryUiccATR() = %X, want %X", got, tt.want)
			}
			if err := <-errc; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReaderRadioState(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMCommandWithService(server, 1, ServiceBasicConnect, CIDRadioState, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(1, ServiceBasicConnect, CIDRadioState, radioStatePayload(RadioSwitchStateOn, RadioSwitchStateOff))); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := reader.RadioState(ctx)
	if err != nil {
		t.Fatalf("RadioState() error = %v", err)
	}
	if got.HwRadioState != RadioSwitchStateOn {
		t.Fatalf("HwRadioState = %d, want %d", got.HwRadioState, RadioSwitchStateOn)
	}
	if got.SwRadioState != RadioSwitchStateOff {
		t.Fatalf("SwRadioState = %d, want %d", got.SwRadioState, RadioSwitchStateOff)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderSetRadioState(t *testing.T) {
	tests := []struct {
		name  string
		state RadioSwitchState
	}{
		{name: "off", state: RadioSwitchStateOff},
		{name: "on", state: RadioSwitchStateOn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			errc := make(chan error, 1)
			go func() {
				defer close(errc)
				defer server.Close()

				wantRequest := binary.LittleEndian.AppendUint32(nil, uint32(tt.state))
				if err := expectMBIMCommandWithService(server, 1, ServiceBasicConnect, CIDRadioState, CommandTypeSet, wantRequest); err != nil {
					errc <- err
					return
				}
				if _, err := server.Write(mbimCommandDone(1, ServiceBasicConnect, CIDRadioState, radioStatePayload(RadioSwitchStateOn, tt.state))); err != nil {
					errc <- err
					return
				}
			}()

			reader := &Reader{conn: client}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			got, err := reader.SetRadioState(ctx, tt.state)
			if err != nil {
				t.Fatalf("SetRadioState() error = %v", err)
			}
			if got.SwRadioState != tt.state {
				t.Fatalf("SwRadioState = %d, want %d", got.SwRadioState, tt.state)
			}
			if err := <-errc; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReaderUICCChannelTransmitsAPDU(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	aid := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	rand := bytes.Repeat([]byte{0x11}, 16)
	autn := bytes.Repeat([]byte{0x22}, 16)
	apduResponse := []byte{0xDB, 0x01, 0xAA, 0x02, 0xBB, 0xCC, 0x03, 0xDD, 0xEE, 0xFF}

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMCommand(server, 1, CIDUiccOpenChannel, mustDecodeHex(t, "07000000100000000000000001000000A0000000871004")); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(1, ServiceMsUiccLowLevelAccess, CIDUiccOpenChannel, mbimUICCOpenChannelResponseData(0x90, 3, nil))); err != nil {
			errc <- err
			return
		}

		data := append([]byte{byte(len(rand))}, rand...)
		data = append(data, byte(len(autn)))
		data = append(data, autn...)
		wantRequest := apdu.Request{CLA: 0x00, INS: 0x88, P1: 0x00, P2: 0x81, Data: data}
		wantAPDU := wantRequest.APDU()
		wantAPDUData := binary.LittleEndian.AppendUint32(nil, 3)
		wantAPDUData = binary.LittleEndian.AppendUint32(wantAPDUData, uint32(UiccSecureMessagingNone))
		wantAPDUData = binary.LittleEndian.AppendUint32(wantAPDUData, uint32(UiccClassByteTypeInterIndustry))
		wantAPDUData = binary.LittleEndian.AppendUint32(wantAPDUData, uint32(len(wantAPDU)))
		wantAPDUData = binary.LittleEndian.AppendUint32(wantAPDUData, 20)
		wantAPDUData = append(wantAPDUData, wantAPDU...)
		if err := expectMBIMCommand(server, 2, CIDUiccAPDU, wantAPDUData); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(2, ServiceMsUiccLowLevelAccess, CIDUiccAPDU, mbimUICCResponseData(0x90, apduResponse))); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommand(server, 3, CIDUiccCloseChannel, mustDecodeHex(t, "0300000001000000")); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(3, ServiceMsUiccLowLevelAccess, CIDUiccCloseChannel, mustDecodeHex(t, "90000000"))); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	channel, err := reader.OpenChannel(ctx, aid)
	if err != nil {
		t.Fatalf("OpenChannel() error = %v", err)
	}
	data := append([]byte{byte(len(rand))}, rand...)
	data = append(data, byte(len(autn)))
	data = append(data, autn...)
	apduRequest := apdu.Request{CLA: 0x00, INS: 0x88, P1: 0x00, P2: 0x81, Data: data}
	req := apduRequest.APDU()
	got, status, err := reader.TransmitAPDU(ctx, channel, req)
	if err != nil {
		t.Fatalf("TransmitAPDU() error = %v", err)
	}
	if status != 0x90 {
		t.Fatalf("TransmitAPDU() status = %#x, want 0x90", status)
	}
	if want := apduResponse; !bytes.Equal(got, want) {
		t.Fatalf("TransmitAPDU() = %X, want %X", got, want)
	}
	if err := reader.CloseChannel(ctx, channel); err != nil {
		t.Fatalf("CloseChannel() error = %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderUiccResetAndTerminalCapability(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	terminalCapabilities := [][]byte{
		{0x0A, 0x0B, 0x0C, 0x0D, 0x0A},
		{0xA0, 0xB0, 0xC0},
	}
	terminalCapabilityPayload := mustDecodeHex(t, "0200000014000000050000001C000000030000000A0B0C0D0A000000A0B0C000")

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMCommandWithType(server, 1, CIDUiccReset, CommandTypeSet, mustDecodeHex(t, "01000000")); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(1, ServiceMsUiccLowLevelAccess, CIDUiccReset, mustDecodeHex(t, "01000000"))); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithType(server, 2, CIDUiccReset, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(2, ServiceMsUiccLowLevelAccess, CIDUiccReset, mustDecodeHex(t, "00000000"))); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithType(server, 3, CIDUiccTerminalCapability, CommandTypeSet, terminalCapabilityPayload); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(3, ServiceMsUiccLowLevelAccess, CIDUiccTerminalCapability, nil)); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithType(server, 4, CIDUiccTerminalCapability, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(4, ServiceMsUiccLowLevelAccess, CIDUiccTerminalCapability, terminalCapabilityPayload)); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resetStatus, err := reader.SetUiccReset(ctx, UiccPassThroughActionEnable)
	if err != nil {
		t.Fatalf("SetUiccReset() error = %v", err)
	}
	if resetStatus != UiccPassThroughStatusEnabled {
		t.Fatalf("SetUiccReset() status = %d, want %d", resetStatus, UiccPassThroughStatusEnabled)
	}

	resetStatus, err = reader.QueryUiccReset(ctx)
	if err != nil {
		t.Fatalf("QueryUiccReset() error = %v", err)
	}
	if resetStatus != UiccPassThroughStatusDisabled {
		t.Fatalf("QueryUiccReset() status = %d, want %d", resetStatus, UiccPassThroughStatusDisabled)
	}

	if err := reader.SetUiccTerminalCapability(ctx, terminalCapabilities); err != nil {
		t.Fatalf("SetUiccTerminalCapability() error = %v", err)
	}

	gotCapabilities, err := reader.QueryUiccTerminalCapability(ctx)
	if err != nil {
		t.Fatalf("QueryUiccTerminalCapability() error = %v", err)
	}
	if len(gotCapabilities) != len(terminalCapabilities) {
		t.Fatalf("QueryUiccTerminalCapability() length = %d, want %d", len(gotCapabilities), len(terminalCapabilities))
	}
	for i := range terminalCapabilities {
		if !slices.Equal(gotCapabilities[i], terminalCapabilities[i]) {
			t.Fatalf("QueryUiccTerminalCapability()[%d] = %X, want %X", i, gotCapabilities[i], terminalCapabilities[i])
		}
	}

	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderSTKEnvelopeChecksSupport(t *testing.T) {
	envelope := []byte{0xD1, 0x09, 0x82, 0x02, 0x83, 0x81, 0x8B, 0x03, 0x00, 0x7F, 0xF6}

	tests := []struct {
		name    string
		support bool
		wantErr bool
	}{
		{name: "supported", support: true},
		{name: "not expected", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			errc := make(chan error, 1)
			go func() {
				defer close(errc)
				defer server.Close()

				if err := expectMBIMCommandWithService(server, 1, ServiceSTK, CIDSTKEnvelope, CommandTypeQuery, nil); err != nil {
					errc <- err
					return
				}
				support := make([]byte, stkEnvelopeSupportLength)
				if tt.support {
					setBit(support, envelope[0])
				}
				if _, err := server.Write(mbimCommandDone(1, ServiceSTK, CIDSTKEnvelope, support)); err != nil {
					errc <- err
					return
				}
				if !tt.support {
					return
				}

				if err := expectMBIMCommandWithService(server, 2, ServiceSTK, CIDSTKEnvelope, CommandTypeSet, envelope); err != nil {
					errc <- err
					return
				}
				if _, err := server.Write(mbimCommandDone(2, ServiceSTK, CIDSTKEnvelope, nil)); err != nil {
					errc <- err
					return
				}
			}()

			reader := &Reader{conn: client}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			err := reader.STKEnvelope(ctx, envelope)
			if (err != nil) != tt.wantErr {
				t.Fatalf("STKEnvelope() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := <-errc; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReaderSTKEnvelopeUsesCachedSupport(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	envelope := []byte{0xD1, 0x09, 0x82, 0x02, 0x83, 0x81, 0x8B, 0x03, 0x00, 0x7F, 0xF6}
	support := make([]byte, stkEnvelopeSupportLength)
	setBit(support, envelope[0])

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMCommandWithService(server, 1, ServiceSTK, CIDSTKEnvelope, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(1, ServiceSTK, CIDSTKEnvelope, support)); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithService(server, 2, ServiceSTK, CIDSTKEnvelope, CommandTypeSet, envelope); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(2, ServiceSTK, CIDSTKEnvelope, nil)); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := reader.QuerySTKEnvelopeSupport(ctx); err != nil {
		t.Fatalf("QuerySTKEnvelopeSupport() error = %v", err)
	}
	if err := reader.STKEnvelope(ctx, envelope); err != nil {
		t.Fatalf("STKEnvelope() error = %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderSTKPACAndTerminalResponse(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	pacHostControl := bytes.Repeat([]byte{0x5A}, stkPACHostControlLength)
	pacSupport := make([]byte, stkPACSupportLength)
	pacSupport[0x21] = byte(STKPACHandledByHostFunctionAbleToHandle)
	terminalResponse := []byte{0x81, 0x03, 0x01, 0x21, 0x00}
	terminalRequest := binary.LittleEndian.AppendUint32(nil, uint32(len(terminalResponse)))
	terminalRequest = append(terminalRequest, terminalResponse...)
	terminalInfo := binary.LittleEndian.AppendUint32(nil, 12)
	terminalInfo = binary.LittleEndian.AppendUint32(terminalInfo, 0)
	terminalInfo = binary.LittleEndian.AppendUint32(terminalInfo, 0x9000)

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMCommandWithService(server, 1, ServiceSTK, CIDSTKPAC, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(1, ServiceSTK, CIDSTKPAC, pacSupport)); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithService(server, 2, ServiceSTK, CIDSTKPAC, CommandTypeSet, pacHostControl); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(2, ServiceSTK, CIDSTKPAC, pacSupport)); err != nil {
			errc <- err
			return
		}

		if err := expectMBIMCommandWithService(server, 3, ServiceSTK, CIDSTKTerminalResponse, CommandTypeSet, terminalRequest); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(3, ServiceSTK, CIDSTKTerminalResponse, terminalInfo)); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	info, err := reader.QuerySTKPAC(ctx)
	if err != nil {
		t.Fatalf("QuerySTKPAC() error = %v", err)
	}
	if info.PacSupport[0x21] != STKPACHandledByHostFunctionAbleToHandle {
		t.Fatalf("QuerySTKPAC().PacSupport[0x21] = %d", info.PacSupport[0x21])
	}

	info, err = reader.SetSTKPAC(ctx, pacHostControl)
	if err != nil {
		t.Fatalf("SetSTKPAC() error = %v", err)
	}
	if info.PacSupport[0x21] != STKPACHandledByHostFunctionAbleToHandle {
		t.Fatalf("SetSTKPAC().PacSupport[0x21] = %d", info.PacSupport[0x21])
	}

	terminalResp, err := reader.STKTerminalResponse(ctx, terminalResponse)
	if err != nil {
		t.Fatalf("STKTerminalResponse() error = %v", err)
	}
	if terminalResp.StatusWords != 0x9000 {
		t.Fatalf("STKTerminalResponse().StatusWords = %#x, want 0x9000", terminalResp.StatusWords)
	}

	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderReadSTKPAC(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	command := []byte{0xD0, 0x03, 0x81, 0x01, 0x21}
	payload := binary.LittleEndian.AppendUint32(nil, uint32(STKPACTypeProactiveCommand))
	payload = append(payload, command...)

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if _, err := server.Write(mbimIndication(ServiceSTK, CIDSTKPAC, payload)); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := reader.ReadSTKPAC(ctx)
	if err != nil {
		t.Fatalf("ReadSTKPAC() error = %v", err)
	}
	if got.Type != STKPACTypeProactiveCommand {
		t.Fatalf("ReadSTKPAC().Type = %d, want %d", got.Type, STKPACTypeProactiveCommand)
	}
	if !bytes.Equal(got.Command, command) {
		t.Fatalf("ReadSTKPAC().Command = %X, want %X", got.Command, command)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderReadSTKPACContinuesAfterDeadlineExceeded(t *testing.T) {
	command := []byte{0xD0, 0x03, 0x81, 0x01, 0x21}
	payload := binary.LittleEndian.AppendUint32(nil, uint32(STKPACTypeProactiveCommand))
	payload = append(payload, command...)
	conn := &deadlineExceededConn{
		read: bytes.NewReader(mbimIndication(ServiceSTK, CIDSTKPAC, payload)),
	}
	reader := &Reader{conn: conn}

	got, err := reader.ReadSTKPAC(context.Background())
	if err != nil {
		t.Fatalf("ReadSTKPAC() error = %v", err)
	}
	if !bytes.Equal(got.Command, command) {
		t.Fatalf("ReadSTKPAC().Command = %X, want %X", got.Command, command)
	}
	if reads := conn.reads.Load(); reads < 2 {
		t.Fatalf("Read() calls = %d, want at least 2", reads)
	}
}

func TestReaderQueuesSTKPACDuringCommand(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	command := []byte{0xD0, 0x03, 0x81, 0x01, 0x21}
	payload := binary.LittleEndian.AppendUint32(nil, uint32(STKPACTypeProactiveCommand))
	payload = append(payload, command...)
	atr := mustDecodeHex(t, "03000000080000003B9F96")

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMCommandWithType(server, 1, CIDUiccATR, CommandTypeQuery, nil); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimIndication(ServiceSTK, CIDSTKPAC, payload)); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDone(1, ServiceMsUiccLowLevelAccess, CIDUiccATR, atr)); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := reader.QueryUiccATR(ctx); err != nil {
		t.Fatalf("QueryUiccATR() error = %v", err)
	}
	got, err := reader.ReadSTKPAC(ctx)
	if err != nil {
		t.Fatalf("ReadSTKPAC() error = %v", err)
	}
	if !bytes.Equal(got.Command, command) {
		t.Fatalf("ReadSTKPAC().Command = %X, want %X", got.Command, command)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderReadSTKPACPreservesQueuedIndications(t *testing.T) {
	tests := []struct {
		name     string
		commands [][]byte
	}{
		{
			name: "two queued PACs",
			commands: [][]byte{
				{0xD0, 0x03, 0x81, 0x01, 0x21},
				{0xD0, 0x03, 0x81, 0x01, 0x22},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			atr := mustDecodeHex(t, "03000000080000003B9F96")
			errc := make(chan error, 1)
			go func() {
				defer close(errc)
				defer server.Close()

				if err := expectMBIMCommandWithType(server, 1, CIDUiccATR, CommandTypeQuery, nil); err != nil {
					errc <- err
					return
				}
				for _, command := range tt.commands {
					payload := binary.LittleEndian.AppendUint32(nil, uint32(STKPACTypeProactiveCommand))
					payload = append(payload, command...)
					if _, err := server.Write(mbimIndication(ServiceSTK, CIDSTKPAC, payload)); err != nil {
						errc <- err
						return
					}
				}
				if _, err := server.Write(mbimCommandDone(1, ServiceMsUiccLowLevelAccess, CIDUiccATR, atr)); err != nil {
					errc <- err
					return
				}
			}()

			reader := &Reader{conn: client}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			if _, err := reader.QueryUiccATR(ctx); err != nil {
				t.Fatalf("QueryUiccATR() error = %v", err)
			}
			for i, want := range tt.commands {
				got, err := reader.ReadSTKPAC(ctx)
				if err != nil {
					t.Fatalf("ReadSTKPAC(%d) error = %v", i, err)
				}
				if !bytes.Equal(got.Command, want) {
					t.Fatalf("ReadSTKPAC(%d).Command = %X, want %X", i, got.Command, want)
				}
			}
			if err := <-errc; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReaderWatchSTKPAC(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	command := []byte{0xD0, 0x03, 0x81, 0x01, 0x21}
	payload := binary.LittleEndian.AppendUint32(nil, uint32(STKPACTypeNotification))
	payload = append(payload, command...)

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if _, err := server.Write(mbimIndication(ServiceSTK, CIDSTKPAC, payload)); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pacs, err := reader.WatchSTKPAC(ctx)
	if err != nil {
		t.Fatalf("WatchSTKPAC() error = %v", err)
	}
	select {
	case got := <-pacs:
		if got.Type != STKPACTypeNotification {
			t.Fatalf("WatchSTKPAC().Type = %d, want %d", got.Type, STKPACTypeNotification)
		}
		if !bytes.Equal(got.Command, command) {
			t.Fatalf("WatchSTKPAC().Command = %X, want %X", got.Command, command)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReaderAuthenticateAKAReturnsAUTSOnSyncFailure(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	rand := bytes.Repeat([]byte{0x11}, 16)
	autn := bytes.Repeat([]byte{0x22}, 16)
	auts := bytes.Repeat([]byte{0xA5}, 14)
	wantRequest := append(slices.Clone(rand), autn...)

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer server.Close()

		if err := expectMBIMCommandWithService(server, 1, ServiceAuth, CIDAuthAKA, CommandTypeQuery, wantRequest); err != nil {
			errc <- err
			return
		}
		if _, err := server.Write(mbimCommandDoneStatus(1, ServiceAuth, CIDAuthAKA, StatusAuthSyncFailure, mbimAKAAuthInfo(nil, nil, nil, auts))); err != nil {
			errc <- err
			return
		}
	}()

	reader := &Reader{conn: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := reader.AuthenticateAKA(ctx, rand, autn)
	if !errors.Is(err, StatusAuthSyncFailure) {
		t.Fatalf("AuthenticateAKA() error = %v, want StatusAuthSyncFailure", err)
	}
	if got == nil {
		t.Fatal("AuthenticateAKA() response = nil")
	}
	if !bytes.Equal(got.AUTS, auts) {
		t.Fatalf("AuthenticateAKA().AUTS = %X, want %X", got.AUTS, auts)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestRequestTransmitAcceptsResponseLargerThanControlTransfer(t *testing.T) {
	payload := bytes.Repeat([]byte{0xDB}, defaultMaxControlTransfer+256)
	response := mbimCommandDone(1, ServiceMsUiccLowLevelAccess, CIDUiccAPDU, mbimUICCResponseData(0x90, payload))
	conn := &scriptMBIMConn{read: bytes.NewReader(response)}
	request := (&APDURequest{
		TransactionID:   1,
		Channel:         3,
		SecureMessaging: UiccSecureMessagingNone,
		ClassByteType:   UiccClassByteTypeInterIndustry,
		Command:         []byte{0x00, 0x88, 0x00, 0x81},
	}).Request()

	if err := request.Transmit(context.Background(), conn); err != nil {
		t.Fatalf("Transmit() error = %v", err)
	}
	got := request.Response.(*APDUResponse)
	if !bytes.Equal(got.Response, payload) {
		t.Fatalf("response length = %d, want %d", len(got.Response), len(payload))
	}
}

func TestRequestTransmitContinuesAfterDeadlineExceeded(t *testing.T) {
	conn := &deadlineExceededConn{
		read: bytes.NewReader(mbimOpenDone(1)),
	}
	req := (&OpenDeviceRequest{TransactionID: 1}).Request()

	if err := req.Transmit(context.Background(), conn); err != nil {
		t.Fatalf("Transmit() error = %v", err)
	}
	if reads := conn.reads.Load(); reads < 2 {
		t.Fatalf("Read() calls = %d, want at least 2", reads)
	}
}

func TestRequestTransmitSkipsMismatchedCommandDone(t *testing.T) {
	payload := subscriberReadyPayload(t, SubscriberReadyStateInitialized, "001010123456789", "89014103211118510720", ReadyInfoNone)
	mismatchedFrames, err := fragmentedMessage{
		data:         mbimCommandDone(1, ServiceBasicConnect, CIDUiccAPDU, bytes.Repeat([]byte{0xAA}, 80)),
		maxFrameSize: 64,
	}.Frames()
	if err != nil {
		t.Fatalf("Frames() error = %v", err)
	}
	frames := append([][]byte{
		mbimCommandDoneStatus(1, ServiceMsUiccLowLevelAccess, CIDUiccAPDU, StatusFailure, nil),
		mbimCommandDoneTruncatedResponse(1, ServiceMsUiccLowLevelAccess, CIDUiccAPDU),
	}, mismatchedFrames...)
	frames = append(frames, mbimCommandDone(1, ServiceBasicConnect, CIDSubscriberReadyStatus, payload))
	conn := &scriptMBIMConn{
		read: bytes.NewReader(bytes.Join(frames, nil)),
	}
	request := SubscriberReadyStatusRequest{TransactionID: 1}
	req := request.Request()

	if err := req.Transmit(context.Background(), conn); err != nil {
		t.Fatalf("Transmit() error = %v", err)
	}
	if request.Response.ReadyState != SubscriberReadyStateInitialized {
		t.Fatalf("ReadyState = %d, want %d", request.Response.ReadyState, SubscriberReadyStateInitialized)
	}
}

func TestDeviceSlotMappingsResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []SlotMapping
		wantErr bool
	}{
		{
			name: "empty",
			data: []byte{0, 0, 0, 0},
			want: nil,
		},
		{
			name: "single mapping",
			data: slotMappingsPayload(2),
			want: []SlotMapping{{Slot: 2}},
		},
		{
			name:    "truncated table",
			data:    []byte{1, 0, 0, 0},
			wantErr: true,
		},
		{
			name:    "bad slot size",
			data:    []byte{1, 0, 0, 0, 12, 0, 0, 0, 2, 0, 0, 0, 0, 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DeviceSlotMappingsResponse
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got.SlotMappings) != len(tt.want) {
				t.Fatalf("SlotMappings length = %d, want %d", len(got.SlotMappings), len(tt.want))
			}
			for i := range tt.want {
				if got.SlotMappings[i] != tt.want[i] {
					t.Fatalf("SlotMappings[%d] = %+v, want %+v", i, got.SlotMappings[i], tt.want[i])
				}
			}
		})
	}
}

func TestDeviceServicesResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name            string
		data            []byte
		wantServices    int
		wantVersionCID  bool
		wantMaxSessions uint32
		wantErr         bool
	}{
		{
			name:            "empty",
			data:            mustDecodeHex(t, "0000000003000000"),
			wantMaxSessions: 3,
		},
		{
			name: "basic connect extensions",
			data: deviceServicesPayload(DeviceService{
				ServiceID:       ServiceMsBasicConnectExtensions,
				MaxDSSInstances: 1,
				CIDs:            []uint32{CIDDeviceSlotMappings, CIDVersion},
			}),
			wantServices:    1,
			wantVersionCID:  true,
			wantMaxSessions: 3,
		},
		{
			name:    "truncated payload",
			data:    []byte{1, 0, 0, 0},
			wantErr: true,
		},
		{
			name:    "truncated service table",
			data:    mustDecodeHex(t, "0100000000000000"),
			wantErr: true,
		},
		{
			name:    "truncated CID list",
			data:    mustDecodeHex(t, "0100000003000000100000001C0000003D01DCC5FEF54D050D3ABEF7058E9AAF0000000000000000020000000F000000"),
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
			if tt.wantErr {
				return
			}
			if got.MaxDSSSessions != tt.wantMaxSessions {
				t.Fatalf("MaxDSSSessions = %d, want %d", got.MaxDSSSessions, tt.wantMaxSessions)
			}
			if len(got.Services) != tt.wantServices {
				t.Fatalf("Services length = %d, want %d", len(got.Services), tt.wantServices)
			}
			if got.SupportsCID(ServiceMsBasicConnectExtensions, CIDVersion) != tt.wantVersionCID {
				t.Fatalf("SupportsCID(version) = %v, want %v", got.SupportsCID(ServiceMsBasicConnectExtensions, CIDVersion), tt.wantVersionCID)
			}
		})
	}
}

func TestRadioStateInfoUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    RadioStateInfo
		wantErr bool
	}{
		{
			name: "radio state",
			data: radioStatePayload(RadioSwitchStateOn, RadioSwitchStateOff),
			want: RadioStateInfo{
				HwRadioState: RadioSwitchStateOn,
				SwRadioState: RadioSwitchStateOff,
			},
		},
		{name: "truncated", data: []byte{1, 0, 0, 0}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got RadioStateInfo
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSubscriberReadyStatusResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		data    []byte
		want    SubscriberReadyStatusResponse
		wantErr bool
	}{
		{
			name: "ready",
			data: subscriberReadyPayload(t, SubscriberReadyStateInitialized, "001010123456789", "89014103211118510720", ReadyInfoProtectUniqueID, "+15551234567"),
			want: SubscriberReadyStatusResponse{
				ReadyState:            SubscriberReadyStateInitialized,
				SubscriberID:          "001010123456789",
				SIMICCID:              "89014103211118510720",
				ReadyInfo:             ReadyInfoProtectUniqueID,
				TelephoneNumbersCount: 1,
				SlotID:                activeSubscriberSlot,
				TelephoneNumbers:      []string{"+15551234567"},
			},
		},
		{
			name:    "ready EX4",
			version: mbimExVersion40,
			data: subscriberReadyPayloadEx4(
				t,
				SubscriberReadyStateInitialized,
				SubscriberReadyStatusFlagESIM|SubscriberReadyStatusFlagSIMSlotActive,
				1,
				"001010123456789",
				"89014103211118510720",
				ReadyInfoProtectUniqueID,
				"+15551234567",
			),
			want: SubscriberReadyStatusResponse{
				MBIMExVersion:         mbimExVersion40,
				ReadyState:            SubscriberReadyStateInitialized,
				Flags:                 SubscriberReadyStatusFlagESIM | SubscriberReadyStatusFlagSIMSlotActive,
				SubscriberID:          "001010123456789",
				SIMICCID:              "89014103211118510720",
				ReadyInfo:             ReadyInfoProtectUniqueID,
				TelephoneNumbersCount: 1,
				SlotID:                1,
				TelephoneNumbers:      []string{"+15551234567"},
			},
		},
		{
			name:    "truncated",
			data:    []byte{0x01, 0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "truncated telephone table",
			data:    mustDecodeHex(t, "01000000000000000000000000000000000000000000000001000000"),
			wantErr: true,
		},
		{
			name: "zero string offset with nonzero size",
			data: corruptSubscriberReadyPayload(t, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 0)
			}),
			wantErr: true,
		},
		{
			name: "odd UTF-16 string size",
			data: corruptSubscriberReadyPayload(t, func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], 1)
			}),
			wantErr: true,
		},
		{
			name: "overlapping string buffers",
			data: corruptSubscriberReadyPayload(t, func(data []byte) {
				binary.LittleEndian.PutUint32(data[12:16], binary.LittleEndian.Uint32(data[4:8]))
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
			if tt.wantErr {
				return
			}
			if got.ReadyState != tt.want.ReadyState ||
				got.SubscriberID != tt.want.SubscriberID ||
				got.SIMICCID != tt.want.SIMICCID ||
				got.ReadyInfo != tt.want.ReadyInfo ||
				got.TelephoneNumbersCount != tt.want.TelephoneNumbersCount ||
				got.Flags != tt.want.Flags ||
				got.SlotID != tt.want.SlotID {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}
			if !slices.Equal(got.TelephoneNumbers, tt.want.TelephoneNumbers) {
				t.Fatalf("TelephoneNumbers = %+v, want %+v", got.TelephoneNumbers, tt.want.TelephoneNumbers)
			}
		})
	}
}

func TestApplicationListResponseUnmarshalBinary(t *testing.T) {
	payload := mustDecodeHex(t,
		"0100000001000000000000003C000000"+
			"180000003C000000"+
			"0400000020000000100000003000000008000000020000003800000002000000"+
			"A0000000871002FF34FF0789312E30FF"+
			"4D6F766973746172"+
			"01810000",
	)
	var got ApplicationListResponse
	if err := got.UnmarshalBinary(payload); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if got.Version != 1 || got.ActiveApplicationIndex != 0 || got.ApplicationListSizeBytes != 60 || len(got.Applications) != 1 {
		t.Fatalf("response = %+v", got)
	}
	app := got.Applications[0]
	if app.Type != UiccApplicationTypeUSIM {
		t.Fatalf("Application Type = %d, want %d", app.Type, UiccApplicationTypeUSIM)
	}
	if !bytes.Equal(app.AID, mustDecodeHex(t, "A0000000871002FF34FF0789312E30FF")) {
		t.Fatalf("Application AID = %X", app.AID)
	}
	if app.Label != "Movistar" {
		t.Fatalf("Application Label = %q, want Movistar", app.Label)
	}
	if app.PinKeyReferenceCount != 2 {
		t.Fatalf("Application PinKeyReferenceCount = %d, want 2", app.PinKeyReferenceCount)
	}
	if !bytes.Equal(app.PinKeyReferences, []byte{0x01, 0x81}) {
		t.Fatalf("Application PinKeyReferences = %X, want 0181", app.PinKeyReferences)
	}
}

func TestReadBinaryRequestData(t *testing.T) {
	req := (&ReadBinaryRequest{
		TransactionID: 1,
		ApplicationID: []byte{0xA0, 0x00},
		FilePath:      []byte{0x6F, 0xAD},
		Size:          4,
	}).Request()
	command := req.Command.(*Command)
	if command.CommandID != CIDUiccReadBinary {
		t.Fatalf("CommandID = %#x, want %#x", command.CommandID, CIDUiccReadBinary)
	}
	want := mustDecodeHex(t, "010000002C000000020000003000000002000000000000000400000000000000000000000000000000000000A00000006FAD0000")
	if !bytes.Equal(command.Data, want) {
		t.Fatalf("Data = %X, want %X", command.Data, want)
	}
}

func TestFileStatusRequestData(t *testing.T) {
	req := (&FileStatusRequest{
		TransactionID: 1,
		ApplicationID: []byte{0xA0, 0x00},
		FilePath:      []byte{0x6F, 0xAD},
	}).Request()
	command := req.Command.(*Command)
	if command.CommandID != CIDUiccFileStatus {
		t.Fatalf("CommandID = %#x, want %#x", command.CommandID, CIDUiccFileStatus)
	}
	want := mustDecodeHex(t, "0100000014000000020000001800000002000000A00000006FAD0000")
	if !bytes.Equal(command.Data, want) {
		t.Fatalf("Data = %X, want %X", command.Data, want)
	}
}

func TestFileStatusResponseUnmarshalBinary(t *testing.T) {
	data := mustDecodeHex(t,
		"01000000900000000000000002000000"+
			"01000000030000000400000009000000"+
			"01000000020000000300000004000000",
	)
	var got FileStatusResponse
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if got.Version != 1 ||
		got.StatusWord1 != 0x90 ||
		got.StatusWord2 != 0x00 ||
		got.FileAccessibility != UiccFileAccessibilityShareable ||
		got.FileType != UiccFileTypeWorkingEF ||
		got.FileStructure != UiccFileStructureLinear ||
		got.FileItemCount != 4 ||
		got.FileItemSize != 9 ||
		got.AccessConditionRead != PinTypeCustom ||
		got.AccessConditionUpdate != PinTypePIN1 ||
		got.AccessConditionActivate != PinTypePIN2 ||
		got.AccessConditionDeactivate != PinTypeDeviceSIM {
		t.Fatalf("UnmarshalBinary() = %+v", got)
	}
}

func TestReadBinaryResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		sw1     uint32
		sw2     uint32
		resp    []byte
		wantErr bool
	}{
		{
			name: "response",
			data: mustDecodeHex(t, "0100000090000000000000001400000002000000AABB"),
			sw1:  0x90,
			sw2:  0x00,
			resp: []byte{0xAA, 0xBB},
		},
		{
			name:    "truncated",
			data:    []byte{0x90},
			wantErr: true,
		},
		{
			name:    "truncated response",
			data:    mustDecodeHex(t, "0100000090000000000000001400000002000000AA"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ReadBinaryResponse
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.StatusWord1 != tt.sw1 || got.StatusWord2 != tt.sw2 {
				t.Fatalf("StatusWord = %#x%#x, want %#x%#x", got.StatusWord1, got.StatusWord2, tt.sw1, tt.sw2)
			}
			if !bytes.Equal(got.Data, tt.resp) {
				t.Fatalf("Data = %X, want %X", got.Data, tt.resp)
			}
		})
	}
}

func slotMappingsPayload(slot uint32) []byte {
	var data []byte
	data = binary.LittleEndian.AppendUint32(data, 1)
	data = binary.LittleEndian.AppendUint32(data, 12)
	data = binary.LittleEndian.AppendUint32(data, 4)
	data = binary.LittleEndian.AppendUint32(data, slot)
	return data
}

func corruptSubscriberReadyPayload(t *testing.T, mutate func([]byte)) []byte {
	t.Helper()
	data := subscriberReadyPayload(t, SubscriberReadyStateInitialized, "00101", "8901", ReadyInfoNone, "+1555")
	mutate(data)
	return data
}

func subscriberReadyPayload(t *testing.T, readyState SubscriberReadyState, subscriberID, iccid string, readyInfo ReadyInfo, numbers ...string) []byte {
	t.Helper()

	headerSize := 28 + len(numbers)*8
	data := make([]byte, 0, headerSize)
	data = binary.LittleEndian.AppendUint32(data, uint32(readyState))

	refs := make([][]byte, 0, 2+len(numbers))
	refs = append(refs, utf16Bytes(subscriberID), utf16Bytes(iccid))
	for _, number := range numbers {
		refs = append(refs, utf16Bytes(number))
	}

	offset := uint32(headerSize)
	for _, ref := range refs[:2] {
		data = binary.LittleEndian.AppendUint32(data, offset)
		data = binary.LittleEndian.AppendUint32(data, uint32(len(ref)))
		offset += uint32(len(ref))
	}
	data = binary.LittleEndian.AppendUint32(data, uint32(readyInfo))
	data = binary.LittleEndian.AppendUint32(data, uint32(len(numbers)))
	for _, ref := range refs[2:] {
		data = binary.LittleEndian.AppendUint32(data, offset)
		data = binary.LittleEndian.AppendUint32(data, uint32(len(ref)))
		offset += uint32(len(ref))
	}
	for _, ref := range refs {
		data = append(data, ref...)
	}
	return data
}

func subscriberReadyPayloadEx4(t *testing.T, readyState SubscriberReadyState, flags SubscriberReadyStatusFlags, slotID uint32, subscriberID, iccid string, readyInfo ReadyInfo, numbers ...string) []byte {
	t.Helper()

	headerSize := 36 + len(numbers)*8
	data := make([]byte, 0, headerSize)
	data = binary.LittleEndian.AppendUint32(data, uint32(readyState))
	data = binary.LittleEndian.AppendUint32(data, uint32(flags))

	refs := make([][]byte, 0, 2+len(numbers))
	refs = append(refs, utf16Bytes(subscriberID), utf16Bytes(iccid))
	for _, number := range numbers {
		refs = append(refs, utf16Bytes(number))
	}

	offset := uint32(headerSize)
	for _, ref := range refs[:2] {
		data = binary.LittleEndian.AppendUint32(data, offset)
		data = binary.LittleEndian.AppendUint32(data, uint32(len(ref)))
		offset += uint32(len(ref))
	}
	data = binary.LittleEndian.AppendUint32(data, uint32(readyInfo))
	data = binary.LittleEndian.AppendUint32(data, uint32(len(numbers)))
	data = binary.LittleEndian.AppendUint32(data, slotID)
	for _, ref := range refs[2:] {
		data = binary.LittleEndian.AppendUint32(data, offset)
		data = binary.LittleEndian.AppendUint32(data, uint32(len(ref)))
		offset += uint32(len(ref))
	}
	for _, ref := range refs {
		data = append(data, ref...)
	}
	return data
}

func deviceServicesPayload(services ...DeviceService) []byte {
	data := binary.LittleEndian.AppendUint32(nil, uint32(len(services)))
	data = binary.LittleEndian.AppendUint32(data, 3)

	elements := make([][]byte, len(services))
	offset := 8 + len(services)*8
	for i, service := range services {
		element := append([]byte(nil), service.ServiceID[:]...)
		element = binary.LittleEndian.AppendUint32(element, service.DSSPayload)
		element = binary.LittleEndian.AppendUint32(element, service.MaxDSSInstances)
		element = binary.LittleEndian.AppendUint32(element, uint32(len(service.CIDs)))
		for _, cid := range service.CIDs {
			element = binary.LittleEndian.AppendUint32(element, cid)
		}
		elements[i] = element
		data = binary.LittleEndian.AppendUint32(data, uint32(offset))
		data = binary.LittleEndian.AppendUint32(data, uint32(len(element)))
		offset += len(element)
	}

	for _, element := range elements {
		data = append(data, element...)
	}
	return data
}

func deviceCapsPayload(maxSessions uint32) []byte {
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data[28:32], maxSessions)
	return data
}

func radioStatePayload(hw, sw RadioSwitchState) []byte {
	data := binary.LittleEndian.AppendUint32(nil, uint32(hw))
	return binary.LittleEndian.AppendUint32(data, uint32(sw))
}

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	data, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	return data
}

func expectMBIMCommand(conn net.Conn, transactionID, commandID uint32, wantData []byte) error {
	return expectMBIMCommandWithType(conn, transactionID, commandID, CommandTypeSet, wantData)
}

func expectMBIMCommandWithType(conn net.Conn, transactionID, commandID uint32, commandType CommandType, wantData []byte) error {
	return expectMBIMCommandWithService(conn, transactionID, ServiceMsUiccLowLevelAccess, commandID, commandType, wantData)
}

func expectMBIMCommandWithService(conn net.Conn, transactionID uint32, service [16]byte, commandID uint32, commandType CommandType, wantData []byte) error {
	frame, err := readFrame(conn)
	if err != nil {
		return err
	}
	if got := binary.LittleEndian.Uint32(frame[8:12]); got != transactionID {
		return fmt.Errorf("transaction ID = %d, want %d", got, transactionID)
	}
	var gotService [16]byte
	copy(gotService[:], frame[20:36])
	if gotService != service {
		return fmt.Errorf("service = % X, want % X", gotService, service)
	}
	if got := binary.LittleEndian.Uint32(frame[36:40]); got != commandID {
		return fmt.Errorf("command ID = %d, want %d", got, commandID)
	}
	if got := CommandType(binary.LittleEndian.Uint32(frame[40:44])); got != commandType {
		return fmt.Errorf("command type = %d, want %d", got, commandType)
	}
	dataLength := binary.LittleEndian.Uint32(frame[44:48])
	if dataLength > uint32(len(frame)-48) {
		return errors.New("request data is truncated")
	}
	gotData := frame[48 : 48+dataLength]
	if !bytes.Equal(gotData, wantData) {
		return fmt.Errorf("request data = %X, want %X", gotData, wantData)
	}
	return nil
}

func expectMBIMOpen(conn net.Conn, transactionID uint32) error {
	frame, err := readFrame(conn)
	if err != nil {
		return err
	}
	if got := MessageType(binary.LittleEndian.Uint32(frame[:4])); got != MessageTypeOpen {
		return fmt.Errorf("message type = %#x, want %#x", got, MessageTypeOpen)
	}
	if got := binary.LittleEndian.Uint32(frame[8:12]); got != transactionID {
		return fmt.Errorf("transaction ID = %d, want %d", got, transactionID)
	}
	return nil
}

func expectNoMBIMCommand(conn net.Conn, timeout time.Duration) error {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	frame, err := readFrame(conn)
	if err == nil {
		return fmt.Errorf("unexpected MBIM command: %X", frame)
	}
	if !timeoutError(err) {
		return fmt.Errorf("reading next MBIM command: %w", err)
	}
	return nil
}

func mbimCommandDone(transactionID uint32, service [16]byte, commandID uint32, data []byte) []byte {
	return mbimCommandDoneStatus(transactionID, service, commandID, StatusNone, data)
}

func mbimCommandDoneStatus(transactionID uint32, service [16]byte, commandID uint32, status Status, data []byte) []byte {
	messageLength := uint32(48 + len(data))
	buf := binary.LittleEndian.AppendUint32(nil, uint32(MessageTypeCommandDone))
	buf = binary.LittleEndian.AppendUint32(buf, messageLength)
	buf = binary.LittleEndian.AppendUint32(buf, transactionID)
	buf = binary.LittleEndian.AppendUint32(buf, 1)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = append(buf, service[:]...)
	buf = binary.LittleEndian.AppendUint32(buf, commandID)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(status))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(data)))
	return append(buf, data...)
}

func mbimIndication(service [16]byte, commandID uint32, data []byte) []byte {
	messageLength := uint32(44 + len(data))
	buf := binary.LittleEndian.AppendUint32(nil, uint32(MessageTypeIndicateStatus))
	buf = binary.LittleEndian.AppendUint32(buf, messageLength)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = binary.LittleEndian.AppendUint32(buf, 1)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = append(buf, service[:]...)
	buf = binary.LittleEndian.AppendUint32(buf, commandID)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(data)))
	return append(buf, data...)
}

func mbimCommandDoneTruncatedResponse(transactionID uint32, service [16]byte, commandID uint32) []byte {
	buf := mbimCommandDoneStatus(transactionID, service, commandID, StatusNone, nil)
	binary.LittleEndian.PutUint32(buf[44:48], 4)
	return buf
}

func mbimOpenDone(transactionID uint32) []byte {
	buf := binary.LittleEndian.AppendUint32(nil, uint32(MessageTypeOpenDone))
	buf = binary.LittleEndian.AppendUint32(buf, 16)
	buf = binary.LittleEndian.AppendUint32(buf, transactionID)
	return binary.LittleEndian.AppendUint32(buf, uint32(StatusNone))
}

type scriptMBIMConn struct {
	read *bytes.Reader
}

func (c *scriptMBIMConn) Read(p []byte) (int, error)       { return c.read.Read(p) }
func (c *scriptMBIMConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *scriptMBIMConn) Close() error                     { return nil }
func (c *scriptMBIMConn) SetReadDeadline(time.Time) error  { return nil }
func (c *scriptMBIMConn) SetWriteDeadline(time.Time) error { return nil }
func (c *scriptMBIMConn) MaxControlTransfer() int          { return defaultMaxControlTransfer }

type deadlineExceededConn struct {
	scriptMBIMConn
	read  *bytes.Reader
	reads atomic.Int32
}

func (c *deadlineExceededConn) Read(p []byte) (int, error) {
	if c.reads.Add(1) == 1 {
		return 0, os.ErrDeadlineExceeded
	}
	return c.read.Read(p)
}

func mbimUICCResponseData(status uint32, response []byte) []byte {
	data := binary.LittleEndian.AppendUint32(nil, status)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(response)))
	data = binary.LittleEndian.AppendUint32(data, 12)
	return append(data, response...)
}

func mbimUICCOpenChannelResponseData(status, channel uint32, response []byte) []byte {
	data := binary.LittleEndian.AppendUint32(nil, status)
	data = binary.LittleEndian.AppendUint32(data, channel)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(response)))
	data = binary.LittleEndian.AppendUint32(data, 16)
	return append(data, response...)
}

func mbimAKAAuthInfo(res, ik, ck, auts []byte) []byte {
	data := make([]byte, 66)
	copy(data[:16], res)
	binary.LittleEndian.PutUint32(data[16:20], uint32(len(res)))
	copy(data[20:36], ik)
	copy(data[36:52], ck)
	copy(data[52:66], auts)
	return data
}

func setBit(data []byte, bit byte) {
	data[int(bit)/8] |= byte(1 << (bit % 8))
}
