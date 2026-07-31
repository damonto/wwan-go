package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type AuthAKAPRequest struct {
	TransactionID uint32
	Rand          []byte
	AUTN          []byte
	NetworkName   string
	Response      *AuthAKAPResponse
}

func (r *AuthAKAPRequest) Request() *Request {
	data := make([]byte, 40)
	copy(data[0:16], r.Rand)
	copy(data[16:32], r.AUTN)
	data = appendRefValue(data, 32, utf16Bytes(r.NetworkName))
	r.Response = new(AuthAKAPResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceAuth, CIDAuthAKAP, CommandTypeQuery, data),
		Response:      r.Response,
	}
}

type AuthAKAPResponse struct {
	RES  []byte
	IK   []byte
	CK   []byte
	AUTS []byte
}

func (r *AuthAKAPResponse) UnmarshalBinary(data []byte) error {
	var response AuthAKAResponse
	if err := response.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("parsing MBIM auth AKA prime: %w", err)
	}
	*r = AuthAKAPResponse{
		RES:  response.RES,
		IK:   response.IK,
		CK:   response.CK,
		AUTS: response.AUTS,
	}
	return nil
}

type AuthSIMRequest struct {
	TransactionID uint32
	Rand1         []byte
	Rand2         []byte
	Rand3         []byte
	N             uint32
	Response      *AuthSIMResponse
}

func (r *AuthSIMRequest) Request() *Request {
	data := make([]byte, 52)
	copy(data[0:16], r.Rand1)
	copy(data[16:32], r.Rand2)
	copy(data[32:48], r.Rand3)
	binary.LittleEndian.PutUint32(data[48:52], r.N)
	r.Response = new(AuthSIMResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceAuth, CIDAuthSIM, CommandTypeQuery, data),
		Response:      r.Response,
	}
}

type AuthSIMResponse struct {
	SRES1 uint32
	Kc1   uint64
	SRES2 uint32
	Kc2   uint64
	SRES3 uint32
	Kc3   uint64
	N     uint32
}

func (r *AuthSIMResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 40 {
		return fmt.Errorf("parsing MBIM SIM authentication: payload length is %d, want 40", len(data))
	}
	n := binary.LittleEndian.Uint32(data[36:40])
	if n != 2 && n != 3 {
		return fmt.Errorf("parsing MBIM SIM authentication: response count %d, want 2 or 3", n)
	}
	*r = AuthSIMResponse{
		SRES1: binary.LittleEndian.Uint32(data[0:4]),
		Kc1:   binary.LittleEndian.Uint64(data[4:12]),
		SRES2: binary.LittleEndian.Uint32(data[12:16]),
		Kc2:   binary.LittleEndian.Uint64(data[16:24]),
		SRES3: binary.LittleEndian.Uint32(data[24:28]),
		Kc3:   binary.LittleEndian.Uint64(data[28:36]),
		N:     n,
	}
	return nil
}

func (c *Client) AuthenticateAKAP(ctx context.Context, rand, autn []byte, networkName string) (*AuthAKAPResponse, error) {
	if len(rand) != 16 {
		return nil, fmt.Errorf("authenticating MBIM AKA prime: RAND length %d, want 16", len(rand))
	}
	if len(autn) != 16 {
		return nil, fmt.Errorf("authenticating MBIM AKA prime: AUTN length %d, want 16", len(autn))
	}
	request := AuthAKAPRequest{
		TransactionID: c.nextTransactionID(),
		Rand:          slices.Clone(rand),
		AUTN:          slices.Clone(autn),
		NetworkName:   networkName,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		if errors.Is(err, StatusAuthSyncFailure) {
			return request.Response, fmt.Errorf("authenticating MBIM AKA prime: %w", err)
		}
		return nil, fmt.Errorf("authenticating MBIM AKA prime: %w", err)
	}
	return request.Response, nil
}

func (c *Client) AuthenticateSIM(ctx context.Context, rands [][]byte) (AuthSIMResponse, error) {
	if len(rands) != 2 && len(rands) != 3 {
		return AuthSIMResponse{}, fmt.Errorf("authenticating MBIM SIM: challenge count %d, want 2 or 3", len(rands))
	}
	for i, rand := range rands {
		if len(rand) != 16 {
			return AuthSIMResponse{}, fmt.Errorf("authenticating MBIM SIM: RAND %d length %d, want 16", i+1, len(rand))
		}
	}
	request := AuthSIMRequest{
		TransactionID: c.nextTransactionID(),
		Rand1:         slices.Clone(rands[0]),
		Rand2:         slices.Clone(rands[1]),
		N:             uint32(len(rands)),
	}
	if len(rands) == 3 {
		request.Rand3 = slices.Clone(rands[2])
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return AuthSIMResponse{}, fmt.Errorf("authenticating MBIM SIM: %w", err)
	}
	return *request.Response, nil
}

type AuthAKARequest struct {
	TransactionID uint32
	Rand          []byte
	AUTN          []byte
	Response      *AuthAKAResponse
}

func (r *AuthAKARequest) Request() *Request {
	data := make([]byte, 0, len(r.Rand)+len(r.AUTN))
	data = append(data, r.Rand...)
	data = append(data, r.AUTN...)

	r.Response = new(AuthAKAResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceAuth,
			CIDAuthAKA,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type AuthAKAResponse struct {
	RES  []byte
	CK   []byte
	IK   []byte
	AUTS []byte
}

func (r *AuthAKAResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 66 {
		return fmt.Errorf("parsing MBIM auth AKA: payload length is %d, want 66", len(data))
	}
	resLength := int(binary.LittleEndian.Uint32(data[16:20]))
	if resLength > 16 {
		return fmt.Errorf("parsing MBIM auth AKA: RES length %d exceeds 16", resLength)
	}
	r.RES = slices.Clone(data[:resLength])
	r.IK = slices.Clone(data[20:36])
	r.CK = slices.Clone(data[36:52])
	r.AUTS = slices.Clone(data[52:66])
	return nil
}

func (c *Client) AuthenticateAKA(ctx context.Context, rand, autn []byte) (*AuthAKAResponse, error) {
	if len(rand) != 16 {
		return nil, fmt.Errorf("authenticating MBIM AKA: RAND length %d, want 16", len(rand))
	}
	if len(autn) != 16 {
		return nil, fmt.Errorf("authenticating MBIM AKA: AUTN length %d, want 16", len(autn))
	}

	request := AuthAKARequest{
		TransactionID: c.nextTransactionID(),
		Rand:          slices.Clone(rand),
		AUTN:          slices.Clone(autn),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		if errors.Is(err, StatusAuthSyncFailure) {
			return request.Response, fmt.Errorf("authenticating MBIM AKA: %w", err)
		}
		return nil, fmt.Errorf("authenticating MBIM AKA: %w", err)
	}
	return request.Response, nil
}
