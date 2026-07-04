package stk

import (
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/damonto/uicc-go/usim/simfile"
	"github.com/damonto/uicc-go/usim/tlv"
)

type Envelope struct {
	Tag  byte
	TLVs tlv.Items
	Raw  []byte
}

func RawEnvelope(data []byte) Envelope {
	return Envelope{Raw: slices.Clone(data)}
}

func NewEnvelope(tag byte, tlvs ...tlv.Item) Envelope {
	return Envelope{Tag: tag, TLVs: tlv.CloneItems(tlvs)}
}

func (e Envelope) MarshalBinary() ([]byte, error) {
	if len(e.Raw) > 0 {
		return slices.Clone(e.Raw), nil
	}
	data, err := e.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("building envelope: %w", err)
	}
	return tlv.WrapBER(e.Tag, data)
}

func (e Envelope) WriteTo(w io.Writer) (int64, error) {
	data, err := e.MarshalBinary()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return int64(n), err
}

func MenuSelection(itemID byte, helpRequested bool) Envelope {
	tlvs := tlv.Items{
		tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceKeypad), byte(DeviceUICC)}),
		tlv.NewComprehension(tlvItemID, []byte{itemID}),
	}
	if helpRequested {
		tlvs = append(tlvs, tlv.NewComprehension(tlvHelpRequest, nil))
	}
	return NewEnvelope(TagMenuSelection, tlvs...)
}

func EventDownload(event Event, source, destination DeviceID, extra ...tlv.Item) Envelope {
	tlvs := tlv.Items{
		tlv.NewComprehension(tlvEventList, []byte{byte(event)}),
		tlv.NewComprehension(tlvDeviceIDs, []byte{byte(source), byte(destination)}),
	}
	tlvs = append(tlvs, tlv.CloneItems(extra)...)
	return NewEnvelope(TagEventDownload, tlvs...)
}

func DataAvailable(status ChannelStatus, available byte) Envelope {
	return EventDownload(EventDataAvailable, DeviceTerminal, DeviceUICC,
		tlv.NewComprehension(tlvChannelStatus, status.bytes()),
		tlv.NewComprehension(tlvChannelDataLen, []byte{available}),
	)
}

func ChannelStatusEvent(status ChannelStatus, bearer *BearerDescription, address *OtherAddress) Envelope {
	extra := tlv.Items{tlv.NewComprehension(tlvChannelStatus, status.bytes())}
	if bearer != nil {
		extra = append(extra, tlv.NewComprehension(tlvBearerDesc, bearer.bytes()))
	}
	if address != nil {
		extra = append(extra, tlv.NewComprehension(tlvOtherAddress, address.bytes()))
	}
	return EventDownload(EventChannelStatus, DeviceTerminal, DeviceUICC, extra...)
}

type SMSPPDownload struct {
	ServiceCenterAddress string
	TPDU                 []byte
}

func (c SMSPPDownload) Envelope() (Envelope, error) {
	if len(c.TPDU) == 0 {
		return Envelope{}, errors.New("building SMS-PP download envelope: TPDU is empty")
	}

	tlvs := tlv.Items{
		tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceNetwork), byte(DeviceUICC)}),
	}
	address, err := simfile.Address(c.ServiceCenterAddress).MarshalBinary()
	if err != nil {
		return Envelope{}, fmt.Errorf("building SMS-PP download envelope: %w", err)
	}
	if len(address) != 0 {
		tlvs = append(tlvs, tlv.NewComprehension(tlvAddress, address))
	}
	tlvs = append(tlvs, tlv.NewComprehension(tlvSMSTPDU, c.TPDU))
	return NewEnvelope(TagSMSPPDownload, tlvs...), nil
}

func (c SMSPPDownload) MarshalBinary() ([]byte, error) {
	envelope, err := c.Envelope()
	if err != nil {
		return nil, err
	}
	return envelope.MarshalBinary()
}

func EnvelopeType(data []byte) uint16 {
	if len(data) == 0 {
		return 0x05
	}
	switch data[0] {
	case TagMenuSelection:
		return 0x01
	case TagSMSPPDownload:
		return 0x09
	case TagEventDownload:
		_, body, err := tlv.UnwrapBER(data)
		if err != nil {
			return 0x05
		}
		var tlvs tlv.Items
		if err := tlvs.UnmarshalBinary(body); err != nil {
			return 0x05
		}
		item, ok := tlvs.Find(tlvEventList)
		if !ok || len(item.Value) == 0 {
			return 0x05
		}
		switch Event(item.Value[0]) {
		case EventUserActivity:
			return 0x02
		case EventIdleScreenAvailable:
			return 0x03
		case EventLanguageSelection:
			return 0x04
		case EventBrowserTermination:
			return 0x06
		case EventHCIConnectivity:
			return 0x08
		case EventMTCall:
			return 0x0A
		case EventCallConnected:
			return 0x0B
		case EventCallDisconnected:
			return 0x0C
		default:
			return 0x05
		}
	default:
		return 0x05
	}
}
