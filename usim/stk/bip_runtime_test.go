package stk

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestBIPTCPClient(t *testing.T) {
	remoteConn := make(chan net.Conn, 1)
	envelopes := make(chan Envelope, 2)
	bip := &BIP{
		BufferSize: 8,
		DialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" {
				t.Fatalf("network = %q, want tcp", network)
			}
			if address != "127.0.0.1:8080" {
				t.Fatalf("address = %q, want 127.0.0.1:8080", address)
			}
			local, remote := net.Pipe()
			remoteConn <- remote
			return local, nil
		},
		SendEnvelope: func(_ context.Context, envelope Envelope) error {
			envelopes <- envelope
			return nil
		},
	}
	bip.SetEvents([]Event{EventDataAvailable, EventChannelStatus})
	t.Cleanup(func() {
		if err := bip.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	openResp, err := bip.OpenChannel(context.Background(), OpenChannelCommand{
		BufferSize: 16,
		OtherAddresses: []OtherAddress{{
			Type:    AddressTypeIPv4,
			Address: []byte{127, 0, 0, 1},
		}},
		TransportLevel: &TransportLevel{
			Protocol: TransportTCPClientRemote,
			Port:     8080,
		},
	})
	if err != nil {
		t.Fatalf("OpenChannel() error = %v", err)
	}
	if openResp.Result != ResultCommandPerformed || openResp.BufferSize == nil || *openResp.BufferSize != 8 {
		t.Fatalf("OpenChannel() = %+v, want OK buffer 8", openResp)
	}
	if len(openResp.ChannelStatuses) != 1 || openResp.ChannelStatuses[0].Identifier != 1 || !openResp.ChannelStatuses[0].LinkEstablished() {
		t.Fatalf("OpenChannel() statuses = %+v, want channel 1 established", openResp.ChannelStatuses)
	}

	remote := <-remoteConn
	t.Cleanup(func() {
		if err := remote.Close(); err != nil {
			return
		}
	})

	read := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(remote, buf); err != nil {
			return
		}
		read <- buf
	}()

	sendResp, err := bip.SendData(context.Background(), SendDataCommand{
		ChannelID: 1,
		Data:      []byte("ping"),
	})
	if err != nil {
		t.Fatalf("SendData() store error = %v", err)
	}
	if sendResp.Result != ResultCommandPerformed || sendResp.ChannelDataLen == nil || *sendResp.ChannelDataLen != 4 {
		t.Fatalf("SendData() store = %+v, want OK available 4", sendResp)
	}
	flushResp, err := bip.SendData(context.Background(), SendDataCommand{
		ChannelID:       1,
		SendImmediately: true,
	})
	if err != nil {
		t.Fatalf("SendData() flush error = %v", err)
	}
	if flushResp.Result != ResultCommandPerformed || flushResp.ChannelDataLen == nil || *flushResp.ChannelDataLen != 8 {
		t.Fatalf("SendData() flush = %+v, want OK available 8", flushResp)
	}
	select {
	case got := <-read:
		if !bytes.Equal(got, []byte("ping")) {
			t.Fatalf("remote read = %q, want ping", got)
		}
	case <-time.After(time.Second):
		t.Fatal("remote read timed out")
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := remote.Write([]byte("pong"))
		writeDone <- err
	}()
	select {
	case envelope := <-envelopes:
		raw, err := envelope.MarshalBinary()
		if err != nil {
			t.Fatalf("DataAvailable.MarshalBinary() error = %v", err)
		}
		if len(raw) == 0 || raw[0] != TagEventDownload {
			t.Fatalf("DataAvailable envelope = % X, want event download", raw)
		}
	case <-time.After(time.Second):
		t.Fatal("data available envelope timed out")
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("remote write error = %v", err)
	}

	receiveResp, err := bip.ReceiveData(context.Background(), ReceiveDataCommand{ChannelID: 1, Length: 4})
	if err != nil {
		t.Fatalf("ReceiveData() error = %v", err)
	}
	if receiveResp.Result != ResultCommandPerformed || !bytes.Equal(receiveResp.ChannelData, []byte("pong")) {
		t.Fatalf("ReceiveData() = %+v, want pong", receiveResp)
	}
	if receiveResp.ChannelDataLen == nil || *receiveResp.ChannelDataLen != 0 {
		t.Fatalf("ReceiveData() remaining = %+v, want 0", receiveResp.ChannelDataLen)
	}

	statusResp, err := bip.GetChannelStatus(context.Background(), GetChannelStatusCommand{})
	if err != nil {
		t.Fatalf("GetChannelStatus() error = %v", err)
	}
	if len(statusResp.ChannelStatuses) != 1 || statusResp.ChannelStatuses[0].Identifier != 1 {
		t.Fatalf("GetChannelStatus() statuses = %+v, want channel 1", statusResp.ChannelStatuses)
	}

	closeResp, err := bip.CloseChannel(context.Background(), CloseChannelCommand{ChannelID: 1})
	if err != nil {
		t.Fatalf("CloseChannel() error = %v", err)
	}
	if closeResp.Result != ResultCommandPerformed {
		t.Fatalf("CloseChannel() = %+v, want OK", closeResp)
	}

	invalidResp, err := bip.SendData(context.Background(), SendDataCommand{ChannelID: 1, Data: []byte("x")})
	if err != nil {
		t.Fatalf("SendData() invalid error = %v", err)
	}
	if invalidResp.Result != ResultBearerIndependentProtocolError || !bytes.Equal(invalidResp.AdditionalInfo, []byte{byte(BIPCauseInvalidChannelID)}) {
		t.Fatalf("SendData() invalid = %+v, want invalid channel BIP error", invalidResp)
	}
}

func TestBIPSendDataTxBuffer(t *testing.T) {
	tests := []struct {
		name      string
		buffer    uint16
		commands  []SendDataCommand
		wantSpace []byte
		wantWrite []byte
		wantCause BIPCause
	}{
		{
			name:   "store then flush",
			buffer: 8,
			commands: []SendDataCommand{
				{ChannelID: 1, Data: []byte("ping")},
				{ChannelID: 1, Data: []byte("pong"), SendImmediately: true},
			},
			wantSpace: []byte{4, 8},
			wantWrite: []byte("pingpong"),
		},
		{
			name:      "overflow",
			buffer:    4,
			commands:  []SendDataCommand{{ChannelID: 1, Data: []byte("12345")}},
			wantCause: BIPCauseNoSpecificCause,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local, remote := net.Pipe()
			t.Cleanup(func() { _ = remote.Close() })
			bip := &BIP{
				channels: map[byte]*bipChannel{
					1: newBIPChannel(1, local, tt.buffer),
				},
			}
			t.Cleanup(func() { _ = bip.Close() })

			wrote := make(chan []byte, 1)
			if len(tt.wantWrite) > 0 {
				go func() {
					buf := make([]byte, len(tt.wantWrite))
					if _, err := io.ReadFull(remote, buf); err != nil {
						return
					}
					wrote <- buf
				}()
			}

			for i, cmd := range tt.commands {
				got, err := bip.SendData(context.Background(), cmd)
				if err != nil {
					t.Fatalf("SendData() error = %v", err)
				}
				if tt.wantCause != BIPCauseNoSpecificCause || len(tt.wantSpace) == 0 {
					if got.Result != ResultBearerIndependentProtocolError || !bytes.Equal(got.AdditionalInfo, []byte{byte(tt.wantCause)}) {
						t.Fatalf("SendData() = %+v, want BIP cause 0x%02X", got, tt.wantCause)
					}
					continue
				}
				if got.Result != ResultCommandPerformed || got.ChannelDataLen == nil || *got.ChannelDataLen != tt.wantSpace[i] {
					t.Fatalf("SendData() = %+v, want available %d", got, tt.wantSpace[i])
				}
			}

			if len(tt.wantWrite) == 0 {
				return
			}
			select {
			case got := <-wrote:
				if !bytes.Equal(got, tt.wantWrite) {
					t.Fatalf("remote read = %q, want %q", got, tt.wantWrite)
				}
			case <-time.After(time.Second):
				t.Fatal("remote read timed out")
			}
		})
	}
}

func TestBIPSendDataKeepsTxBufferOnImmediateWriteError(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "write error",
			data: []byte("ping"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := newBIPChannel(1, failingWriteConn{}, 8)
			bip := &BIP{channels: map[byte]*bipChannel{1: channel}}

			got, err := bip.SendData(context.Background(), SendDataCommand{
				ChannelID:       1,
				Data:            tt.data,
				SendImmediately: true,
			})
			if err != nil {
				t.Fatalf("SendData() error = %v", err)
			}
			if got.Result != ResultBearerIndependentProtocolError || !bytes.Equal(got.AdditionalInfo, []byte{byte(BIPCauseChannelClosed)}) {
				t.Fatalf("SendData() = %+v, want channel closed BIP error", got)
			}
			if !bytes.Equal(channel.tx, tt.data) {
				t.Fatalf("tx buffer = %q, want %q", channel.tx, tt.data)
			}
		})
	}
}

func TestBIPReceiveDataMissingInformation(t *testing.T) {
	tests := []struct {
		name       string
		buffer     []byte
		length     byte
		wantResult ResultCode
		wantData   []byte
		wantRemain byte
	}{
		{
			name:       "enough data",
			buffer:     []byte("pong"),
			length:     2,
			wantResult: ResultCommandPerformed,
			wantData:   []byte("po"),
			wantRemain: 2,
		},
		{
			name:       "missing data",
			buffer:     []byte("go"),
			length:     4,
			wantResult: ResultMissingInformation,
			wantData:   []byte("go"),
			wantRemain: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local, remote := net.Pipe()
			t.Cleanup(func() { _ = remote.Close() })
			channel := newBIPChannel(1, local, 8)
			channel.rx = append(channel.rx, tt.buffer...)
			bip := &BIP{channels: map[byte]*bipChannel{1: channel}}
			t.Cleanup(func() { _ = bip.Close() })

			got, err := bip.ReceiveData(context.Background(), ReceiveDataCommand{ChannelID: 1, Length: tt.length})
			if err != nil {
				t.Fatalf("ReceiveData() error = %v", err)
			}
			if got.Result != tt.wantResult || !bytes.Equal(got.ChannelData, tt.wantData) {
				t.Fatalf("ReceiveData() = %+v, want result 0x%02X data %q", got, tt.wantResult, tt.wantData)
			}
			if got.ChannelDataLen == nil || *got.ChannelDataLen != tt.wantRemain {
				t.Fatalf("ReceiveData() remaining = %+v, want %d", got.ChannelDataLen, tt.wantRemain)
			}
		})
	}
}

type failingWriteConn struct{}

func (failingWriteConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (failingWriteConn) Write([]byte) (int, error)        { return 0, net.ErrClosed }
func (failingWriteConn) Close() error                     { return nil }
func (failingWriteConn) LocalAddr() net.Addr              { return testAddr("local") }
func (failingWriteConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (failingWriteConn) SetDeadline(time.Time) error      { return nil }
func (failingWriteConn) SetReadDeadline(time.Time) error  { return nil }
func (failingWriteConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }

func TestBIPDataAvailableEventList(t *testing.T) {
	tests := []struct {
		name       string
		events     []Event
		wantNotify bool
	}{
		{
			name:       "event enabled",
			events:     []Event{EventDataAvailable},
			wantNotify: true,
		},
		{
			name: "event disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local, remote := net.Pipe()
			t.Cleanup(func() { _ = remote.Close() })
			notified := make(chan []byte, 1)
			bip := &BIP{
				BufferSize: 16,
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					return local, nil
				},
				SendDataAvailable: func(_ context.Context, status ChannelStatus, data []byte, available byte, remaining uint16) error {
					if status.Identifier != 1 || available != 4 || remaining != 0 {
						t.Fatalf("data available status/available/remaining = %+v/%d/%d, want channel 1/4/0", status, available, remaining)
					}
					notified <- data
					return nil
				},
			}
			bip.SetEvents(tt.events)
			t.Cleanup(func() { _ = bip.Close() })

			if _, err := bip.OpenChannel(context.Background(), OpenChannelCommand{
				BufferSize: 16,
				OtherAddresses: []OtherAddress{{
					Type:    AddressTypeIPv4,
					Address: []byte{127, 0, 0, 1},
				}},
				TransportLevel: &TransportLevel{
					Protocol: TransportTCPClientRemote,
					Port:     8080,
				},
			}); err != nil {
				t.Fatalf("OpenChannel() error = %v", err)
			}

			done := make(chan error, 1)
			go func() {
				_, err := remote.Write([]byte("pong"))
				done <- err
			}()
			if err := <-done; err != nil {
				t.Fatalf("remote write error = %v", err)
			}

			select {
			case got := <-notified:
				if !tt.wantNotify {
					t.Fatalf("data available notification = %q, want none", got)
				}
				if !bytes.Equal(got, []byte("pong")) {
					t.Fatalf("data available payload = %q, want pong", got)
				}
			case <-time.After(50 * time.Millisecond):
				if tt.wantNotify {
					t.Fatal("data available notification timed out")
				}
			}
		})
	}
}

func TestBIPOpenChannelErrors(t *testing.T) {
	tests := []struct {
		name string
		bip  func() *BIP
		cmd  OpenChannelCommand
		want BIPCause
	}{
		{
			name: "missing transport",
			cmd: OpenChannelCommand{
				BufferSize: 16,
			},
			want: BIPCauseBadLaunchParameters,
		},
		{
			name: "unsupported protocol",
			cmd: OpenChannelCommand{
				BufferSize:     16,
				TransportLevel: &TransportLevel{Protocol: TransportDirect},
			},
			want: BIPCauseTransportUnavailable,
		},
		{
			name: "dial error",
			bip: func() *BIP {
				return &BIP{
					DialContext: func(context.Context, string, string) (net.Conn, error) {
						return nil, net.ErrClosed
					},
				}
			},
			cmd: OpenChannelCommand{
				BufferSize: 16,
				OtherAddresses: []OtherAddress{{
					Type:    AddressTypeIPv4,
					Address: []byte{127, 0, 0, 1},
				}},
				TransportLevel: &TransportLevel{
					Protocol: TransportTCPClientRemote,
					Port:     8080,
				},
			},
			want: BIPCauseRemoteDeviceUnreachable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bip := &BIP{}
			if tt.bip != nil {
				bip = tt.bip()
			}
			got, err := bip.OpenChannel(context.Background(), tt.cmd)
			if err != nil {
				t.Fatalf("OpenChannel() error = %v", err)
			}
			if got.Result != ResultBearerIndependentProtocolError || !bytes.Equal(got.AdditionalInfo, []byte{byte(tt.want)}) {
				t.Fatalf("OpenChannel() = %+v, want BIP cause 0x%02X", got, tt.want)
			}
		})
	}
}

func TestOpenChannelDialTargetAddresses(t *testing.T) {
	tests := []struct {
		name      string
		cmd       OpenChannelCommand
		wantAddr  string
		wantLocal bool
		wantCause BIPCause
	}{
		{
			name: "dynamic local and destination",
			cmd: OpenChannelCommand{
				LocalAddress:       &OtherAddress{},
				DestinationAddress: &OtherAddress{Type: AddressTypeIPv4, Address: []byte{192, 0, 2, 1}},
				TransportLevel:     &TransportLevel{Protocol: TransportTCPClientRemote, Port: 80},
			},
			wantAddr:  "192.0.2.1:80",
			wantCause: BIPCauseNoSpecificCause,
		},
		{
			name: "empty destination does not fall back to local",
			cmd: OpenChannelCommand{
				LocalAddress:       &OtherAddress{Type: AddressTypeIPv4, Address: []byte{127, 0, 0, 1}},
				DestinationAddress: &OtherAddress{},
				TransportLevel:     &TransportLevel{Protocol: TransportTCPClientRemote, Port: 80},
			},
			wantCause: BIPCauseBadLaunchParameters,
		},
		{
			name: "explicit local",
			cmd: OpenChannelCommand{
				LocalAddress:       &OtherAddress{Type: AddressTypeIPv4, Address: []byte{127, 0, 0, 1}},
				DestinationAddress: &OtherAddress{Type: AddressTypeIPv4, Address: []byte{192, 0, 2, 1}},
				TransportLevel:     &TransportLevel{Protocol: TransportUDPClientRemote, Port: 5000},
			},
			wantAddr:  "192.0.2.1:5000",
			wantLocal: true,
			wantCause: BIPCauseNoSpecificCause,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network, address, local, cause := tt.cmd.dialTarget()
			if cause != tt.wantCause {
				t.Fatalf("dialTarget() cause = 0x%02X, want 0x%02X", cause, tt.wantCause)
			}
			if cause != BIPCauseNoSpecificCause {
				return
			}
			if address != tt.wantAddr {
				t.Fatalf("dialTarget() address = %q, want %q", address, tt.wantAddr)
			}
			if (local != nil) != tt.wantLocal {
				t.Fatalf("dialTarget() local = %+v, want present %t", local, tt.wantLocal)
			}
			if network == "" {
				t.Fatal("dialTarget() network is empty")
			}
		})
	}
}
