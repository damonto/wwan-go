package qcom

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

type boundNASScanTransport struct {
	nasIndicationTransport
}

func (t *boundNASScanTransport) QMIService() ServiceType {
	return ServiceNAS
}

func TestIncrementalNetworkScan(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "partial then complete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanType := NASNetworkScanPLMN
			transport := &boundNASScanTransport{
				nasIndicationTransport: nasIndicationTransport{
					fakeTransport: fakeTransport{
						t: t,
						calls: []transportCall{{
							check: func(req Request) {
								if req.Service != ServiceNAS || req.ClientID != 0 || req.MessageID != MessageNASIncrementalNetworkScan {
									t.Fatalf("request = %+v, want incremental NAS scan", req)
								}
								assertTLV(t, req.TLVs, 0x10, []byte{byte(NASNetworkScanPLMN)})
							},
							resp: successResponse(MessageNASIncrementalNetworkScan),
						}},
					},
				},
			}
			client := &Client{transport: transport}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			updates, err := client.IncrementalNetworkScan(ctx, NASIncrementalNetworkScanConfig{ScanType: &scanType})
			if err != nil {
				t.Fatalf("IncrementalNetworkScan() error = %v", err)
			}
			if transport.messageID != MessageNASIncrementalNetworkScan {
				t.Fatalf("Indications() message = 0x%04X, want incremental scan", transport.messageID)
			}

			networks := binary.LittleEndian.AppendUint16(nil, 1)
			networks = binary.LittleEndian.AppendUint16(networks, 460)
			networks = binary.LittleEndian.AppendUint16(networks, 1)
			networks = append(networks, byte(NASNetworkInUseAvailable), byte(NASRadioInterfaceLTE), 0, 7)
			networks = append(networks, []byte("Carrier")...)
			transport.emit(Indication{Service: ServiceNAS, MessageID: MessageNASIncrementalNetworkScan, TLVs: tlv.TLVs{
				tlv.Uint(0x01, uint32(NASIncrementalNetworkScanPartial)),
				tlv.Bytes(0x10, networks),
			}})
			transport.emit(Indication{Service: ServiceNAS, MessageID: MessageNASIncrementalNetworkScan, TLVs: tlv.TLVs{
				tlv.Uint(0x01, uint32(NASIncrementalNetworkScanComplete)),
			}})

			select {
			case update := <-updates:
				if update.Status != NASIncrementalNetworkScanPartial || len(update.Networks) != 1 || update.Networks[0].PLMN.Description != "Carrier" {
					t.Fatalf("partial update = %+v", update)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for partial scan update")
			}
			select {
			case update := <-updates:
				if update.Status != NASIncrementalNetworkScanComplete {
					t.Fatalf("terminal update = %+v", update)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for terminal scan update")
			}
			select {
			case _, ok := <-updates:
				if ok {
					t.Fatal("updates channel is open after terminal status")
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for scan channel close")
			}
		})
	}
}

func TestIncrementalNetworkScanRejectsMalformedData(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{name: "scan type out of range", fn: func() error {
			scanType := NASNetworkScanType(5)
			_, err := (NASIncrementalNetworkScanConfig{ScanType: &scanType}).MarshalTLVs()
			return err
		}},
		{name: "status missing", fn: func() error {
			var update NASIncrementalNetworkScanUpdate
			return update.UnmarshalTLVs(nil)
		}},
		{name: "status truncated", fn: func() error {
			var update NASIncrementalNetworkScanUpdate
			return update.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{0})})
		}},
		{name: "network truncated", fn: func() error {
			var update NASIncrementalNetworkScanUpdate
			return update.UnmarshalTLVs(tlv.TLVs{
				tlv.Uint(0x01, uint32(NASIncrementalNetworkScanPartial)),
				tlv.Bytes(0x10, []byte{1, 0}),
			})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("call() error = nil, want error")
			}
		})
	}
}

func TestIncrementalNetworkScanAbortsOnCancellation(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "context cancellation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var targetTransactionID uint16
			transport := &boundNASScanTransport{
				nasIndicationTransport: nasIndicationTransport{
					fakeTransport: fakeTransport{
						t: t,
						calls: []transportCall{
							{
								check: func(req Request) {
									targetTransactionID = req.TransactionID
								},
								resp: successResponse(MessageNASIncrementalNetworkScan),
							},
							{
								check: func(req Request) {
									if req.MessageID != MessageNASAbort {
										t.Fatalf("MessageID = 0x%04X, want NAS Abort", req.MessageID)
									}
									assertTLV(t, req.TLVs, 0x01, binary.LittleEndian.AppendUint16(nil, targetTransactionID))
								},
								resp: successResponse(MessageNASAbort),
							},
						},
					},
				},
			}
			client := &Client{transport: transport}
			ctx, cancel := context.WithCancel(context.Background())
			updates, err := client.IncrementalNetworkScan(ctx, NASIncrementalNetworkScanConfig{})
			if err != nil {
				t.Fatalf("IncrementalNetworkScan() error = %v", err)
			}
			cancel()
			select {
			case _, ok := <-updates:
				if ok {
					t.Fatal("updates channel remained open after cancellation")
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for scan cancellation")
			}
			transport.waitCalls(t, 2)
		})
	}
}
