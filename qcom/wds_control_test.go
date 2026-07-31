package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSControlRequestEncoding(t *testing.T) {
	delay := 1500 * time.Millisecond
	sendSCRI := true
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "channel rates",
			request: func() (Request, error) {
				return (WDSGetCurrentChannelRateRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageWDSGetCurrentChannelRate,
		},
		{
			name: "all packet statistics by default",
			request: func() (Request, error) {
				return (WDSGetPacketStatisticsRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageWDSGetPacketStatistics,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(WDSStatisticsAll)),
			},
		},
		{
			name: "selected packet statistics",
			request: func() (Request, error) {
				return (WDSGetPacketStatisticsRequest{
					ClientID:      7,
					TransactionID: 9,
					Mask:          WDSStatisticsTxBytes | WDSStatisticsRxBytes,
				}).Request(), nil
			},
			wantMessage: MessageWDSGetPacketStatistics,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(WDSStatisticsTxBytes|WDSStatisticsRxBytes)),
			},
		},
		{
			name: "go dormant",
			request: func() (Request, error) {
				return (WDSGoDormantRequest{
					ClientID:      7,
					TransactionID: 9,
					Config:        WDSGoDormantConfig{Delay: &delay, SendSCRI: &sendSCRI},
				}).Request()
			},
			wantMessage: MessageWDSGoDormant,
			wantTLVs: map[byte][]byte{
				0x10: binary.LittleEndian.AppendUint32(nil, 1500),
				0x11: {1},
			},
		},
		{
			name: "go active",
			request: func() (Request, error) {
				return (WDSGoActiveRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageWDSGoActive,
		},
		{
			name: "dormancy status",
			request: func() (Request, error) {
				return (WDSGetDormancyStatusRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageWDSGetDormancyStatus,
		},
		{
			name: "legacy bearer",
			request: func() (Request, error) {
				return (WDSGetDataBearerTechnologyRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageWDSGetDataBearerTechnology,
		},
		{
			name: "current bearer",
			request: func() (Request, error) {
				return (WDSGetCurrentDataBearerTechnologyRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageWDSGetCurrentDataBearerTechnology,
		},
		{
			name: "extended bearer",
			request: func() (Request, error) {
				return (WDSGetDataBearerTechnologyExRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageWDSGetDataBearerTechnologyEx,
		},
		{
			name: "bind subscription",
			request: func() (Request, error) {
				return (WDSBindSubscriptionRequest{ClientID: 7, TransactionID: 9, Subscription: WDSSubscriptionSecondary}).Request()
			},
			wantMessage: MessageWDSBindSubscription,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(WDSSubscriptionSecondary)),
			},
		},
		{
			name: "get bound subscription",
			request: func() (Request, error) {
				return (WDSGetBindSubscriptionRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageWDSGetBindSubscription,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceWDS || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = service 0x%X client %d transaction %d message 0x%04X", got.Service, got.ClientID, got.TransactionID, got.MessageID)
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

func TestWDSControlRequestValidation(t *testing.T) {
	negative := -time.Millisecond
	tooLong := time.Duration(1<<32) * time.Millisecond
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "negative dormancy delay",
			call: func() error {
				_, err := (WDSGoDormantRequest{Config: WDSGoDormantConfig{Delay: &negative}}).Request()
				return err
			},
		},
		{
			name: "excessive dormancy delay",
			call: func() error {
				_, err := (WDSGoDormantRequest{Config: WDSGoDormantConfig{Delay: &tooLong}}).Request()
				return err
			},
		},
		{
			name: "invalid subscription",
			call: func() error {
				_, err := (WDSBindSubscriptionRequest{Subscription: 4}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("validation error = nil, want non-nil")
			}
		})
	}
}

func TestWDSCurrentChannelRateResponseUnmarshalTLVs(t *testing.T) {
	legacy := appendUint32sForTest(100, 200, 300, 400)
	extended := binary.LittleEndian.AppendUint32(nil, uint32(WDSChannelRateMegabitsPerSecond))
	for _, value := range []uint64{1, 2, 3, 4} {
		extended = binary.LittleEndian.AppendUint64(extended, value)
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDSChannelRates
		wantErr bool
	}{
		{
			name: "legacy rates",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, legacy)},
			want: WDSChannelRates{Unit: WDSChannelRateBitsPerSecond, CurrentTx: 100, CurrentRx: 200, MaximumTx: 300, MaximumRx: 400},
		},
		{
			name: "extended rates preferred",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, legacy), tlv.Bytes(0x10, extended)},
			want: WDSChannelRates{Unit: WDSChannelRateMegabitsPerSecond, CurrentTx: 1, CurrentRx: 2, MaximumTx: 3, MaximumRx: 4, Extended: true},
		},
		{name: "mandatory rates missing", wantErr: true},
		{name: "mandatory rates truncated", tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 15))}, wantErr: true},
		{name: "mandatory rates trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 17))}, wantErr: true},
		{name: "extended rates truncated", tlvs: tlv.TLVs{tlv.Bytes(0x01, legacy), tlv.Bytes(0x10, make([]byte, 35))}, wantErr: true},
		{name: "extended rates trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x01, legacy), tlv.Bytes(0x10, make([]byte, 37))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response WDSGetCurrentChannelRateResponse
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
			if response.Rates != tt.want {
				t.Fatalf("Rates = %+v, want %+v", response.Rates, tt.want)
			}
		})
	}
}

func TestWDSPacketStatisticsResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDSPacketStatistics
		wantErr bool
	}{
		{
			name: "all counters",
			tlvs: tlv.TLVs{
				tlv.Uint(0x10, uint32(1)),
				tlv.Uint(0x11, uint32(2)),
				tlv.Uint(0x12, uint32(3)),
				tlv.Uint(0x13, uint32(4)),
				tlv.Uint(0x14, uint32(5)),
				tlv.Uint(0x15, uint32(6)),
				tlv.Bytes(0x19, binary.LittleEndian.AppendUint64(nil, 7)),
				tlv.Bytes(0x1A, binary.LittleEndian.AppendUint64(nil, 8)),
				tlv.Bytes(0x1B, binary.LittleEndian.AppendUint64(nil, 9)),
				tlv.Bytes(0x1C, binary.LittleEndian.AppendUint64(nil, 10)),
				tlv.Uint(0x1D, uint32(11)),
				tlv.Uint(0x1E, uint32(12)),
			},
			want: WDSPacketStatistics{
				TxPackets: 1, TxPacketsKnown: true,
				RxPackets: 2, RxPacketsKnown: true,
				TxErrors: 3, TxErrorsKnown: true,
				RxErrors: 4, RxErrorsKnown: true,
				TxOverflows: 5, TxOverflowsKnown: true,
				RxOverflows: 6, RxOverflowsKnown: true,
				TxBytes: 7, TxBytesKnown: true,
				RxBytes: 8, RxBytesKnown: true,
				LastCallTxBytes: 9, LastCallTxBytesKnown: true,
				LastCallRxBytes: 10, LastCallRxBytesKnown: true,
				TxDropped: 11, TxDroppedKnown: true,
				RxDropped: 12, RxDroppedKnown: true,
			},
		},
		{name: "optional counters absent"},
		{name: "uint32 counter truncated", tlvs: tlv.TLVs{tlv.Bytes(0x14, make([]byte, 3))}, wantErr: true},
		{name: "uint32 counter trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x14, make([]byte, 5))}, wantErr: true},
		{name: "uint64 counter truncated", tlvs: tlv.TLVs{tlv.Bytes(0x1A, make([]byte, 7))}, wantErr: true},
		{name: "uint64 counter trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x1A, make([]byte, 9))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response WDSGetPacketStatisticsResponse
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
			if response.Statistics != tt.want {
				t.Fatalf("Statistics = %+v, want %+v", response.Statistics, tt.want)
			}
		})
	}
}

func TestWDSControlResponsesUnmarshalTLVs(t *testing.T) {
	currentBearer := append([]byte{byte(WDSCurrentBearerNetwork3GPP)}, binary.LittleEndian.AppendUint32(nil, 0x20)...)
	currentBearer = binary.LittleEndian.AppendUint32(currentBearer, 0x1000)
	extendedBearer := binary.LittleEndian.AppendUint32(nil, uint32(WDSBearerNetwork3GPP))
	extendedBearer = binary.LittleEndian.AppendUint32(extendedBearer, uint32(WDSBearerRAT5G))
	extendedBearer = binary.LittleEndian.AppendUint64(extendedBearer, uint64(WDSBearerServiceOption5GSub6|WDSBearerServiceOption5GSA))

	tests := []struct {
		name    string
		call    func(tlv.TLVs) error
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{
			name: "dormancy active",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDormancyStatusResponse
				if err := response.UnmarshalTLVs(tlvs); err != nil {
					return err
				}
				if response.Status != WDSDormancyActive {
					t.Fatalf("Status = %d, want %d", response.Status, WDSDormancyActive)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(WDSDormancyActive))},
		},
		{
			name: "legacy bearer",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDataBearerTechnologyResponse
				if err := response.UnmarshalTLVs(tlvs); err != nil {
					return err
				}
				if response.Technology.Current != WDSDataBearerLTE || !response.Technology.LastKnown || response.Technology.Last != WDSDataBearerUnknown {
					t.Fatalf("Technology = %+v", response.Technology)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(WDSDataBearerLTE)), tlv.Bytes(0x10, []byte{0xFF})},
		},
		{
			name: "current bearer masks",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetCurrentDataBearerTechnologyResponse
				if err := response.UnmarshalTLVs(tlvs); err != nil {
					return err
				}
				want := WDSCurrentBearerTechnology{Network: WDSCurrentBearerNetwork3GPP, RATMask: 0x20, ServiceOptionMask: 0x1000}
				if response.Technology.Current != want {
					t.Fatalf("Current = %+v, want %+v", response.Technology.Current, want)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Bytes(0x01, currentBearer)},
		},
		{
			name: "extended 5G bearer",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDataBearerTechnologyExResponse
				if err := response.UnmarshalTLVs(tlvs); err != nil {
					return err
				}
				if !response.Technology.CurrentKnown || response.Technology.Current.RAT != WDSBearerRAT5G ||
					response.Technology.Current.ServiceOptions != WDSBearerServiceOption5GSub6|WDSBearerServiceOption5GSA {
					t.Fatalf("Technology = %+v", response.Technology)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Bytes(0x10, extendedBearer)},
		},
		{
			name: "bound subscription",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetBindSubscriptionResponse
				if err := response.UnmarshalTLVs(tlvs); err != nil {
					return err
				}
				if !response.SubscriptionKnown || response.Subscription != WDSSubscriptionTertiary {
					t.Fatalf("response = %+v", response)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Uint(0x10, uint32(WDSSubscriptionTertiary))},
		},
		{
			name: "dormancy missing",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDormancyStatusResponse
				return response.UnmarshalTLVs(tlvs)
			},
			wantErr: true,
		},
		{
			name: "legacy bearer missing",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDataBearerTechnologyResponse
				return response.UnmarshalTLVs(tlvs)
			},
			wantErr: true,
		},
		{
			name: "current bearer truncated",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetCurrentDataBearerTechnologyResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, make([]byte, 8))},
			wantErr: true,
		},
		{
			name: "extended bearer truncated",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDataBearerTechnologyExResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, make([]byte, 15))},
			wantErr: true,
		},
		{
			name: "bound subscription truncated",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetBindSubscriptionResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, make([]byte, 3))},
			wantErr: true,
		},
		{
			name: "dormancy trailing data",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDormancyStatusResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2))},
			wantErr: true,
		},
		{
			name: "legacy bearer trailing data",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDataBearerTechnologyResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2))},
			wantErr: true,
		},
		{
			name: "current bearer trailing data",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetCurrentDataBearerTechnologyResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, make([]byte, 10))},
			wantErr: true,
		},
		{
			name: "extended bearer trailing data",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetDataBearerTechnologyExResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, make([]byte, 17))},
			wantErr: true,
		},
		{
			name: "bound subscription trailing data",
			call: func(tlvs tlv.TLVs) error {
				var response WDSGetBindSubscriptionResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, make([]byte, 5))},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
		})
	}
}

func TestClientWDSControlOperations(t *testing.T) {
	legacyRates := appendUint32sForTest(1, 2, 3, 4)
	tests := []struct {
		name    string
		message MessageID
		call    func(*Client) error
		resp    Response
	}{
		{
			name:    "channel rates",
			message: MessageWDSGetCurrentChannelRate,
			call: func(c *Client) error {
				rates, err := c.WDSChannelRates(context.Background())
				if err == nil && rates.CurrentRx != 2 {
					t.Fatalf("rates = %+v", rates)
				}
				return err
			},
			resp: successResponse(MessageWDSGetCurrentChannelRate, tlv.Bytes(0x01, legacyRates)),
		},
		{
			name:    "statistics",
			message: MessageWDSGetPacketStatistics,
			call: func(c *Client) error {
				statistics, err := c.WDSPacketStatistics(context.Background(), WDSStatisticsTxPackets)
				if err == nil && (!statistics.TxPacketsKnown || statistics.TxPackets != 7) {
					t.Fatalf("statistics = %+v", statistics)
				}
				return err
			},
			resp: successResponse(MessageWDSGetPacketStatistics, tlv.Uint(0x10, uint32(7))),
		},
		{
			name:    "go dormant",
			message: MessageWDSGoDormant,
			call: func(c *Client) error {
				return c.WDSGoDormant(context.Background(), WDSGoDormantConfig{})
			},
			resp: successResponse(MessageWDSGoDormant),
		},
		{
			name:    "go active",
			message: MessageWDSGoActive,
			call: func(c *Client) error {
				return c.WDSGoActive(context.Background())
			},
			resp: successResponse(MessageWDSGoActive),
		},
		{
			name:    "dormancy status",
			message: MessageWDSGetDormancyStatus,
			call: func(c *Client) error {
				status, err := c.WDSDormancyStatus(context.Background())
				if err == nil && status != WDSDormancyDormant {
					t.Fatalf("status = %d", status)
				}
				return err
			},
			resp: successResponse(MessageWDSGetDormancyStatus, tlv.Uint(0x01, uint8(WDSDormancyDormant))),
		},
		{
			name:    "legacy bearer",
			message: MessageWDSGetDataBearerTechnology,
			call: func(c *Client) error {
				technology, err := c.WDSDataBearerTechnology(context.Background())
				if err == nil && technology.Current != WDSDataBearerLTE {
					t.Fatalf("technology = %+v", technology)
				}
				return err
			},
			resp: successResponse(MessageWDSGetDataBearerTechnology, tlv.Uint(0x01, uint8(WDSDataBearerLTE))),
		},
		{
			name:    "current bearer",
			message: MessageWDSGetCurrentDataBearerTechnology,
			call: func(c *Client) error {
				_, err := c.WDSCurrentDataBearerTechnology(context.Background())
				return err
			},
			resp: successResponse(MessageWDSGetCurrentDataBearerTechnology, tlv.Bytes(0x01, append([]byte{2}, make([]byte, 8)...))),
		},
		{
			name:    "extended bearer",
			message: MessageWDSGetDataBearerTechnologyEx,
			call: func(c *Client) error {
				technology, err := c.WDSDataBearerTechnologyExtended(context.Background())
				if err == nil && !technology.CurrentKnown {
					t.Fatalf("technology = %+v", technology)
				}
				return err
			},
			resp: successResponse(MessageWDSGetDataBearerTechnologyEx, tlv.Bytes(0x10, make([]byte, 16))),
		},
		{
			name:    "bind subscription",
			message: MessageWDSBindSubscription,
			call: func(c *Client) error {
				return c.WDSBindSubscription(context.Background(), WDSSubscriptionPrimary)
			},
			resp: successResponse(MessageWDSBindSubscription),
		},
		{
			name:    "bound subscription",
			message: MessageWDSGetBindSubscription,
			call: func(c *Client) error {
				subscription, err := c.WDSBoundSubscription(context.Background())
				if err == nil && subscription != WDSSubscriptionSecondary {
					t.Fatalf("subscription = %d", subscription)
				}
				return err
			},
			resp: successResponse(MessageWDSGetBindSubscription, tlv.Uint(0x10, uint32(WDSSubscriptionSecondary))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWDS || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			if err := tt.call(client); err != nil {
				t.Fatalf("operation error = %v", err)
			}
		})
	}
}

func TestPDNSessionWDSControlUsesSessionClient(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		call    func(*PDNSession) error
		resp    Response
	}{
		{
			name:    "channel rates",
			message: MessageWDSGetCurrentChannelRate,
			call: func(s *PDNSession) error {
				_, err := s.ChannelRates(context.Background())
				return err
			},
			resp: successResponse(MessageWDSGetCurrentChannelRate, tlv.Bytes(0x01, make([]byte, 16))),
		},
		{
			name:    "statistics",
			message: MessageWDSGetPacketStatistics,
			call: func(s *PDNSession) error {
				_, err := s.PacketStatistics(context.Background(), WDSStatisticsTxBytes)
				return err
			},
			resp: successResponse(MessageWDSGetPacketStatistics),
		},
		{
			name:    "go dormant",
			message: MessageWDSGoDormant,
			call: func(s *PDNSession) error {
				return s.GoDormant(context.Background(), WDSGoDormantConfig{})
			},
			resp: successResponse(MessageWDSGoDormant),
		},
		{
			name:    "go active",
			message: MessageWDSGoActive,
			call: func(s *PDNSession) error {
				return s.GoActive(context.Background())
			},
			resp: successResponse(MessageWDSGoActive),
		},
		{
			name:    "dormancy status",
			message: MessageWDSGetDormancyStatus,
			call: func(s *PDNSession) error {
				_, err := s.DormancyStatus(context.Background())
				return err
			},
			resp: successResponse(MessageWDSGetDormancyStatus, tlv.Uint(0x01, uint8(WDSDormancyActive))),
		},
		{
			name:    "extended bearer",
			message: MessageWDSGetDataBearerTechnologyEx,
			call: func(s *PDNSession) error {
				_, err := s.DataBearerTechnologyExtended(context.Background())
				return err
			},
			resp: successResponse(MessageWDSGetDataBearerTechnologyEx),
		},
		{
			name:    "bind subscription",
			message: MessageWDSBindSubscription,
			call: func(s *PDNSession) error {
				return s.bindSubscription(context.Background(), WDSSubscriptionTertiary)
			},
			resp: successResponse(MessageWDSBindSubscription),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWDS || req.ClientID != 11 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1}
			session := &PDNSession{client: client, wdsClientID: 11, wdsClientReady: true, timeout: time.Second}
			if err := tt.call(session); err != nil {
				t.Fatalf("session operation error = %v", err)
			}
		})
	}
}

func TestPDNSessionWDSControlRejectsUnavailableSession(t *testing.T) {
	tests := []struct {
		name    string
		session *PDNSession
	}{
		{name: "nil"},
		{name: "closed", session: &PDNSession{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.session.ChannelRates(context.Background())
			if err == nil {
				t.Fatal("ChannelRates() error = nil, want non-nil")
			}
		})
	}
}

func appendUint32sForTest(values ...uint32) []byte {
	var out []byte
	for _, value := range values {
		out = binary.LittleEndian.AppendUint32(out, value)
	}
	return out
}
