package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestCATServiceState(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want CATServiceState
	}{
		{
			name: "all masks",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x01, encodeCATMasks(0x00000003, 0x00000001)),
				tlv.Bytes(0x10, encodeCATMasks(0x00000004, 0x00000000)),
				tlv.Uint(0x11, uint32(0x0000001F)),
			},
			want: CATServiceState{
				RawGlobalMask:     0x00000003,
				RawClientMask:     0x00000001,
				DecodedGlobalMask: 0x00000004,
				DecodedClientMask: 0x00000000,
				FullFunctionMask:  0x0000001F,
			},
		},
		{
			name: "missing optional masks",
			want: CATServiceState{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceCAT2 || req.ClientID != 7 || req.MessageID != MessageCATGetServiceState {
						t.Fatalf("request = %+v", req)
					}
				},
				resp: successResponse(MessageCATGetServiceState, tt.tlvs...),
			}}}
			reader := &Client{
				transport:  transport,
				slot:       1,
				catService: ServiceCAT2,
				clientIDs:  map[ServiceType]uint8{ServiceCAT2: 7},
			}

			got, err := NewCAT(reader).ServiceState(context.Background())
			if err != nil {
				t.Fatalf("ServiceState() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ServiceState() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCATForceClaimEvents(t *testing.T) {
	tests := []struct {
		name       string
		clientID   uint8
		calls      []transportCall
		want       CATEventClaim
		wantErr    bool
		wantCalls  int
		wantCatCID uint8
	}{
		{
			name:     "claims without conflict",
			clientID: 7,
			calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceCAT2 || req.ClientID != 7 || req.MessageID != MessageCATSetEventReport {
						t.Fatalf("request = %+v", req)
					}
					assertTLV(t, req.TLVs, 0x10, []byte{0x01, 0x00, 0x00, 0x00})
					assertTLV(t, req.TLVs, 0x12, []byte{0x01})
				},
				resp: successResponse(MessageCATSetEventReport),
			}},
			want:       CATEventClaim{Service: ServiceCAT2, ClientID: 7},
			wantCalls:  1,
			wantCatCID: 7,
		},
		{
			name:     "releases lower owner",
			clientID: 5,
			calls: []transportCall{
				{
					resp: errorResponse(MessageCATSetEventReport, QMIErrorInvalidOperation, tlv.Uint(0x10, uint32(0x01))),
				},
				{
					check: func(req Request) {
						if req.MessageID != MessageCATGetServiceState {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, MessageCATGetServiceState)
						}
					},
					resp: successResponse(MessageCATGetServiceState, tlv.Bytes(0x01, encodeCATMasks(0x01, 0x00))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceCAT2 || req.ClientID != 1 || req.MessageID != MessageCATGetServiceState {
							t.Fatalf("request = %+v", req)
						}
					},
					resp: successResponse(MessageCATGetServiceState, tlv.Bytes(0x01, encodeCATMasks(0x01, 0x01))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageReleaseClientID {
							t.Fatalf("request = %+v", req)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceCAT2), 1})
					},
					resp: successResponse(MessageReleaseClientID),
				},
				{
					resp: successResponse(MessageCATSetEventReport),
				},
			},
			want: CATEventClaim{
				Service:          ServiceCAT2,
				ClientID:         5,
				ReleasedClientID: 1,
				StateBefore:      CATServiceState{RawGlobalMask: 1},
			},
			wantCalls:  5,
			wantCatCID: 5,
		},
		{
			name:     "finds higher owner",
			clientID: 1,
			calls: []transportCall{
				{
					resp: errorResponse(MessageCATSetEventReport, QMIErrorInvalidOperation, tlv.Uint(0x10, uint32(0x01))),
				},
				{
					resp: successResponse(MessageCATGetServiceState, tlv.Bytes(0x01, encodeCATMasks(0x01, 0x00))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceCAT2 || req.ClientID != 2 || req.MessageID != MessageCATGetServiceState {
							t.Fatalf("request = %+v", req)
						}
					},
					resp: successResponse(MessageCATGetServiceState, tlv.Bytes(0x01, encodeCATMasks(0x01, 0x00))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceCAT2 || req.ClientID != 3 || req.MessageID != MessageCATGetServiceState {
							t.Fatalf("request = %+v", req)
						}
					},
					resp: successResponse(MessageCATGetServiceState, tlv.Bytes(0x01, encodeCATMasks(0x01, 0x00))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceCAT2 || req.ClientID != 4 || req.MessageID != MessageCATGetServiceState {
							t.Fatalf("request = %+v", req)
						}
					},
					resp: successResponse(MessageCATGetServiceState, tlv.Bytes(0x01, encodeCATMasks(0x01, 0x00))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceCAT2 || req.ClientID != 5 || req.MessageID != MessageCATGetServiceState {
							t.Fatalf("request = %+v", req)
						}
					},
					resp: successResponse(MessageCATGetServiceState, tlv.Bytes(0x01, encodeCATMasks(0x01, 0x01))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageReleaseClientID {
							t.Fatalf("request = %+v", req)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceCAT2), 5})
					},
					resp: successResponse(MessageReleaseClientID),
				},
				{
					resp: successResponse(MessageCATSetEventReport),
				},
			},
			want: CATEventClaim{
				Service:          ServiceCAT2,
				ClientID:         1,
				ReleasedClientID: 5,
				StateBefore:      CATServiceState{RawGlobalMask: 1},
			},
			wantCalls:  8,
			wantCatCID: 1,
		},
		{
			name:     "rejects non raw conflict",
			clientID: 3,
			calls: []transportCall{
				{
					resp: errorResponse(MessageCATSetEventReport, QMIErrorInvalidOperation),
				},
				{
					resp: successResponse(MessageReleaseClientID),
				},
			},
			wantErr:    true,
			wantCalls:  2,
			wantCatCID: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: tt.calls}
			reader := &Client{
				transport:  transport,
				slot:       1,
				catService: ServiceCAT2,
				clientIDs:  map[ServiceType]uint8{ServiceCAT2: tt.clientID},
			}

			got, err := NewCAT(reader).ForceClaimEvents(context.Background(), CATEventClaimConfig{RawMask: 1})
			if tt.wantErr {
				if err == nil {
					t.Fatal("ForceClaimEvents() error = nil, want non-nil")
				}
			} else if err != nil {
				t.Fatalf("ForceClaimEvents() error = %v", err)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ForceClaimEvents() = %+v, want %+v", got, tt.want)
			}
			if got := transport.callCount(); got != tt.wantCalls {
				t.Fatalf("Do() calls = %d, want %d", got, tt.wantCalls)
			}
			if got := reader.clientIDs[reader.catService]; got != tt.wantCatCID {
				t.Fatalf("CAT client ID = %d, want %d", got, tt.wantCatCID)
			}
		})
	}
}

func TestCATForceClaimCommandsSubscribesBeforeClaim(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "setup menu",
			raw:  []byte{0xD0, 0x09, 0x81, 0x03, 0x01, 0x25, 0x00, 0x82, 0x02, 0x81, 0x82},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscribed := false
			releaseStarted := make(chan struct{})
			allowRelease := make(chan struct{})
			transport := &fakeIndicationTransport{
				onSubscribe: func() { subscribed = true },
				fakeTransport: fakeTransport{
					t: t,
					calls: []transportCall{
						{
							check: func(req Request) {
								if !subscribed {
									t.Fatal("CAT event registration started before indication subscription")
								}
								if req.MessageID != MessageCATSetEventReport {
									t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, MessageCATSetEventReport)
								}
							},
							resp: successResponse(MessageCATSetEventReport),
						},
						{
							check: func(req Request) {
								if req.MessageID != MessageReleaseClientID {
									t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, MessageReleaseClientID)
								}
								close(releaseStarted)
								<-allowRelease
							},
							resp: successResponse(MessageReleaseClientID),
						},
					},
				},
			}
			client := &Client{
				transport:  transport,
				slot:       1,
				catService: ServiceCAT2,
				clientIDs:  map[ServiceType]uint8{ServiceCAT2: 7},
			}

			ctx, cancel := context.WithCancel(context.Background())
			commands, claim, err := NewCAT(client).ForceClaimCommands(ctx, CATEventClaimConfig{RawMask: 1 << 3})
			if err != nil {
				cancel()
				t.Fatalf("ForceClaimCommands() error = %v", err)
			}
			if claim.Service != ServiceCAT2 || claim.ClientID != 7 {
				cancel()
				t.Fatalf("ForceClaimCommands() claim = %+v", claim)
			}

			value := binary.LittleEndian.AppendUint32(nil, 0x01020304)
			value = binary.LittleEndian.AppendUint16(value, uint16(len(tt.raw)))
			value = append(value, tt.raw...)
			transport.emit(Indication{TLVs: tlv.TLVs{tlv.Bytes(0x13, value)}})

			select {
			case got := <-commands:
				if got.Ref != 0x01020304 || !bytes.Equal(got.Data, tt.raw) {
					t.Fatalf("command = %+v, want ref 0x01020304 data % X", got, tt.raw)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for command")
			}
			cancel()
			select {
			case <-releaseStarted:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for CAT client release")
			}
			select {
			case _, ok := <-commands:
				if !ok {
					t.Fatal("commands closed before CAT client release completed")
				}
				t.Fatal("received unexpected command during CAT client release")
			default:
			}
			close(allowRelease)
			select {
			case _, ok := <-commands:
				if ok {
					t.Fatal("received unexpected command after cancellation")
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for commands to close")
			}
			if _, ok := client.clientIDs[ServiceCAT2]; ok {
				t.Fatal("CAT client is still retained after commands closed")
			}
		})
	}
}

func encodeCATMasks(global, client uint32) []byte {
	value := binary.LittleEndian.AppendUint32(nil, global)
	return binary.LittleEndian.AppendUint32(value, client)
}
