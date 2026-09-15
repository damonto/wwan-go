package contract

import (
	"errors"
	"fmt"
	"testing"
)

func TestMessageDecodeErrors(t *testing.T) {
	firstCause := errors.New("truncated PDU")
	secondCause := errors.New("truncated user data")
	first := &MessageDecodeError{Ref: MessageRef{Storage: MessageStorageSIM, ID: 2}, Err: firstCause}
	second := &MessageDecodeError{Ref: MessageRef{Storage: MessageStorageDevice, ID: 2}, Err: secondCause}
	tests := []struct {
		name     string
		err      error
		want     *MessageDecodeError
		wantList bool
		causes   []error
	}{
		{name: "single message", err: first, want: first, causes: []error{firstCause}},
		{
			name:     "wrapped partial list preserves all causes",
			err:      fmt.Errorf("read inbox: %w", &MessageListError{Errors: []*MessageDecodeError{first, second}}),
			want:     first,
			wantList: true,
			causes:   []error{firstCause, secondCause},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := errors.AsType[*MessageDecodeError](tt.err)
			if !ok || got != tt.want {
				t.Fatalf("AsType[MessageDecodeError]() = %v, %t; want %v, true", got, ok, tt.want)
			}
			if _, ok := errors.AsType[*MessageListError](tt.err); ok != tt.wantList {
				t.Errorf("AsType[MessageListError]() = %t, want %t", ok, tt.wantList)
			}
			for _, cause := range tt.causes {
				if !errors.Is(tt.err, cause) {
					t.Errorf("Is(%v) = false", cause)
				}
			}
		})
	}
}
