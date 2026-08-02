package qmi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/damonto/wwan-go/qcom"
)

const (
	ipaDownlinkMaxDatagrams uint32 = 32
	ipaDownlinkMaxSize      uint32 = 32768
)

var ipaAggregationProtocols = [...]qcom.WDAAggregationProtocol{
	qcom.WDAAggregationQMAPv5,
	qcom.WDAAggregationQMAPv4,
	qcom.WDAAggregationQMAP,
}

func (b *Backend) prepareIPALink(ctx context.Context, port Port) (*rmnetLink, error) {
	if port.Name == "" {
		return nil, errors.New("preparing IPA data port: base interface is empty")
	}

	b.ipaMu.Lock()
	defer b.ipaMu.Unlock()

	endpoint := qcom.DataEndpoint{
		Type:        qcom.DataEndpointEmbedded,
		InterfaceID: port.QMIEndpoint.InterfaceNumber,
	}
	if _, ready := b.ipaReady[port.Name]; !ready {
		if err := b.openIPADPMPort(ctx, port.SysPath, endpoint); err != nil {
			return nil, err
		}
		if err := b.configureIPAWDA(ctx, endpoint); err != nil {
			return nil, err
		}
		if b.ipaReady == nil {
			b.ipaReady = make(map[string]struct{})
		}
		b.ipaReady[port.Name] = struct{}{}
	}

	newLink := b.newRMNetLink
	if newLink == nil {
		newLink = createRMNetLink
	}
	link, err := newLink(ctx, port.Name, ipaRMNetFlags(port.SysPath))
	if err != nil {
		return nil, fmt.Errorf("creating IPA rmnet link on %s: %w", port.Name, err)
	}
	return link, nil
}

func (b *Backend) openIPADPMPort(ctx context.Context, sysPath string, endpoint qcom.DataEndpoint) error {
	config, ok := ipaDPMConfig(sysPath, endpoint)
	if !ok {
		return nil
	}
	if err := b.client.DPMOpenPort(ctx, config); err != nil {
		return fmt.Errorf("opening IPA DPM port: %w", err)
	}
	return nil
}

func ipaDPMConfig(sysPath string, endpoint qcom.DataEndpoint) (qcom.DPMOpenPortConfig, bool) {
	if sysPath == "" {
		return qcom.DPMOpenPortConfig{}, false
	}
	txID, txOK := readIPASysfsUint32(filepath.Join(sysPath, "device", "modem", "tx_endpoint_id"))
	rxID, rxOK := readIPASysfsUint32(filepath.Join(sysPath, "device", "modem", "rx_endpoint_id"))
	if !txOK || !rxOK || txID == 0 || rxID == 0 {
		return qcom.DPMOpenPortConfig{}, false
	}
	return qcom.DPMOpenPortConfig{HardwareDataPorts: []qcom.DPMHardwareDataPort{{
		Endpoint:     endpoint,
		ConsumerPipe: txID,
		ProducerPipe: rxID,
	}}}, true
}

func readIPASysfsUint32(path string) (uint32, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(value), true
}

func (b *Backend) configureIPAWDA(ctx context.Context, endpoint qcom.DataEndpoint) error {
	current, useEndpoint, err := b.readIPAWDADataFormat(ctx, endpoint, false)
	if err != nil {
		return fmt.Errorf("reading IPA WDA data format: %w", err)
	}

	var attempts []error
	for _, protocol := range ipaAggregationProtocols {
		if ipaWDAFormatMatches(current, protocol) {
			return nil
		}

		updated, err := b.client.SetWDADataFormat(ctx, ipaWDADataFormatConfig(endpoint, protocol, useEndpoint))
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("setting IPA WDA data format: %w", ctx.Err())
			}
			setErr := fmt.Errorf("setting %s aggregation: %w", ipaAggregationName(protocol), err)
			current, useEndpoint, err = b.readIPAWDADataFormat(ctx, endpoint, useEndpoint)
			if err != nil {
				return errors.Join(
					setErr,
					fmt.Errorf("checking IPA WDA data format after setting %s: %w", ipaAggregationName(protocol), err),
				)
			}
			if ipaWDAFormatMatches(current, protocol) {
				return nil
			}
			attempts = append(attempts, setErr)
			continue
		}
		if ipaWDAFormatMatches(updated, protocol) {
			return nil
		}

		current, useEndpoint, err = b.readIPAWDADataFormat(ctx, endpoint, useEndpoint)
		if err != nil {
			return fmt.Errorf("checking IPA WDA data format after setting %s: %w", ipaAggregationName(protocol), err)
		}
		if ipaWDAFormatMatches(current, protocol) {
			return nil
		}
		attempts = append(attempts, fmt.Errorf(
			"setting %s aggregation: modem reported link=%d uplink=%d downlink=%d",
			ipaAggregationName(protocol),
			current.LinkLayerProtocol,
			current.UplinkAggregation,
			current.DownlinkAggregation,
		))
	}
	return fmt.Errorf("negotiating IPA WDA data format: %w", errors.Join(attempts...))
}

func (b *Backend) readIPAWDADataFormat(
	ctx context.Context,
	endpoint qcom.DataEndpoint,
	useEndpoint bool,
) (qcom.WDADataFormat, bool, error) {
	if useEndpoint {
		format, err := b.client.WDADataFormatForEndpoint(ctx, &endpoint)
		return format, true, err
	}
	format, err := b.client.WDADataFormat(ctx)
	if !errors.Is(err, qcom.QMIErrorMissingArgument) {
		return format, false, err
	}
	format, err = b.client.WDADataFormatForEndpoint(ctx, &endpoint)
	return format, true, err
}

func ipaWDADataFormatConfig(
	endpoint qcom.DataEndpoint,
	protocol qcom.WDAAggregationProtocol,
	useEndpoint bool,
) qcom.WDADataFormatConfig {
	linkLayer := qcom.WDALinkLayerRawIP
	uplink := protocol
	downlink := protocol
	maxDatagrams := ipaDownlinkMaxDatagrams
	maxSize := ipaDownlinkMaxSize
	config := qcom.WDADataFormatConfig{
		LinkLayerProtocol:    &linkLayer,
		UplinkAggregation:    &uplink,
		DownlinkAggregation:  &downlink,
		DownlinkMaxDatagrams: &maxDatagrams,
		DownlinkMaxSize:      &maxSize,
	}
	if useEndpoint {
		config.Endpoint = &endpoint
	}
	return config
}

func ipaWDAFormatMatches(format qcom.WDADataFormat, protocol qcom.WDAAggregationProtocol) bool {
	return format.LinkLayerProtocolKnown && format.LinkLayerProtocol == qcom.WDALinkLayerRawIP &&
		format.UplinkAggregationKnown && format.UplinkAggregation == protocol &&
		format.DownlinkAggregationKnown && format.DownlinkAggregation == protocol
}

func ipaAggregationName(protocol qcom.WDAAggregationProtocol) string {
	switch protocol {
	case qcom.WDAAggregationQMAPv5:
		return "QMAPv5"
	case qcom.WDAAggregationQMAPv4:
		return "QMAPv4"
	case qcom.WDAAggregationQMAP:
		return "QMAP"
	default:
		return fmt.Sprintf("aggregation %d", protocol)
	}
}

func ipaRMNetFlags(sysPath string) uint32 {
	flags := uint32(rmnetFlagIngressDeaggregation)
	if sysPath == "" {
		return flags
	}
	if value, err := os.ReadFile(filepath.Join(sysPath, "device", "feature", "rx_offload")); err == nil {
		switch {
		case strings.HasPrefix(string(value), "MAPv4"):
			flags |= rmnetFlagIngressMAPChecksumV4
		case strings.HasPrefix(string(value), "MAPv5"):
			flags |= rmnetFlagIngressMAPChecksumV5
		}
	}
	if value, err := os.ReadFile(filepath.Join(sysPath, "device", "feature", "tx_offload")); err == nil {
		switch {
		case strings.HasPrefix(string(value), "MAPv4"):
			flags |= rmnetFlagEgressMAPChecksumV4
		case strings.HasPrefix(string(value), "MAPv5"):
			flags |= rmnetFlagEgressMAPChecksumV5
		}
	}
	return flags
}
