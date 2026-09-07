package mbim

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestWriteFull(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	errWrite := errors.New("device disconnected")
	tests := []struct {
		name      string
		results   []writeResult
		wantN     int
		wantErr   bool
		wantCause error
	}{
		{name: "full write", wantN: len(data)},
		{name: "partial writes", results: []writeResult{{n: 2}}, wantN: len(data)},
		{name: "zero progress", results: []writeResult{{}}, wantErr: true, wantCause: io.ErrShortWrite},
		{name: "write error", results: []writeResult{{err: errWrite}}, wantErr: true, wantCause: errWrite},
		{name: "negative with error", results: []writeResult{{n: -1, err: errWrite}}, wantErr: true, wantCause: errWrite},
		{name: "negative without error", results: []writeResult{{n: -1}}, wantErr: true},
		{name: "oversized with error", results: []writeResult{{n: 5, err: errWrite}}, wantErr: true, wantCause: errWrite},
		{name: "oversized without error", results: []writeResult{{n: 5}}, wantErr: true},
		{name: "partial with error", results: []writeResult{{n: 2, err: errWrite}}, wantN: 2, wantErr: true, wantCause: errWrite},
		{name: "full with error", results: []writeResult{{n: 4, err: errWrite}}, wantN: 4, wantErr: true, wantCause: errWrite},
		{name: "negative after partial", results: []writeResult{{n: 2}, {n: -1, err: errWrite}}, wantN: 2, wantErr: true, wantCause: errWrite},
		{name: "oversized after partial", results: []writeResult{{n: 2}, {n: 3}}, wantN: 2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &scriptedWriter{results: tt.results}
			n, err := writeFull(w, data)
			if n != tt.wantN || (err != nil) != tt.wantErr {
				t.Fatalf("writeFull() = (%d, %v), want count %d, error %t", n, err, tt.wantN, tt.wantErr)
			}
			if tt.wantCause != nil && !errors.Is(err, tt.wantCause) {
				t.Errorf("writeFull() error = %v, want %v", err, tt.wantCause)
			}
			if want := data[:tt.wantN]; !bytes.Equal(w.data, want) {
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
