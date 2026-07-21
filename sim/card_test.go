package sim

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/apdu"
	simcard "github.com/damonto/wwan-go/sim/card"
	"github.com/damonto/wwan-go/sim/command"
	"github.com/damonto/wwan-go/sim/simfile"
)

type scriptTransmitter struct {
	steps []scriptStep
	idx   int
}

type scriptStep struct {
	wantAPDU string
	resp     []byte
}

type loadReader struct {
	impiErr error
	impuErr error
}

func (s *scriptTransmitter) Close() error { return nil }

func (s *scriptTransmitter) Transmit(_ context.Context, req []byte) ([]byte, error) {
	step := s.steps[s.idx]
	s.idx++
	if got := stringsUpperHex(req); got != step.wantAPDU {
		return nil, apdu.StatusError{SW: 0x6F00}
	}
	return step.resp, nil
}

func (r *loadReader) Close() error { return nil }

func (r *loadReader) ListApplications(context.Context) ([]simcard.Application, error) {
	return []simcard.Application{
		{AID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}, Label: "USIM"},
		{AID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}, Label: "ISIM"},
	}, nil
}

func (r *loadReader) FileAttributes(_ context.Context, file simcard.FileRef) (simcard.FileAttributes, error) {
	switch stringsUpperHex(file.Path) {
	case "2FE2":
		return transparentFile(10), nil
	case "6F07":
		return transparentFile(9), nil
	case "6FAD":
		return transparentFile(4), nil
	case "6F02":
		return transparentFile(32), nil
	case "6F04":
		return linearFixedFile(32, 1), nil
	case "6F3E", "6F42", "7F106FE5":
		return simcard.FileAttributes{}, errors.New("file not found")
	default:
		return simcard.FileAttributes{}, errors.New("unexpected file")
	}
}

func (r *loadReader) ReadTransparent(_ context.Context, req simcard.TransparentRead) ([]byte, error) {
	switch stringsUpperHex(req.File.Path) {
	case "2FE2":
		return []byte{0x98, 0x68, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0}, nil
	case "6F07":
		return []byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98}, nil
	case "6FAD":
		return []byte{0x00, 0x00, 0x00, 0x02}, nil
	case "6F02":
		if r.impiErr != nil {
			return nil, r.impiErr
		}
		return tlvTextBinary("alice@ims.example.com", 32), nil
	default:
		return nil, errors.New("unexpected transparent read")
	}
}

func (r *loadReader) ReadRecord(_ context.Context, req simcard.RecordRead) ([]byte, error) {
	if stringsUpperHex(req.File.Path) != "6F04" {
		return nil, errors.New("unexpected record read")
	}
	if r.impuErr != nil {
		return nil, r.impuErr
	}
	return tlvTextRecord("sip:alice@ims.example.com", 32), nil
}

func (r *loadReader) Authenticate3G(context.Context, simcard.AuthenticateRequest) ([]byte, error) {
	return nil, errors.New("unexpected authentication")
}

func (r *loadReader) SMSPPDownload(context.Context, simcard.SMSPPDownloadRequest) (simcard.SMSPPDownloadResponse, error) {
	return simcard.SMSPPDownloadResponse{}, errors.New("unexpected SMS-PP download")
}

func transparentFile(size uint16) simcard.FileAttributes {
	return simcard.FileAttributes{
		FileStructure: simfile.StructureTransparent,
		FileSize:      size,
	}
}

func linearFixedFile(recordSize, recordCount uint16) simcard.FileAttributes {
	return simcard.FileAttributes{
		FileStructure: simfile.StructureLinearFixed,
		RecordSize:    recordSize,
		RecordCount:   recordCount,
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		tx      simcard.Reader
		logger  *slog.Logger
		wantErr bool
	}{
		{name: "nil reader", wantErr: true},
		{name: "default logger", tx: &loadReader{}},
		{name: "custom logger", tx: &loadReader{}, logger: slog.Default()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card, err := New(context.Background(), tt.tx, tt.logger)
			if tt.wantErr {
				if err == nil {
					t.Fatal("New() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if card.logger == nil {
				t.Fatal("New().logger = nil")
			}
		})
	}
}

func TestMNCFromIMSI(t *testing.T) {
	tests := []struct {
		name      string
		imsi      string
		mncLength int
		want      string
		wantErr   bool
	}{
		{
			name:      "two digit MNC",
			imsi:      "001010123456789",
			mncLength: 2,
			want:      "01",
		},
		{
			name:      "three digit MNC",
			imsi:      "001001123456789",
			mncLength: 3,
			want:      "001",
		},
		{
			name:      "IMSI too short",
			imsi:      "0010",
			mncLength: 2,
			wantErr:   true,
		},
		{
			name:      "unsupported MNC length",
			imsi:      "001010123456789",
			mncLength: 1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mncFromIMSI(tt.imsi, tt.mncLength)
			if tt.wantErr {
				if err == nil {
					t.Fatal("mncFromIMSI() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("mncFromIMSI() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("mncFromIMSI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCardAKA(t *testing.T) {
	aid := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
	rand := bytes.Repeat([]byte{0x01}, 16)
	autn := bytes.Repeat([]byte{0x02}, 16)
	res := []byte{0x11, 0x22, 0x33, 0x44}
	ck := bytes.Repeat([]byte{0xAA}, 16)
	ik := bytes.Repeat([]byte{0xBB}, 16)
	auts := bytes.Repeat([]byte{0xAA}, 14)

	tests := []struct {
		name  string
		body  []byte
		check func(t *testing.T, got AKAResult)
	}{
		{
			name: "success",
			body: append(append(append(append([]byte{0xDB, byte(len(res))}, res...), byte(len(ck))), ck...), append([]byte{byte(len(ik))}, ik...)...),
			check: func(t *testing.T, got AKAResult) {
				t.Helper()
				if !bytes.Equal(got.RES, res) || !bytes.Equal(got.CK, ck) || !bytes.Equal(got.IK, ik) {
					t.Fatalf("AKA() = %+v, want RES=%X CK=%X IK=%X", got, res, ck, ik)
				}
			},
		},
		{
			name: "synchronization failure",
			body: append([]byte{0xDC, byte(len(auts))}, auts...),
			check: func(t *testing.T, got AKAResult) {
				t.Helper()
				if !got.SynchronizationFailed() {
					t.Fatalf("AKA() = %+v, want synchronization failure", got)
				}
				if !bytes.Equal(got.AUTS, auts) {
					t.Fatalf("AKA().AUTS = %X, want %X", got.AUTS, auts)
				}
			},
		},
		{
			name: "authentication reject",
			body: []byte{0xDC, 0x00},
			check: func(t *testing.T, got AKAResult) {
				t.Helper()
				if !got.AuthenticationRejected() {
					t.Fatalf("AKA() = %+v, want authentication reject", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mustMarshal(t, command.Authenticate3G{Rand: rand, AUTN: autn})
			card := &Card{
				tx: newAPDUReader(t, &scriptTransmitter{steps: []scriptStep{
					{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
					{wantAPDU: stringsUpperHex(req), resp: []byte{0x61, byte(len(tt.body))}},
					{wantAPDU: "00C00000" + stringsUpperHex([]byte{byte(len(tt.body))}), resp: append(append([]byte{}, tt.body...), 0x90, 0x00)},
				}}),
				logger: slog.Default(),
				aid:    aid,
			}

			got, err := card.AKA(context.Background(), rand, autn)
			if err != nil {
				t.Fatalf("AKA() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestCardAKAFallsBackToISIM(t *testing.T) {
	rand := bytes.Repeat([]byte{0x01}, 16)
	autn := bytes.Repeat([]byte{0x02}, 16)
	res := []byte{0x11, 0x22, 0x33, 0x44}
	ck := bytes.Repeat([]byte{0xAA}, 16)
	ik := bytes.Repeat([]byte{0xBB}, 16)
	body := append(append(append(append([]byte{0xDB, byte(len(res))}, res...), byte(len(ck))), ck...), append([]byte{byte(len(ik))}, ik...)...)

	card := &Card{
		tx: &akaFallbackReader{
			usimAID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02},
			isimAID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04},
			body:    body,
		},
		logger:  slog.Default(),
		aid:     []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02},
		isimAID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04},
	}

	got, err := card.AKA(context.Background(), rand, autn)
	if err != nil {
		t.Fatalf("AKA() error = %v", err)
	}
	if !bytes.Equal(got.RES, res) {
		t.Fatalf("AKA().RES = %X, want %X", got.RES, res)
	}
}

type akaFallbackReader struct {
	usimAID []byte
	isimAID []byte
	body    []byte
}

func (r *akaFallbackReader) Close() error { return nil }
func (r *akaFallbackReader) ListApplications(context.Context) ([]simcard.Application, error) {
	return nil, errors.New("unexpected list applications")
}
func (r *akaFallbackReader) FileAttributes(context.Context, simcard.FileRef) (simcard.FileAttributes, error) {
	return simcard.FileAttributes{}, errors.New("unexpected file attributes")
}
func (r *akaFallbackReader) ReadTransparent(context.Context, simcard.TransparentRead) ([]byte, error) {
	return nil, errors.New("unexpected transparent read")
}
func (r *akaFallbackReader) ReadRecord(context.Context, simcard.RecordRead) ([]byte, error) {
	return nil, errors.New("unexpected record read")
}
func (r *akaFallbackReader) Authenticate3G(_ context.Context, req simcard.AuthenticateRequest) ([]byte, error) {
	switch {
	case bytes.Equal(req.AID, r.usimAID):
		return nil, errors.New("authentication failed")
	case bytes.Equal(req.AID, r.isimAID):
		return r.body, nil
	default:
		return nil, errors.New("unexpected aid")
	}
}
func (r *akaFallbackReader) SMSPPDownload(context.Context, simcard.SMSPPDownloadRequest) (simcard.SMSPPDownloadResponse, error) {
	return simcard.SMSPPDownloadResponse{}, errors.New("unexpected SMS-PP download")
}

func TestNewISIMMandatoryFileErrors(t *testing.T) {
	tests := []struct {
		name        string
		steps       []scriptStep
		wantErrText string
	}{
		{
			name: "impi read failure",
			steps: []scriptStep{
				{wantAPDU: "00A40004023F00", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
				{wantAPDU: "00A40004022F00", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x14, 0x01, 0x80, 0x02, 0x00, 0x14, 0x90, 0x00}},
				{wantAPDU: "00B2010414", resp: []byte{0x61, 0x0F, 0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04, 0x50, 0x04, 0x49, 0x53, 0x49, 0x4D, 0x90, 0x00}},
				{wantAPDU: "00A4040407A0000000871004", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
				{wantAPDU: "00A40004026F02", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x10, 0x90, 0x00}},
				{wantAPDU: "00B0000010", resp: []byte{0x6A, 0x82}},
			},
			wantErrText: "reading EF_IMPI",
		},
		{
			name: "impu first record failure",
			steps: []scriptStep{
				{wantAPDU: "00A40004023F00", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
				{wantAPDU: "00A40004022F00", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x14, 0x01, 0x80, 0x02, 0x00, 0x14, 0x90, 0x00}},
				{wantAPDU: "00B2010414", resp: []byte{0x61, 0x0F, 0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04, 0x50, 0x04, 0x49, 0x53, 0x49, 0x4D, 0x90, 0x00}},
				{wantAPDU: "00A4040407A0000000871004", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
				{wantAPDU: "00A40004026F02", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x10, 0x90, 0x00}},
				{wantAPDU: "00B0000010", resp: append([]byte("alice@ims\xFF\xFF\xFF\xFF\xFF\xFF\xFF"), 0x90, 0x00)},
				{wantAPDU: "00A4040407A0000000871004", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
				{wantAPDU: "00A40004026F03", resp: []byte{0x6A, 0x82}},
				{wantAPDU: "00A4040407A0000000871004", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
				{wantAPDU: "00A40004026F04", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x10, 0x02, 0x80, 0x02, 0x00, 0x20, 0x90, 0x00}},
				{wantAPDU: "00B2010410", resp: []byte{0xFF, 0xFF, 0xFF, 0x90, 0x00}},
				{wantAPDU: "00B2020410", resp: []byte{0xFF, 0xFF, 0xFF, 0x90, 0x00}},
			},
			wantErrText: "reading EF_IMPU",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := &Card{
				tx:     newAPDUReader(t, &scriptTransmitter{steps: tt.steps}),
				logger: slog.Default(),
			}

			err := card.loadISIM(context.Background())
			if err == nil {
				t.Fatal("loadISIM() error = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("loadISIM() error = %v, want text %q", err, tt.wantErrText)
			}
		})
	}
}

func TestNewSkipsUnavailableISIM(t *testing.T) {
	tests := []struct {
		name    string
		impiErr error
		impuErr error
	}{
		{
			name:    "IMPI read failure",
			impiErr: errors.New("invalid argument"),
		},
		{
			name:    "IMPU read failure after IMPI read",
			impuErr: errors.New("invalid argument"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card, err := New(context.Background(), &loadReader{
				impiErr: tt.impiErr,
				impuErr: tt.impuErr,
			}, nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if got := card.IMSI(); got != "001010123456789" {
				t.Fatalf("IMSI() = %q, want %q", got, "001010123456789")
			}
			if got := card.MNC(); got != "01" {
				t.Fatalf("MNC() = %q, want %q", got, "01")
			}
			if got := card.MNCLength(); got != 2 {
				t.Fatalf("MNCLength() = %d, want %d", got, 2)
			}
			if got := card.PrivateIdentity(); got != "" {
				t.Fatalf("PrivateIdentity() = %q, want empty", got)
			}
			if got := card.PublicIdentity(); got != "" {
				t.Fatalf("PublicIdentity() = %q, want empty", got)
			}
			if got := card.HomeDomain(); got != "" {
				t.Fatalf("HomeDomain() = %q, want empty", got)
			}
		})
	}
}

func TestNewServiceCenterPSIPreference(t *testing.T) {
	tx := newAPDUReader(t, &scriptTransmitter{steps: []scriptStep{
		{wantAPDU: "00A40004023F00", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004022F00", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x14, 0x01, 0x80, 0x02, 0x00, 0x14, 0x90, 0x00}},
		{wantAPDU: "00B2010414", resp: []byte{0x61, 0x0F, 0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02, 0x50, 0x04, 0x55, 0x53, 0x49, 0x4D, 0x90, 0x00}},
		{wantAPDU: "00A40004023F00", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004022FE2", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x0A, 0x90, 0x00}},
		{wantAPDU: "00B000000A", resp: []byte{0x98, 0x68, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0, 0x90, 0x00}},
		{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026F07", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x09, 0x90, 0x00}},
		{wantAPDU: "00B0000009", resp: []byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98, 0x90, 0x00}},
		{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026FAD", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x04, 0x90, 0x00}},
		{wantAPDU: "00B0000004", resp: []byte{0x00, 0x00, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026F3E", resp: []byte{0x6A, 0x82}},
		{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026F3F", resp: []byte{0x6A, 0x82}},
		{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026F46", resp: []byte{0x6A, 0x82}},
		{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026F42", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x1C, 0x01, 0x80, 0x02, 0x00, 0x1C, 0x90, 0x00}},
		{wantAPDU: "00B201041C", resp: append(smscRecord(28, 0x91, 0x55, 0x15, 0x00, 0x00, 0x00, 0xF0), 0x90, 0x00)},
		{wantAPDU: "00A4040407A0000000871002", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40804047F106FE5", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x20, 0x01, 0x80, 0x02, 0x00, 0x20, 0x90, 0x00}},
		{wantAPDU: "00B2010420", resp: append(tlvTextRecord("sip:usim-smsc@example.com", 32), 0x90, 0x00)},
		{wantAPDU: "00A40004023F00", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004022F00", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x14, 0x02, 0x80, 0x02, 0x00, 0x14, 0x90, 0x00}},
		{wantAPDU: "00B2010414", resp: []byte{0x61, 0x0F, 0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02, 0x50, 0x04, 0x55, 0x53, 0x49, 0x4D, 0x90, 0x00}},
		{wantAPDU: "00B2020414", resp: []byte{0x61, 0x0F, 0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04, 0x50, 0x04, 0x49, 0x53, 0x49, 0x4D, 0x90, 0x00}},
		{wantAPDU: "00A4040407A0000000871004", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026F02", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x20, 0x90, 0x00}},
		{wantAPDU: "00B0000020", resp: append(tlvTextBinary("alice@ims.example.com", 32), 0x90, 0x00)},
		{wantAPDU: "00A4040407A0000000871004", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026F03", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x18, 0x90, 0x00}},
		{wantAPDU: "00B0000018", resp: append(tlvTextBinary("ims.example.com", 24), 0x90, 0x00)},
		{wantAPDU: "00A4040407A0000000871004", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40004026F04", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x20, 0x01, 0x80, 0x02, 0x00, 0x20, 0x90, 0x00}},
		{wantAPDU: "00B2010420", resp: append(tlvTextRecord("sip:alice@ims.example.com", 32), 0x90, 0x00)},
		{wantAPDU: "00A4040407A0000000871004", resp: []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x38, 0x80, 0x02, 0x00, 0x02, 0x90, 0x00}},
		{wantAPDU: "00A40804047F106FE5", resp: []byte{0x62, 0x0B, 0x82, 0x05, 0x42, 0x21, 0x00, 0x20, 0x01, 0x80, 0x02, 0x00, 0x20, 0x90, 0x00}},
		{wantAPDU: "00B2010420", resp: append(tlvTextRecord("sip:isim-smsc@example.com", 32), 0x90, 0x00)},
	}})

	card, err := New(context.Background(), tx, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := card.SMSC(); got != "+55510000000" {
		t.Fatalf("SMSC() = %q, want %q", got, "+55510000000")
	}
	if got := card.ServiceCenter().PSI; got != "sip:isim-smsc@example.com" {
		t.Fatalf("ServiceCenter().PSI = %q, want %q", got, "sip:isim-smsc@example.com")
	}
}

func stringsUpperHex(data []byte) string {
	return strings.ToUpper(hex.EncodeToString(data))
}

func tlvTextBinary(value string, size int) []byte {
	data := append([]byte{0x80, byte(len(value))}, []byte(value)...)
	for len(data) < size {
		data = append(data, 0xFF)
	}
	return data
}

func tlvTextRecord(value string, size int) []byte {
	return tlvTextBinary(value, size)
}

func smscRecord(size int, toa byte, digits ...byte) []byte {
	record := make([]byte, size)
	start := size - 28 + 13
	record[start] = byte(len(digits) + 1)
	record[start+1] = toa
	copy(record[start+2:], digits)
	return record
}
