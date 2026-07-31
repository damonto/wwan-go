package qcom

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestClientATR(t *testing.T) {
	tests := []struct {
		name    string
		resp    Response
		err     error
		want    []byte
		wantErr string
	}{
		{
			name: "success",
			resp: successResponse(MessageGetATR, tlv.Bytes(0x10, []byte{
				0x03, 0x3B, 0x9F, 0x96,
			})),
			want: []byte{0x3B, 0x9F, 0x96},
		},
		{
			name:    "missing ATR TLV",
			resp:    successResponse(MessageGetATR),
			wantErr: "ATR TLV missing",
		},
		{
			name: "truncated ATR",
			resp: successResponse(MessageGetATR, tlv.Bytes(0x10, []byte{
				0x03, 0x3B,
			})),
			wantErr: "ATR length 3 exceeds remaining 1",
		},
		{
			name:    "ATR trailing data",
			resp:    successResponse(MessageGetATR, tlv.Bytes(0x10, []byte{0x01, 0x3B, 0x00})),
			wantErr: "ATR has 1 trailing bytes",
		},
		{
			name:    "QMI failure",
			resp:    errorResponse(MessageGetATR, QMIErrorNotSupported),
			wantErr: QMIErrorNotSupported.Error(),
		},
		{
			name:    "transport error",
			err:     errors.New("transport closed"),
			wantErr: "transport closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{
				t: t,
				calls: []transportCall{
					{
						check: func(req Request) {
							if req.MessageID != MessageGetATR {
								t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, MessageGetATR)
							}
							assertRequestTimeout(t, req, DefaultRequestTimeout)
							assertTLV(t, req.TLVs, 0x01, []byte{0x02})
						},
						resp: tt.resp,
						err:  tt.err,
					},
				},
			}
			reader := &Client{
				transport: transport,
				slot:      2,
				clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
			}

			got, err := reader.ATR(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ATR() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ATR() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("ATR() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestDecodeATR(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []byte
		wantErr string
	}{
		{
			name: "empty ATR",
			data: []byte{0x00},
			want: []byte{},
		},
		{
			name: "ATR",
			data: []byte{0x02, 0x3B, 0x9F},
			want: []byte{0x3B, 0x9F},
		},
		{
			name:    "missing length",
			wantErr: "ATR length is missing",
		},
		{
			name:    "truncated value",
			data:    []byte{0x02, 0x3B},
			wantErr: "ATR length 2 exceeds remaining 1",
		},
		{
			name:    "trailing data",
			data:    []byte{0x01, 0x3B, 0x00},
			wantErr: "ATR has 1 trailing bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeATR(tt.data)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("decodeATR() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeATR() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("decodeATR() = % X, want % X", got, tt.want)
			}
		})
	}
}
