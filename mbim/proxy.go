package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type ProxyConfigRequest struct {
	TransactionID uint32
	DevicePath    string
	Timeout       uint32
	Response      *ProxyConfigResponse
}

func (r *ProxyConfigRequest) Request() *Request {
	devicePath := utf16Bytes(r.DevicePath)
	data := make([]byte, 0, 12+len(devicePath))
	data = binary.LittleEndian.AppendUint32(data, 12)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(devicePath)))
	data = binary.LittleEndian.AppendUint32(data, r.Timeout)
	data = append(data, devicePath...)

	r.Response = new(ProxyConfigResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMbimProxyControl,
			CIDProxyControlConfiguration,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type ProxyConfigResponse struct{}

func (*ProxyConfigResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("parsing MBIM proxy configuration response: payload length %d, want 0", len(data))
	}
	return nil
}

func (c *Client) configureProxy(ctx context.Context, device string) error {
	request := ProxyConfigRequest{
		TransactionID: c.nextTransactionID(),
		DevicePath:    device,
		Timeout:       30,
	}
	if err := request.Request().Transmit(ctx, c.conn); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("opening MBIM client: device %s is not connected", device)
		}
		return fmt.Errorf("configuring MBIM proxy for %s: %w", device, err)
	}
	return nil
}

// ReadProxyVersion waits for the next MBIM proxy version notification.
func (c *Client) ReadProxyVersion(ctx context.Context) (VersionInfo, error) {
	indication, err := c.NextIndication(ctx, ServiceMbimProxyControl, CIDProxyControlVersion)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("reading MBIM proxy version notification: %w", err)
	}
	var version VersionInfo
	if err := version.UnmarshalBinary(indication.InformationBuffer); err != nil {
		return VersionInfo{}, fmt.Errorf("reading MBIM proxy version notification: %w", err)
	}
	return version, nil
}

// WatchProxyVersion streams MBIM proxy version notifications until ctx is done.
func (c *Client) WatchProxyVersion(ctx context.Context) (<-chan VersionInfo, error) {
	results, err := c.WatchProxyVersionResults(ctx)
	if err != nil {
		return nil, err
	}
	return watchValues(ctx, results), nil
}

// WatchProxyVersionResults streams MBIM proxy version notifications and
// reports receiver or payload errors through the terminal result.
func (c *Client) WatchProxyVersionResults(ctx context.Context) (<-chan WatchResult[VersionInfo], error) {
	indications, err := c.WatchIndicationResults(ctx, ServiceMbimProxyControl, CIDProxyControlVersion)
	if err != nil {
		return nil, fmt.Errorf("watching MBIM proxy version notifications: %w", err)
	}
	return watchDecoded(ctx, indications, "watching MBIM proxy version notifications", func(data []byte) (VersionInfo, error) {
		var version VersionInfo
		if err := version.UnmarshalBinary(data); err != nil {
			return VersionInfo{}, err
		}
		return version, nil
	}), nil
}
