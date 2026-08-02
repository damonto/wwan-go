package qmi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestIPADPMConfig(t *testing.T) {
	endpoint := qcom.DataEndpoint{Type: qcom.DataEndpointEmbedded, InterfaceID: 1}
	tests := []struct {
		name   string
		tx     string
		rx     string
		wantOK bool
		want   qcom.DPMHardwareDataPort
	}{
		{name: "missing endpoint IDs"},
		{name: "zero endpoint ID", tx: "0\n", rx: "22\n"},
		{name: "invalid endpoint ID", tx: "not-a-number\n", rx: "22\n"},
		{
			name:   "maps IPA pipes",
			tx:     "11\n",
			rx:     "22\n",
			wantOK: true,
			want: qcom.DPMHardwareDataPort{
				Endpoint:     endpoint,
				ConsumerPipe: 11,
				ProducerPipe: 22,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysPath := t.TempDir()
			writeIPASysfs(t, sysPath, "device/modem/tx_endpoint_id", tt.tx)
			writeIPASysfs(t, sysPath, "device/modem/rx_endpoint_id", tt.rx)

			config, ok := ipaDPMConfig(sysPath, endpoint)
			if ok != tt.wantOK {
				t.Fatalf("ipaDPMConfig() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if len(config.HardwareDataPorts) != 1 || !reflect.DeepEqual(config.HardwareDataPorts[0], tt.want) {
				t.Fatalf("HardwareDataPorts = %#v, want %#v", config.HardwareDataPorts, []qcom.DPMHardwareDataPort{tt.want})
			}
		})
	}
}

func TestIPARMNetFlags(t *testing.T) {
	tests := []struct {
		name string
		rx   string
		tx   string
		want uint32
	}{
		{name: "missing offload files", want: rmnetFlagIngressDeaggregation},
		{
			name: "MAPv4 offload",
			rx:   "MAPv4\n",
			tx:   "MAPv4\n",
			want: rmnetFlagIngressDeaggregation | rmnetFlagIngressMAPChecksumV4 | rmnetFlagEgressMAPChecksumV4,
		},
		{
			name: "MAPv5 offload",
			rx:   "MAPv5\n",
			tx:   "MAPv5\n",
			want: rmnetFlagIngressDeaggregation | rmnetFlagIngressMAPChecksumV5 | rmnetFlagEgressMAPChecksumV5,
		},
		{
			name: "mixed and unknown offload",
			rx:   "MAPv5-extra\n",
			tx:   "unknown\n",
			want: rmnetFlagIngressDeaggregation | rmnetFlagIngressMAPChecksumV5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysPath := t.TempDir()
			writeIPASysfs(t, sysPath, "device/feature/rx_offload", tt.rx)
			writeIPASysfs(t, sysPath, "device/feature/tx_offload", tt.tx)
			if got := ipaRMNetFlags(sysPath); got != tt.want {
				t.Errorf("ipaRMNetFlags() = %#x, want %#x", got, tt.want)
			}
		})
	}
}

func TestIPAWDADataFormatConfig(t *testing.T) {
	endpoint := qcom.DataEndpoint{Type: qcom.DataEndpointEmbedded, InterfaceID: 1}
	tests := []struct {
		name         string
		protocol     qcom.WDAAggregationProtocol
		useEndpoint  bool
		wantEndpoint bool
	}{
		{name: "QMAPv5", protocol: qcom.WDAAggregationQMAPv5},
		{name: "QMAPv4 with endpoint", protocol: qcom.WDAAggregationQMAPv4, useEndpoint: true, wantEndpoint: true},
		{name: "QMAP", protocol: qcom.WDAAggregationQMAP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ipaWDADataFormatConfig(endpoint, tt.protocol, tt.useEndpoint)
			if config.LinkLayerProtocol == nil || *config.LinkLayerProtocol != qcom.WDALinkLayerRawIP {
				t.Errorf("LinkLayerProtocol = %#v, want raw IP", config.LinkLayerProtocol)
			}
			if config.UplinkAggregation == nil || *config.UplinkAggregation != tt.protocol {
				t.Errorf("UplinkAggregation = %#v, want %d", config.UplinkAggregation, tt.protocol)
			}
			if config.DownlinkAggregation == nil || *config.DownlinkAggregation != tt.protocol {
				t.Errorf("DownlinkAggregation = %#v, want %d", config.DownlinkAggregation, tt.protocol)
			}
			if config.DownlinkMaxDatagrams == nil || *config.DownlinkMaxDatagrams != ipaDownlinkMaxDatagrams {
				t.Errorf("DownlinkMaxDatagrams = %#v, want %d", config.DownlinkMaxDatagrams, ipaDownlinkMaxDatagrams)
			}
			if config.DownlinkMaxSize == nil || *config.DownlinkMaxSize != ipaDownlinkMaxSize {
				t.Errorf("DownlinkMaxSize = %#v, want %d", config.DownlinkMaxSize, ipaDownlinkMaxSize)
			}
			if (config.Endpoint != nil) != tt.wantEndpoint {
				t.Fatalf("Endpoint present = %v, want %v", config.Endpoint != nil, tt.wantEndpoint)
			}
			if config.Endpoint != nil && *config.Endpoint != endpoint {
				t.Errorf("Endpoint = %#v, want %#v", *config.Endpoint, endpoint)
			}
		})
	}
}

func TestConfigureIPAWDA(t *testing.T) {
	errUnsupported := errors.New("aggregation is unsupported")
	tests := []struct {
		name             string
		current          qcom.WDAAggregationProtocol
		setErrors        map[qcom.WDAAggregationProtocol]error
		requireEndpoint  bool
		wantSets         []qcom.WDAAggregationProtocol
		wantGets         []bool
		wantSetEndpoints []bool
		wantErr          bool
	}{
		{name: "keeps QMAPv5", current: qcom.WDAAggregationQMAPv5, wantGets: []bool{false}},
		{
			name:             "upgrades QMAPv4",
			current:          qcom.WDAAggregationQMAPv4,
			wantSets:         []qcom.WDAAggregationProtocol{qcom.WDAAggregationQMAPv5},
			wantGets:         []bool{false},
			wantSetEndpoints: []bool{false},
		},
		{
			name:             "requeries QMAPv4 after QMAPv5 error",
			current:          qcom.WDAAggregationQMAPv4,
			setErrors:        map[qcom.WDAAggregationProtocol]error{qcom.WDAAggregationQMAPv5: errUnsupported},
			wantSets:         []qcom.WDAAggregationProtocol{qcom.WDAAggregationQMAPv5},
			wantGets:         []bool{false, false},
			wantSetEndpoints: []bool{false},
		},
		{
			name:    "falls back to QMAP",
			current: qcom.WDAAggregationDisabled,
			setErrors: map[qcom.WDAAggregationProtocol]error{
				qcom.WDAAggregationQMAPv5: errUnsupported,
				qcom.WDAAggregationQMAPv4: errUnsupported,
			},
			wantSets: []qcom.WDAAggregationProtocol{
				qcom.WDAAggregationQMAPv5,
				qcom.WDAAggregationQMAPv4,
				qcom.WDAAggregationQMAP,
			},
			wantGets:         []bool{false, false, false},
			wantSetEndpoints: []bool{false, false, false},
		},
		{
			name:             "retries with endpoint when required",
			current:          qcom.WDAAggregationDisabled,
			requireEndpoint:  true,
			wantSets:         []qcom.WDAAggregationProtocol{qcom.WDAAggregationQMAPv5},
			wantGets:         []bool{false, true},
			wantSetEndpoints: []bool{true},
		},
		{
			name:    "reports exhaustion",
			current: qcom.WDAAggregationDisabled,
			setErrors: map[qcom.WDAAggregationProtocol]error{
				qcom.WDAAggregationQMAPv5: errUnsupported,
				qcom.WDAAggregationQMAPv4: errUnsupported,
				qcom.WDAAggregationQMAP:   errUnsupported,
			},
			wantSets: []qcom.WDAAggregationProtocol{
				qcom.WDAAggregationQMAPv5,
				qcom.WDAAggregationQMAPv4,
				qcom.WDAAggregationQMAP,
			},
			wantGets:         []bool{false, false, false, false},
			wantSetEndpoints: []bool{false, false, false},
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &wdaNegotiationTransport{
				current:         tt.current,
				setErrors:       tt.setErrors,
				requireEndpoint: tt.requireEndpoint,
			}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			defer func() {
				if err := client.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}()

			backend := New(client, "")
			err = backend.configureIPAWDA(context.Background(), qcom.DataEndpoint{Type: qcom.DataEndpointEmbedded, InterfaceID: 1})
			if (err != nil) != tt.wantErr {
				t.Fatalf("configureIPAWDA() error = %v, want error %v", err, tt.wantErr)
			}
			if !slices.Equal(transport.sets, tt.wantSets) {
				t.Errorf("Set protocols = %v, want %v", transport.sets, tt.wantSets)
			}
			if !slices.Equal(transport.getEndpoints, tt.wantGets) {
				t.Errorf("Get endpoint flags = %v, want %v", transport.getEndpoints, tt.wantGets)
			}
			if !slices.Equal(transport.setEndpoints, tt.wantSetEndpoints) {
				t.Errorf("Set endpoint flags = %v, want %v", transport.setEndpoints, tt.wantSetEndpoints)
			}
		})
	}
}

func TestResetInvalidatesIPAReady(t *testing.T) {
	errReset := errors.New("reset rejected")
	tests := []struct {
		name      string
		resetErr  error
		wantErr   bool
		wantReady bool
	}{
		{name: "successful reset invalidates cache"},
		{name: "failed reset preserves cache", resetErr: errReset, wantErr: true, wantReady: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &resetTransport{err: tt.resetErr}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			defer func() {
				if err := client.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}()

			backend := New(client, "")
			backend.ipaReady = map[string]struct{}{"rmnet_ipa0": {}}
			err = backend.Reset(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Reset() error = %v, want error %v", err, tt.wantErr)
			}
			_, ready := backend.ipaReady["rmnet_ipa0"]
			if ready != tt.wantReady {
				t.Errorf("IPA cache ready = %v, want %v", ready, tt.wantReady)
			}
		})
	}
}

func TestConnectPortIPALifecycle(t *testing.T) {
	errBind := errors.New("bind rejected")
	errLink := errors.New("link creation rejected")
	tests := []struct {
		name          string
		bindErr       error
		linkErr       error
		wantErr       error
		wantInterface string
		wantTail      []string
		wantNoWDS     bool
	}{
		{
			name:          "successful session stops WDS before deleting link",
			wantInterface: "qmap1.2",
			wantTail:      []string{"wds-stop", "link-close"},
		},
		{
			name:     "WDS failure deletes link",
			bindErr:  errBind,
			wantErr:  errBind,
			wantTail: []string{"wds-bind", "link-close"},
		},
		{
			name:      "link failure skips WDS",
			linkErr:   errLink,
			wantErr:   errLink,
			wantTail:  []string{"wda-get", "link-create"},
			wantNoWDS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := &ipaEventLog{}
			transport := &ipaBearerTransport{events: events, bindErr: tt.bindErr}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			defer func() {
				if err := client.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}()

			backend := New(client, "")
			backend.newRMNetLink = func(context.Context, string, uint32) (*rmnetLink, error) {
				events.add("link-create")
				if tt.linkErr != nil {
					return nil, tt.linkErr
				}
				return &rmnetLink{
					Name:  "qmap1.2",
					MuxID: 3,
					Index: 10,
					close: func(context.Context) error {
						events.add("link-close")
						return nil
					},
				}, nil
			}

			session, err := backend.ConnectPort(context.Background(), ConnectConfig{
				APN:       "internet",
				IPFamily:  IPFamilyIPv4,
				Interface: "rmnet_ipa0",
			}, Port{
				Type:    PortNetwork,
				Name:    "rmnet_ipa0",
				SysPath: t.TempDir(),
				QMIEndpoint: QMIEndpoint{
					Type:            QMIEndpointEmbedded,
					InterfaceNumber: 1,
				},
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ConnectPort() error = %v, want %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ConnectPort() error = %v", err)
				}
				if got := session.Info().Network.Interface; got != tt.wantInterface {
					t.Errorf("Network.Interface = %q, want %q", got, tt.wantInterface)
				}
				assertIPABindRequest(t, transport.requests, 3)
				if err := session.Close(); err != nil {
					t.Errorf("Session.Close() error = %v", err)
				}
			}

			gotEvents := events.values()
			if !hasSuffix(gotEvents, tt.wantTail) {
				t.Errorf("events = %v, want suffix %v", gotEvents, tt.wantTail)
			}
			if tt.wantNoWDS && slices.ContainsFunc(gotEvents, func(event string) bool {
				return len(event) >= 4 && event[:4] == "wds-"
			}) {
				t.Errorf("events = %v, want no WDS requests", gotEvents)
			}
		})
	}
}

func writeIPASysfs(t *testing.T, sysPath, relativePath, value string) {
	t.Helper()
	if value == "" {
		return
	}
	path := filepath.Join(sysPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

type wdaNegotiationTransport struct {
	current         qcom.WDAAggregationProtocol
	setErrors       map[qcom.WDAAggregationProtocol]error
	requireEndpoint bool
	sets            []qcom.WDAAggregationProtocol
	getEndpoints    []bool
	setEndpoints    []bool
}

func (t *wdaNegotiationTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	switch req.MessageID {
	case qcom.MessageWDAGetDataFormat:
		_, hasEndpoint := tlv.Value(req.TLVs, 0x10)
		t.getEndpoints = append(t.getEndpoints, hasEndpoint)
		if t.requireEndpoint && !hasEndpoint {
			return testErrorResponse(req, qcom.QMIErrorMissingArgument), nil
		}
		return testResponse(req, wdaFormatTLVs(t.current)...), nil
	case qcom.MessageWDASetDataFormat:
		_, hasEndpoint := tlv.Value(req.TLVs, 0x17)
		t.setEndpoints = append(t.setEndpoints, hasEndpoint)
		value, ok := tlv.Value(req.TLVs, 0x12)
		if !ok || len(value) != 4 {
			return qcom.Response{}, errors.New("WDA uplink aggregation TLV is missing")
		}
		protocol := qcom.WDAAggregationProtocol(binary.LittleEndian.Uint32(value))
		t.sets = append(t.sets, protocol)
		if err := t.setErrors[protocol]; err != nil {
			return qcom.Response{}, err
		}
		t.current = protocol
		return testResponse(req, wdaFormatTLVs(protocol)...), nil
	default:
		return qcom.Response{}, fmt.Errorf("unexpected QMI message 0x%04X", req.MessageID)
	}
}

func (*wdaNegotiationTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (*wdaNegotiationTransport) Close() error { return nil }

type resetTransport struct {
	err error
}

func (t *resetTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	if t.err != nil {
		return qcom.Response{}, t.err
	}
	return testResponse(req), nil
}

func (*resetTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (*resetTransport) Close() error { return nil }

type ipaEventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *ipaEventLog) add(event string) {
	l.mu.Lock()
	l.events = append(l.events, event)
	l.mu.Unlock()
}

func (l *ipaEventLog) values() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.events)
}

type ipaBearerTransport struct {
	events   *ipaEventLog
	bindErr  error
	requests []qcom.Request
}

func (t *ipaBearerTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	t.requests = append(t.requests, req)
	switch {
	case req.Service == qcom.ServiceWDA && req.MessageID == qcom.MessageWDAGetDataFormat:
		t.events.add("wda-get")
		return testResponse(req, wdaFormatTLVs(qcom.WDAAggregationQMAPv5)...), nil
	case req.Service == qcom.ServiceWDS && req.MessageID == qcom.MessageWDSSetClientIPFamily:
		t.events.add("wds-family")
	case req.Service == qcom.ServiceWDS && req.MessageID == qcom.MessageWDSBindMuxDataPort:
		t.events.add("wds-bind")
		if t.bindErr != nil {
			return qcom.Response{}, t.bindErr
		}
	case req.Service == qcom.ServiceWDS && req.MessageID == qcom.MessageWDSStartNetworkInterface:
		t.events.add("wds-start")
		return testResponse(req, tlv.Uint(0x01, uint32(0x01020304))), nil
	case req.Service == qcom.ServiceWDS && req.MessageID == qcom.MessageWDSGetRuntimeSettings:
		t.events.add("wds-runtime")
	case req.Service == qcom.ServiceWDS && req.MessageID == qcom.MessageWDSStopNetworkInterface:
		t.events.add("wds-stop")
	default:
		return qcom.Response{}, fmt.Errorf("unexpected QMI request service=0x%02X message=0x%04X", req.Service, req.MessageID)
	}
	return testResponse(req), nil
}

func (*ipaBearerTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (*ipaBearerTransport) Close() error { return nil }

func testResponse(req qcom.Request, extra ...tlv.TLV) qcom.Response {
	result := tlv.Bytes(0x02, []byte{0, 0, 0, 0})
	return qcom.Response{
		Service:       req.Service,
		ClientID:      req.ClientID,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		TLVs:          append(tlv.TLVs{result}, extra...),
	}
}

func testErrorResponse(req qcom.Request, code qcom.QMIError) qcom.Response {
	result := binary.LittleEndian.AppendUint16(nil, uint16(qcom.QMIResultFailure))
	result = binary.LittleEndian.AppendUint16(result, uint16(code))
	return qcom.Response{
		Service:       req.Service,
		ClientID:      req.ClientID,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		TLVs:          tlv.TLVs{tlv.Bytes(0x02, result)},
	}
}

func wdaFormatTLVs(protocol qcom.WDAAggregationProtocol) []tlv.TLV {
	return []tlv.TLV{
		tlv.Uint(0x11, uint32(qcom.WDALinkLayerRawIP)),
		tlv.Uint(0x12, uint32(protocol)),
		tlv.Uint(0x13, uint32(protocol)),
	}
}

func assertIPABindRequest(t *testing.T, requests []qcom.Request, wantMuxID uint8) {
	t.Helper()
	index := slices.IndexFunc(requests, func(req qcom.Request) bool {
		return req.Service == qcom.ServiceWDS && req.MessageID == qcom.MessageWDSBindMuxDataPort
	})
	if index < 0 {
		t.Fatal("WDS Bind Mux Data Port request is missing")
	}
	req := requests[index]
	endpoint, ok := tlv.Value(req.TLVs, 0x10)
	if !ok || len(endpoint) != 8 {
		t.Fatalf("endpoint TLV = % X, want 8 bytes", endpoint)
	}
	if got := qcom.DataEndpointType(binary.LittleEndian.Uint32(endpoint[:4])); got != qcom.DataEndpointEmbedded {
		t.Errorf("endpoint type = %d, want embedded", got)
	}
	if got := binary.LittleEndian.Uint32(endpoint[4:]); got != 1 {
		t.Errorf("endpoint interface = %d, want 1", got)
	}
	muxID, ok := tlv.Value(req.TLVs, 0x11)
	if !ok || len(muxID) != 1 || muxID[0] != wantMuxID {
		t.Errorf("mux ID TLV = % X, want %d", muxID, wantMuxID)
	}
}

func hasSuffix[T comparable](values, suffix []T) bool {
	return len(values) >= len(suffix) && slices.Equal(values[len(values)-len(suffix):], suffix)
}
