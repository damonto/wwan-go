package stk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const defaultBIPBufferSize uint16 = 4096

type BIP struct {
	BufferSize        uint16
	DialContext       func(context.Context, string, string) (net.Conn, error)
	SendEnvelope      func(context.Context, Envelope) error
	SendDataAvailable func(context.Context, ChannelStatus, []byte, byte, uint16) error
	SendChannelStatus func(context.Context, ChannelStatus) error

	mu       sync.Mutex
	channels map[byte]*bipChannel
	events   map[Event]bool
}

func (b *BIP) OpenChannel(ctx context.Context, cmd OpenChannelCommand) (TerminalResponse, error) {
	bufferSize := b.channelBufferSize(cmd.BufferSize)
	response := func(cause BIPCause) TerminalResponse {
		tr := BIPError(cause)
		tr.BufferSize = &bufferSize
		if cmd.BearerDescription != nil {
			tr.BearerDescription = cmd.BearerDescription
		}
		return tr
	}

	id, ok := b.allocateChannel()
	if !ok {
		return response(BIPCauseNoChannelAvailable), nil
	}

	network, address, local, cause := cmd.dialTarget()
	if cause != BIPCauseNoSpecificCause {
		b.releaseChannel(id)
		return response(cause), nil
	}

	conn, err := b.dial(ctx, network, address, local)
	if err != nil {
		b.releaseChannel(id)
		return response(BIPCauseRemoteDeviceUnreachable), nil
	}

	channel := newBIPChannel(id, conn, bufferSize)
	b.setChannel(id, channel)
	go b.closeOnDone(ctx, channel)
	go b.readLoop(ctx, channel)

	status := NewChannelStatus(id, true, ChannelStatusNoInfo)
	if cmd.BearerDescription != nil {
		return OpenChannelOK(status, bufferSize, *cmd.BearerDescription), nil
	}
	return OpenChannelOK(status, bufferSize), nil
}

func (b *BIP) CloseChannel(_ context.Context, cmd CloseChannelCommand) (TerminalResponse, error) {
	channel, ok := b.channel(cmd.ChannelID)
	if !ok {
		return BIPError(BIPCauseInvalidChannelID), nil
	}
	if err := channel.close(true, ChannelStatusNoInfo); err != nil {
		return BIPError(BIPCauseChannelClosed), nil
	}
	b.releaseChannel(cmd.ChannelID)
	return OK(), nil
}

func (b *BIP) SendData(ctx context.Context, cmd SendDataCommand) (TerminalResponse, error) {
	channel, ok := b.channel(cmd.ChannelID)
	if !ok {
		return BIPError(BIPCauseInvalidChannelID), nil
	}
	if channel.closed() {
		return BIPError(BIPCauseChannelClosed), nil
	}
	available, err := channel.write(ctx, cmd.Data, cmd.SendImmediately)
	if errors.Is(err, errBIPTxBufferFull) {
		return BIPError(BIPCauseNoSpecificCause), nil
	}
	if err != nil {
		return BIPError(BIPCauseChannelClosed), nil
	}
	return SendDataOK(available), nil
}

func (b *BIP) ReceiveData(_ context.Context, cmd ReceiveDataCommand) (TerminalResponse, error) {
	channel, ok := b.channel(cmd.ChannelID)
	if !ok {
		return BIPError(BIPCauseInvalidChannelID), nil
	}
	if channel.closed() {
		return BIPError(BIPCauseChannelClosed), nil
	}
	data, remaining, missing := channel.read(int(cmd.Length))
	response := ReceiveDataOK(data, remaining)
	if missing {
		response.Result = ResultMissingInformation
	}
	return response, nil
}

func (b *BIP) GetChannelStatus(context.Context, GetChannelStatusCommand) (TerminalResponse, error) {
	b.mu.Lock()
	channels := make([]*bipChannel, 0, len(b.channels))
	for _, channel := range b.channels {
		if channel != nil {
			channels = append(channels, channel)
		}
	}
	b.mu.Unlock()

	statuses := make([]ChannelStatus, 0, len(channels))
	for _, channel := range channels {
		statuses = append(statuses, channel.status())
	}
	return GetChannelStatusOK(statuses...), nil
}

func (b *BIP) SetEvents(events []Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = make(map[Event]bool, len(events))
	for _, event := range events {
		b.events[event] = true
	}
}

func (b *BIP) Close() error {
	b.mu.Lock()
	channels := make([]*bipChannel, 0, len(b.channels))
	for _, channel := range b.channels {
		if channel != nil {
			channels = append(channels, channel)
		}
	}
	b.channels = nil
	b.mu.Unlock()

	var err error
	for _, channel := range channels {
		err = errors.Join(err, channel.close(true, ChannelStatusNoInfo))
	}
	return err
}

func (b *BIP) allocateChannel() (byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.channels == nil {
		b.channels = make(map[byte]*bipChannel)
	}
	for id := byte(1); id <= 7; id++ {
		if _, ok := b.channels[id]; !ok {
			b.channels[id] = nil
			return id, true
		}
	}
	return 0, false
}

func (b *BIP) releaseChannel(id byte) {
	b.mu.Lock()
	delete(b.channels, id)
	b.mu.Unlock()
}

func (b *BIP) setChannel(id byte, channel *bipChannel) {
	b.mu.Lock()
	if b.channels == nil {
		b.channels = make(map[byte]*bipChannel)
	}
	b.channels[id] = channel
	b.mu.Unlock()
}

func (b *BIP) channel(id byte) (*bipChannel, bool) {
	b.mu.Lock()
	channel, ok := b.channels[id]
	b.mu.Unlock()
	return channel, ok && channel != nil
}

func (b *BIP) channelBufferSize(requested uint16) uint16 {
	size := b.BufferSize
	if size == 0 {
		size = requested
	}
	if size == 0 {
		size = defaultBIPBufferSize
	}
	if requested != 0 && size > requested {
		return requested
	}
	return size
}

func (b *BIP) dial(ctx context.Context, network, address string, local *OtherAddress) (net.Conn, error) {
	if b.DialContext != nil {
		return b.DialContext(ctx, network, address)
	}

	dialer := net.Dialer{}
	if local != nil {
		addr, err := local.netAddr(network)
		if err != nil {
			return nil, err
		}
		dialer.LocalAddr = addr
	}
	return dialer.DialContext(ctx, network, address)
}

func (b *BIP) closeOnDone(ctx context.Context, channel *bipChannel) {
	select {
	case <-ctx.Done():
		if err := channel.close(true, ChannelStatusNoInfo); err != nil {
			return
		}
	case <-channel.done:
	}
}

func (b *BIP) readLoop(ctx context.Context, channel *bipChannel) {
	buf := make([]byte, min(int(channel.bufferSize), 4096))
	for {
		n, err := channel.conn.Read(buf)
		if n > 0 {
			notify, data, available, remaining := channel.append(buf[:n])
			if notify && b.eventEnabled(EventDataAvailable) {
				if sendErr := b.dataAvailable(ctx, channel.status(), data, available, remaining); sendErr != nil {
					return
				}
			}
		}
		if err != nil {
			if channel.closedByTerminal() {
				return
			}
			if closeErr := channel.close(false, ChannelStatusLinkDropped); closeErr != nil {
				return
			}
			if b.eventEnabled(EventChannelStatus) {
				if sendErr := b.channelStatus(ctx, channel.status()); sendErr != nil {
					return
				}
			}
			return
		}
	}
}

func (b *BIP) eventEnabled(event Event) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.events[event]
}

func (b *BIP) dataAvailable(ctx context.Context, status ChannelStatus, data []byte, available byte, remaining uint16) error {
	if b.SendDataAvailable != nil {
		return b.SendDataAvailable(ctx, status, data, available, remaining)
	}
	if b.SendEnvelope != nil {
		return b.SendEnvelope(ctx, DataAvailable(status, available))
	}
	return nil
}

func (b *BIP) channelStatus(ctx context.Context, status ChannelStatus) error {
	if b.SendChannelStatus != nil {
		return b.SendChannelStatus(ctx, status)
	}
	if b.SendEnvelope != nil {
		return b.SendEnvelope(ctx, ChannelStatusEvent(status, nil, nil))
	}
	return nil
}

type bipChannel struct {
	id         byte
	conn       net.Conn
	bufferSize uint16

	mu       sync.Mutex
	rx       []byte
	tx       []byte
	statusV  ChannelStatus
	done     chan struct{}
	once     sync.Once
	closedV  bool
	terminal bool
}

func newBIPChannel(id byte, conn net.Conn, bufferSize uint16) *bipChannel {
	return &bipChannel{
		id:         id,
		conn:       conn,
		bufferSize: bufferSize,
		statusV:    NewChannelStatus(id, true, ChannelStatusNoInfo),
		done:       make(chan struct{}),
	}
}

func (c *bipChannel) append(data []byte) (bool, []byte, byte, uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closedV {
		return false, nil, availableByte(len(c.rx)), 0
	}
	wasEmpty := len(c.rx) == 0
	free := int(c.bufferSize) - len(c.rx)
	if free <= 0 {
		return false, nil, availableByte(len(c.rx)), 0
	}
	if len(data) > free {
		data = data[:free]
	}
	c.rx = append(c.rx, data...)
	remaining := len(c.rx) - len(data)
	if remaining > int(^uint16(0)) {
		remaining = int(^uint16(0))
	}
	return wasEmpty && len(data) > 0, append([]byte{}, data...), availableByte(len(c.rx)), uint16(remaining)
}

func (c *bipChannel) read(length int) ([]byte, byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	missing := length > len(c.rx)
	if length > len(c.rx) {
		length = len(c.rx)
	}
	data := make([]byte, length)
	copy(data, c.rx[:length])
	c.rx = append([]byte{}, c.rx[length:]...)
	return data, availableByte(len(c.rx)), missing
}

func (c *bipChannel) write(ctx context.Context, data []byte, immediate bool) (byte, error) {
	c.mu.Lock()
	if c.closedV {
		c.mu.Unlock()
		return 0, net.ErrClosed
	}
	free := int(c.bufferSize) - len(c.tx)
	if len(data) > free {
		available := availableByte(free)
		c.mu.Unlock()
		return available, errBIPTxBufferFull
	}
	c.tx = append(c.tx, data...)
	if !immediate {
		available := availableByte(int(c.bufferSize) - len(c.tx))
		c.mu.Unlock()
		return available, nil
	}
	payload := append([]byte{}, c.tx...)
	c.mu.Unlock()

	if err := writeAll(ctx, c.conn, payload); err != nil {
		return 0, err
	}

	c.mu.Lock()
	if len(c.tx) >= len(payload) {
		c.tx = append([]byte{}, c.tx[len(payload):]...)
	} else {
		c.tx = c.tx[:0]
	}
	available := availableByte(int(c.bufferSize) - len(c.tx))
	c.mu.Unlock()
	return available, nil
}

func (c *bipChannel) close(terminal bool, info byte) error {
	var err error
	c.once.Do(func() {
		c.mu.Lock()
		c.closedV = true
		c.terminal = terminal
		c.statusV = NewChannelStatus(c.id, false, info)
		c.mu.Unlock()
		err = c.conn.Close()
		close(c.done)
	})
	return err
}

func (c *bipChannel) status() ChannelStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusV
}

func (c *bipChannel) closed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closedV
}

func (c *bipChannel) closedByTerminal() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closedV && c.terminal
}

func writeAll(ctx context.Context, conn net.Conn, data []byte) error {
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("setting BIP write deadline: %w", err)
		}
		defer func() {
			if err := conn.SetWriteDeadline(time.Time{}); err != nil {
				return
			}
		}()
	}

	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func (cmd OpenChannelCommand) dialTarget() (string, string, *OtherAddress, BIPCause) {
	if cmd.TransportLevel == nil {
		return "", "", nil, BIPCauseBadLaunchParameters
	}

	network := ""
	switch cmd.TransportLevel.Protocol {
	case TransportTCPClientRemote, TransportTCPClientLocal:
		network = "tcp"
	case TransportUDPClientRemote, TransportUDPClientLocal:
		network = "udp"
	default:
		return "", "", nil, BIPCauseTransportUnavailable
	}

	address := cmd.remoteAddress()
	if address == nil {
		return "", "", nil, BIPCauseBadLaunchParameters
	}
	ip, err := address.ip()
	if err != nil {
		return "", "", nil, BIPCauseBadLaunchParameters
	}

	local := cmd.localAddress()
	return network, net.JoinHostPort(ip.String(), strconv.Itoa(int(cmd.TransportLevel.Port))), local, BIPCauseNoSpecificCause
}

func (cmd OpenChannelCommand) remoteAddress() *OtherAddress {
	if cmd.DestinationAddress != nil {
		if len(cmd.DestinationAddress.Address) > 0 {
			return cmd.DestinationAddress
		}
		return nil
	}
	for i := len(cmd.OtherAddresses) - 1; i >= 0; i-- {
		if len(cmd.OtherAddresses[i].Address) > 0 {
			return &cmd.OtherAddresses[i]
		}
	}
	return nil
}

func (cmd OpenChannelCommand) localAddress() *OtherAddress {
	if cmd.LocalAddress != nil {
		if len(cmd.LocalAddress.Address) == 0 {
			return nil
		}
		return cmd.LocalAddress
	}
	if len(cmd.OtherAddresses) < 2 {
		return nil
	}
	if len(cmd.OtherAddresses[0].Address) == 0 {
		return nil
	}
	return &cmd.OtherAddresses[0]
}

func (a OtherAddress) ip() (net.IP, error) {
	switch a.Type {
	case AddressTypeIPv4:
		if len(a.Address) != net.IPv4len {
			return nil, fmt.Errorf("parsing BIP IPv4 address: length %d, want %d", len(a.Address), net.IPv4len)
		}
	case AddressTypeIPv6:
		if len(a.Address) != net.IPv6len {
			return nil, fmt.Errorf("parsing BIP IPv6 address: length %d, want %d", len(a.Address), net.IPv6len)
		}
	default:
		return nil, fmt.Errorf("parsing BIP address: type 0x%02X is unsupported", a.Type)
	}
	return append(net.IP(nil), a.Address...), nil
}

func (a OtherAddress) netAddr(network string) (net.Addr, error) {
	ip, err := a.ip()
	if err != nil {
		return nil, err
	}
	switch network {
	case "tcp":
		return &net.TCPAddr{IP: ip}, nil
	case "udp":
		return &net.UDPAddr{IP: ip}, nil
	default:
		return nil, fmt.Errorf("building BIP local address: network %q is unsupported", network)
	}
}

func availableByte(n int) byte {
	if n < 0 {
		return 0
	}
	if n > 0xFF {
		return 0xFF
	}
	return byte(n)
}

var errBIPTxBufferFull = errors.New("BIP Tx buffer full")
