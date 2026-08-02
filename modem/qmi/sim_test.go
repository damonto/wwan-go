package qmi

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

type simInfoTestTransport struct {
	appState          qcom.ApplicationState
	rawICCID          []byte
	dmsICCID          string
	imsi              string
	transparentReadOK bool
}

func (t *simInfoTestTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	switch req.MessageID {
	case qcom.MessageRegisterEvents:
		return simInfoTestResponse(req), nil
	case qcom.MessageGetCardStatus:
		return simInfoTestResponse(req, tlv.Bytes(0x10, encodeSIMCardStatus(t.appState))), nil
	case qcom.MessageReadTransparent:
		if !t.transparentReadOK {
			return simInfoTestErrorResponse(req, qcom.QMIErrorNotSupported), nil
		}
		data := binary.LittleEndian.AppendUint16(nil, uint16(len(t.rawICCID)))
		data = append(data, t.rawICCID...)
		return simInfoTestResponse(req,
			tlv.Bytes(0x10, []byte{0x90, 0x00}),
			tlv.Bytes(0x11, data),
		), nil
	case qcom.MessageDMSUIMGetICCID:
		return simInfoTestResponse(req, tlv.Bytes(0x01, []byte(t.dmsICCID))), nil
	case qcom.MessageDMSUIMGetIMSI:
		return simInfoTestResponse(req, tlv.Bytes(0x01, []byte(t.imsi))), nil
	case qcom.MessageDMSGetMSISDN:
		return simInfoTestResponse(req, tlv.Bytes(0x01, nil)), nil
	default:
		return qcom.Response{}, fmt.Errorf("unexpected QMI message 0x%04X", req.MessageID)
	}
}

func (*simInfoTestTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (*simInfoTestTransport) Close() error { return nil }

func (*simInfoTestTransport) Indications(ctx context.Context, _ qcom.ServiceType, _ uint8, _ qcom.MessageID) (<-chan qcom.Indication, error) {
	out := make(chan qcom.Indication)
	go func() {
		<-ctx.Done()
		close(out)
	}()
	return out, nil
}

func TestSIMInfoFallsBackToDMSICCID(t *testing.T) {
	const (
		iccid = "8986000000000000000"
		imsi  = "310260000000000"
	)
	tests := []struct {
		name              string
		appState          qcom.ApplicationState
		transparentReadOK bool
		wantState         SIMState
		wantIMSI          string
	}{
		{
			name:      "locked SIM",
			appState:  qcom.ApplicationStatePIN1OrUPINRequired,
			wantState: SIMStateLocked,
		},
		{
			name:      "EF_ICCID unavailable",
			appState:  qcom.ApplicationStateReady,
			wantState: SIMStateReady,
			wantIMSI:  imsi,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &simInfoTestTransport{
				appState:          tt.appState,
				dmsICCID:          iccid,
				imsi:              imsi,
				transparentReadOK: tt.transparentReadOK,
			}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			backend := New(client, "/dev/test")
			backend.metadataKey = iccid
			backend.metadata = SIMInfo{ICCID: iccid}

			got, err := backend.SIMInfo(t.Context())
			if err != nil {
				t.Fatalf("SIMInfo() error = %v", err)
			}
			if got.State != tt.wantState || got.ICCID != iccid || got.IMSI != tt.wantIMSI {
				t.Fatalf("SIMInfo() = %+v, want state %d ICCID %q IMSI %q", got, tt.wantState, iccid, tt.wantIMSI)
			}
		})
	}
}

func TestWatchSIMEnrichesInitialSnapshot(t *testing.T) {
	const (
		iccid = "8986000000000000000"
		imsi  = "310260000000000"
	)
	tests := []struct {
		name string
	}{
		{name: "ready SIM without subsequent indications"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &simInfoTestTransport{
				appState:          qcom.ApplicationStateReady,
				rawICCID:          []byte{0x98, 0x68, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0},
				dmsICCID:          iccid,
				imsi:              imsi,
				transparentReadOK: true,
			}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			backend := New(client, "/dev/test")
			backend.enrichDelay = 10 * time.Millisecond
			backend.metadataKey = iccid
			backend.metadata = SIMInfo{ICCID: iccid}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			stream, err := backend.WatchSIM(ctx)
			if err != nil {
				t.Fatalf("WatchSIM() error = %v", err)
			}
			initial := receiveSIMInfo(t, ctx, stream)
			if initial.Err != nil {
				t.Fatalf("initial result error = %v", initial.Err)
			}
			if initial.Value.ICCID != iccid || initial.Value.IMSI != "" {
				t.Fatalf("initial SIM info = %+v, want ICCID %q without IMSI", initial.Value, iccid)
			}

			enriched := receiveSIMInfo(t, ctx, stream)
			if enriched.Err != nil {
				t.Fatalf("enriched result error = %v", enriched.Err)
			}
			if enriched.Value.ICCID != iccid || enriched.Value.IMSI != imsi {
				t.Fatalf("enriched SIM info = %+v, want ICCID %q IMSI %q", enriched.Value, iccid, imsi)
			}
		})
	}
}

func receiveSIMInfo(t *testing.T, ctx context.Context, stream <-chan Result[SIMInfo]) Result[SIMInfo] {
	t.Helper()
	select {
	case result, ok := <-stream:
		if !ok {
			t.Fatal("SIM stream closed")
		}
		return result
	case <-ctx.Done():
		t.Fatalf("waiting for SIM info: %v", ctx.Err())
		return Result[SIMInfo]{}
	}
}

func simInfoTestResponse(req qcom.Request, values ...tlv.TLV) qcom.Response {
	return qcom.Response{
		Service:       req.Service,
		ClientID:      req.ClientID,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		TLVs: append(tlv.TLVs{
			tlv.Bytes(0x02, []byte{0x00, 0x00, 0x00, 0x00}),
		}, values...),
	}
}

func simInfoTestErrorResponse(req qcom.Request, qmiErr qcom.QMIError) qcom.Response {
	result := binary.LittleEndian.AppendUint16([]byte{0x01, 0x00}, uint16(qmiErr))
	return qcom.Response{
		Service:       req.Service,
		ClientID:      req.ClientID,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		TLVs:          tlv.TLVs{tlv.Bytes(0x02, result)},
	}
}

func encodeSIMCardStatus(appState qcom.ApplicationState) []byte {
	value := make([]byte, 8)
	value = append(value,
		0x01,
		byte(qcom.CardStatePresent), 0x00, 0x00, 0x00, 0x00,
		0x01,
		byte(qcom.ApplicationTypeUSIM), byte(appState),
	)
	return append(value, make([]byte, 12)...)
}
