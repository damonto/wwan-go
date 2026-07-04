package usim

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	usimcard "github.com/damonto/uicc-go/usim/card"
	"github.com/damonto/uicc-go/usim/command"
	"github.com/damonto/uicc-go/usim/stk"
	"github.com/damonto/uicc-go/usim/tlv"
)

type apduScriptTransmitter struct {
	t     *testing.T
	steps []apduScriptStep
	idx   int
}

type apduScriptStep struct {
	wantAPDU string
	resp     []byte
	err      error
}

func mustMarshal(t *testing.T, value interface{ MarshalBinary() ([]byte, error) }) []byte {
	t.Helper()
	data, err := value.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	return data
}

func (s *apduScriptTransmitter) Transmit(_ context.Context, req []byte) ([]byte, error) {
	s.t.Helper()
	if s.idx >= len(s.steps) {
		s.t.Fatalf("Transmit() got unexpected request %X", req)
	}
	step := s.steps[s.idx]
	s.idx++
	if got := strings.ToUpper(hex.EncodeToString(req)); got != step.wantAPDU {
		s.t.Fatalf("Transmit() APDU = %s, want %s", got, step.wantAPDU)
	}
	if step.err != nil {
		return nil, step.err
	}
	return step.resp, nil
}

func (s *apduScriptTransmitter) Close() error {
	return nil
}

func newAPDUReader(t *testing.T, tx usimcard.Transmitter) *Reader {
	t.Helper()
	reader, err := NewReader(tx)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	return reader
}

func TestReaderImplementsReader(t *testing.T) {
	var _ usimcard.Reader = (*Reader)(nil)
}

func TestNewRejectsNilTransmitter(t *testing.T) {
	tests := []struct {
		name string
		tx   usimcard.Transmitter
	}{
		{name: "nil transmitter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewReader(tt.tx); err == nil {
				t.Fatal("NewReader() error = nil, want non-nil")
			}
		})
	}
}

func TestReaderReadTransparentUsesExpectedAPDUs(t *testing.T) {
	aid := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
	reader := newAPDUReader(t, &apduScriptTransmitter{
		t: t,
		steps: []apduScriptStep{
			{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
			{wantAPDU: "00A40004026F07", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x09, 0x90, 0x00}},
			{wantAPDU: "00B0000009", resp: []byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98, 0x90, 0x00}},
		},
	})

	got, err := reader.ReadTransparent(context.Background(), usimcard.TransparentRead{
		File:   usimcard.FileRef{AID: aid, Path: []byte{0x6F, 0x07}},
		Length: 9,
	})
	if err != nil {
		t.Fatalf("ReadTransparent() error = %v", err)
	}
	if gotText := strings.ToUpper(hex.EncodeToString(got)); gotText != "080910101032547698" {
		t.Fatalf("ReadTransparent() = %s", gotText)
	}
}

func TestReaderReadRecordFollowsGetResponse(t *testing.T) {
	reader := newAPDUReader(t, &apduScriptTransmitter{
		t: t,
		steps: []apduScriptStep{
			{wantAPDU: "00A40004023F00", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
			{wantAPDU: "00A40004022F00", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x14, 0x01, 0x80, 0x02, 0x00, 0x14, 0x90, 0x00}},
			{wantAPDU: "00B2010414", resp: []byte{0x61, 0x0F, 0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02, 0x50, 0x04, 0x55, 0x53, 0x49, 0x4D, 0x90, 0x00}},
			{wantAPDU: "00C000000F", resp: []byte{0x61, 0x0F, 0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02, 0x50, 0x04, 0x55, 0x53, 0x49, 0x4D, 0x90, 0x00}},
		},
	})

	apps, err := reader.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("ListApplications() error = %v", err)
	}
	if len(apps) != 1 || strings.ToUpper(hex.EncodeToString(apps[0].AID)) != "A0000000871002" {
		t.Fatalf("ListApplications() = %+v", apps)
	}
}

func TestReaderAuthenticate3GUsesExpectedAPDU(t *testing.T) {
	rand := []byte{0x01, 0x02, 0x03, 0x04}
	autn := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	want := mustMarshal(t, command.Authenticate3G{Rand: rand, AUTN: autn})

	reader := newAPDUReader(t, &apduScriptTransmitter{
		t: t,
		steps: []apduScriptStep{
			{
				wantAPDU: strings.ToUpper(hex.EncodeToString(want)),
				resp:     []byte{0xDB, 0x02, 0x11, 0x22, 0x90, 0x00},
			},
		},
	})

	got, err := reader.Authenticate3G(context.Background(), usimcard.AuthenticateRequest{Rand: rand, AUTN: autn})
	if err != nil {
		t.Fatalf("Authenticate3G() error = %v", err)
	}
	if gotText := strings.ToUpper(hex.EncodeToString(got)); gotText != "DB021122" {
		t.Fatalf("Authenticate3G() = %s", gotText)
	}
}

func TestReaderSMSPPDownloadUsesEnvelopeAPDU(t *testing.T) {
	reader := newAPDUReader(t, &apduScriptTransmitter{
		t: t,
		steps: []apduScriptStep{
			{
				wantAPDU: "80C2000010D10E8202838186039121438B03007FF600",
				resp:     []byte{0x91, 0x08},
			},
		},
	})

	got, err := reader.SMSPPDownload(context.Background(), usimcard.SMSPPDownloadRequest{
		ServiceCenterAddress: "+1234",
		TPDU:                 []byte{0x00, 0x7F, 0xF6},
	})
	if err != nil {
		t.Fatalf("SMSPPDownload() error = %v", err)
	}
	if got.SW1 != 0x91 || got.SW2 != 0x08 {
		t.Fatalf("SMSPPDownload() status = %02X%02X, want 9108", got.SW1, got.SW2)
	}
}

func TestReaderAuthenticate3GSelectsApplicationWhenAIDProvided(t *testing.T) {
	aid := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
	rand := []byte{0x01, 0x02, 0x03, 0x04}
	autn := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	want := mustMarshal(t, command.Authenticate3G{Rand: rand, AUTN: autn})

	reader := newAPDUReader(t, &apduScriptTransmitter{
		t: t,
		steps: []apduScriptStep{
			{
				wantAPDU: "00A4040407A0000000871002",
				resp:     []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00},
			},
			{
				wantAPDU: strings.ToUpper(hex.EncodeToString(want)),
				resp:     []byte{0xDB, 0x02, 0x11, 0x22, 0x90, 0x00},
			},
		},
	})

	got, err := reader.Authenticate3G(context.Background(), usimcard.AuthenticateRequest{
		AID:  aid,
		Rand: rand,
		AUTN: autn,
	})
	if err != nil {
		t.Fatalf("Authenticate3G() error = %v", err)
	}
	if gotText := strings.ToUpper(hex.EncodeToString(got)); gotText != "DB021122" {
		t.Fatalf("Authenticate3G() = %s", gotText)
	}
}

func TestReaderAuthenticate3GClearsSelectedFileCache(t *testing.T) {
	aid := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
	rand := []byte{0x01, 0x02, 0x03, 0x04}
	autn := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	auth := mustMarshal(t, command.Authenticate3G{Rand: rand, AUTN: autn})

	reader := newAPDUReader(t, &apduScriptTransmitter{
		t: t,
		steps: []apduScriptStep{
			{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
			{wantAPDU: "00A40004026F07", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x09, 0x90, 0x00}},
			{wantAPDU: "00B0000009", resp: []byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98, 0x90, 0x00}},
			{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
			{wantAPDU: strings.ToUpper(hex.EncodeToString(auth)), resp: []byte{0xDB, 0x02, 0x11, 0x22, 0x90, 0x00}},
			{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
			{wantAPDU: "00A40004026F07", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x09, 0x90, 0x00}},
			{wantAPDU: "00B0000009", resp: []byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98, 0x90, 0x00}},
		},
	})

	req := usimcard.TransparentRead{
		File:   usimcard.FileRef{AID: aid, Path: []byte{0x6F, 0x07}},
		Length: 9,
	}
	if _, err := reader.ReadTransparent(context.Background(), req); err != nil {
		t.Fatalf("ReadTransparent() before Authenticate3G error = %v", err)
	}
	if _, err := reader.Authenticate3G(context.Background(), usimcard.AuthenticateRequest{
		AID:  aid,
		Rand: rand,
		AUTN: autn,
	}); err != nil {
		t.Fatalf("Authenticate3G() error = %v", err)
	}
	if _, err := reader.ReadTransparent(context.Background(), req); err != nil {
		t.Fatalf("ReadTransparent() after Authenticate3G error = %v", err)
	}
}

func TestReaderCloseDelegatesToTransmitter(t *testing.T) {
	tx := &closingTransmitter{}
	reader := newAPDUReader(t, tx)
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !tx.closed {
		t.Fatal("Close() did not close transmitter")
	}
}

type closingTransmitter struct {
	closed bool
}

func (t *closingTransmitter) Transmit(context.Context, []byte) ([]byte, error) {
	return nil, errors.New("unexpected transmit")
}

func (t *closingTransmitter) Close() error {
	t.closed = true
	return nil
}

func TestReaderSTKFetchesPendingCommand(t *testing.T) {
	raw := proactive(t,
		tlv.NewComprehension(0x01, []byte{0x01, byte(stk.CommandDisplayText), 0x00}),
		tlv.NewComprehension(0x02, []byte{byte(stk.DeviceUICC), byte(stk.DeviceDisplay)}),
		tlv.NewComprehension(0x0D, []byte{0x04, 'H', 'i'}),
	)
	tx := &fakeAPDUTransmitter{
		t: t,
		calls: []apduCall{
			{want: []byte{0x80, 0x10, 0x00, 0x00, 0x01, 0x01}, resp: []byte{0x90, 0x00}},
			{want: []byte{0x80, 0xF2, 0x00, 0x00, 0x00}, resp: []byte{0x91, byte(len(raw))}},
			{want: []byte{0x80, 0x12, 0x00, 0x00, byte(len(raw))}, resp: append(append([]byte(nil), raw...), 0x90, 0x00)},
		},
	}
	reader := newTestSTKReader(t, tx)
	reader.stkPollInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	commands, err := reader.Commands(ctx, stk.Profile{Data: []byte{0x01}})
	if err != nil {
		t.Fatalf("Commands() error = %v", err)
	}

	select {
	case got := <-commands:
		if got.Command.CommandDetails().Type != stk.CommandDisplayText {
			t.Fatalf("command type = 0x%02X, want DisplayText", got.Command.CommandDetails().Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command")
	}
}

func TestReaderSTKTerminalResponseAPDU(t *testing.T) {
	tx := &fakeAPDUTransmitter{
		t: t,
		calls: []apduCall{{
			want: []byte{0x80, 0x14, 0x00, 0x00, 0x03, 0x81, 0x01, 0x00},
			resp: []byte{0x90, 0x00},
		}},
	}
	reader := newTestSTKReader(t, tx)
	if err := reader.TerminalResponse(context.Background(), 0, []byte{0x81, 0x01, 0x00}); err != nil {
		t.Fatalf("TerminalResponse() error = %v", err)
	}
}

func TestReaderSTKExtendedTerminalResponseAPDU(t *testing.T) {
	response := bytes.Repeat([]byte{0xAA}, 0x100)
	want := []byte{0x80, 0x14, 0x00, 0x00, 0x00, 0x01, 0x00}
	want = append(want, response...)
	tx := &fakeAPDUTransmitter{
		t: t,
		calls: []apduCall{{
			want: want,
			resp: []byte{0x90, 0x00},
		}},
	}
	reader := newTestSTKReader(t, tx)
	if err := reader.TerminalResponse(context.Background(), 0, response); err != nil {
		t.Fatalf("TerminalResponse() error = %v", err)
	}
}

func TestReaderSTKEnvelopeAPDU(t *testing.T) {
	envelope, err := stk.MenuSelection(0x02, false).MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	want := []byte{0x80, 0xC2, 0x00, 0x00, byte(len(envelope))}
	want = append(want, envelope...)
	want = append(want, 0x00)
	tx := &fakeAPDUTransmitter{
		t: t,
		calls: []apduCall{{
			want: want,
			resp: []byte{0x90, 0x00},
		}},
	}
	reader := newTestSTKReader(t, tx)
	resp, err := reader.Envelope(context.Background(), envelope)
	if err != nil {
		t.Fatalf("Envelope() error = %v", err)
	}
	if !resp.OK() {
		t.Fatalf("Envelope() response = %02X%02X, want 9000", resp.SW1, resp.SW2)
	}
}

func newTestSTKReader(t *testing.T, tx *fakeAPDUTransmitter) *Reader {
	t.Helper()
	r, err := NewReader(tx)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	return r
}

type apduCall struct {
	want []byte
	resp []byte
}

type fakeAPDUTransmitter struct {
	t     *testing.T
	calls []apduCall
	idx   int
}

func (t *fakeAPDUTransmitter) Transmit(_ context.Context, req []byte) ([]byte, error) {
	t.t.Helper()
	if t.idx >= len(t.calls) {
		t.t.Fatalf("Transmit() unexpected request % X", req)
	}
	call := t.calls[t.idx]
	t.idx++
	if !bytes.Equal(req, call.want) {
		t.t.Fatalf("Transmit() request = % X, want % X", req, call.want)
	}
	return append([]byte(nil), call.resp...), nil
}

func (t *fakeAPDUTransmitter) Close() error {
	return nil
}
