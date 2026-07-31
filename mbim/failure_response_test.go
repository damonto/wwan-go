package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

func TestBasicConnectFailureResponsesPreservePayload(t *testing.T) {
	tests := []struct {
		name        string
		commandID   uint32
		commandType CommandType
		requestData []byte
		response    func() []byte
		run         func(context.Context, *Client) (uint32, error)
		want        uint32
	}{
		{
			name:        "PIN",
			commandID:   CIDPin,
			commandType: CommandTypeQuery,
			response: func() []byte {
				data := make([]byte, 12)
				binary.LittleEndian.PutUint32(data[0:4], uint32(PinTypePIN1))
				binary.LittleEndian.PutUint32(data[4:8], uint32(PinStateLocked))
				binary.LittleEndian.PutUint32(data[8:12], 46)
				return data
			},
			run: func(ctx context.Context, client *Client) (uint32, error) {
				info, err := client.PIN(ctx)
				return info.RemainingAttempts, err
			},
			want: 46,
		},
		{
			name:        "register state",
			commandID:   CIDRegisterState,
			commandType: CommandTypeQuery,
			response: func() []byte {
				data := registrationStatePayloadForTest("310260", "Carrier", "Roaming")
				binary.LittleEndian.PutUint32(data[0:4], 42)
				return data
			},
			run: func(ctx context.Context, client *Client) (uint32, error) {
				info, err := client.RegistrationState(ctx)
				return info.NwError, err
			},
			want: 42,
		},
		{
			name:        "packet service set",
			commandID:   CIDPacketService,
			commandType: CommandTypeSet,
			requestData: binary.LittleEndian.AppendUint32(nil, uint32(PacketServiceActionAttach)),
			response: func() []byte {
				data := packetServicePayloadForTest(PacketServiceStateDetached)
				binary.LittleEndian.PutUint32(data[0:4], 43)
				return data
			},
			run: func(ctx context.Context, client *Client) (uint32, error) {
				info, err := client.SetPacketService(ctx, PacketServiceActionAttach)
				return info.NwError, err
			},
			want: 43,
		},
		{
			name:        "connect",
			commandID:   CIDConnect,
			commandType: CommandTypeQuery,
			requestData: func() []byte {
				data := make([]byte, 36)
				binary.LittleEndian.PutUint32(data[0:4], 7)
				return data
			}(),
			response: func() []byte {
				data := connectInfoPayloadForTest(7, ActivationStateDeactivated, ContextIPTypeDefault, ContextTypeInternet)
				binary.LittleEndian.PutUint32(data[32:36], 44)
				return data
			},
			run: func(ctx context.Context, client *Client) (uint32, error) {
				info, err := client.QueryConnect(ctx, 7)
				return info.NwError, err
			},
			want: 44,
		},
		{
			name:        "service activation",
			commandID:   CIDServiceActivation,
			commandType: CommandTypeSet,
			requestData: []byte{0xaa},
			response: func() []byte {
				return binary.LittleEndian.AppendUint32([]byte{}, 45)
			},
			run: func(ctx context.Context, client *Client) (uint32, error) {
				info, err := client.ActivateService(ctx, []byte{0xaa})
				return info.NwError, err
			},
			want: 45,
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
				if err := expectMBIMCommandWithService(
					serverConn,
					1,
					ServiceBasicConnect,
					tt.commandID,
					tt.commandType,
					tt.requestData,
				); err != nil {
					errCh <- err
					return
				}
				_, err := serverConn.Write(mbimCommandDoneStatus(
					1,
					ServiceBasicConnect,
					tt.commandID,
					StatusFailure,
					tt.response(),
				))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := tt.run(ctx, &Client{conn: clientConn})
			if !errors.Is(err, StatusFailure) {
				t.Fatalf("client call error = %v, want StatusFailure", err)
			}
			if got != tt.want {
				t.Fatalf("response marker = %d, want %d", got, tt.want)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}
