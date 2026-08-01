package mbim

import (
	"context"
	"fmt"
	"slices"
	"unicode/utf16"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/sms"
)

const (
	ussdDCSGSM7 = uint32(0x0f)
	ussdDCSUCS2 = uint32(0x48)
)

func (b *Backend) InitiateUSSD(ctx context.Context, text string) (USSDMessage, error) {
	dcs, payload, err := encodeMBIMUSSD(text)
	if err != nil {
		return USSDMessage{}, err
	}
	info, err := b.client.USSD(ctx, mbimproto.USSDActionInitiate, dcs, payload)
	if err != nil {
		return USSDMessage{}, fmt.Errorf("initiating MBIM USSD: %w", err)
	}
	return mbimUSSDMessage(info)
}

func (b *Backend) RespondUSSD(ctx context.Context, text string) (USSDMessage, error) {
	dcs, payload, err := encodeMBIMUSSD(text)
	if err != nil {
		return USSDMessage{}, err
	}
	info, err := b.client.USSD(ctx, mbimproto.USSDActionContinue, dcs, payload)
	if err != nil {
		return USSDMessage{}, fmt.Errorf("responding to MBIM USSD: %w", err)
	}
	return mbimUSSDMessage(info)
}

func (b *Backend) CancelUSSD(ctx context.Context) error {
	if _, err := b.client.USSD(ctx, mbimproto.USSDActionCancel, 0, nil); err != nil {
		return fmt.Errorf("canceling MBIM USSD: %w", err)
	}
	return nil
}

func (b *Backend) WatchUSSD(ctx context.Context) (<-chan Result[USSDMessage], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	updates, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceUSSD, mbimproto.CIDUSSD)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching MBIM USSD: %w", err)
	}
	if err := b.ensureWatchNotifications(ctx, mbimproto.DeviceServiceSubscribeEntry{
		ServiceID: mbimproto.ServiceUSSD,
		CIDs:      []uint32{mbimproto.CIDUSSD},
	}); err != nil {
		cancel()
		return nil, fmt.Errorf("watching MBIM USSD: %w", err)
	}
	out := make(chan Result[USSDMessage], 8)
	go func() {
		defer close(out)
		defer cancel()
		sendError := func(err error) {
			// The watcher terminates immediately after this best-effort error report.
			_ = sendStreamResult(watchCtx, out, Result[USSDMessage]{Err: err})
		}
		for update := range updates {
			if update.Err != nil {
				sendError(update.Err)
				return
			}
			var info mbimproto.USSDInfo
			if err := info.UnmarshalBinary(update.Value.InformationBuffer); err != nil {
				sendError(fmt.Errorf("decoding MBIM USSD event: %w", err))
				return
			}
			message, err := mbimUSSDMessage(info)
			if err != nil {
				sendError(err)
				return
			}
			if !sendStreamResult(watchCtx, out, Result[USSDMessage]{Value: message}) {
				return
			}
		}
	}()
	return out, nil
}

func encodeMBIMUSSD(text string) (uint32, []byte, error) {
	if septets, ok := sms.EncodeGSM7(text); ok {
		// Seven septets occupy seven octets and otherwise leave an ambiguous
		// all-zero eighth septet. TS 23.038 uses CR as padding in this case.
		if len(septets)%8 == 7 {
			septets = append(septets, 0x0d)
		}
		packed, _ := sms.PackSeptets(septets, nil)
		if len(packed) > 160 {
			return 0, nil, fmt.Errorf("encoding MBIM USSD: payload length %d exceeds 160", len(packed))
		}
		return ussdDCSGSM7, packed, nil
	}
	payload := sms.UCS2Bytes(utf16.Encode([]rune(text)))
	if len(payload) > 160 {
		return 0, nil, fmt.Errorf("encoding MBIM USSD: payload length %d exceeds 160", len(payload))
	}
	return ussdDCSUCS2, payload, nil
}

func mbimUSSDMessage(info mbimproto.USSDInfo) (USSDMessage, error) {
	message := USSDMessage{State: mbimUSSDState(info), DCS: info.DataCodingScheme, Data: slices.Clone(info.Payload)}
	switch info.DataCodingScheme {
	case ussdDCSGSM7:
		septets := sms.UnpackSeptets(info.Payload, 0, len(info.Payload)*8/7)
		if len(info.Payload)%7 == 0 && len(septets) > 0 && septets[len(septets)-1] == 0x0d {
			septets = septets[:len(septets)-1]
		}
		text, err := sms.DecodeGSM7(septets)
		if err != nil {
			return USSDMessage{}, fmt.Errorf("decoding MBIM USSD: %w", err)
		}
		message.Text = text
	case ussdDCSUCS2:
		text, err := sms.DecodeUCS2(info.Payload)
		if err != nil {
			return USSDMessage{}, fmt.Errorf("decoding MBIM USSD: %w", err)
		}
		message.Text = text
	default:
		if len(info.Payload) != 0 {
			return USSDMessage{}, fmt.Errorf("decoding MBIM USSD: data coding scheme %#x is not supported", info.DataCodingScheme)
		}
	}
	return message, nil
}

func mbimUSSDState(info mbimproto.USSDInfo) USSDState {
	switch info.Response {
	case mbimproto.USSDResponseActionRequired:
		return USSDStateUserResponse
	case mbimproto.USSDResponseNoActionRequired:
		return USSDStateNetworkResponse
	case mbimproto.USSDResponseTerminatedByNetwork, mbimproto.USSDResponseOtherLocalClient, mbimproto.USSDResponseOperationNotSupported, mbimproto.USSDResponseNetworkTimeout:
		return USSDStateTerminated
	default:
		return USSDStateUnknown
	}
}
