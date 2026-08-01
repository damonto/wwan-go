package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const maxAuthenticateFieldLength = 255

func (r AuthenticateRequest) MarshalBinary() ([]byte, error) {
	if len(r.Rand) > maxAuthenticateFieldLength {
		return nil, fmt.Errorf("marshaling QMI UIM authenticate request: rand length %d exceeds %d", len(r.Rand), maxAuthenticateFieldLength)
	}
	if len(r.AUTN) > maxAuthenticateFieldLength {
		return nil, fmt.Errorf("marshaling QMI UIM authenticate request: autn length %d exceeds %d", len(r.AUTN), maxAuthenticateFieldLength)
	}

	body := make([]byte, 0, len(r.Rand)+len(r.AUTN)+2)
	body = append(body, byte(len(r.Rand)))
	body = append(body, r.Rand...)
	body = append(body, byte(len(r.AUTN)))
	body = append(body, r.AUTN...)

	data := make([]byte, 0, 3+len(body))
	data = append(data, byte(r.Context))
	data = binary.LittleEndian.AppendUint16(data, uint16(len(body)))
	data = append(data, body...)
	return data, nil
}

func (r *AuthenticateRequest) UnmarshalBinary(data []byte) error {
	if len(data) < 3 {
		return fmt.Errorf("unmarshaling QMI UIM authenticate request: length %d is too short", len(data))
	}

	r.Context = AuthContext(data[0])
	bodyLen := int(binary.LittleEndian.Uint16(data[1:3]))
	if len(data) != 3+bodyLen {
		return fmt.Errorf("unmarshaling QMI UIM authenticate request: body length %d does not match actual length %d", bodyLen, len(data)-3)
	}

	body := data[3:]
	if len(body) < 1 {
		return errors.New("unmarshaling QMI UIM authenticate request: rand length is missing")
	}
	randLen := int(body[0])
	body = body[1:]
	if len(body) < randLen+1 {
		return fmt.Errorf("unmarshaling QMI UIM authenticate request: rand length %d exceeds remaining %d", randLen, len(body))
	}
	r.Rand = slices.Clone(body[:randLen])
	body = body[randLen:]

	autnLen := int(body[0])
	body = body[1:]
	if len(body) != autnLen {
		return fmt.Errorf("unmarshaling QMI UIM authenticate request: autn length %d does not match actual length %d", autnLen, len(body))
	}
	r.AUTN = slices.Clone(body)
	return nil
}

func (c *Client) Authenticate(ctx context.Context, req AuthenticateRequest) ([]byte, error) {
	response, err := c.authenticateResponse(ctx, req)
	if err != nil {
		return nil, err
	}
	value, ok := tlv.Value(response.TLVs, 0x11)
	if !ok {
		return nil, errors.New("authenticating QMI UIM: authenticate result TLV missing")
	}
	var result qmiLength16Bytes
	if err := result.UnmarshalBinary(value); err != nil {
		return nil, fmt.Errorf("authenticating QMI UIM: %w", err)
	}
	return result, nil
}

func (c *Client) authenticateResponse(
	ctx context.Context,
	req AuthenticateRequest,
) (Response, error) {
	value, err := req.MarshalBinary()
	if err != nil {
		return Response{}, err
	}
	sessionValue, err := putSessionValue(req.Session, req.AID)
	if err != nil {
		return Response{}, fmt.Errorf("authenticating QMI UIM: %w", err)
	}

	resp, err := c.request(ctx, MessageAuthenticate, tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, value),
	})
	if err != nil {
		return Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}
