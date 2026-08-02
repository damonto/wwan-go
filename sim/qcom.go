package sim

import (
	"context"
	"errors"
	"fmt"
	"slices"

	qc "github.com/damonto/wwan-go/qcom"
	simcard "github.com/damonto/wwan-go/sim/card"
	"github.com/damonto/wwan-go/sim/command"
	"github.com/damonto/wwan-go/sim/simfile"
	"github.com/damonto/wwan-go/sim/stk"
)

var (
	qcomUSIMAIDPrefix = []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
	qcomISIMAIDPrefix = []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	qcomCurrentADF    = []byte{0x7F, 0xFF}
)

type QCOM struct {
	client *qc.Client
}

func NewQCOM(client *qc.Client) (*QCOM, error) {
	if client == nil {
		return nil, errors.New("creating QCOM adapter: client is nil")
	}
	return &QCOM{client: client}, nil
}

func (r *QCOM) ListApplications(ctx context.Context) ([]simcard.Application, error) {
	attrs, err := r.FileAttributes(ctx, simcard.FileRef{Path: efDirFile})
	if err != nil {
		return nil, fmt.Errorf("reading EF_DIR: %w", err)
	}
	if attrs.FileStructure != simfile.StructureLinearFixed {
		return nil, errors.New("reading EF_DIR: unexpected file structure")
	}

	apps := make([]simcard.Application, 0, attrs.RecordCount)
	for recordID := uint16(1); recordID <= attrs.RecordCount; recordID++ {
		record, err := r.ReadRecord(ctx, simcard.RecordRead{
			File:   simcard.FileRef{Path: efDirFile},
			Record: recordID,
			Length: attrs.RecordSize,
		})
		if err != nil {
			return nil, fmt.Errorf("reading EF_DIR record %d: %w", recordID, err)
		}

		var parsed simfile.EFDirRecord
		if err := parsed.UnmarshalBinary(record); err != nil {
			return nil, fmt.Errorf("parsing EF_DIR record %d: %w", recordID, err)
		}
		if len(parsed.AID) == 0 {
			continue
		}
		apps = append(apps, simcard.Application{
			AID:   slices.Clone(parsed.AID),
			Label: parsed.Label,
		})
	}
	return apps, nil
}

func (r *QCOM) FileAttributes(ctx context.Context, file simcard.FileRef) (simcard.FileAttributes, error) {
	attrs, err := r.fileAttributes(ctx, file)
	if err != nil {
		return simcard.FileAttributes{}, err
	}
	return simcard.FileAttributes{
		FileStructure: simfile.FileStructure(attrs.FileStructure),
		FileType:      simfile.FileType(attrs.FileType),
		RecordSize:    attrs.RecordSize,
		RecordCount:   attrs.RecordCount,
		FileSize:      attrs.FileSize,
	}, nil
}

func (r *QCOM) ReadTransparent(ctx context.Context, req simcard.TransparentRead) ([]byte, error) {
	files, err := r.files(req.File)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i, file := range files {
		data, err := r.client.ReadTransparent(ctx, qc.TransparentRead{
			File:   file,
			Offset: req.Offset,
			Length: req.Length,
		})
		if err == nil {
			return data, nil
		}
		lastErr = err
		if i == 0 && len(files) > 1 && retryableQCOMSessionError(err) {
			continue
		}
		break
	}
	return nil, lastErr
}

func (r *QCOM) ReadRecord(ctx context.Context, req simcard.RecordRead) ([]byte, error) {
	files, err := r.files(req.File)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i, file := range files {
		data, err := r.client.ReadRecord(ctx, qc.RecordRead{
			File:   file,
			Record: req.Record,
			Length: req.Length,
		})
		if err == nil {
			return data, nil
		}
		lastErr = err
		if i == 0 && len(files) > 1 && retryableQCOMSessionError(err) {
			continue
		}
		break
	}
	return nil, lastErr
}

func (r *QCOM) Authenticate3G(ctx context.Context, req simcard.AuthenticateRequest) ([]byte, error) {
	requests, err := r.authRequests(req)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i, request := range requests {
		data, err := r.client.Authenticate(ctx, request)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if i == 0 && len(requests) > 1 && retryableQCOMSessionError(err) {
			continue
		}
		break
	}
	return nil, lastErr
}

func (r *QCOM) SMSPPDownload(ctx context.Context, req simcard.SMSPPDownloadRequest) (simcard.SMSPPDownloadResponse, error) {
	envelope, err := command.SMSPPDownload{
		ServiceCenterAddress: req.ServiceCenterAddress,
		TPDU:                 slices.Clone(req.TPDU),
	}.Envelope()
	if err != nil {
		return simcard.SMSPPDownloadResponse{}, fmt.Errorf("building SMS-PP envelope: %w", err)
	}

	resp, err := r.client.SendEnvelope(ctx, envelope)
	if err != nil {
		return simcard.SMSPPDownloadResponse{}, err
	}
	return simcard.SMSPPDownloadResponse{
		SW1:  resp.SW1,
		SW2:  resp.SW2,
		Data: slices.Clone(resp.Data),
	}, nil
}

func (r *QCOM) STK() (*STK, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("creating QCOM STK: client is nil")
	}
	return newSTK(r)
}

func (r *QCOM) Commands(ctx context.Context, profile stk.Profile) (<-chan STKSession, error) {
	cat, err := r.qcomCAT()
	if err != nil {
		return nil, err
	}
	commands, err := cat.Commands(ctx, profile.QMIEventMask(), profile.QMIFullFunctionMask())
	if err != nil {
		return nil, err
	}

	out := make(chan STKSession, 8)
	go func() {
		defer close(out)
		for raw := range commands {
			var proactive stk.ProactiveCommand
			if err := proactive.UnmarshalBinary(raw.Data); err != nil {
				continue
			}
			select {
			case out <- STKSession{
				Ref:          raw.Ref,
				Command:      proactive.Command,
				responseKind: qcomResponseKind(raw),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (r *QCOM) TerminalResponse(ctx context.Context, ref uint32, response []byte) error {
	cat, err := r.qcomCAT()
	if err != nil {
		return err
	}
	return cat.TerminalResponse(ctx, ref, slices.Clone(response))
}

func (r *QCOM) Respond(ctx context.Context, session STKSession, response stk.TerminalResponse) error {
	cat, err := r.qcomCAT()
	if err != nil {
		return err
	}
	if session.responseKind == stkResponseEventConfirmation {
		confirmation, ok := qcomEventConfirmation(session.Command, response)
		if !ok {
			return fmt.Errorf("sending QMI CAT event confirmation: command %T does not support event confirmation", session.Command)
		}
		return cat.EventConfirmation(ctx, confirmation)
	}
	if session.responseKind == stkResponseDefault {
		if confirmation, ok := qcomEventConfirmation(session.Command, response); ok {
			return cat.EventConfirmation(ctx, confirmation)
		}
	}
	data, err := response.MarshalFor(session.Command)
	if err != nil {
		return err
	}
	return cat.TerminalResponse(ctx, session.Ref, data)
}

func qcomResponseKind(command qc.CATCommand) stkResponseKind {
	if !command.ExpectedResponseKnown {
		return stkResponseDefault
	}
	if command.ExpectedResponse == qc.CATExpectedResponseEventConfirmation {
		return stkResponseEventConfirmation
	}
	return stkResponseTerminal
}

func (r *QCOM) Envelope(ctx context.Context, envelope []byte) (stk.EnvelopeResponse, error) {
	cat, err := r.qcomCAT()
	if err != nil {
		return stk.EnvelopeResponse{}, err
	}
	resp, err := cat.Envelope(ctx, envelope, stk.EnvelopeType(envelope))
	if err != nil {
		return stk.EnvelopeResponse{}, err
	}
	return stk.EnvelopeResponse{SW1: resp.SW1, SW2: resp.SW2, Data: slices.Clone(resp.Data)}, nil
}

func (r *QCOM) Close() error {
	return r.client.Close()
}

func (r *QCOM) qcomCAT() (*qc.CAT, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("using QCOM STK: client is nil")
	}
	return qc.NewCAT(r.client), nil
}

func qcomEventConfirmation(command stk.Command, response stk.TerminalResponse) (qc.CATEventConfirmation, bool) {
	confirmed := response.Result < stk.ResultUserTermination
	notDisplayed := false

	switch cmd := command.(type) {
	case stk.OpenChannelCommand:
		return qc.CATEventConfirmation{
			UserConfirmed: &confirmed,
			IconDisplayed: &notDisplayed,
		}, true
	case stk.CloseChannelCommand, stk.ReceiveDataCommand, stk.SendDataCommand:
		return qc.CATEventConfirmation{IconDisplayed: &notDisplayed}, true
	case stk.SimpleCommand:
		// Qualcomm performs these commands in the modem; the
		// control point only confirms whether their alpha/icon was displayed.
		switch cmd.Details.Type {
		case stk.CommandRefresh,
			stk.CommandSendSS,
			stk.CommandSendUSSD,
			stk.CommandSendSMS,
			stk.CommandSendDTMF,
			stk.CommandRunATCommand:
			return qc.CATEventConfirmation{IconDisplayed: &notDisplayed}, true
		}
	}
	return qc.CATEventConfirmation{}, false
}

func (r *QCOM) fileAttributes(ctx context.Context, file simcard.FileRef) (qc.FileAttributes, error) {
	files, err := r.files(file)
	if err != nil {
		return qc.FileAttributes{}, err
	}
	var lastErr error
	for i, file := range files {
		attrs, err := r.client.GetFileAttributes(ctx, file)
		if err == nil {
			return attrs, nil
		}
		lastErr = err
		if i == 0 && len(files) > 1 && retryableQCOMSessionError(err) {
			continue
		}
		break
	}
	return qc.FileAttributes{}, lastErr
}

func (r *QCOM) files(file simcard.FileRef) ([]qc.File, error) {
	path := qcomFilePath(file)
	if len(file.AID) == 0 {
		return []qc.File{{Session: qc.SessionPrimaryGWProvisioning, Path: path}}, nil
	}
	if hasPrefix(file.AID, qcomISIMAIDPrefix) {
		cardSession, err := qcomCardSession(r.client.Slot())
		if err != nil {
			return nil, err
		}
		nonProvisioningSession, err := qcomNonProvisioningSession(r.client.Slot())
		if err != nil {
			return nil, err
		}
		return []qc.File{
			{Session: nonProvisioningSession, AID: slices.Clone(file.AID), Path: path},
			{Session: cardSession, Path: path},
		}, nil
	}
	if hasPrefix(file.AID, qcomUSIMAIDPrefix) {
		return []qc.File{{Session: qc.SessionPrimaryGWProvisioning, Path: path}}, nil
	}
	return []qc.File{{Session: qc.SessionPrimaryGWProvisioning, AID: slices.Clone(file.AID), Path: path}}, nil
}

func (r *QCOM) authRequests(req simcard.AuthenticateRequest) ([]qc.AuthenticateRequest, error) {
	ctx := qc.AuthContext3G
	if req.EAPAKA || hasPrefix(req.AID, qcomISIMAIDPrefix) {
		ctx = qc.AuthContextIMSAKA
	}
	if len(req.AID) == 0 {
		return []qc.AuthenticateRequest{{
			Session: qc.SessionPrimaryGWProvisioning,
			Context: ctx,
			Rand:    slices.Clone(req.Rand),
			AUTN:    slices.Clone(req.AUTN),
		}}, nil
	}
	if hasPrefix(req.AID, qcomISIMAIDPrefix) {
		cardSession, err := qcomCardSession(r.client.Slot())
		if err != nil {
			return nil, err
		}
		nonProvisioningSession, err := qcomNonProvisioningSession(r.client.Slot())
		if err != nil {
			return nil, err
		}
		return []qc.AuthenticateRequest{
			{Session: cardSession, AID: slices.Clone(req.AID), Context: ctx, Rand: slices.Clone(req.Rand), AUTN: slices.Clone(req.AUTN)},
			{Session: nonProvisioningSession, AID: slices.Clone(req.AID), Context: ctx, Rand: slices.Clone(req.Rand), AUTN: slices.Clone(req.AUTN)},
		}, nil
	}
	if hasPrefix(req.AID, qcomUSIMAIDPrefix) {
		return []qc.AuthenticateRequest{{
			Session: qc.SessionPrimaryGWProvisioning,
			Context: ctx,
			Rand:    slices.Clone(req.Rand),
			AUTN:    slices.Clone(req.AUTN),
		}}, nil
	}
	return []qc.AuthenticateRequest{{
		Session: qc.SessionPrimaryGWProvisioning,
		AID:     slices.Clone(req.AID),
		Context: ctx,
		Rand:    slices.Clone(req.Rand),
		AUTN:    slices.Clone(req.AUTN),
	}}, nil
}

func qcomFilePath(file simcard.FileRef) []byte {
	if len(file.AID) != 0 {
		return joinBytes(masterFile, qcomCurrentADF, file.Path)
	}
	if hasPrefix(file.Path, masterFile) {
		return slices.Clone(file.Path)
	}
	return joinBytes(masterFile, file.Path)
}

func qcomCardSession(slot uint8) (qc.Session, error) {
	switch slot {
	case 1:
		return qc.SessionCardSlot1, nil
	case 2:
		return qc.SessionCardSlot2, nil
	case 3:
		return qc.SessionCardSlot3, nil
	case 4:
		return qc.SessionCardSlot4, nil
	case 5:
		return qc.SessionCardSlot5, nil
	default:
		return 0, fmt.Errorf("mapping card session: slot %d is out of range", slot)
	}
}

func qcomNonProvisioningSession(slot uint8) (qc.Session, error) {
	switch slot {
	case 1:
		return qc.SessionNonProvisioningSlot1, nil
	case 2:
		return qc.SessionNonProvisioningSlot2, nil
	case 3:
		return qc.SessionNonProvisioningSlot3, nil
	case 4:
		return qc.SessionNonProvisioningSlot4, nil
	case 5:
		return qc.SessionNonProvisioningSlot5, nil
	default:
		return 0, fmt.Errorf("mapping nonprovisioning session: slot %d is out of range", slot)
	}
}

func retryableQCOMSessionError(err error) bool {
	return errors.Is(err, qc.QMIErrorSessionInactive) ||
		errors.Is(err, qc.QMIErrorSessionInvalid) ||
		errors.Is(err, qc.QMIErrorInvalidSessionType) ||
		errors.Is(err, qc.QMIErrorAuthenticationFailed) ||
		errors.Is(err, qc.QMIErrorAccessDenied)
}

func hasPrefix(value, prefix []byte) bool {
	return len(value) >= len(prefix) && slices.Equal(value[:len(prefix)], prefix)
}

func joinBytes(parts ...[]byte) []byte {
	total := 0
	for _, part := range parts {
		total += len(part)
	}

	buf := make([]byte, 0, total)
	for _, part := range parts {
		buf = append(buf, part...)
	}
	return buf
}
