package usim

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/damonto/uicc-go/mbim"
	usimcard "github.com/damonto/uicc-go/usim/card"
	"github.com/damonto/uicc-go/usim/command"
	"github.com/damonto/uicc-go/usim/simfile"
	"github.com/damonto/uicc-go/usim/stk"
)

const (
	mbimSTKCleanupTimeout          = 5 * time.Second
	mbimSTKPACHostControlLength    = 32
	mbimSTKQueuedCommandBufferSize = 8
)

type MBIM struct {
	reader *mbim.Reader
}

func NewMBIM(reader *mbim.Reader) (*MBIM, error) {
	if reader == nil {
		return nil, errors.New("creating MBIM adapter: reader is nil")
	}
	return &MBIM{reader: reader}, nil
}

// OpenIMSPDN starts the LTE IMS PDN through the MBIM modem backing this reader.
func (r *MBIM) OpenIMSPDN(ctx context.Context, cfg IMSPDNConfig) (*IMSPDNSession, error) {
	if r == nil || r.reader == nil {
		return nil, errors.New("opening IMS PDN: MBIM reader is nil")
	}
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	session, err := r.reader.OpenIMSPDN(ctx, mbim.IMSPDNConfig{
		APN:            normalized.APN,
		IPType:         mbimContextIPTypeForPDNType(normalized.PDNType),
		RequestTimeout: normalized.RequestTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("opening IMS PDN: %w", err)
	}
	return &IMSPDNSession{
		info: func() IMSPDNInfo {
			info := session.Info()
			return IMSPDNInfo{
				SessionID:       info.SessionID,
				LocalIPv4:       info.LocalIPv4,
				LocalIPv6:       info.LocalIPv6,
				PCSCFIPs:        info.PCSCFIPs,
				VoPSKnown:       info.VoPSKnown,
				VoPSSupported:   info.VoPSSupported,
				PacketDataReady: info.PacketDataReady,
			}
		},
		close: session.Close,
	}, nil
}

func (r *MBIM) ListApplications(ctx context.Context) ([]usimcard.Application, error) {
	apps, err := r.reader.ListApplications(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]usimcard.Application, 0, len(apps))
	for _, app := range apps {
		out = append(out, usimcard.Application{
			AID:   slices.Clone(app.AID),
			Label: app.Label,
		})
	}
	return out, nil
}

func (r *MBIM) FileAttributes(ctx context.Context, file usimcard.FileRef) (usimcard.FileAttributes, error) {
	attrs, err := r.reader.FileAttributes(ctx, mbim.FileRef{
		AID:  slices.Clone(file.AID),
		Path: slices.Clone(file.Path),
	})
	if err != nil {
		return usimcard.FileAttributes{}, err
	}
	return usimcard.FileAttributes{
		FileStructure: simfile.FileStructure(attrs.FileStructure),
		FileType:      simfile.FileType(attrs.FileType),
		RecordSize:    attrs.RecordSize,
		RecordCount:   attrs.RecordCount,
		FileSize:      attrs.FileSize,
	}, nil
}

func (r *MBIM) ReadTransparent(ctx context.Context, req usimcard.TransparentRead) ([]byte, error) {
	return r.reader.ReadTransparent(ctx, mbim.TransparentRead{
		File: mbim.FileRef{
			AID:  slices.Clone(req.File.AID),
			Path: slices.Clone(req.File.Path),
		},
		Offset: req.Offset,
		Length: req.Length,
	})
}

func (r *MBIM) ReadRecord(ctx context.Context, req usimcard.RecordRead) ([]byte, error) {
	return r.reader.ReadRecord(ctx, mbim.RecordRead{
		File: mbim.FileRef{
			AID:  slices.Clone(req.File.AID),
			Path: slices.Clone(req.File.Path),
		},
		Record: req.Record,
	})
}

func (r *MBIM) Authenticate3G(ctx context.Context, req usimcard.AuthenticateRequest) ([]byte, error) {
	resp, err := r.reader.AuthenticateAKA(ctx, req.Rand, req.AUTN)
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

func (r *MBIM) SMSPPDownload(ctx context.Context, req usimcard.SMSPPDownloadRequest) (usimcard.SMSPPDownloadResponse, error) {
	envelope, err := command.SMSPPDownload{
		ServiceCenterAddress: req.ServiceCenterAddress,
		TPDU:                 slices.Clone(req.TPDU),
	}.Envelope()
	if err != nil {
		return usimcard.SMSPPDownloadResponse{}, fmt.Errorf("building SMS-PP envelope: %w", err)
	}
	if err := r.reader.STKEnvelope(ctx, envelope); err != nil {
		return usimcard.SMSPPDownloadResponse{}, err
	}
	return usimcard.SMSPPDownloadResponse{SW1: 0x90, SW2: 0x00}, nil
}

func (r *MBIM) STK() (*STK, error) {
	if r == nil || r.reader == nil {
		return nil, errors.New("creating MBIM STK: reader is nil")
	}
	return newSTK(r)
}

func (r *MBIM) Commands(ctx context.Context, profile stk.Profile) (<-chan STKSession, error) {
	if r == nil || r.reader == nil {
		return nil, errors.New("watching MBIM STK commands: reader is nil")
	}

	watchCtx, cancel := context.WithCancel(ctx)
	pacs, err := r.reader.WatchSTKPAC(watchCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching MBIM STK commands: %w", err)
	}
	if err := r.setSTKPAC(ctx, profile); err != nil {
		cancel()
		return nil, fmt.Errorf("watching MBIM STK commands: %w", err)
	}

	out := make(chan STKSession, mbimSTKQueuedCommandBufferSize)
	go func() {
		defer close(out)
		defer cancel()
		defer r.clearSTKPAC()

		for pac := range pacs {
			if pac.Type != mbim.STKPACTypeProactiveCommand {
				continue
			}
			var proactive stk.ProactiveCommand
			if err := proactive.UnmarshalBinary(pac.Command); err != nil {
				continue
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

func (r *MBIM) TerminalResponse(ctx context.Context, _ uint32, response []byte) error {
	if r == nil || r.reader == nil {
		return errors.New("sending MBIM STK terminal response: reader is nil")
	}
	_, err := r.reader.STKTerminalResponse(ctx, response)
	return err
}

func (r *MBIM) Respond(ctx context.Context, session STKSession, response stk.TerminalResponse) error {
	data, err := response.MarshalFor(session.Command)
	if err != nil {
		return err
	}
	return r.TerminalResponse(ctx, session.Ref, data)
}

func (r *MBIM) Envelope(ctx context.Context, envelope []byte) (stk.EnvelopeResponse, error) {
	if r == nil || r.reader == nil {
		return stk.EnvelopeResponse{}, errors.New("running MBIM STK envelope: reader is nil")
	}
	if err := r.reader.STKEnvelope(ctx, envelope); err != nil {
		return stk.EnvelopeResponse{}, err
	}
	return stk.EnvelopeResponse{}, nil
}

func (r *MBIM) Close() error {
	return r.reader.Close()
}

func (r *MBIM) setSTKPAC(ctx context.Context, profile stk.Profile) error {
	_, err := r.reader.SetSTKPAC(ctx, mbimPACHostControl(profile))
	return err
}

func (r *MBIM) clearSTKPAC() {
	ctx, cancel := context.WithTimeout(context.Background(), mbimSTKCleanupTimeout)
	defer cancel()
	_, _ = r.reader.SetSTKPAC(ctx, make([]byte, mbimSTKPACHostControlLength))
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

func mbimContextIPTypeForPDNType(pdnType string) mbim.ContextIPType {
	switch pdnType {
	case "ipv4":
		return mbim.ContextIPTypeIPv4
	case "ipv6":
		return mbim.ContextIPTypeIPv6
	default:
		return mbim.ContextIPTypeIPv4v6
	}
}
