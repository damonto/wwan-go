package qcom

import (
	"context"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASIndicationRegisterRequest(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name   string
		config NASIndicationConfig
		want   map[byte][]byte
	}{
		{name: "empty", want: map[byte][]byte{}},
		{
			name: "common indications",
			config: NASIndicationConfig{
				SystemSelection: &enabled,
				OperatorName:    &enabled,
				ServingSystem:   &enabled,
				NetworkTime:     &disabled,
				SystemInfo:      &enabled,
				SignalInfo:      &enabled,
				ErrorRate:       &enabled,
				CurrentPLMNName: &enabled,
				RFBandInfo:      &enabled,
				EDRXChange:      &enabled,
				NetworkReject: &NASNetworkRejectIndicationConfig{
					Enabled: true, SuppressSystemInfo: true,
				},
			},
			want: map[byte][]byte{
				nasTLVRegisterSystemSelection: {1},
				nasTLVRegisterOperatorName:    {1},
				nasTLVRegisterServingSystem:   {1},
				nasTLVRegisterNetworkTime:     {0},
				nasTLVRegisterSystemInfo:      {1},
				nasTLVRegisterSignalInfo:      {1},
				nasTLVRegisterErrorRate:       {1},
				nasTLVRegisterCurrentPLMNName: {1},
				nasTLVRegisterRFBandInfo:      {1},
				nasTLVRegisterNetworkReject:   {1, 1},
				nasTLVRegisterEDRXChange:      {1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := (NASIndicationRegisterRequest{
				ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Config: tt.config,
			}).Request()
			if req.Service != ServiceNAS || req.ClientID != 7 || req.TransactionID != 9 ||
				req.MessageID != MessageNASIndicationRegister || req.Timeout != 3*time.Second {
				t.Fatalf("Request() = %+v", req)
			}
			if len(req.TLVs) != len(tt.want) {
				t.Fatalf("TLVs len = %d, want %d", len(req.TLVs), len(tt.want))
			}
			for typ, value := range tt.want {
				assertTLV(t, req.TLVs, typ, value)
			}
		})
	}
}

func TestNASIndicationRegistrationReferences(t *testing.T) {
	tests := []struct {
		name         string
		registration nasIndicationRegistration
		tlvType      byte
	}{
		{name: "system selection", registration: nasIndicationSystemSelection, tlvType: nasTLVRegisterSystemSelection},
		{name: "operator name", registration: nasIndicationOperatorName, tlvType: nasTLVRegisterOperatorName},
		{name: "serving system", registration: nasIndicationServingSystem, tlvType: nasTLVRegisterServingSystem},
		{name: "network time", registration: nasIndicationNetworkTime, tlvType: nasTLVRegisterNetworkTime},
		{name: "system info", registration: nasIndicationSystemInfo, tlvType: nasTLVRegisterSystemInfo},
		{name: "signal info", registration: nasIndicationSignalInfo, tlvType: nasTLVRegisterSignalInfo},
		{name: "error rate", registration: nasIndicationErrorRate, tlvType: nasTLVRegisterErrorRate},
		{name: "current PLMN name", registration: nasIndicationCurrentPLMNName, tlvType: nasTLVRegisterCurrentPLMNName},
		{name: "RF band info", registration: nasIndicationRFBandInfo, tlvType: nasTLVRegisterRFBandInfo},
		{name: "network reject", registration: nasIndicationNetworkReject, tlvType: nasTLVRegisterNetworkReject},
		{name: "eDRX change", registration: nasIndicationEDRXChange, tlvType: nasTLVRegisterEDRXChange},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) {
						want := []byte{1}
						if tt.registration == nasIndicationNetworkReject {
							want = []byte{1, 0}
						}
						assertTLV(t, req.TLVs, tt.tlvType, want)
					},
					resp: successResponse(MessageNASIndicationRegister),
				},
				{
					check: func(req Request) {
						want := []byte{0}
						if tt.registration == nasIndicationNetworkReject {
							want = []byte{0, 0}
						}
						assertTLV(t, req.TLVs, tt.tlvType, want)
					},
					resp: successResponse(MessageNASIndicationRegister),
				},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}

			if err := client.acquireNASIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("first acquireNASIndication() error = %v", err)
			}
			if err := client.acquireNASIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("second acquireNASIndication() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after two acquires = %d, want 1", got)
			}

			client.releaseNASIndication(tt.registration)
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after first release = %d, want 1", got)
			}
			client.releaseNASIndication(tt.registration)
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after final release = %d, want 2", got)
			}
		})
	}
}

func TestNASWatchersMessageMapping(t *testing.T) {
	tests := []struct {
		name    string
		call    func(context.Context, *Client) error
		wantID  MessageID
		wantTLV byte
	}{
		{
			name: "operator name",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchOperatorName(ctx)
				return err
			},
			wantID: MessageNASOperatorName, wantTLV: nasTLVRegisterOperatorName,
		},
		{
			name: "serving system",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchServingSystem(ctx)
				return err
			},
			wantID: MessageNASGetServingSystem, wantTLV: nasTLVRegisterServingSystem,
		},
		{
			name: "network time",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchNetworkTime(ctx)
				return err
			},
			wantID: MessageNASNetworkTime, wantTLV: nasTLVRegisterNetworkTime,
		},
		{
			name: "system info",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchSystemInfo(ctx)
				return err
			},
			wantID: MessageNASSysInfo, wantTLV: nasTLVRegisterSystemInfo,
		},
		{
			name: "signal info",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchSignalInfo(ctx)
				return err
			},
			wantID: MessageNASSignalInfo, wantTLV: nasTLVRegisterSignalInfo,
		},
		{
			name: "error rate",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchErrorRate(ctx)
				return err
			},
			wantID: MessageNASSetEventReport, wantTLV: nasTLVRegisterErrorRate,
		},
		{
			name: "current PLMN name",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchCurrentPLMNName(ctx)
				return err
			},
			wantID: MessageNASCurrentPLMNName, wantTLV: nasTLVRegisterCurrentPLMNName,
		},
		{
			name: "RF band info",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchRFBandInfo(ctx)
				return err
			},
			wantID: MessageNASRFBandInfo, wantTLV: nasTLVRegisterRFBandInfo,
		},
		{
			name: "system selection",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchSystemSelection(ctx)
				return err
			},
			wantID: MessageNASGetSystemSelectionPreference, wantTLV: nasTLVRegisterSystemSelection,
		},
		{
			name: "network reject",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchNetworkReject(ctx)
				return err
			},
			wantID: MessageNASNetworkReject, wantTLV: nasTLVRegisterNetworkReject,
		},
		{
			name: "eDRX change",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.NASWatchEDRXParameters(ctx)
				return err
			},
			wantID: MessageNASEDRXChangeInfo, wantTLV: nasTLVRegisterEDRXChange,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &nasIndicationTransport{
				fakeTransport: fakeTransport{t: t, calls: []transportCall{
					{
						check: func(req Request) {
							want := []byte{1}
							if tt.wantTLV == nasTLVRegisterNetworkReject {
								want = []byte{1, 0}
							}
							assertTLV(t, req.TLVs, tt.wantTLV, want)
						},
						resp: successResponse(MessageNASIndicationRegister),
					},
					{
						check: func(req Request) {
							want := []byte{0}
							if tt.wantTLV == nasTLVRegisterNetworkReject {
								want = []byte{0, 0}
							}
							assertTLV(t, req.TLVs, tt.wantTLV, want)
						},
						resp: successResponse(MessageNASIndicationRegister),
					},
				}},
			}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			if err := tt.call(ctx, client); err != nil {
				cancel()
				t.Fatalf("watch() error = %v", err)
			}
			if transport.messageID != tt.wantID {
				cancel()
				t.Fatalf("Indications() message = 0x%04X, want 0x%04X", transport.messageID, tt.wantID)
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func TestNASWatchServingSystemDecodesIndication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := &nasIndicationTransport{
		fakeTransport: fakeTransport{t: t, calls: []transportCall{
			{resp: successResponse(MessageNASIndicationRegister)},
			{resp: successResponse(MessageNASIndicationRegister)},
		}},
	}
	client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
	out, err := client.NASWatchServingSystem(ctx)
	if err != nil {
		t.Fatalf("NASWatchServingSystem() error = %v", err)
	}
	transport.emit(Indication{
		Service: ServiceNAS, ClientID: 7, MessageID: MessageNASGetServingSystem,
		TLVs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 1, 1, 2, 1, 8})},
	})
	select {
	case serving := <-out:
		if serving.RegistrationState != NASRegistrationRegistered ||
			len(serving.RadioInterfaces) != 1 || serving.RadioInterfaces[0] != NASRadioInterfaceLTE {
			t.Fatalf("serving system = %+v", serving)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for serving-system indication")
	}
	cancel()
	transport.waitCalls(t, 2)
}

func TestNASWatchNetworkRejectDecodesIndication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := &nasIndicationTransport{
		fakeTransport: fakeTransport{t: t, calls: []transportCall{
			{resp: successResponse(MessageNASIndicationRegister)},
			{resp: successResponse(MessageNASIndicationRegister)},
		}},
	}
	client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
	out, err := client.NASWatchNetworkReject(ctx)
	if err != nil {
		t.Fatalf("NASWatchNetworkReject() error = %v", err)
	}
	transport.emit(Indication{
		Service: ServiceNAS, ClientID: 7, MessageID: MessageNASNetworkReject,
		TLVs: tlv.TLVs{
			tlv.Uint(0x01, uint8(NASRadioInterfaceLTE)),
			tlv.Uint(0x02, uint8(NASNetworkServicePS)),
			tlv.Uint(0x03, uint8(NASRejectCongestion)),
		},
	})
	select {
	case reject := <-out:
		if reject.RadioInterface != NASRadioInterfaceLTE || reject.ServiceDomain != NASNetworkServicePS ||
			reject.Cause != NASRejectCongestion {
			t.Fatalf("network reject = %+v", reject)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for network-reject indication")
	}
	cancel()
	transport.waitCalls(t, 2)
}

func TestNASCurrentPLMNNameUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{
			name: "all fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{0x36, 0x01, 0x04, 0x01, 1}),
				tlv.Bytes(0x11, []byte{1, 2, 'S', 0}),
				tlv.Bytes(0x12, []byte{0, 0, 0, 3, 'N', 'e', 't'}),
				tlv.Bytes(0x13, []byte{1, 0, 0, 4, 'L', 0, 'N', 0}),
				tlv.Bytes(0x14, []byte{7, 0, 0, 0}),
				tlv.Bytes(0x15, []byte{1, 0, 0, 0, 1, 0, 0, 0}),
				tlv.Bytes(0x16, []byte{1, 0, 0, 0}),
				tlv.Bytes(0x17, []byte{byte(NASRadioInterfaceLTE)}),
				tlv.Bytes(0x18, []byte{1, 1, 'A', 0, 1, 'B', 0, 2, 0, 0, 0}),
				tlv.Bytes(0x19, []byte{2, 'C', 0, 'D', 0}),
				tlv.Bytes(0x1A, []byte{3, 0, 0, 0}),
				tlv.Bytes(0x1B, []byte{'E', 0, 0, 0}),
				tlv.Bytes(0x1C, []byte{1, 2, 3}),
			},
		},
		{name: "PLMN length", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1})}, wantErr: true},
		{name: "short name trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{0, 0, 0, 0, 1})}, wantErr: true},
		{name: "radio width", tlvs: tlv.TLVs{tlv.Bytes(0x17, []byte{8, 0})}, wantErr: true},
		{name: "tracking area width", tlvs: tlv.TLVs{tlv.Bytes(0x1C, []byte{1, 2})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASCurrentPLMNName
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !got.PLMNKnown || got.PLMN.String() != "310260" ||
				!got.ServiceProviderKnown || len(got.ServiceProvider.Data) != 2 ||
				!got.ShortKnown || string(got.Short.Data) != "Net" ||
				!got.LongKnown || !got.DisplayBitsKnown || !got.HomeNetworkKnown ||
				!got.RadioInterfaceKnown || got.RadioInterface != NASRadioInterfaceLTE ||
				!got.LocalizedKnown || len(got.Localized) != 1 ||
				!got.AdditionalInfoKnown || len(got.AdditionalInfo) != 2 ||
				!got.SourceKnown || got.Source != NASNetworkNameSourceNITZ ||
				!got.ServiceProviderExtKnown || len(got.ServiceProviderExtended) != 1 ||
				!got.NR5GTrackingAreaCodeKnown || got.NR5GTrackingAreaCode != [3]byte{1, 2, 3} {
				t.Fatalf("current PLMN name = %+v", got)
			}
		})
	}
}

func TestNASRFBandInfoUnmarshalIndicationTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{
			name: "all fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{8, 41, 0, 0x72, 0x06}),
				tlv.Bytes(0x10, []byte{8, 42, 0}),
				tlv.Bytes(0x11, []byte{12, 78, 0, 0xC4, 9, 0, 0}),
				tlv.Bytes(0x12, []byte{12, 13, 0, 0, 0}),
				tlv.Bytes(0x13, []byte{2, 0, 0, 0}),
			},
		},
		{name: "mandatory missing", wantErr: true},
		{name: "mandatory array encoding", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 8, 41, 0, 0x72, 0x06})}, wantErr: true},
		{name: "extended width", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{8, 41, 0, 0, 0}), tlv.Bytes(0x11, []byte{12})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASRFBandInfo
			err := got.UnmarshalIndicationTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalIndicationTLVs() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalIndicationTLVs() error = %v", err)
			}
			if !got.Extended || len(got.Bands) != 1 || got.Bands[0].RadioInterface != NASRadioInterfaceNR5G ||
				got.Bands[0].Band != 78 || got.Bands[0].Channel != 2500 ||
				!got.DedicatedBandsKnown || len(got.DedicatedBands) != 1 || got.DedicatedBands[0].Band != 42 ||
				!got.BandwidthsKnown || len(got.Bandwidths) != 1 || got.Bandwidths[0].Bandwidth != 13 ||
				!got.CIoTLTEModeKnown || got.CIoTLTEMode != NASCIoTLTEModeM1 {
				t.Fatalf("RF band indication = %+v", got)
			}
		})
	}
}

func TestNASEDRXParametersUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{
			name: "all fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{1}),
				tlv.Bytes(0x11, []byte{5}),
				tlv.Bytes(0x12, []byte{3}),
				tlv.Bytes(0x13, []byte{byte(NASRadioInterfaceLTE)}),
				tlv.Bytes(0x14, []byte{3, 0, 0, 0}),
			},
		},
		{name: "enabled width", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 0})}, wantErr: true},
		{name: "cycle width", tlvs: tlv.TLVs{tlv.Bytes(0x11, nil)}, wantErr: true},
		{name: "paging width", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0})}, wantErr: true},
		{name: "radio width", tlvs: tlv.TLVs{tlv.Bytes(0x13, nil)}, wantErr: true},
		{name: "LTE mode width", tlvs: tlv.TLVs{tlv.Bytes(0x14, []byte{1})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASEDRXParameters
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !got.EnabledKnown || !got.Enabled || !got.CycleLengthKnown || got.CycleLength != 5 ||
				!got.PagingTimeWindowKnown || got.PagingTimeWindow != 3 ||
				!got.RadioInterfaceKnown || got.RadioInterface != NASRadioInterfaceLTE ||
				!got.CIoTLTEModeKnown || got.CIoTLTEMode != NASCIoTLTEModeNB1 {
				t.Fatalf("eDRX parameters = %+v", got)
			}
		})
	}
}

type nasIndicationTransport struct {
	fakeTransport
	indications chan Indication
	messageID   MessageID
}

func (t *nasIndicationTransport) Indications(ctx context.Context, _ ServiceType, _ uint8, id MessageID) (<-chan Indication, error) {
	t.messageID = id
	t.indications = make(chan Indication, 4)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *nasIndicationTransport) emit(indication Indication) {
	t.indications <- indication
}

func (t *nasIndicationTransport) waitCalls(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if t.callCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	tb.Fatalf("Do() calls = %d, want at least %d", t.callCount(), want)
}
