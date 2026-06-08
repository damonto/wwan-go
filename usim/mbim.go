package usim

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/uicc-go/mbim"
	usimcard "github.com/damonto/uicc-go/usim/card"
	"github.com/damonto/uicc-go/usim/command"
	"github.com/damonto/uicc-go/usim/simfile"
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
		return nil, err
	}
	payload, err := (akaResponse{AuthAKAResponse: resp}).MarshalBinary()
	if err != nil {
		return nil, err
	}
	return payload, nil
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

func (r *MBIM) Close() error {
	return r.reader.Close()
}

type akaResponse struct {
	*mbim.AuthAKAResponse
}

func (r akaResponse) MarshalBinary() ([]byte, error) {
	if len(r.RES) != 0 {
		if err := checkAKAField("RES", r.RES); err != nil {
			return nil, err
		}
		if err := checkAKAField("CK", r.CK); err != nil {
			return nil, err
		}
		if err := checkAKAField("IK", r.IK); err != nil {
			return nil, err
		}

		out := make([]byte, 0, 3+len(r.RES)+len(r.CK)+len(r.IK))
		out = append(out, 0xDB, byte(len(r.RES)))
		out = append(out, r.RES...)
		out = append(out, byte(len(r.CK)))
		out = append(out, r.CK...)
		out = append(out, byte(len(r.IK)))
		out = append(out, r.IK...)
		return out, nil
	}
	if slices.ContainsFunc(r.AUTS, func(b byte) bool { return b != 0 }) {
		if err := checkAKAField("AUTS", r.AUTS); err != nil {
			return nil, err
		}
		out := make([]byte, 0, 2+len(r.AUTS))
		out = append(out, 0xDC, byte(len(r.AUTS)))
		out = append(out, r.AUTS...)
		return out, nil
	}
	return []byte{0xDC, 0x00}, nil
}

func checkAKAField(name string, value []byte) error {
	if len(value) > 0xff {
		return fmt.Errorf("marshaling AKA response: %s exceeds 255 bytes", name)
	}
	return nil
}
