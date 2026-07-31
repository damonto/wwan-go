package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestSARConfigMarshalBinary(t *testing.T) {
	states := []SARConfigState{
		{AntennaIndex: 2, BackoffIndex: 7},
		{AntennaIndex: 4, BackoffIndex: 9},
	}
	wantStates := mustDecodeHex(t, "0100000001000000020000001c00000008000000240000000800000002000000070000000400000009000000")
	tests := []struct {
		name    string
		config  SARConfig
		want    []byte
		wantErr bool
	}{
		{
			name: "OS controlled with two states",
			config: SARConfig{
				Mode:         SARControlModeOS,
				BackoffState: SARBackoffStateEnabled,
				States:       states,
			},
			want: wantStates,
		},
		{name: "zero value", config: SARConfig{}, want: make([]byte, 12)},
		{name: "reserved control mode", config: SARConfig{Mode: SARControlModeOS + 1}, wantErr: true},
		{name: "reserved backoff state", config: SARConfig{BackoffState: SARBackoffStateEnabled + 1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.config.MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = %x, want %x", got, tt.want)
			}
		})
	}
}

func TestSARConfigInfoUnmarshalBinary(t *testing.T) {
	valid := mustDecodeHex(t, "010000000100000000000000020000002000000008000000280000000800000002000000070000000400000009000000")
	want := SARConfigInfo{
		Mode:            SARControlModeOS,
		BackoffState:    SARBackoffStateEnabled,
		WiFiIntegration: SARWiFiHardwareStateIntegrated,
		States: []SARConfigState{
			{AntennaIndex: 2, BackoffIndex: 7},
			{AntennaIndex: 4, BackoffIndex: 9},
		},
	}
	tests := []struct {
		name    string
		data    []byte
		want    SARConfigInfo
		wantErr bool
	}{
		{name: "two states", data: valid, want: want},
		{
			name: "no states",
			data: mustDecodeHex(t, "00000000000000000100000000000000"),
			want: SARConfigInfo{WiFiIntegration: SARWiFiHardwareStateNotIntegrated, States: []SARConfigState{}},
		},
		{name: "truncated", data: valid[:15], wantErr: true},
		{
			name:    "reserved control mode",
			data:    mutateUint32ForSARTest(valid, 0, uint32(SARControlModeOS+1)),
			wantErr: true,
		},
		{
			name:    "reserved backoff state",
			data:    mutateUint32ForSARTest(valid, 4, uint32(SARBackoffStateEnabled+1)),
			wantErr: true,
		},
		{
			name:    "reserved Wi-Fi integration",
			data:    mutateUint32ForSARTest(valid, 8, uint32(SARWiFiHardwareStateNotIntegrated+1)),
			wantErr: true,
		},
		{name: "truncated reference table", data: valid[:23], wantErr: true},
		{
			name:    "wrong state size",
			data:    mutateUint32ForSARTest(valid, 20, 4),
			wantErr: true,
		},
		{name: "trailing data", data: append(bytes.Clone(valid), 0, 0, 0, 0), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SARConfigInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTransmissionStatusInfoUnmarshalBinary(t *testing.T) {
	valid := mustDecodeHex(t, "01000000010000001e000000")
	want := TransmissionStatusInfo{
		ChannelNotification: TransmissionNotificationStatusEnabled,
		State:               TransmissionStateActive,
		HysteresisTimer:     30,
	}
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "active", data: valid},
		{name: "truncated", data: valid[:11], wantErr: true},
		{name: "trailing data", data: append(bytes.Clone(valid), 0), wantErr: true},
		{
			name:    "reserved notification status",
			data:    mutateUint32ForSARTest(valid, 0, uint32(TransmissionNotificationStatusEnabled+1)),
			wantErr: true,
		},
		{
			name:    "reserved transmission state",
			data:    mutateUint32ForSARTest(valid, 4, uint32(TransmissionStateActive+1)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got TransmissionStatusInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != want {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestMSSARRequests(t *testing.T) {
	config := SARConfig{
		Mode:         SARControlModeOS,
		BackoffState: SARBackoffStateEnabled,
		States: []SARConfigState{
			{AntennaIndex: 2, BackoffIndex: 7},
			{AntennaIndex: 4, BackoffIndex: 9},
		},
	}
	configData := mustDecodeHex(t, "0100000001000000020000001c00000008000000240000000800000002000000070000000400000009000000")
	transmissionData := mustDecodeHex(t, "010000001e000000")
	tests := []struct {
		name        string
		request     *Request
		commandID   uint32
		commandType CommandType
		wantData    []byte
	}{
		{name: "configuration query", request: (&SARConfigRequest{TransactionID: 1}).Request(), commandID: CIDMsSARConfig, commandType: CommandTypeQuery},
		{
			name:        "configuration set",
			request:     (&SARConfigSetRequest{TransactionID: 1, Config: config}).Request(),
			commandID:   CIDMsSARConfig,
			commandType: CommandTypeSet,
			wantData:    configData,
		},
		{name: "transmission query", request: (&TransmissionStatusRequest{TransactionID: 1}).Request(), commandID: CIDMsSARTransmissionStatus, commandType: CommandTypeQuery},
		{
			name: "transmission set",
			request: (&TransmissionStatusSetRequest{
				TransactionID:       1,
				ChannelNotification: TransmissionNotificationStatusEnabled,
				HysteresisTimer:     30,
			}).Request(),
			commandID:   CIDMsSARTransmissionStatus,
			commandType: CommandTypeSet,
			wantData:    transmissionData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.request.Command.(*Command)
			if command.ServiceID != ServiceMsSAR || command.CommandID != tt.commandID || command.CommandType != tt.commandType {
				t.Fatalf("command = service %x CID %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if !bytes.Equal(command.Data, tt.wantData) {
				t.Fatalf("command data = %x, want %x", command.Data, tt.wantData)
			}
		})
	}
}

func TestMSSARClientAPIs(t *testing.T) {
	config := SARConfig{
		Mode:         SARControlModeOS,
		BackoffState: SARBackoffStateEnabled,
		States: []SARConfigState{
			{AntennaIndex: 2, BackoffIndex: 7},
			{AntennaIndex: 4, BackoffIndex: 9},
		},
	}
	configRequest := mustDecodeHex(t, "0100000001000000020000001c00000008000000240000000800000002000000070000000400000009000000")
	configResponse := mustDecodeHex(t, "010000000100000000000000020000002000000008000000280000000800000002000000070000000400000009000000")
	transmissionRequest := mustDecodeHex(t, "010000001e000000")
	transmissionResponse := mustDecodeHex(t, "01000000010000001e000000")
	tests := []struct {
		name        string
		commandID   uint32
		commandType CommandType
		requestData []byte
		response    []byte
		run         func(context.Context, *Client) error
	}{
		{
			name:        "configuration query",
			commandID:   CIDMsSARConfig,
			commandType: CommandTypeQuery,
			response:    configResponse,
			run: func(ctx context.Context, client *Client) error {
				got, err := client.SARConfig(ctx)
				if err == nil && len(got.States) != 2 {
					t.Fatalf("SARConfig() states = %v", got.States)
				}
				return err
			},
		},
		{
			name:        "configuration set",
			commandID:   CIDMsSARConfig,
			commandType: CommandTypeSet,
			requestData: configRequest,
			response:    configResponse,
			run: func(ctx context.Context, client *Client) error {
				got, err := client.SetSARConfig(ctx, config)
				if err == nil && len(got.States) != 2 {
					t.Fatalf("SetSARConfig() states = %v", got.States)
				}
				return err
			},
		},
		{
			name:        "transmission query",
			commandID:   CIDMsSARTransmissionStatus,
			commandType: CommandTypeQuery,
			response:    transmissionResponse,
			run: func(ctx context.Context, client *Client) error {
				got, err := client.TransmissionStatus(ctx)
				if err == nil && got.State != TransmissionStateActive {
					t.Fatalf("TransmissionStatus() = %+v", got)
				}
				return err
			},
		},
		{
			name:        "transmission set",
			commandID:   CIDMsSARTransmissionStatus,
			commandType: CommandTypeSet,
			requestData: transmissionRequest,
			response:    transmissionResponse,
			run: func(ctx context.Context, client *Client) error {
				got, err := client.SetTransmissionStatus(ctx, TransmissionNotificationStatusEnabled, 30)
				if err == nil && got.HysteresisTimer != 30 {
					t.Fatalf("SetTransmissionStatus() = %+v", got)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				if err := expectMBIMCommandWithService(serverConn, 1, ServiceMsSAR, tt.commandID, tt.commandType, tt.requestData); err != nil {
					errCh <- err
					return
				}
				_, err := serverConn.Write(mbimCommandDone(1, ServiceMsSAR, tt.commandID, tt.response))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn}
			if err := tt.run(ctx, client); err != nil {
				t.Fatalf("client API error = %v", err)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMSSARClientValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Client) error
	}{
		{
			name: "reserved control mode",
			run: func(client *Client) error {
				_, err := client.SetSARConfig(context.Background(), SARConfig{Mode: SARControlModeOS + 1})
				return err
			},
		},
		{
			name: "reserved backoff state",
			run: func(client *Client) error {
				_, err := client.SetSARConfig(context.Background(), SARConfig{BackoffState: SARBackoffStateEnabled + 1})
				return err
			},
		},
		{
			name: "reserved notification status",
			run: func(client *Client) error {
				_, err := client.SetTransmissionStatus(context.Background(), TransmissionNotificationStatusEnabled+1, 0)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(new(Client)); err == nil {
				t.Fatal("client API error = nil, want non-nil")
			}
		})
	}
}

func TestMSSARTransmissionNotificationAPIs(t *testing.T) {
	payload := mustDecodeHex(t, "01000000010000001e000000")
	tests := []struct {
		name  string
		watch bool
	}{
		{name: "read"},
		{name: "watch", watch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				_, err := serverConn.Write(mbimIndication(ServiceMsSAR, CIDMsSARTransmissionStatus, payload))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn}
			var got TransmissionStatusInfo
			if tt.watch {
				updates, err := client.WatchTransmissionStatus(ctx)
				if err != nil {
					t.Fatalf("WatchTransmissionStatus() error = %v", err)
				}
				select {
				case got = <-updates:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			} else {
				var err error
				got, err = client.ReadTransmissionStatus(ctx)
				if err != nil {
					t.Fatalf("ReadTransmissionStatus() error = %v", err)
				}
			}
			if got.State != TransmissionStateActive || got.HysteresisTimer != 30 {
				t.Fatalf("transmission status = %+v", got)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mutateUint32ForSARTest(data []byte, offset int, value uint32) []byte {
	result := bytes.Clone(data)
	binary.LittleEndian.PutUint32(result[offset:offset+4], value)
	return result
}
