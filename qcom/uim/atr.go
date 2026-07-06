package uim

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func (r *Reader) ATR(ctx context.Context) ([]byte, error) {
	resp, err := r.request(ctx, qcom.MessageGetATR, tlv.TLVs{
		tlv.Uint(0x01, r.slot),
	})
	if err != nil {
		return nil, fmt.Errorf("reading QMI UIM ATR: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return nil, fmt.Errorf("reading QMI UIM ATR: %w", err)
	}

	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		return nil, errors.New("reading QMI UIM ATR: ATR TLV missing")
	}

	atr, err := decodeATR(value)
	if err != nil {
		return nil, fmt.Errorf("reading QMI UIM ATR: %w", err)
	}
	return atr, nil
}

func decodeATR(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("ATR length is missing")
	}

	length := int(data[0])
	if len(data) < 1+length {
		return nil, fmt.Errorf("ATR length %d exceeds remaining %d", length, len(data)-1)
	}
	return slices.Clone(data[1 : 1+length]), nil
}
