package mbim

import (
	"context"
	"encoding/binary"
	"fmt"
)

type NITZInfo struct {
	MBIMExVersion                   uint16
	Year                            uint32
	Month                           uint32
	Day                             uint32
	Hour                            uint32
	Minute                          uint32
	Second                          uint32
	TimeZoneOffsetMinutes           uint32
	DaylightSavingTimeOffsetMinutes uint32
	DataClass                       DataClass
}

func (i *NITZInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 36 {
		return fmt.Errorf("parsing MBIM NITZ information: payload length %d, want 36", len(data))
	}
	version := nitzVersion(i.MBIMExVersion)
	dataClass := DataClass(binary.LittleEndian.Uint32(data[32:36]))
	if !validDataClass(version, dataClass) {
		return fmt.Errorf("parsing MBIM NITZ information: data class %#x contains bits reserved in MBIMEx %#x", dataClass, version)
	}
	*i = NITZInfo{
		MBIMExVersion:                   version,
		Year:                            binary.LittleEndian.Uint32(data[:4]),
		Month:                           binary.LittleEndian.Uint32(data[4:8]),
		Day:                             binary.LittleEndian.Uint32(data[8:12]),
		Hour:                            binary.LittleEndian.Uint32(data[12:16]),
		Minute:                          binary.LittleEndian.Uint32(data[16:20]),
		Second:                          binary.LittleEndian.Uint32(data[20:24]),
		TimeZoneOffsetMinutes:           binary.LittleEndian.Uint32(data[24:28]),
		DaylightSavingTimeOffsetMinutes: binary.LittleEndian.Uint32(data[28:32]),
		DataClass:                       dataClass,
	}
	return nil
}

type NITZRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *NITZInfo
}

func (r *NITZRequest) Request() *Request {
	r.Response = &NITZInfo{MBIMExVersion: nitzVersion(r.MBIMExVersion)}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceMsVoiceExtensions, CIDMsVoiceExtensionsNITZ, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

func (c *Client) NITZ(ctx context.Context) (NITZInfo, error) {
	request := NITZRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return NITZInfo{}, fmt.Errorf("reading MBIM NITZ information: %w", err)
	}
	return *request.Response, nil
}

// ReadNITZ waits for the next Network Identity and Time Zone notification.
func (c *Client) ReadNITZ(ctx context.Context) (NITZInfo, error) {
	indication, err := c.NextIndication(ctx, ServiceMsVoiceExtensions, CIDMsVoiceExtensionsNITZ)
	if err != nil {
		return NITZInfo{}, fmt.Errorf("reading MBIM NITZ notification: %w", err)
	}
	info := NITZInfo{MBIMExVersion: nitzVersion(c.mbimExVersion)}
	if err := info.UnmarshalBinary(indication.InformationBuffer); err != nil {
		return NITZInfo{}, fmt.Errorf("reading MBIM NITZ notification: %w", err)
	}
	return info, nil
}

// WatchNITZ streams Network Identity and Time Zone notifications until ctx is done.
func (c *Client) WatchNITZ(ctx context.Context) (<-chan NITZInfo, error) {
	results, err := c.WatchNITZResults(ctx)
	if err != nil {
		return nil, err
	}
	return watchValues(ctx, results), nil
}

// WatchNITZResults streams Network Identity and Time Zone notifications and
// reports receiver or payload errors through the terminal result.
func (c *Client) WatchNITZResults(ctx context.Context) (<-chan WatchResult[NITZInfo], error) {
	indications, err := c.WatchIndicationResults(ctx, ServiceMsVoiceExtensions, CIDMsVoiceExtensionsNITZ)
	if err != nil {
		return nil, fmt.Errorf("watching MBIM NITZ notifications: %w", err)
	}
	return watchDecoded(ctx, indications, "watching MBIM NITZ notifications", func(data []byte) (NITZInfo, error) {
		info := NITZInfo{MBIMExVersion: nitzVersion(c.mbimExVersion)}
		if err := info.UnmarshalBinary(data); err != nil {
			return NITZInfo{}, err
		}
		return info, nil
	}), nil
}

func nitzVersion(version uint16) uint16 {
	if version == 0 {
		return mbimExVersion10
	}
	return version
}
