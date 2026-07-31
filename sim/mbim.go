package sim

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/damonto/wwan-go/mbim"
	simcard "github.com/damonto/wwan-go/sim/card"
	"github.com/damonto/wwan-go/sim/command"
	"github.com/damonto/wwan-go/sim/simfile"
	"github.com/damonto/wwan-go/sim/stk"
)

const (
	mbimSTKCleanupTimeout          = 5 * time.Second
	mbimSTKPACHostControlLength    = 32
	mbimSTKQueuedCommandBufferSize = 8
)

type MBIM struct {
	client *mbim.Client
}

func NewMBIM(client *mbim.Client) (*MBIM, error) {
	if client == nil {
		return nil, errors.New("creating MBIM adapter: client is nil")
	}
	return &MBIM{client: client}, nil
}

func (m *MBIM) ListApplications(ctx context.Context) ([]simcard.Application, error) {
	apps, err := m.client.ListApplications(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]simcard.Application, 0, len(apps))
	for _, app := range apps {
		out = append(out, simcard.Application{
			AID:   slices.Clone(app.AID),
			Label: app.Label,
		})
	}
	return out, nil
}

func (m *MBIM) FileAttributes(ctx context.Context, file simcard.FileRef) (simcard.FileAttributes, error) {
	attrs, err := m.client.FileAttributes(ctx, mbim.FileRef{
		AID:  slices.Clone(file.AID),
		Path: slices.Clone(file.Path),
	})
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

func (m *MBIM) ReadTransparent(ctx context.Context, req simcard.TransparentRead) ([]byte, error) {
	return m.client.ReadTransparent(ctx, mbim.TransparentRead{
		File: mbim.FileRef{
			AID:  slices.Clone(req.File.AID),
			Path: slices.Clone(req.File.Path),
		},
		Offset: req.Offset,
		Length: req.Length,
	})
}

func (m *MBIM) ReadRecord(ctx context.Context, req simcard.RecordRead) ([]byte, error) {
	return m.client.ReadRecord(ctx, mbim.RecordRead{
		File: mbim.FileRef{
			AID:  slices.Clone(req.File.AID),
			Path: slices.Clone(req.File.Path),
		},
		Record: req.Record,
	})
}

func (m *MBIM) Authenticate3G(ctx context.Context, req simcard.AuthenticateRequest) ([]byte, error) {
	resp, err := m.client.AuthenticateAKA(ctx, req.Rand, req.AUTN)
	if err != nil {
		if !errors.Is(err, mbim.StatusAuthSyncFailure) || resp == nil {
			return nil, err
		}
	}

	result := command.Authenticate3GResult{Reject: true}
	if len(resp.RES) != 0 {
		result = command.Authenticate3GResult{
			RES: slices.Clone(resp.RES),
			CK:  slices.Clone(resp.CK),
			IK:  slices.Clone(resp.IK),
		}
	} else if slices.ContainsFunc(resp.AUTS, func(b byte) bool { return b != 0 }) {
		result = command.Authenticate3GResult{AUTS: slices.Clone(resp.AUTS)}
	}
	return result.MarshalBinary()
}

func (m *MBIM) SMSPPDownload(ctx context.Context, req simcard.SMSPPDownloadRequest) (simcard.SMSPPDownloadResponse, error) {
	envelope, err := command.SMSPPDownload{
		ServiceCenterAddress: req.ServiceCenterAddress,
		TPDU:                 slices.Clone(req.TPDU),
	}.Envelope()
	if err != nil {
		return simcard.SMSPPDownloadResponse{}, fmt.Errorf("building SMS-PP envelope: %w", err)
	}
	if err := m.client.STKEnvelope(ctx, envelope); err != nil {
		return simcard.SMSPPDownloadResponse{}, err
	}
	return simcard.SMSPPDownloadResponse{SW1: 0x90, SW2: 0x00}, nil
}

func (m *MBIM) STK() (*STK, error) {
	if m == nil || m.client == nil {
		return nil, errors.New("creating MBIM STK: client is nil")
	}
	return newSTK(m)
}

func (m *MBIM) Commands(ctx context.Context, profile stk.Profile) (<-chan STKSession, error) {
	if m == nil || m.client == nil {
		return nil, errors.New("watching MBIM STK commands: client is nil")
	}

	watchCtx, cancel := context.WithCancel(ctx)
	pacs, err := m.client.WatchSTKPACResults(watchCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching MBIM STK commands: %w", err)
	}
	if err := m.setSTKPAC(ctx, profile); err != nil {
		cancel()
		return nil, fmt.Errorf("watching MBIM STK commands: %w", err)
	}

	out := make(chan STKSession, mbimSTKQueuedCommandBufferSize)
	go func() {
		defer close(out)
		defer cancel()
		defer m.clearSTKPAC()

		for result := range pacs {
			if result.Err != nil {
				select {
				case out <- STKSession{Err: result.Err}:
				case <-ctx.Done():
				}
				return
			}
			pac := result.Value
			if pac.Type != mbim.STKPACTypeProactiveCommand {
				continue
			}
			var proactive stk.ProactiveCommand
			if err := proactive.UnmarshalBinary(pac.Command); err != nil {
				select {
				case out <- STKSession{Err: fmt.Errorf("parsing MBIM STK proactive command: %w", err)}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case out <- STKSession{Command: proactive.Command}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (m *MBIM) TerminalResponse(ctx context.Context, _ uint32, response []byte) error {
	if m == nil || m.client == nil {
		return errors.New("sending MBIM STK terminal response: client is nil")
	}
	_, err := m.client.STKTerminalResponse(ctx, response)
	return err
}

func (m *MBIM) Respond(ctx context.Context, session STKSession, response stk.TerminalResponse) error {
	data, err := response.MarshalFor(session.Command)
	if err != nil {
		return err
	}
	return m.TerminalResponse(ctx, session.Ref, data)
}

func (m *MBIM) Envelope(ctx context.Context, envelope []byte) (stk.EnvelopeResponse, error) {
	if m == nil || m.client == nil {
		return stk.EnvelopeResponse{}, errors.New("running MBIM STK envelope: client is nil")
	}
	if err := m.client.STKEnvelope(ctx, envelope); err != nil {
		return stk.EnvelopeResponse{}, err
	}
	return stk.EnvelopeResponse{}, nil
}

func (m *MBIM) Close() error {
	return m.client.Close()
}

func (m *MBIM) setSTKPAC(ctx context.Context, profile stk.Profile) error {
	_, err := m.client.SetSTKPAC(ctx, mbimPACHostControl(profile))
	return err
}

func (m *MBIM) clearSTKPAC() {
	ctx, cancel := context.WithTimeout(context.Background(), mbimSTKCleanupTimeout)
	defer cancel()
	_, _ = m.client.SetSTKPAC(ctx, make([]byte, mbimSTKPACHostControlLength))
}

func mbimPACHostControl(profile stk.Profile) []byte {
	control := make([]byte, mbimSTKPACHostControlLength)
	for _, command := range profile.ProactiveCommandTypes() {
		bit := int(command)
		if bit < len(control)*8 {
			control[bit/8] |= 1 << (bit % 8)
		}
	}
	return control
}
