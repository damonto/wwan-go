package stk

import (
	"fmt"
	"slices"

	"github.com/damonto/uicc-go/usim/tlv"
)

type TerminalResponse struct {
	Result            ResultCode
	AdditionalInfo    []byte
	Text              *Text
	ItemIdentifier    *byte
	Duration          *Duration
	Language          string
	ChannelStatus     []byte
	ChannelStatuses   []ChannelStatus
	ChannelData       []byte
	ChannelDataLen    *byte
	BearerDescription *BearerDescription
	BufferSize        *uint16
	OtherAddresses    []OtherAddress
	RAPDU             []byte
	ATResponse        []byte
	ExtraTLVs         tlv.Items
}

func OK() TerminalResponse {
	return TerminalResponse{Result: ResultCommandPerformed}
}

func Result(result ResultCode, additionalInfo ...byte) TerminalResponse {
	return TerminalResponse{Result: result, AdditionalInfo: slices.Clone(additionalInfo)}
}

func (r TerminalResponse) MarshalFor(cmd Command) ([]byte, error) {
	details := cmd.CommandDetails()
	items := tlv.Items{
		tlv.NewComprehension(tlvCommandDetails, []byte{details.Number, byte(details.Type), details.Qualifier}),
		tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceTerminal), byte(DeviceUICC)}),
		tlv.NewComprehension(tlvResult, append([]byte{byte(r.Result)}, r.AdditionalInfo...)),
	}

	if r.Text != nil {
		value, err := r.Text.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("building terminal response text: %w", err)
		}
		items = append(items, tlv.NewComprehension(tlvTextString, value))
	}
	if r.ItemIdentifier != nil {
		items = append(items, tlv.NewComprehension(tlvItemID, []byte{*r.ItemIdentifier}))
	}
	if r.Duration != nil {
		items = append(items, tlv.NewComprehension(tlvDuration, []byte{r.Duration.Unit, r.Duration.Interval}))
	}
	if r.Language != "" {
		items = append(items, tlv.NewComprehension(tlvLanguage, []byte(r.Language)))
	}
	if len(r.ChannelStatus) > 0 {
		items = append(items, tlv.NewComprehension(tlvChannelStatus, r.ChannelStatus))
	}
	for _, status := range r.ChannelStatuses {
		value, err := status.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("building terminal response channel status: %w", err)
		}
		items = append(items, tlv.NewComprehension(tlvChannelStatus, value))
	}
	if r.ChannelData != nil {
		items = append(items, tlv.NewComprehension(tlvChannelData, r.ChannelData))
	}
	if r.ChannelDataLen != nil {
		items = append(items, tlv.NewComprehension(tlvChannelDataLen, []byte{*r.ChannelDataLen}))
	}
	if r.BearerDescription != nil {
		value, err := r.BearerDescription.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("building terminal response bearer description: %w", err)
		}
		items = append(items, tlv.NewComprehension(tlvBearerDesc, value))
	}
	if r.BufferSize != nil {
		items = append(items, tlv.NewComprehension(tlvBufferSize, []byte{byte(*r.BufferSize >> 8), byte(*r.BufferSize)}))
	}
	for _, address := range r.OtherAddresses {
		value, err := address.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("building terminal response other address: %w", err)
		}
		items = append(items, tlv.NewComprehension(tlvOtherAddress, value))
	}
	if len(r.RAPDU) > 0 {
		items = append(items, tlv.NewComprehension(tlvRAPDU, r.RAPDU))
	}
	if len(r.ATResponse) > 0 {
		items = append(items, tlv.NewComprehension(tlvATResponse, r.ATResponse))
	}
	items = append(items, tlv.CloneItems(r.ExtraTLVs)...)

	data, err := items.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("building terminal response: %w", err)
	}
	return data, nil
}
