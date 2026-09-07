package qrtr

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/damonto/wwan-go/qcom"
)

func TestRequestWriteTo(t *testing.T) {
	req := Request{Request: qcom.Request{
		Service:       qcom.ServiceUIM,
		ClientID:      7,
		TransactionID: 1,
		MessageID:     qcom.MessageGetCardStatus,
	}}
	wire, err := req.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}
	size := len(wire)
	errWrite := errors.New("device disconnected")
	tests := []struct {
		name      string
		results   []writeResult
		wantN     int
		wantErr   bool
		wantCause error
	}{
		{name: "full write", wantN: size},
		{name: "partial writes", results: []writeResult{{n: 2}}, wantN: size},
		{name: "zero progress", results: []writeResult{{}}, wantErr: true, wantCause: io.ErrShortWrite},
		{name: "write error", results: []writeResult{{err: errWrite}}, wantErr: true, wantCause: errWrite},
		{name: "negative with error", results: []writeResult{{n: -1, err: errWrite}}, wantErr: true, wantCause: errWrite},
		{name: "negative without error", results: []writeResult{{n: -1}}, wantErr: true},
		{name: "oversized with error", results: []writeResult{{n: size + 1, err: errWrite}}, wantErr: true, wantCause: errWrite},
		{name: "oversized without error", results: []writeResult{{n: size + 1}}, wantErr: true},
		{name: "partial with error", results: []writeResult{{n: 2, err: errWrite}}, wantN: 2, wantErr: true, wantCause: errWrite},
		{name: "full with error", results: []writeResult{{n: size, err: errWrite}}, wantN: size, wantErr: true, wantCause: errWrite},
		{name: "negative after partial", results: []writeResult{{n: 2}, {n: -1, err: errWrite}}, wantN: 2, wantErr: true, wantCause: errWrite},
		{name: "oversized after partial", results: []writeResult{{n: 2}, {n: size - 1}}, wantN: 2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &scriptedWriter{results: tt.results}
			n, err := req.WriteTo(w)
			if n != int64(tt.wantN) || (err != nil) != tt.wantErr {
				t.Fatalf("WriteTo() = (%d, %v), want count %d, error %t", n, err, tt.wantN, tt.wantErr)
			}
			if tt.wantCause != nil && !errors.Is(err, tt.wantCause) {
				t.Errorf("WriteTo() error = %v, want %v", err, tt.wantCause)
			}
			if want := wire[:tt.wantN]; !bytes.Equal(w.data, want) {
				t.Errorf("written data = % X, want % X", w.data, want)
			}
		})
	}
}

type writeResult struct {
	n   int
	err error
}

type scriptedWriter struct {
	results []writeResult
	data    []byte
}

func (w *scriptedWriter) Write(p []byte) (int, error) {
	if len(w.results) == 0 {
		w.data = append(w.data, p...)
		return len(p), nil
	}
	result := w.results[0]
	w.results = w.results[1:]
	if result.n > 0 && result.n <= len(p) {
		w.data = append(w.data, p[:result.n]...)
	}
	return result.n, result.err
}
