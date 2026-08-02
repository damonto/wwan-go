package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestMonitorTLVEncoding(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (tlv.TLVs, error)
		check   func(*testing.T, tlv.TLVs)
		wantErr bool
	}{
		{
			name: "card status register",
			build: func() (tlv.TLVs, error) {
				return registerEventsTLVs(eventRegistrationCardStatus), nil
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, binary.LittleEndian.AppendUint32(nil, eventRegistrationCardStatus))
			},
		},
		{
			name: "slot register",
			build: func() (tlv.TLVs, error) {
				return registerEventsTLVs(eventRegistrationPhysicalSlotStatus), nil
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, binary.LittleEndian.AppendUint32(nil, eventRegistrationPhysicalSlotStatus))
			},
		},
		{
			name: "refresh files",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterTLVs(RefreshRegisterRequest{
					Session:     SessionCardSlot1,
					AID:         []byte{0xA0, 0x00},
					VoteForInit: true,
					Files: []RefreshFile{{
						Path: []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0xAD},
					}},
				}, true)
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, []byte{byte(SessionCardSlot1), 0x02, 0xA0, 0x00})
				assertTLV(t, tlvs, 0x02, []byte{
					0x01, 0x01,
					0x01, 0x00,
					0xAD, 0x6F,
					0x04, 0x00, 0x3F, 0xFF, 0x7F,
				})
			},
		},
		{
			name: "refresh files unregister",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterTLVs(RefreshRegisterRequest{
					Session: SessionCardSlot1,
					Files:   []RefreshFile{{Path: []byte{0x3F, 0x00, 0x2F, 0xE2}}},
				}, false)
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x02, []byte{
					0x00, 0x00,
					0x01, 0x00,
					0xE2, 0x2F,
					0x02, 0x00, 0x3F,
				})
			},
		},
		{
			name: "refresh all",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterAllTLVs(SessionCardSlot1, nil, true)
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, []byte{byte(SessionCardSlot1), 0x00})
				assertTLV(t, tlvs, 0x02, []byte{0x01})
			},
		},
		{
			name: "refresh complete",
			build: func() (tlv.TLVs, error) {
				return refreshCompleteTLVs(SessionCardSlot1, []byte{0xA0, 0x00}, true)
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, []byte{byte(SessionCardSlot1), 0x02, 0xA0, 0x00})
				assertTLV(t, tlvs, 0x02, []byte{0x01})
			},
		},
		{
			name: "maximum refresh files",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterTLVs(RefreshRegisterRequest{
					Files: slices.Repeat([]RefreshFile{{Path: []byte{0x3F, 0x00, 0x2F, 0xE2}}}, uimRefreshFilesMax),
				}, true)
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				value, ok := tlv.Value(tlvs, 0x02)
				if !ok || len(value) < 4 {
					t.Fatalf("refresh register TLV = % X, present %t", value, ok)
				}
				if got := binary.LittleEndian.Uint16(value[2:4]); got != uimRefreshFilesMax {
					t.Fatalf("refresh file count = %d, want %d", got, uimRefreshFilesMax)
				}
			},
		},
		{
			name: "too many refresh files",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterTLVs(RefreshRegisterRequest{
					Files: slices.Repeat([]RefreshFile{{Path: []byte{0x3F, 0x00, 0x2F, 0xE2}}}, uimRefreshFilesMax+1),
				}, true)
			},
			wantErr: true,
		},
		{
			name: "reject odd path",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterTLVs(RefreshRegisterRequest{
					Files: []RefreshFile{{Path: []byte{0x3F}}},
				}, true)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.build()
			if tt.wantErr {
				if err == nil {
					t.Fatal("build() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("build() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestWatchCardStatus(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forwards ready card status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := &fakeIndicationTransport{
				fakeTransport: fakeTransport{
					t: t,
					calls: []transportCall{
						{
							check: func(req Request) {
								if req.MessageID != MessageRegisterEvents {
									t.Fatalf("MessageID = 0x%04X, want register events", req.MessageID)
								}
								assertTLV(t, req.TLVs, 0x01, binary.LittleEndian.AppendUint32(nil, eventRegistrationCardStatus))
							},
							resp: successResponse(MessageRegisterEvents),
						},
						{
							check: func(req Request) {
								assertTLV(t, req.TLVs, 0x01, []byte{0, 0, 0, 0})
							},
							resp: successResponse(MessageRegisterEvents),
						},
					},
				},
			}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			statuses, err := client.WatchCardStatus(ctx)
			if err != nil {
				t.Fatalf("WatchCardStatus() error = %v", err)
			}
			transport.emit(Indication{
				Service:   ServiceUIM,
				ClientID:  7,
				MessageID: MessageCardStatus,
				TLVs:      tlv.TLVs{tlv.Bytes(0x10, encodeCardStatus(true))},
			})

			select {
			case status := <-statuses:
				if !status.Ready() {
					t.Fatalf("card status = %+v, want ready USIM", status)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for card status")
			}

			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func TestDecodeRefreshEvent(t *testing.T) {
	eventValue := []byte{
		byte(RefreshStageStart),
		byte(RefreshModeFCN),
		byte(SessionCardSlot1),
		0x02, 0xA0, 0x00,
		0x01, 0x00,
		0xAD, 0x6F,
		0x04, 0x00, 0x3F, 0xFF, 0x7F,
	}

	var got RefreshEvent
	if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, eventValue)}); err != nil {
		t.Fatalf("decodeRefreshEvent() error = %v", err)
	}
	if got.Stage != RefreshStageStart || got.Mode != RefreshModeFCN || got.Session != SessionCardSlot1 {
		t.Fatalf("decodeRefreshEvent() = %+v", got)
	}
	if !bytes.Equal(got.AID, []byte{0xA0, 0x00}) {
		t.Fatalf("AID = % X, want A0 00", got.AID)
	}
	if len(got.Files) != 1 {
		t.Fatalf("Files length = %d, want 1", len(got.Files))
	}
	if got.Files[0].FileID != 0x6FAD || !bytes.Equal(got.Files[0].Path, []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0xAD}) {
		t.Fatalf("Files[0] = %+v", got.Files[0])
	}
}

func TestWatchSlotStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := &fakeIndicationTransport{
		fakeTransport: fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req Request) {
						if req.MessageID != MessageRegisterEvents {
							t.Fatalf("MessageID = 0x%04X, want register events", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, binary.LittleEndian.AppendUint32(nil, eventRegistrationPhysicalSlotStatus))
					},
					resp: successResponse(MessageRegisterEvents),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{0, 0, 0, 0})
					},
					resp: successResponse(MessageRegisterEvents),
				},
			},
		},
	}
	reader := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

	statuses, err := reader.WatchSlotStatus(ctx)
	if err != nil {
		t.Fatalf("WatchSlotStatus() error = %v", err)
	}
	transport.emit(Indication{
		Service:   ServiceUIM,
		ClientID:  7,
		MessageID: MessageSlotStatus,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x10, encodeSlotStatus(1)),
			tlv.Bytes(0x11, encodeSlotInformation()),
		},
	})

	select {
	case status := <-statuses:
		if status.ActiveSlot != 1 {
			t.Fatalf("ActiveSlot = %d, want 1", status.ActiveSlot)
		}
		if status.Slots[1].CardProtocol != CardProtocolUICC || !status.Slots[1].IsEUICC {
			t.Fatalf("Slots[1] = %+v, want UICC eUICC slot information", status.Slots[1])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for slot status")
	}

	cancel()
	transport.waitCalls(t, 2)
}

func TestUIMEventRegistrationReferences(t *testing.T) {
	tests := []struct {
		name string
		mask uint32
	}{
		{name: "card status", mask: eventRegistrationCardStatus},
		{name: "physical slot status", mask: eventRegistrationPhysicalSlotStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, binary.LittleEndian.AppendUint32(nil, tt.mask))
					},
					resp: successResponse(MessageRegisterEvents),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{0, 0, 0, 0})
					},
					resp: successResponse(MessageRegisterEvents),
				},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			if err := client.acquireUIMEvents(context.Background(), tt.mask); err != nil {
				t.Fatalf("first acquireUIMEvents() error = %v", err)
			}
			if err := client.acquireUIMEvents(context.Background(), tt.mask); err != nil {
				t.Fatalf("second acquireUIMEvents() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after two acquires = %d, want 1", got)
			}

			client.releaseUIMEvents(tt.mask)
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after first release = %d, want 1", got)
			}
			client.releaseUIMEvents(tt.mask)
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after final release = %d, want 2", got)
			}
		})
	}
}

func TestWatchRefreshFilesCompletesStartEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventValue := []byte{
		byte(RefreshStageStart),
		byte(RefreshModeFCN),
		byte(SessionCardSlot1),
		0x00,
		0x01, 0x00,
		0xAD, 0x6F,
		0x04, 0x00, 0x3F, 0xFF, 0x7F,
	}
	transport := &fakeIndicationTransport{
		fakeTransport: fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req Request) {
						if req.MessageID != MessageRefreshRegister {
							t.Fatalf("MessageID = 0x%04X, want refresh register", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x02, []byte{
							0x01, 0x00,
							0x01, 0x00,
							0xAD, 0x6F,
							0x04, 0x00, 0x3F, 0xFF, 0x7F,
						})
					},
					resp: successResponse(MessageRefreshRegister),
				},
				{
					check: func(req Request) {
						if req.MessageID != MessageRefreshComplete {
							t.Fatalf("MessageID = 0x%04X, want refresh complete", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionCardSlot1), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{0x01})
					},
					resp: successResponse(MessageRefreshComplete),
				},
				{
					check: func(req Request) {
						if req.MessageID != MessageRefreshRegister {
							t.Fatalf("MessageID = 0x%04X, want refresh unregister", req.MessageID)
						}
						value, ok := tlv.Value(req.TLVs, 0x02)
						if !ok || len(value) == 0 || value[0] != 0 {
							t.Fatalf("unregister info TLV = % X, want register flag 0", value)
						}
					},
					resp: successResponse(MessageRefreshRegister),
				},
			},
		},
	}
	reader := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

	events, err := reader.WatchRefreshFiles(ctx, RefreshRegisterRequest{
		Session: SessionCardSlot1,
		Files: []RefreshFile{{
			Path: []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0xAD},
		}},
	})
	if err != nil {
		t.Fatalf("WatchRefreshFiles() error = %v", err)
	}
	transport.emit(Indication{
		Service:   ServiceUIM,
		ClientID:  7,
		MessageID: MessageRefresh,
		TLVs:      tlv.TLVs{tlv.Bytes(0x10, eventValue)},
	})

	select {
	case event := <-events:
		if event.Stage != RefreshStageStart || event.Mode != RefreshModeFCN {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh event")
	}

	transport.waitCalls(t, 2)
	cancel()
	transport.waitCalls(t, 3)
}

func TestWatchRefreshFilesCompletesWhenConsumerIsSlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventValue := []byte{
		byte(RefreshStageStart),
		byte(RefreshModeFCN),
		byte(SessionCardSlot1),
		0x00,
		0x00, 0x00,
	}
	calls := []transportCall{
		{
			check: func(req Request) {
				if req.MessageID != MessageRefreshRegister {
					t.Fatalf("MessageID = 0x%04X, want refresh register", req.MessageID)
				}
			},
			resp: successResponse(MessageRefreshRegister),
		},
	}
	for range 10 {
		calls = append(calls, transportCall{
			check: func(req Request) {
				if req.MessageID != MessageRefreshComplete {
					t.Fatalf("MessageID = 0x%04X, want refresh complete", req.MessageID)
				}
			},
			resp: successResponse(MessageRefreshComplete),
		})
	}
	calls = append(calls, transportCall{
		check: func(req Request) {
			if req.MessageID != MessageRefreshRegister {
				t.Fatalf("MessageID = 0x%04X, want refresh unregister", req.MessageID)
			}
		},
		resp: successResponse(MessageRefreshRegister),
	})

	transport := &fakeIndicationTransport{
		fakeTransport: fakeTransport{
			t:     t,
			calls: calls,
		},
	}
	reader := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

	_, err := reader.WatchRefreshFiles(ctx, RefreshRegisterRequest{
		Session: SessionCardSlot1,
		Files: []RefreshFile{{
			Path: []byte{0x3F, 0x00, 0x2F, 0xE2},
		}},
	})
	if err != nil {
		t.Fatalf("WatchRefreshFiles() error = %v", err)
	}
	for range 10 {
		transport.emit(Indication{
			Service:   ServiceUIM,
			ClientID:  7,
			MessageID: MessageRefresh,
			TLVs:      tlv.TLVs{tlv.Bytes(0x10, eventValue)},
		})
	}

	transport.waitCalls(t, 11)
	cancel()
	transport.waitCalls(t, 12)
}

type fakeIndicationTransport struct {
	fakeTransport
	indications chan Indication
	onSubscribe func()
}

func (t *fakeIndicationTransport) Indications(ctx context.Context, _ ServiceType, _ uint8, _ MessageID) (<-chan Indication, error) {
	if t.onSubscribe != nil {
		t.onSubscribe()
	}
	t.indications = make(chan Indication, 8)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *fakeIndicationTransport) emit(ind Indication) {
	t.indications <- ind
}

func (t *fakeIndicationTransport) waitCalls(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if t.callCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	tb.Fatalf("Do() calls = %d, want at least %d", t.callCount(), want)
}
