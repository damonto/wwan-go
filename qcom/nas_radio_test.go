package qcom

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASRadioRequests(t *testing.T) {
	tests := []struct {
		name      string
		messageID MessageID
		check     func(*testing.T, Request)
		call      func(*Client) error
		response  Response
	}{
		{name: "reset", messageID: MessageNASReset, call: func(c *Client) error { return c.NASReset(context.Background()) }, response: successResponse(MessageNASReset)},
		{
			name:      "Tx Rx info",
			messageID: MessageNASGetTxRxInfo,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(NASRadioInterfaceLTE)})
			},
			call:     func(c *Client) error { _, err := c.TxRxInfo(context.Background(), NASRadioInterfaceLTE); return err },
			response: successResponse(MessageNASGetTxRxInfo),
		},
		{name: "CDMA position", messageID: MessageNASGetCDMAPositionInfo, call: func(c *Client) error { _, err := c.CDMAPositionInfo(context.Background()); return err }, response: successResponse(MessageNASGetCDMAPositionInfo)},
		{name: "force search", messageID: MessageNASForceNetworkSearch, call: func(c *Client) error { return c.ForceNetworkSearch(context.Background()) }, response: successResponse(MessageNASForceNetworkSearch)},
		{name: "DRX", messageID: MessageNASGetDRX, call: func(c *Client) error { _, err := c.DRX(context.Background()); return err }, response: successResponse(MessageNASGetDRX, tlv.Uint(0x10, uint32(NASDRXCN7T64)))},
		{name: "carrier aggregation", messageID: MessageNASGetLTECarrierAggregationInfo, call: func(c *Client) error { _, err := c.LTECarrierAggregationInfo(context.Background()); return err }, response: successResponse(MessageNASGetLTECarrierAggregationInfo)},
		{name: "EN-DC", messageID: MessageNASGetENDCConfig, call: func(c *Client) error { _, err := c.ENDCConfig(context.Background()); return err }, response: successResponse(MessageNASGetENDCConfig)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != tt.messageID {
						t.Fatalf("request = %+v, want NAS message 0x%04X", req, tt.messageID)
					}
					if tt.check != nil {
						tt.check(t, req)
					} else if len(req.TLVs) != 0 {
						t.Fatalf("TLV count = %d, want 0", len(req.TLVs))
					}
				},
				resp: tt.response,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			if err := tt.call(client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestNASRadioDecoding(t *testing.T) {
	rx := []byte{1}
	for _, value := range []int32{-850, -95, -70, -110} {
		rx = binary.LittleEndian.AppendUint32(rx, uint32(value))
	}
	rx = binary.LittleEndian.AppendUint32(rx, 90)
	txPower := int32(-50)
	tx := append([]byte{1}, binary.LittleEndian.AppendUint32(nil, uint32(txPower))...)

	cdma := []byte{1, 1}
	cdma = binary.LittleEndian.AppendUint32(cdma, uint32(NASCDMAPilotActive))
	cdma = binary.LittleEndian.AppendUint16(cdma, 1)
	cdma = binary.LittleEndian.AppendUint16(cdma, 2)
	cdma = binary.LittleEndian.AppendUint16(cdma, 3)
	cdma = binary.LittleEndian.AppendUint16(cdma, 4)
	cdma = binary.LittleEndian.AppendUint16(cdma, 5)
	latitude := int32(-6)
	longitude := int32(7)
	cdma = binary.LittleEndian.AppendUint32(cdma, uint32(latitude))
	cdma = binary.LittleEndian.AppendUint32(cdma, uint32(longitude))
	cdma = binary.LittleEndian.AppendUint64(cdma, 8)

	primary := binary.LittleEndian.AppendUint16(nil, 100)
	primary = binary.LittleEndian.AppendUint16(primary, 1800)
	primary = binary.LittleEndian.AppendUint32(primary, 5)
	primary = binary.LittleEndian.AppendUint16(primary, 120)
	secondary := []byte{1}
	secondary = binary.LittleEndian.AppendUint16(secondary, 101)
	secondary = binary.LittleEndian.AppendUint16(secondary, 1900)
	secondary = binary.LittleEndian.AppendUint32(secondary, 3)
	secondary = binary.LittleEndian.AppendUint16(secondary, 121)
	secondary = binary.LittleEndian.AppendUint32(secondary, uint32(NASSecondaryCellConfiguredActivated))
	secondary = append(secondary, 2)

	tests := []struct {
		name string
		fn   func(*testing.T) error
	}{
		{
			name: "Tx Rx",
			fn: func(t *testing.T) error {
				var got NASTxRxInfo
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, rx), tlv.Bytes(0x12, tx)})
				if err == nil && (len(got.RXChains) != 1 || got.RXChains[0].RXPower != -850 || !got.TXKnown || got.TX.Power != -50) {
					t.Fatalf("Tx/Rx info = %+v", got)
				}
				return err
			},
		},
		{
			name: "CDMA position",
			fn: func(t *testing.T) error {
				var got NASCDMAPositionInfo
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, cdma)})
				if err == nil && (!got.Known || !got.UEInIdle || len(got.BaseStations) != 1 || got.BaseStations[0].Latitude != -6 || got.BaseStations[0].GPSTimeMillis != 8) {
					t.Fatalf("CDMA position = %+v", got)
				}
				return err
			},
		},
		{
			name: "carrier aggregation",
			fn: func(t *testing.T) error {
				var got NASLTECarrierAggregationInfo
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Uint(0x11, uint32(5)),
					tlv.Bytes(0x13, primary),
					tlv.Bytes(0x15, secondary),
				})
				if err == nil && (!got.DLBandwidthKnown || !got.PrimaryCellKnown || len(got.SecondaryCells) != 1 || got.SecondaryCells[0].Index != 2 || got.SecondaryCells[0].State != NASSecondaryCellConfiguredActivated) {
					t.Fatalf("carrier aggregation = %+v", got)
				}
				return err
			},
		},
		{
			name: "EN-DC",
			fn: func(t *testing.T) error {
				var got NASENDCConfig
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x10, uint8(1)), tlv.Uint(0x11, uint8(0))})
				if err == nil && (!got.EnabledKnown || !got.Enabled || !got.ImmediateSCGReleaseKnown || got.ImmediateSCGRelease) {
					t.Fatalf("EN-DC config = %+v", got)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(t); err != nil {
				t.Fatalf("decode() error = %v", err)
			}
		})
	}
}

func TestNASRadioDecodersRejectMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*testing.T) error
	}{
		{name: "RX chain truncated", fn: func(*testing.T) error {
			var got NASTxRxInfo
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, make([]byte, 20))})
		}},
		{name: "CDMA count truncated", fn: func(*testing.T) error {
			var got NASCDMAPositionInfo
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, []byte{1})})
		}},
		{name: "primary cell truncated", fn: func(*testing.T) error {
			var got NASLTECarrierAggregationInfo
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x13, make([]byte, 9))})
		}},
		{name: "DRX value", fn: func(t *testing.T) error {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				resp: successResponse(MessageNASGetDRX, tlv.Uint(0x10, uint32(5))),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			_, err := client.DRX(context.Background())
			return err
		}},
		{name: "EN-DC flag truncated", fn: func(*testing.T) error {
			var got NASENDCConfig
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, nil)})
		}},
		{name: "EN-DC enabled boolean", fn: func(*testing.T) error {
			var got NASENDCConfig
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x10, uint8(2))})
		}},
		{name: "EN-DC immediate-release boolean", fn: func(*testing.T) error {
			var got NASENDCConfig
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x11, uint8(2))})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(t); err == nil {
				t.Fatal("decode() error = nil, want error")
			}
		})
	}
}
