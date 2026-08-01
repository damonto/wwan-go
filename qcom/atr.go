package qcom

import (
	"context"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func (c *Client) ATR(ctx context.Context) ([]byte, error) {
	resp, err := c.request(ctx, MessageGetATR, tlv.TLVs{
		tlv.Uint(0x01, c.slot),
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

	var atr qmiLength8Bytes
	if err := atr.UnmarshalBinary(value); err != nil {
		return nil, fmt.Errorf("reading QMI UIM ATR: %w", err)
	}
	return atr, nil
}
