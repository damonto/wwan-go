package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type USSDAction uint32

const (
	USSDActionInitiate USSDAction = 0
	USSDActionContinue USSDAction = 1
	USSDActionCancel   USSDAction = 2
)

type USSDResponse uint32

const (
	USSDResponseNoActionRequired      USSDResponse = 0
	USSDResponseActionRequired        USSDResponse = 1
	USSDResponseTerminatedByNetwork   USSDResponse = 2
	USSDResponseOtherLocalClient      USSDResponse = 3
	USSDResponseOperationNotSupported USSDResponse = 4
	USSDResponseNetworkTimeout        USSDResponse = 5
)

type USSDSessionState uint32

const (
	USSDSessionStateNew      USSDSessionState = 0
	USSDSessionStateExisting USSDSessionState = 1
)

type USSDInfo struct {
	Response         USSDResponse
	SessionState     USSDSessionState
	DataCodingScheme uint32
	Payload          []byte
}

func validUSSDAction(action USSDAction) bool {
	return action <= USSDActionCancel
}

func validUSSDResponse(response USSDResponse) bool {
	return response <= USSDResponseNetworkTimeout
}

type USSDRequest struct {
	TransactionID    uint32
	Action           USSDAction
	DataCodingScheme uint32
	Payload          []byte
	Response         *USSDInfo
}

func (r *USSDRequest) Request() *Request {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], uint32(r.Action))
	binary.LittleEndian.PutUint32(data[4:8], r.DataCodingScheme)
	data = appendRefValue(data, 8, r.Payload)
	r.Response = new(USSDInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command:       command(ServiceUSSD, CIDUSSD, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func (r *USSDInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 20 {
		return errors.New("parsing MBIM USSD response: payload is truncated")
	}
	response := USSDResponse(binary.LittleEndian.Uint32(data[0:4]))
	if !validUSSDResponse(response) {
		return fmt.Errorf("parsing MBIM USSD response: code %d is outside 0..%d", response, USSDResponseNetworkTimeout)
	}
	sessionState := USSDSessionState(binary.LittleEndian.Uint32(data[4:8]))
	if sessionState > USSDSessionStateExisting {
		return fmt.Errorf("parsing MBIM USSD response: session state %d is outside 0..%d", sessionState, USSDSessionStateExisting)
	}
	payloadRef, err := readOffsetSizeRef(data, 12)
	if err != nil {
		return fmt.Errorf("parsing MBIM USSD payload: %w", err)
	}
	if err := validateDataBufferRefs(data, 20, []valueRef{payloadRef}); err != nil {
		return fmt.Errorf("parsing MBIM USSD payload: %w", err)
	}
	if payloadRef.size > 160 {
		return fmt.Errorf("parsing MBIM USSD payload: size %d exceeds 160 bytes", payloadRef.size)
	}
	payloadApplicable := response == USSDResponseNoActionRequired || response == USSDResponseActionRequired
	if payloadApplicable && payloadRef.size == 0 {
		return errors.New("parsing MBIM USSD payload: applicable response has an empty payload")
	}
	if !payloadApplicable && (payloadRef.offset != 0 || payloadRef.size != 0) {
		return fmt.Errorf("parsing MBIM USSD payload: response %d must not carry a payload", response)
	}
	*r = USSDInfo{
		Response:         response,
		SessionState:     sessionState,
		DataCodingScheme: binary.LittleEndian.Uint32(data[8:12]),
		Payload:          payloadRef.bytes(data),
	}
	return nil
}

func (c *Client) USSD(ctx context.Context, action USSDAction, dataCodingScheme uint32, payload []byte) (USSDInfo, error) {
	if !validUSSDAction(action) {
		return USSDInfo{}, fmt.Errorf("running MBIM USSD operation: action %d is outside 0..%d", action, USSDActionCancel)
	}
	if len(payload) > 160 {
		return USSDInfo{}, fmt.Errorf("running MBIM USSD operation: payload length %d exceeds 160 bytes", len(payload))
	}
	if action == USSDActionCancel && len(payload) != 0 {
		return USSDInfo{}, errors.New("running MBIM USSD operation: cancel payload must be empty")
	}
	if action != USSDActionCancel && len(payload) == 0 {
		return USSDInfo{}, errors.New("running MBIM USSD operation: initiate and continue payloads must not be empty")
	}
	request := USSDRequest{
		TransactionID:    c.nextTransactionID(),
		Action:           action,
		DataCodingScheme: dataCodingScheme,
		Payload:          slices.Clone(payload),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return USSDInfo{}, fmt.Errorf("running MBIM USSD operation: %w", err)
	}
	response := *request.Response
	response.Payload = slices.Clone(response.Payload)
	return response, nil
}
