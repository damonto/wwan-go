package qcom

import (
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WDSAbortRequest encodes the library-level WDS Abort command. The target must
// use the same WDS client ID as the operation being aborted.
type WDSAbortRequest struct {
	ClientID            uint8
	TransactionID       uint16
	Timeout             time.Duration
	TargetTransactionID uint16
}

// Request converts the abort operation into a QMI WDS request.
func (r WDSAbortRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSAbort,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, r.TargetTransactionID)},
	}
}
