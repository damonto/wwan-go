package qmi

import (
	"context"
	"fmt"
	"slices"
	"unicode/utf16"

	"github.com/damonto/wwan-go/modem/sms"
	"github.com/damonto/wwan-go/qcom"
)

func (b *Backend) InitiateUSSD(ctx context.Context, text string) (USSDMessage, error) {
	data, err := encodeQMIUSSD(text)
	if err != nil {
		return USSDMessage{}, err
	}
	result, err := b.client.VoiceOriginateUSSD(ctx, data)
	if err != nil {
		return USSDMessage{}, fmt.Errorf("initiating QMI USSD: %w", err)
	}
	message, err := qmiUSSDResult(result)
	if err != nil {
		return USSDMessage{}, err
	}
	message.State = USSDStateNetworkResponse
	return message, nil
}

func (b *Backend) RespondUSSD(ctx context.Context, text string) (USSDMessage, error) {
	data, err := encodeQMIUSSD(text)
	if err != nil {
		return USSDMessage{}, err
	}
	if err := b.client.VoiceAnswerUSSD(ctx, data); err != nil {
		return USSDMessage{}, fmt.Errorf("responding to QMI USSD: %w", err)
	}
	message, err := qmiUSSDData(data)
	if err != nil {
		return USSDMessage{}, err
	}
	message.State = USSDStateActive
	return message, nil
}

func (b *Backend) CancelUSSD(ctx context.Context) error {
	if err := b.client.VoiceCancelUSSD(ctx); err != nil {
		return fmt.Errorf("canceling QMI USSD: %w", err)
	}
	return nil
}

func (b *Backend) WatchUSSD(ctx context.Context) (<-chan Result[USSDMessage], error) {
	updates, err := b.client.VoiceWatchUSSD(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching QMI USSD: %w", err)
	}
	out := make(chan Result[USSDMessage], 8)
	go func() {
		defer close(out)
		for update := range updates {
			if update.Released {
				if !sendStreamResult(ctx, out, Result[USSDMessage]{Value: USSDMessage{State: USSDStateTerminated}}) {
					return
				}
				continue
			}
			message, err := qmiUSSDEvent(update)
			if err != nil {
				sendStreamResult(ctx, out, Result[USSDMessage]{Err: err})
				return
			}
			if !sendStreamResult(ctx, out, Result[USSDMessage]{Value: message}) {
				return
			}
		}
	}()
	return out, nil
}

func encodeQMIUSSD(text string) (qcom.VoiceUSSDData, error) {
	ascii := true
	for _, r := range text {
		if r > 0x7f {
			ascii = false
			break
		}
	}
	if ascii {
		return qcom.VoiceUSSDData{Encoding: qcom.VoiceUSSDEncodingASCII, Data: []byte(text)}, nil
	}
	data := sms.UCS2Bytes(utf16.Encode([]rune(text)))
	if len(data) > 255 {
		return qcom.VoiceUSSDData{}, fmt.Errorf("encoding QMI USSD: payload length %d exceeds 255", len(data))
	}
	return qcom.VoiceUSSDData{Encoding: qcom.VoiceUSSDEncodingUCS2, Data: data}, nil
}

func qmiUSSDResult(result qcom.VoiceUSSDResult) (USSDMessage, error) {
	if len(result.UTF16) != 0 {
		data := sms.UCS2Bytes(result.UTF16)
		return USSDMessage{Text: string(utf16.Decode(result.UTF16)), DCS: uint32(qcom.VoiceUSSDEncodingUCS2), Data: data}, nil
	}
	if result.DataKnown {
		return qmiUSSDData(result.Data)
	}
	return USSDMessage{}, nil
}

func qmiUSSDEvent(event qcom.VoiceUSSDEvent) (USSDMessage, error) {
	message := USSDMessage{State: USSDStateNetworkResponse}
	if event.Action == qcom.VoiceUSSDActionRequired {
		message.State = USSDStateUserResponse
	}
	if len(event.UTF16) != 0 {
		message.Text = string(utf16.Decode(event.UTF16))
		message.Data = sms.UCS2Bytes(event.UTF16)
		message.DCS = uint32(qcom.VoiceUSSDEncodingUCS2)
		return message, nil
	}
	if event.DataKnown {
		decoded, err := qmiUSSDData(event.Data)
		if err != nil {
			return USSDMessage{}, err
		}
		decoded.State = message.State
		return decoded, nil
	}
	return message, nil
}

func qmiUSSDData(data qcom.VoiceUSSDData) (USSDMessage, error) {
	message := USSDMessage{DCS: uint32(data.Encoding), Data: slices.Clone(data.Data)}
	switch data.Encoding {
	case qcom.VoiceUSSDEncodingASCII, qcom.VoiceUSSDEncoding8Bit:
		message.Text = string(data.Data)
	case qcom.VoiceUSSDEncodingUCS2:
		text, err := sms.DecodeUCS2(data.Data)
		if err != nil {
			return USSDMessage{}, fmt.Errorf("decoding QMI USSD: %w", err)
		}
		message.Text = text
	default:
		return USSDMessage{}, fmt.Errorf("decoding QMI USSD: encoding %d is not supported", data.Encoding)
	}
	return message, nil
}
