package qcom

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSConfigureProfileEventListRequest(t *testing.T) {
	tests := []struct {
		name     string
		profiles []WDSProfileID
		want     []byte
		wantErr  string
	}{
		{name: "empty", want: []byte{0}},
		{
			name: "sorted and deduplicated",
			profiles: []WDSProfileID{
				{Type: WDSProfileType3GPP2, Index: 3},
				{Type: WDSProfileType3GPP, Index: 7},
				{Type: WDSProfileType3GPP, Index: 2},
				{Type: WDSProfileType3GPP, Index: 7},
			},
			want: []byte{3, byte(WDSProfileType3GPP), 2, byte(WDSProfileType3GPP), 7, byte(WDSProfileType3GPP2), 3},
		},
		{
			name:     "invalid profile type",
			profiles: []WDSProfileID{{Type: WDSProfileType(3), Index: 1}},
			wantErr:  "profile type",
		},
		{
			name:     "too many profiles",
			profiles: profileEventRegistrationsForTest(wdsMaxProfileEventRegistrations + 1),
			wantErr:  "exceeds maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (WDSConfigureProfileEventListRequest{
				ClientID: 7, TransactionID: 9, Timeout: 2 * time.Second, Profiles: tt.profiles,
			}).Request()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Request() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceWDS || got.ClientID != 7 || got.TransactionID != 9 ||
				got.Timeout != 2*time.Second || got.MessageID != MessageWDSConfigureProfileEventList {
				t.Fatalf("Request() = %+v", got)
			}
			assertTLV(t, got.TLVs, wdsTLVProfileEventRegister, tt.want)
		})
	}
}

func TestWDSProfileChangedIndicationUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDSProfileEvent
		wantErr bool
	}{
		{
			name: "created",
			tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{byte(WDSProfileType3GPP), 7, byte(WDSProfileCreated)})},
			want: WDSProfileEvent{
				Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: 7}, Change: WDSProfileCreated,
			},
		},
		{name: "missing", wantErr: true},
		{name: "length", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0, 1})}, wantErr: true},
		{name: "profile type", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{3, 1, 1})}, wantErr: true},
		{name: "event zero", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0, 1, 0})}, wantErr: true},
		{name: "event high", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0, 1, 5})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSProfileChangedIndication
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Event != tt.want {
				t.Fatalf("event = %+v, want %+v", got.Event, tt.want)
			}
		})
	}
}

func TestWDSConfigureProfileEvents(t *testing.T) {
	tests := []struct {
		name     string
		profiles []WDSProfileID
		resp     Response
		wantErr  bool
	}{
		{
			name:     "configure",
			profiles: []WDSProfileID{{Type: WDSProfileType3GPP, Index: 7}},
			resp:     successResponse(MessageWDSConfigureProfileEventList),
		},
		{
			name:     "modem error",
			profiles: []WDSProfileID{{Type: WDSProfileType3GPP, Index: 7}},
			resp:     errorResponse(MessageWDSConfigureProfileEventList, QMIErrorNotSupported),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageWDSConfigureProfileEventList {
						t.Fatalf("MessageID = 0x%04X", req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x10, []byte{1, byte(WDSProfileType3GPP), 7})
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			err := client.WDSConfigureProfileEvents(context.Background(), tt.profiles)
			if tt.wantErr {
				if err == nil {
					t.Fatal("WDSConfigureProfileEvents() error = nil")
				}
			} else if err != nil {
				t.Fatalf("WDSConfigureProfileEvents() error = %v", err)
			}
		})
	}
}

func TestWDSProfileEventReferences(t *testing.T) {
	profileA := WDSProfileID{Type: WDSProfileType3GPP, Index: 2}
	profileB := WDSProfileID{Type: WDSProfileType3GPP2, Index: 3}
	tests := []struct {
		name string
		run  func(*testing.T, *Client, *fakeTransport)
	}{
		{
			name: "shared and distinct profiles",
			run: func(t *testing.T, client *Client, transport *fakeTransport) {
				ctx := context.Background()
				if err := client.acquireWDSProfileEvents(ctx, 7, []WDSProfileID{profileA}); err != nil {
					t.Fatalf("first acquire error = %v", err)
				}
				if err := client.acquireWDSProfileEvents(ctx, 7, []WDSProfileID{profileA}); err != nil {
					t.Fatalf("second acquire error = %v", err)
				}
				if got := transport.callCount(); got != 2 {
					t.Fatalf("calls after shared acquire = %d, want 2", got)
				}
				if err := client.acquireWDSProfileEvents(ctx, 7, []WDSProfileID{profileB}); err != nil {
					t.Fatalf("distinct acquire error = %v", err)
				}
				client.releaseWDSProfileEvents([]WDSProfileID{profileA})
				if got := transport.callCount(); got != 3 {
					t.Fatalf("calls after partial release = %d, want 3", got)
				}
				client.releaseWDSProfileEvents([]WDSProfileID{profileA})
				client.releaseWDSProfileEvents([]WDSProfileID{profileB})
				if got := transport.callCount(); got != 6 {
					t.Fatalf("calls after final releases = %d, want 6", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) { assertTLV(t, req.TLVs, 0x10, []byte{1, 0, 2}) },
					resp:  successResponse(MessageWDSConfigureProfileEventList),
				},
				{
					check: func(req Request) { assertTLV(t, req.TLVs, 0x19, []byte{1}) },
					resp:  successResponse(MessageWDSIndicationRegister),
				},
				{
					check: func(req Request) { assertTLV(t, req.TLVs, 0x10, []byte{2, 0, 2, 1, 3}) },
					resp:  successResponse(MessageWDSConfigureProfileEventList),
				},
				{
					check: func(req Request) { assertTLV(t, req.TLVs, 0x10, []byte{1, 1, 3}) },
					resp:  successResponse(MessageWDSConfigureProfileEventList),
				},
				{
					check: func(req Request) { assertTLV(t, req.TLVs, 0x10, []byte{0}) },
					resp:  successResponse(MessageWDSConfigureProfileEventList),
				},
				{
					check: func(req Request) { assertTLV(t, req.TLVs, 0x19, []byte{0}) },
					resp:  successResponse(MessageWDSIndicationRegister),
				},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			tt.run(t, client, transport)
		})
	}
}

func TestWDSWatchProfileChanges(t *testing.T) {
	profile := WDSProfileID{Type: WDSProfileType3GPP, Index: 7}
	tests := []struct {
		name string
	}{
		{name: "forwards indication and unregisters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := &wdsProfileIndicationTransport{
				fakeTransport: fakeTransport{t: t, calls: []transportCall{
					{
						check: func(req Request) { assertTLV(t, req.TLVs, 0x10, []byte{1, 0, 7}) },
						resp:  successResponse(MessageWDSConfigureProfileEventList),
					},
					{
						check: func(req Request) { assertTLV(t, req.TLVs, 0x19, []byte{1}) },
						resp:  successResponse(MessageWDSIndicationRegister),
					},
					{
						check: func(req Request) { assertTLV(t, req.TLVs, 0x10, []byte{0}) },
						resp:  successResponse(MessageWDSConfigureProfileEventList),
					},
					{
						check: func(req Request) { assertTLV(t, req.TLVs, 0x19, []byte{0}) },
						resp:  successResponse(MessageWDSIndicationRegister),
					},
				}},
			}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			out, err := client.WDSWatchProfileChanges(ctx, []WDSProfileID{profile})
			if err != nil {
				t.Fatalf("WDSWatchProfileChanges() error = %v", err)
			}
			if transport.messageID != MessageWDSProfileChanged {
				t.Fatalf("Indications() message = 0x%04X", transport.messageID)
			}
			transport.emit(Indication{
				Service: ServiceWDS, ClientID: 7, MessageID: MessageWDSProfileChanged,
				TLVs: tlv.TLVs{tlv.Bytes(0x10, []byte{0, 7, byte(WDSProfileModified)})},
			})
			select {
			case got := <-out:
				want := WDSProfileEvent{Profile: profile, Change: WDSProfileModified}
				if got != want {
					t.Fatalf("event = %+v, want %+v", got, want)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for profile event")
			}
			cancel()
			transport.waitCalls(t, 4)
		})
	}
}

type wdsProfileIndicationTransport struct {
	fakeTransport
	indications chan Indication
	messageID   MessageID
}

func (t *wdsProfileIndicationTransport) Indications(
	ctx context.Context,
	_ ServiceType,
	_ uint8,
	id MessageID,
) (<-chan Indication, error) {
	t.messageID = id
	t.indications = make(chan Indication, 4)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *wdsProfileIndicationTransport) emit(indication Indication) {
	t.indications <- indication
}

func (t *wdsProfileIndicationTransport) waitCalls(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if t.callCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	tb.Fatalf("Do() calls = %d, want at least %d", t.callCount(), want)
}

func profileEventRegistrationsForTest(count int) []WDSProfileID {
	profiles := make([]WDSProfileID, count)
	for index := range profiles {
		profiles[index] = WDSProfileID{
			Type: WDSProfileType(index / 256), Index: uint8(index % 256),
		}
	}
	return profiles
}
