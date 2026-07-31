package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
)

func TestUICCReadRequestsEncodeLocalPINAndData(t *testing.T) {
	applicationID := []byte{0xa0, 0x00}
	filePath := []byte{0x6f, 0xad}
	localPIN := utf16Bytes("12")
	requestData := []byte{0xaa, 0xbb, 0xcc}
	tests := []struct {
		name         string
		request      func() *Request
		fixedSize    int
		fieldOffsets []int
	}{
		{
			name: "read binary",
			request: func() *Request {
				return (&ReadBinaryRequest{
					TransactionID: 1,
					ApplicationID: applicationID,
					FilePath:      filePath,
					Size:          3,
					LocalPIN:      "12",
					Data:          requestData,
				}).Request()
			},
			fixedSize:    44,
			fieldOffsets: []int{4, 12, 28, 36},
		},
		{
			name: "read record",
			request: func() *Request {
				return (&ReadRecordRequest{
					TransactionID: 1,
					ApplicationID: applicationID,
					FilePath:      filePath,
					Record:        1,
					LocalPIN:      "12",
					Data:          requestData,
				}).Request()
			},
			fixedSize:    40,
			fieldOffsets: []int{4, 12, 24, 32},
		},
	}
	wantValues := [][]byte{applicationID, filePath, localPIN, requestData}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.request().Command.(*Command).Data
			if len(data) <= tt.fixedSize {
				t.Fatalf("request length = %d, want data after fixed size %d", len(data), tt.fixedSize)
			}
			for i, fieldOffset := range tt.fieldOffsets {
				offset := binary.LittleEndian.Uint32(data[fieldOffset : fieldOffset+4])
				size := binary.LittleEndian.Uint32(data[fieldOffset+4 : fieldOffset+8])
				if size != uint32(len(wantValues[i])) {
					t.Fatalf("reference %d size = %d, want %d", i, size, len(wantValues[i]))
				}
				end := offset + size
				if end > uint32(len(data)) {
					t.Fatalf("reference %d ends at %d, request length %d", i, end, len(data))
				}
				if got := data[offset:end]; !bytes.Equal(got, wantValues[i]) {
					t.Fatalf("reference %d = %X, want %X", i, got, wantValues[i])
				}
			}
		})
	}
}

func TestNormalizeUICCFile(t *testing.T) {
	tests := []struct {
		name     string
		file     FileRef
		wantPath []byte
		wantErr  bool
	}{
		{
			name:     "MF-relative path",
			file:     FileRef{Path: []byte{0x6F, 0xAD}},
			wantPath: []byte{0x3F, 0x00, 0x6F, 0xAD},
		},
		{
			name:     "ADF-relative path",
			file:     FileRef{AID: []byte{0xA0}, Path: []byte{0x6F, 0xAD}},
			wantPath: []byte{0x7F, 0xFF, 0x6F, 0xAD},
		},
		{
			name:     "absolute MF path",
			file:     FileRef{AID: []byte{0xA0}, Path: []byte{0x3F, 0x00, 0x2F, 0xE2}},
			wantPath: []byte{0x3F, 0x00, 0x2F, 0xE2},
		},
		{
			name:     "absolute ADF path",
			file:     FileRef{AID: []byte{0xA0}, Path: []byte{0x7F, 0xFF, 0x6F, 0xAD}},
			wantPath: []byte{0x7F, 0xFF, 0x6F, 0xAD},
		},
		{
			name:    "empty path",
			file:    FileRef{},
			wantErr: true,
		},
		{
			name:    "application ID too long",
			file:    FileRef{AID: make([]byte, uiccFileApplicationIDMaximumSize+1), Path: []byte{0x6F, 0xAD}},
			wantErr: true,
		},
		{
			name:    "path too long",
			file:    FileRef{Path: make([]byte, uiccFilePathMaximumSize)},
			wantErr: true,
		},
		{
			name:    "odd path length",
			file:    FileRef{Path: []byte{0x6F}},
			wantErr: true,
		},
		{
			name:    "ADF path without application ID",
			file:    FileRef{Path: []byte{0x7F, 0xFF, 0x6F, 0xAD}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeUICCFile(tt.file)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeUICCFile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !bytes.Equal(got.AID, tt.file.AID) {
				t.Fatalf("normalizeUICCFile() AID = %X, want %X", got.AID, tt.file.AID)
			}
			if !bytes.Equal(got.Path, tt.wantPath) {
				t.Fatalf("normalizeUICCFile() path = %X, want %X", got.Path, tt.wantPath)
			}
		})
	}
}

func TestUICCFileClientInputLimits(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context) error
	}{
		{
			name: "file status application ID too long",
			run: func(ctx context.Context) error {
				_, err := new(Client).FileAttributes(ctx, FileRef{
					AID:  make([]byte, uiccFileApplicationIDMaximumSize+1),
					Path: []byte{0x6F, 0xAD},
				})
				return err
			},
		},
		{
			name: "transparent response limit",
			run: func(ctx context.Context) error {
				_, err := new(Client).ReadTransparent(ctx, TransparentRead{
					File:   FileRef{Path: []byte{0x6F, 0xAD}},
					Length: uint16(uiccFileResponseMaximumSize + 1),
				})
				return err
			},
		},
		{
			name: "record number limit",
			run: func(ctx context.Context) error {
				_, err := new(Client).ReadRecord(ctx, RecordRead{
					File:   FileRef{Path: []byte{0x6F, 0xAD}},
					Record: uint16(uiccRecordNumberMaximum + 1),
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := tt.run(ctx); err == nil {
				t.Fatal("client call error = nil, want non-nil")
			}
		})
	}
}

func TestUICCStatusOK(t *testing.T) {
	tests := []struct {
		name   string
		status uint32
		want   bool
	}{
		{name: "empty status", status: 0, want: true},
		{name: "normal success", status: 0x0090, want: true},
		{name: "proactive command without length", status: 0x0091, want: true},
		{name: "proactive command", status: 0x1091, want: true},
		{name: "more response data", status: 0x1061},
		{name: "application error", status: 0x826A},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uiccStatusOK(tt.status); got != tt.want {
				t.Fatalf("uiccStatusOK(%#x) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
