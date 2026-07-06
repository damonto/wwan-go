package uim

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestReaderATR(t *testing.T) {
	tests := []struct {
		name    string
		resp    qcom.Response
		err     error
		want    []byte
		wantErr string
	}{
		{
			name: "success",
			resp: successResponse(qcom.MessageGetATR, tlv.Bytes(0x10, []byte{
				0x03, 0x3B, 0x9F, 0x96,
			})),
			want: []byte{0x3B, 0x9F, 0x96},
		},
		{
			name:    "missing ATR TLV",
			resp:    successResponse(qcom.MessageGetATR),
			wantErr: "ATR TLV missing",
		},
		{
			name: "truncated ATR",
			resp: successResponse(qcom.MessageGetATR, tlv.Bytes(0x10, []byte{
				0x03, 0x3B,
			})),
			wantErr: "ATR length 3 exceeds remaining 1",
		},
		{
			name:    "QMI failure",
			resp:    errorResponse(qcom.MessageGetATR, qcom.QMIErrorNotSupported),
			wantErr: qcom.QMIErrorNotSupported.Error(),
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
						check: func(req qcom.Request) {
							if req.MessageID != qcom.MessageGetATR {
								t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageGetATR)
							}
							assertRequestTimeout(t, req, DefaultRequestTimeout)
							assertTLV(t, req.TLVs, 0x01, []byte{0x02})
						},
						resp: tt.resp,
						err:  tt.err,
					},
				},
			}
			reader := &Reader{
				transport: transport,
				slot:      2,
				clientID:  7,
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
