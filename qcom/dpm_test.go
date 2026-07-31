package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDPMRequestEncoding(t *testing.T) {
	endpoint := func(kind DataEndpointType, id uint32) []byte {
		value := binary.LittleEndian.AppendUint32(nil, uint32(kind))
		return binary.LittleEndian.AppendUint32(value, id)
	}
	stringValue := func(value string) []byte {
		return append([]byte{byte(len(value))}, value...)
	}

	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "open all port types",
			request: func() (Request, error) {
				return (DPMOpenPortRequest{ClientID: 7, TransactionID: 9, Config: DPMOpenPortConfig{
					ControlPorts: []DPMControlPort{{
						Name:            "rmnet_ctrl",
						DefaultEndpoint: DataEndpoint{Type: DataEndpointHSUSB, InterfaceID: 4},
					}},
					HardwareDataPorts: []DPMHardwareDataPort{{
						Endpoint:     DataEndpoint{Type: DataEndpointPCIe, InterfaceID: 2},
						ConsumerPipe: 7,
						ProducerPipe: 8,
					}},
					SoftwareDataPorts: []DPMSoftwareDataPort{{
						Endpoint: DataEndpoint{Type: DataEndpointEmbedded},
						Name:     "rmnet",
					}},
					DataPortBuffers: []DPMDataPortBufferInfo{{
						Endpoint:           DataEndpoint{Type: DataEndpointBAMDMUX, InterfaceID: 1},
						UplinkFIFOSize:     4096,
						DownlinkFIFOSize:   8192,
						DownlinkBufferSize: 2048,
					}},
				}}).Request()
			},
			wantMessage: MessageDPMOpenPort,
			wantTLVs: map[byte][]byte{
				0x10: append(append([]byte{1}, stringValue("rmnet_ctrl")...), endpoint(DataEndpointHSUSB, 4)...),
				0x11: binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(append([]byte{1}, endpoint(DataEndpointPCIe, 2)...), 7), 8),
				0x12: append(append([]byte{1}, endpoint(DataEndpointEmbedded, 0)...), stringValue("rmnet")...),
				0x13: binary.LittleEndian.AppendUint32(
					binary.LittleEndian.AppendUint32(
						binary.LittleEndian.AppendUint32(append([]byte{1}, endpoint(DataEndpointBAMDMUX, 1)...), 4096),
						8192,
					),
					2048,
				),
			},
		},
		{
			name: "close control and data ports",
			request: func() (Request, error) {
				return (DPMClosePortRequest{ClientID: 8, TransactionID: 10, Config: DPMClosePortConfig{
					ControlPortNames: []string{"rmnet_ctrl"},
					DataPorts:        []DataEndpoint{{Type: DataEndpointBAMDMUX, InterfaceID: 1}},
				}}).Request()
			},
			wantMessage: MessageDPMClosePort,
			wantTLVs: map[byte][]byte{
				0x10: append([]byte{1}, stringValue("rmnet_ctrl")...),
				0x11: append([]byte{1}, endpoint(DataEndpointBAMDMUX, 1)...),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceDPM || got.MessageID != tt.wantMessage {
				t.Fatalf("request = service 0x%02X message 0x%04X", got.Service, got.MessageID)
			}
			if len(got.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLVs))
			}
			for kind, want := range tt.wantTLVs {
				value, ok := tlv.Value(got.TLVs, kind)
				if !ok {
					t.Fatalf("TLV 0x%02X missing", kind)
				}
				if !bytes.Equal(value, want) {
					t.Fatalf("TLV 0x%02X = % X, want % X", kind, value, want)
				}
			}
		})
	}
}

func TestDPMRequestValidation(t *testing.T) {
	tooMany := make([]DataEndpoint, dpmPortMax+1)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "open empty",
			call: func() error {
				_, err := (DPMOpenPortRequest{}).Request()
				return err
			},
		},
		{
			name: "close empty",
			call: func() error {
				_, err := (DPMClosePortRequest{}).Request()
				return err
			},
		},
		{
			name: "too many data ports",
			call: func() error {
				_, err := (DPMClosePortRequest{Config: DPMClosePortConfig{DataPorts: tooMany}}).Request()
				return err
			},
		},
		{
			name: "empty name",
			call: func() error {
				_, err := (DPMClosePortRequest{Config: DPMClosePortConfig{ControlPortNames: []string{" "}}}).Request()
				return err
			},
		},
		{
			name: "long name",
			call: func() error {
				_, err := (DPMClosePortRequest{Config: DPMClosePortConfig{ControlPortNames: []string{strings.Repeat("x", dpmPortNameMax+1)}}}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("Request() error = nil, want non-nil")
			}
		})
	}
}

func TestDPMCapabilitiesResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DPMCapabilities
		wantErr bool
	}{
		{
			name: "all fields",
			tlvs: tlv.TLVs{
				tlv.Uint(0x10, uint8(1)),
				tlv.Uint(0x11, uint8(0)),
				tlv.Uint(0x12, uint8(1)),
			},
			want: DPMCapabilities{
				ShimSupported:      true,
				ShimSupportedKnown: true,
				ReverseIPSyncKnown: true,
				CLATSupported:      true,
				CLATSupportedKnown: true,
			},
		},
		{name: "optional fields absent"},
		{name: "truncated field", tlvs: tlv.TLVs{tlv.Bytes(0x11, nil)}, wantErr: true},
		{name: "trailing field", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 0})}, wantErr: true},
		{name: "invalid boolean", tlvs: tlv.TLVs{tlv.Uint(0x11, uint8(2))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response DPMGetCapabilitiesResponse
			err := response.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if response.Capabilities != tt.want {
				t.Fatalf("Capabilities = %+v, want %+v", response.Capabilities, tt.want)
			}
		})
	}
}

func TestClientDPMOperations(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		call    func(*Client) error
		resp    Response
	}{
		{
			name:    "open",
			message: MessageDPMOpenPort,
			call: func(c *Client) error {
				return c.DPMOpenPort(context.Background(), DPMOpenPortConfig{DataPortBuffers: []DPMDataPortBufferInfo{{Endpoint: DataEndpoint{Type: DataEndpointBAMDMUX}}}})
			},
			resp: successResponse(MessageDPMOpenPort),
		},
		{
			name:    "close",
			message: MessageDPMClosePort,
			call: func(c *Client) error {
				return c.DPMClosePort(context.Background(), DPMClosePortConfig{DataPorts: []DataEndpoint{{Type: DataEndpointBAMDMUX}}})
			},
			resp: successResponse(MessageDPMClosePort),
		},
		{
			name:    "capabilities",
			message: MessageDPMGetCapabilities,
			call: func(c *Client) error {
				got, err := c.DPMCapabilities(context.Background())
				if err == nil && (!got.ShimSupportedKnown || !got.ShimSupported) {
					t.Fatalf("DPMCapabilities() = %+v", got)
				}
				return err
			},
			resp: successResponse(MessageDPMGetCapabilities, tlv.Uint(0x10, uint8(1))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceDPM || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceDPM: 7}}
			if err := tt.call(client); err != nil {
				t.Fatalf("operation error = %v", err)
			}
		})
	}
}
