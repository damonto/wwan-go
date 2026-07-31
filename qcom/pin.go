package qcom

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const uimPINMax = 8

// PINID identifies a UIM PIN credential.
type PINID uint8

const (
	PINIDPIN1 PINID = 1 + iota
	PINIDPIN2
	PINIDUniversal
	PINIDHiddenKey
)

// PINKeyReference identifies an application PIN reference from ETSI TS 102 221.
type PINKeyReference uint8

const (
	PINKeyReferenceApplication1 PINKeyReference = 1 + iota
	PINKeyReferenceApplication2
	PINKeyReferenceApplication3
	PINKeyReferenceApplication4
	PINKeyReferenceApplication5
	PINKeyReferenceApplication6
	PINKeyReferenceApplication7
	PINKeyReferenceApplication8
)

// PINOperationResult contains optional modem details returned by a PIN operation.
type PINOperationResult struct {
	VerifyRetries  uint8
	UnblockRetries uint8
	RetriesKnown   bool
	EncryptedPIN1  []byte
}

// PINProtectionRequest enables or disables one PIN for a UIM session.
type PINProtectionRequest struct {
	Session      Session
	AID          []byte
	ID           PINID
	Enable       bool
	PIN          string
	KeyReference *PINKeyReference
}

// PINVerifyRequest verifies one PIN for a UIM session.
type PINVerifyRequest struct {
	Session      Session
	AID          []byte
	ID           PINID
	PIN          string
	KeyReference *PINKeyReference
}

// PINUnblockRequest unblocks one PIN using its PUK and assigns a new value.
type PINUnblockRequest struct {
	Session      Session
	AID          []byte
	ID           PINID
	PUK          string
	NewPIN       string
	KeyReference *PINKeyReference
}

// PINChangeRequest replaces one PIN after verifying its current value.
type PINChangeRequest struct {
	Session      Session
	AID          []byte
	ID           PINID
	OldPIN       string
	NewPIN       string
	KeyReference *PINKeyReference
}

// SetPINProtection enables or disables PIN verification for a UIM session.
func (c *Client) SetPINProtection(ctx context.Context, req PINProtectionRequest) (PINOperationResult, error) {
	if err := validatePINReference(req.AID, req.ID, req.KeyReference, true); err != nil {
		return PINOperationResult{}, fmt.Errorf("setting PIN protection: %w", err)
	}
	if err := validatePINValue(req.PIN); err != nil {
		return PINOperationResult{}, fmt.Errorf("setting PIN protection: validating PIN: %w", err)
	}
	operation := uint8(0)
	if req.Enable {
		operation = 1
	}
	value := []byte{byte(req.ID), operation, byte(len(req.PIN))}
	value = append(value, req.PIN...)
	sessionValue, err := putSessionValue(req.Session, req.AID)
	if err != nil {
		return PINOperationResult{}, fmt.Errorf("setting PIN protection: %w", err)
	}
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, value),
	}
	if req.KeyReference != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*req.KeyReference)))
	}
	result, err := c.pinOperation(ctx, MessageSetPINProtection, tlvs)
	if err != nil {
		return result, fmt.Errorf("setting PIN protection: %w", err)
	}
	return result, nil
}

// VerifyPIN verifies one PIN before protected UIM content is accessed.
func (c *Client) VerifyPIN(ctx context.Context, req PINVerifyRequest) (PINOperationResult, error) {
	if err := validatePINReference(req.AID, req.ID, req.KeyReference, true); err != nil {
		return PINOperationResult{}, fmt.Errorf("verifying PIN: %w", err)
	}
	if err := validatePINValue(req.PIN); err != nil {
		return PINOperationResult{}, fmt.Errorf("verifying PIN: validating PIN: %w", err)
	}
	value := []byte{byte(req.ID), byte(len(req.PIN))}
	value = append(value, req.PIN...)
	sessionValue, err := putSessionValue(req.Session, req.AID)
	if err != nil {
		return PINOperationResult{}, fmt.Errorf("verifying PIN: %w", err)
	}
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, value),
	}
	if req.KeyReference != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(*req.KeyReference)))
	}
	result, err := c.pinOperation(ctx, MessageVerifyPIN, tlvs)
	if err != nil {
		return result, fmt.Errorf("verifying PIN: %w", err)
	}
	return result, nil
}

// UnblockPIN unblocks a PIN with its PUK and assigns a new PIN.
func (c *Client) UnblockPIN(ctx context.Context, req PINUnblockRequest) (PINOperationResult, error) {
	if err := validatePINReference(req.AID, req.ID, req.KeyReference, false); err != nil {
		return PINOperationResult{}, fmt.Errorf("unblocking PIN: %w", err)
	}
	if err := validatePINValue(req.PUK); err != nil {
		return PINOperationResult{}, fmt.Errorf("unblocking PIN: validating PUK: %w", err)
	}
	if err := validatePINValue(req.NewPIN); err != nil {
		return PINOperationResult{}, fmt.Errorf("unblocking PIN: validating new PIN: %w", err)
	}
	value := []byte{byte(req.ID), byte(len(req.PUK))}
	value = append(value, req.PUK...)
	value = append(value, byte(len(req.NewPIN)))
	value = append(value, req.NewPIN...)
	sessionValue, err := putSessionValue(req.Session, req.AID)
	if err != nil {
		return PINOperationResult{}, fmt.Errorf("unblocking PIN: %w", err)
	}
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, value),
	}
	if req.KeyReference != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*req.KeyReference)))
	}
	result, err := c.pinOperation(ctx, MessageUnblockPIN, tlvs)
	if err != nil {
		return result, fmt.Errorf("unblocking PIN: %w", err)
	}
	return result, nil
}

// ChangePIN replaces a PIN after verifying its current value.
func (c *Client) ChangePIN(ctx context.Context, req PINChangeRequest) (PINOperationResult, error) {
	if err := validatePINReference(req.AID, req.ID, req.KeyReference, true); err != nil {
		return PINOperationResult{}, fmt.Errorf("changing PIN: %w", err)
	}
	if err := validatePINValue(req.OldPIN); err != nil {
		return PINOperationResult{}, fmt.Errorf("changing PIN: validating old PIN: %w", err)
	}
	if err := validatePINValue(req.NewPIN); err != nil {
		return PINOperationResult{}, fmt.Errorf("changing PIN: validating new PIN: %w", err)
	}
	value := []byte{byte(req.ID), byte(len(req.OldPIN))}
	value = append(value, req.OldPIN...)
	value = append(value, byte(len(req.NewPIN)))
	value = append(value, req.NewPIN...)
	sessionValue, err := putSessionValue(req.Session, req.AID)
	if err != nil {
		return PINOperationResult{}, fmt.Errorf("changing PIN: %w", err)
	}
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, value),
	}
	if req.KeyReference != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*req.KeyReference)))
	}
	result, err := c.pinOperation(ctx, MessageChangePIN, tlvs)
	if err != nil {
		return result, fmt.Errorf("changing PIN: %w", err)
	}
	return result, nil
}

func (c *Client) pinOperation(ctx context.Context, id MessageID, tlvs tlv.TLVs) (PINOperationResult, error) {
	resp, err := c.request(ctx, id, tlvs)
	if err != nil {
		return PINOperationResult{}, err
	}
	var result PINOperationResult
	decodeErr := result.UnmarshalTLVs(resp.TLVs)
	if err := resultOK(resp); err != nil {
		return result, err
	}
	if decodeErr != nil {
		return PINOperationResult{}, decodeErr
	}
	if _, ok := tlv.Value(resp.TLVs, 0x12); ok {
		return PINOperationResult{}, errors.New("response indication is not supported")
	}
	if err := cardErrorAt(resp.TLVs, 0x13); err != nil {
		return PINOperationResult{}, err
	}
	return result, nil
}

// UnmarshalTLVs decodes PIN retry and encrypted-PIN result fields.
func (r *PINOperationResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var result PINOperationResult
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing PIN retries: TLV length %d, want 2", len(value))
		}
		result.VerifyRetries = value[0]
		result.UnblockRetries = value[1]
		result.RetriesKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) < 1 {
			return errors.New("parsing encrypted PIN1: length is missing")
		}
		length := int(value[0])
		if len(value) != 1+length {
			return errors.New("parsing encrypted PIN1: value is truncated")
		}
		result.EncryptedPIN1 = slices.Clone(value[1:])
	}
	*r = result
	return nil
}

func validatePINReference(aid []byte, id PINID, keyReference *PINKeyReference, allowHidden bool) error {
	if err := validateUIMAIDLength(aid); err != nil {
		return err
	}
	if id < PINIDPIN1 || id > PINIDHiddenKey || !allowHidden && id == PINIDHiddenKey {
		return fmt.Errorf("invalid PIN ID %d", id)
	}
	if keyReference != nil && (*keyReference < PINKeyReferenceApplication1 || *keyReference > PINKeyReferenceApplication8) {
		return fmt.Errorf("invalid PIN key reference %d", *keyReference)
	}
	return nil
}

func validatePINValue(value string) error {
	if value == "" {
		return errors.New("value is empty")
	}
	if len(value) > uimPINMax {
		return fmt.Errorf("value length %d exceeds %d", len(value), uimPINMax)
	}
	for i := range len(value) {
		if value[i] > 0x7F {
			return errors.New("value contains a non-ASCII character")
		}
	}
	return nil
}

func cardErrorAt(tlvs tlv.TLVs, kind byte) error {
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI card result: TLV length %d, want 2", len(value))
	}
	statusWord := uint16(value[0])<<8 | uint16(value[1])
	if statusWord == 0x9000 {
		return nil
	}
	return cardStatusError(statusWord)
}
