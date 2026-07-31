package mbim

import (
	"encoding/binary"
	"fmt"
)

// SessionID identifies an MBIM data session.
type SessionID uint32

// NewSessionIDTLV encodes a session ID TLV.
func NewSessionIDTLV(sessionID SessionID) TLV {
	return TLV{
		Type: TLVTypeSessionID,
		Data: binary.LittleEndian.AppendUint32(nil, uint32(sessionID)),
	}
}

// UnmarshalTLV decodes a session ID TLV.
func (id *SessionID) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypeSessionID {
		return fmt.Errorf("parsing session ID TLV: type is %d, want %d", tlv.Type, TLVTypeSessionID)
	}
	if len(tlv.Data) != 4 {
		return fmt.Errorf("parsing session ID TLV: data length is %d, want 4", len(tlv.Data))
	}
	*id = SessionID(binary.LittleEndian.Uint32(tlv.Data))
	return nil
}
