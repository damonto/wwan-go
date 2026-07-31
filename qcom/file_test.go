package qcom

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWriteRecord(t *testing.T) {
	tests := []struct {
		name string
		req  RecordWrite
	}{
		{
			name: "USIM MSISDN record",
			req: RecordWrite{
				File: File{
					Session: SessionPrimaryGWProvisioning,
					Path:    []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0x40},
				},
				Record: 1,
				Data:   []byte{0xFF, 0x07, 0x91, 0x68, 0x31, 0x08, 0x10, 0x83, 0x00, 0xF8, 0xFF, 0xFF},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{
				t: t,
				calls: []transportCall{{
					check: func(req Request) {
						if req.Service != ServiceUIM || req.MessageID != MessageWriteRecord {
							t.Fatalf("request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(tt.req.File.Session), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{0x40, 0x6F, 0x04, 0x00, 0x3F, 0xFF, 0x7F})

						value, ok := tlv.Value(req.TLVs, 0x03)
						if !ok {
							t.Fatal("write record TLV missing")
						}
						want := []byte{0x01, 0x00, byte(len(tt.req.Data)), 0x00}
						want = append(want, tt.req.Data...)
						if !bytes.Equal(value, want) {
							t.Fatalf("write record TLV = % X, want % X", value, want)
						}
					},
					resp: successResponse(MessageWriteRecord, tlv.Bytes(qmiTLVCardResult, []byte{0x90, 0x00})),
				}},
			}
			reader := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			if err := reader.WriteRecord(context.Background(), tt.req); err != nil {
				t.Fatalf("WriteRecord() error = %v", err)
			}
		})
	}
}

func TestCardErrorRejectsMalformedResult(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
	}{
		{name: "truncated", value: []byte{0x90}},
		{name: "trailing byte", value: []byte{0x90, 0x00, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cardError(tlv.TLVs{tlv.Bytes(qmiTLVCardResult, tt.value)})
			if err == nil {
				t.Fatal("cardError() error = nil, want non-nil")
			}
		})
	}
}

func TestWriteTransparent(t *testing.T) {
	tests := []struct {
		name string
		req  TransparentWrite
	}{
		{
			name: "USIM transparent EF",
			req: TransparentWrite{
				File:   File{Session: SessionPrimaryGWProvisioning, Path: []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0x07}},
				Offset: 2,
				Data:   []byte{0x11, 0x22, 0x33},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceUIM || req.MessageID != MessageWriteTransparent {
						t.Fatalf("request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(tt.req.File.Session), 0})
					assertTLV(t, req.TLVs, 0x02, []byte{0x07, 0x6F, 0x04, 0x00, 0x3F, 0xFF, 0x7F})
					assertTLV(t, req.TLVs, 0x03, []byte{2, 0, 3, 0, 0x11, 0x22, 0x33})
				},
				resp: successResponse(MessageWriteTransparent, tlv.Bytes(qmiTLVCardResult, []byte{0x90, 0x00})),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}
			if err := client.WriteTransparent(context.Background(), tt.req); err != nil {
				t.Fatalf("WriteTransparent() error = %v", err)
			}
		})
	}
}

func TestWriteRecordValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     RecordWrite
		wantErr string
	}{
		{
			name: "zero record",
			req: RecordWrite{
				File: File{Path: []byte{0x6F, 0x40}},
			},
			wantErr: "record number is zero",
		},
		{
			name: "content too long",
			req: RecordWrite{
				File:   File{Path: []byte{0x6F, 0x40}},
				Record: 1,
				Data:   bytes.Repeat([]byte{0xFF}, maxRecordContentLength+1),
			},
			wantErr: "exceeds QMI UIM limit",
		},
		{
			name: "invalid path",
			req: RecordWrite{
				File:   File{Path: []byte{0x6F}},
				Record: 1,
			},
			wantErr: "path length must be an even number of bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Client{transport: &fakeTransport{t: t}, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}
			err := reader.WriteRecord(context.Background(), tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("WriteRecord() error = %v, want text %q", err, tt.wantErr)
			}
		})
	}
}

func TestWriteRecordRejectsResponseIndication(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{
			name: "indication token",
			tlvs: tlv.TLVs{tlv.Uint(0x11, uint32(7))},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Client{
				transport: &fakeTransport{t: t, calls: []transportCall{{
					resp: successResponse(MessageWriteRecord, tt.tlvs...),
				}}},
				slot:      1,
				clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
			}

			err := reader.WriteRecord(context.Background(), RecordWrite{
				File:   File{Path: []byte{0x6F, 0x40}},
				Record: 1,
			})
			if err == nil || !strings.Contains(err.Error(), "response indication is not supported") {
				t.Fatalf("WriteRecord() error = %v, want indication error", err)
			}
		})
	}
}

func TestUIMReadExtensions(t *testing.T) {
	tests := []struct {
		name  string
		call  func(*testing.T, *Client)
		check func(*testing.T, Request)
		resp  Response
	}{
		{
			name: "encrypted transparent data",
			call: func(t *testing.T, client *Client) {
				got, err := client.ReadTransparentResult(context.Background(), TransparentRead{
					File:        File{Path: []byte{0x6F, 0x07}},
					Length:      2,
					EncryptData: true,
				})
				if err != nil {
					t.Fatalf("ReadTransparentResult() error = %v", err)
				}
				if !bytes.Equal(got.Data, []byte{0xAA, 0xBB}) || !got.EncryptedKnown || !got.Encrypted {
					t.Fatalf("ReadTransparentResult() = %+v", got)
				}
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x11, []byte{1})
			},
			resp: successResponse(
				MessageReadTransparent,
				tlv.Bytes(0x11, encodeLengthPrefixed([]byte{0xAA, 0xBB})),
				tlv.Bytes(0x13, []byte{1}),
			),
		},
		{
			name: "record range",
			call: func(t *testing.T, client *Client) {
				got, err := client.ReadRecords(context.Background(), RecordRead{
					File:       File{Path: []byte{0x6F, 0x40}},
					Record:     2,
					LastRecord: 4,
					Length:     2,
				})
				if err != nil {
					t.Fatalf("ReadRecords() error = %v", err)
				}
				want := [][]byte{{1, 2}, {3, 4}, {5, 6}}
				if len(got) != len(want) {
					t.Fatalf("ReadRecords() count = %d, want %d", len(got), len(want))
				}
				for i := range want {
					if !bytes.Equal(got[i], want[i]) {
						t.Fatalf("ReadRecords()[%d] = % X, want % X", i, got[i], want[i])
					}
				}
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x10, []byte{4, 0})
			},
			resp: successResponse(
				MessageReadRecord,
				tlv.Bytes(0x11, encodeLengthPrefixed([]byte{1, 2})),
				tlv.Bytes(0x12, encodeLengthPrefixed([]byte{3, 4, 5, 6})),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) { tt.check(t, req) },
				resp:  tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}
			tt.call(t, client)
		})
	}
}

func TestRawFileAttributes(t *testing.T) {
	value := encodeFileAttributes(10, 0x2FE2, byte(QMIFileTypeTransparent), 0, 0, []byte{0x62, 0x00})
	value[9] = byte(UIMSecuritySingle)
	value[10] = byte(UIMSecurityPIN1 | UIMSecurityADM)
	tests := []struct {
		name    string
		value   []byte
		wantErr bool
	}{
		{name: "security and raw data", value: value},
		{name: "trailing data", value: append(bytes.Clone(value), 0), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got RawFileAttributes
			err := got.UnmarshalBinary(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got.FileID != 0x2FE2 || got.ReadSecurity != (UIMFileSecurity{Logic: UIMSecuritySingle, Attributes: UIMSecurityPIN1 | UIMSecurityADM}) {
				t.Fatalf("attributes = %+v", got)
			}
			if !bytes.Equal(got.Raw, []byte{0x62, 0x00}) {
				t.Fatalf("raw = % X", got.Raw)
			}
		})
	}
}
