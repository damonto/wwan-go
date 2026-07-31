package qcom

import (
	"context"
	"encoding/binary"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestLOCPositionReportUnmarshalTLVs(t *testing.T) {
	dop := appendLOCFloat32(nil, 1.25)
	dop = appendLOCFloat32(dop, 2.5)
	dop = appendLOCFloat32(dop, 3.75)
	gpsTime := binary.LittleEndian.AppendUint16(nil, 2345)
	gpsTime = binary.LittleEndian.AppendUint32(gpsTime, 456789)
	satellites := []byte{2}
	satellites = binary.LittleEndian.AppendUint16(satellites, 7)
	satellites = binary.LittleEndian.AppendUint16(satellites, 22)
	complete := tlv.TLVs{
		tlv.Uint(0x01, uint32(LOCSessionInProgress)),
		tlv.Uint(0x02, uint8(9)),
		tlv.Bytes(0x10, locFloat64Bytes(31.25)),
		tlv.Bytes(0x11, locFloat64Bytes(121.5)),
		tlv.Bytes(0x12, locFloat32Bytes(4.5)),
		tlv.Bytes(0x13, locFloat32Bytes(3.5)),
		tlv.Bytes(0x14, locFloat32Bytes(5.5)),
		tlv.Bytes(0x15, locFloat32Bytes(45)),
		tlv.Uint(0x16, uint8(68)),
		tlv.Uint(0x17, uint32(LOCReliabilityHigh)),
		tlv.Bytes(0x18, locFloat32Bytes(12.5)),
		tlv.Bytes(0x19, locFloat32Bytes(0.75)),
		tlv.Bytes(0x1A, locFloat32Bytes(52.25)),
		tlv.Bytes(0x1B, locFloat32Bytes(48.5)),
		tlv.Bytes(0x1C, locFloat32Bytes(6.25)),
		tlv.Uint(0x1D, uint8(75)),
		tlv.Uint(0x1E, uint32(LOCReliabilityMedium)),
		tlv.Bytes(0x1F, locFloat32Bytes(-0.5)),
		tlv.Bytes(0x20, locFloat32Bytes(180)),
		tlv.Bytes(0x21, locFloat32Bytes(2.25)),
		tlv.Bytes(0x22, locFloat32Bytes(-1.5)),
		tlv.Uint(0x23, uint32(LOCTechnologySatellite|LOCTechnologySensors)),
		tlv.Bytes(0x24, dop),
		tlv.Bytes(0x25, binary.LittleEndian.AppendUint64(nil, 1_712_345_678_901)),
		tlv.Uint(0x26, uint8(18)),
		tlv.Bytes(0x27, gpsTime),
		tlv.Bytes(0x28, locFloat32Bytes(15.5)),
		tlv.Uint(0x29, uint32(LOCTimeSourceExternalInput)),
		tlv.Bytes(0x2A, binary.LittleEndian.AppendUint64(nil, uint64(LOCSensorAccelerometerUsed|LOCSensorAidedPosition))),
		tlv.Uint(0x2B, uint32(3)),
		tlv.Bytes(0x2C, satellites),
		tlv.Uint(0x2D, uint8(1)),
	}
	want := LOCPositionReport{
		Status:                               LOCSessionInProgress,
		SessionID:                            9,
		SessionIDKnown:                       true,
		Latitude:                             31.25,
		LatitudeKnown:                        true,
		Longitude:                            121.5,
		LongitudeKnown:                       true,
		HorizontalUncertaintyCircular:        4.5,
		HorizontalUncertaintyCircularKnown:   true,
		HorizontalUncertaintyEllipticalMinor: 3.5,
		HorizontalUncertaintyEllipticalMinorKnown:   true,
		HorizontalUncertaintyEllipticalMajor:        5.5,
		HorizontalUncertaintyEllipticalMajorKnown:   true,
		HorizontalUncertaintyEllipticalAzimuth:      45,
		HorizontalUncertaintyEllipticalAzimuthKnown: true,
		HorizontalConfidence:                        68,
		HorizontalConfidenceKnown:                   true,
		HorizontalReliability:                       LOCReliabilityHigh,
		HorizontalReliabilityKnown:                  true,
		HorizontalSpeed:                             12.5,
		HorizontalSpeedKnown:                        true,
		SpeedUncertainty:                            0.75,
		SpeedUncertaintyKnown:                       true,
		AltitudeFromEllipsoid:                       52.25,
		AltitudeFromEllipsoidKnown:                  true,
		AltitudeFromSeaLevel:                        48.5,
		AltitudeFromSeaLevelKnown:                   true,
		VerticalUncertainty:                         6.25,
		VerticalUncertaintyKnown:                    true,
		VerticalConfidence:                          75,
		VerticalConfidenceKnown:                     true,
		VerticalReliability:                         LOCReliabilityMedium,
		VerticalReliabilityKnown:                    true,
		VerticalSpeed:                               -0.5,
		VerticalSpeedKnown:                          true,
		Heading:                                     180,
		HeadingKnown:                                true,
		HeadingUncertainty:                          2.25,
		HeadingUncertaintyKnown:                     true,
		MagneticDeviation:                           -1.5,
		MagneticDeviationKnown:                      true,
		Technology:                                  LOCTechnologySatellite | LOCTechnologySensors,
		TechnologyKnown:                             true,
		DOP:                                         LOCDilutionOfPrecision{Position: 1.25, Horizontal: 2.5, Vertical: 3.75},
		DOPKnown:                                    true,
		UTCTimestampMilliseconds:                    1_712_345_678_901,
		UTCTimestampKnown:                           true,
		LeapSeconds:                                 18,
		LeapSecondsKnown:                            true,
		GPSDateTime:                                 LOCGPSDateTime{Weeks: 2345, TimeOfWeekMilliseconds: 456789},
		GPSDateTimeKnown:                            true,
		TimeUncertaintyMilliseconds:                 15.5,
		TimeUncertaintyKnown:                        true,
		TimeSource:                                  LOCTimeSourceExternalInput,
		TimeSourceKnown:                             true,
		SensorDataUsage:                             LOCSensorAccelerometerUsed | LOCSensorAidedPosition,
		SensorDataUsageKnown:                        true,
		SessionFixCount:                             3,
		SessionFixCountKnown:                        true,
		SatellitesUsed:                              []uint16{7, 22},
		SatellitesUsedKnown:                         true,
		AltitudeAssumed:                             true,
		AltitudeAssumedKnown:                        true,
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    LOCPositionReport
		wantErr bool
	}{
		{name: "status only", tlvs: tlv.TLVs{tlv.Uint(0x01, uint32(LOCSessionSuccess))}, want: LOCPositionReport{Status: LOCSessionSuccess}},
		{name: "complete", tlvs: complete, want: want},
		{name: "status missing", tlvs: nil, wantErr: true},
		{name: "latitude truncated", tlvs: tlv.TLVs{tlv.Uint(0x01, uint32(LOCSessionSuccess)), tlv.Bytes(0x10, make([]byte, 7))}, wantErr: true},
		{name: "DOP truncated", tlvs: tlv.TLVs{tlv.Uint(0x01, uint32(LOCSessionSuccess)), tlv.Bytes(0x24, make([]byte, 11))}, wantErr: true},
		{name: "satellite count mismatch", tlvs: tlv.TLVs{tlv.Uint(0x01, uint32(LOCSessionSuccess)), tlv.Bytes(0x2C, []byte{2, 1, 0})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got LOCPositionReport
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("position report = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLOCGNSSSatelliteInfoUnmarshalTLVs(t *testing.T) {
	entry := binary.LittleEndian.AppendUint32(nil, uint32(LOCSatelliteValidSystem|LOCSatelliteValidID|LOCSatelliteValidSignalToNoiseRatio))
	entry = binary.LittleEndian.AppendUint32(entry, uint32(LOCSystemGPS))
	entry = binary.LittleEndian.AppendUint16(entry, 12)
	entry = append(entry, byte(LOCSatelliteHealthy))
	entry = binary.LittleEndian.AppendUint32(entry, uint32(LOCSatelliteTracking))
	entry = append(entry, byte(LOCNavigationHasEphemeris))
	entry = appendLOCFloat32(entry, 35.5)
	entry = appendLOCFloat32(entry, 142.25)
	entry = appendLOCFloat32(entry, 39.75)
	list := append([]byte{1}, entry...)
	want := LOCGNSSSatelliteInfo{
		AltitudeAssumed:      true,
		AltitudeAssumedKnown: true,
		Satellites: []LOCGNSSSatellite{{
			ValidInformation:   LOCSatelliteValidSystem | LOCSatelliteValidID | LOCSatelliteValidSignalToNoiseRatio,
			System:             LOCSystemGPS,
			ID:                 12,
			Health:             LOCSatelliteHealthy,
			Status:             LOCSatelliteTracking,
			NavigationData:     LOCNavigationHasEphemeris,
			ElevationDegrees:   35.5,
			AzimuthDegrees:     142.25,
			SignalToNoiseRatio: 39.75,
		}},
		SatellitesKnown: true,
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{name: "complete", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(1)), tlv.Bytes(0x10, list)}},
		{name: "empty", tlvs: nil},
		{name: "count missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}, wantErr: true},
		{name: "entry truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, append([]byte{1}, entry[:27]...))}, wantErr: true},
		{name: "trailing entry", tlvs: tlv.TLVs{tlv.Bytes(0x10, append(list, 0))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got LOCGNSSSatelliteInfo
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if tt.name == "complete" && !reflect.DeepEqual(got, want) {
				t.Fatalf("satellite info = %+v, want %+v", got, want)
			}
		})
	}
}

func TestLOCAuxiliaryEventUnmarshalTLVs(t *testing.T) {
	timeServerInfo := binary.LittleEndian.AppendUint32(nil, 250)
	timeServerInfo = append(timeServerInfo, 2, 3, 'n', 't', 'p', 4, 'n', 't', 's', '1')
	allowedSizes := binary.LittleEndian.AppendUint32(nil, 4096)
	allowedSizes = binary.LittleEndian.AppendUint32(allowedSizes, 1024)
	fileInfo := binary.LittleEndian.AppendUint32(nil, uint32(LOCPredictedOrbitsFileXTRA))
	fileInfo = binary.LittleEndian.AppendUint32(fileInfo, 720)
	tests := []struct {
		name string
		test func(*testing.T)
	}{
		{
			name: "inject time request",
			test: func(t *testing.T) {
				var got LOCInjectTimeRequest
				if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, timeServerInfo)}); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := LOCInjectTimeRequest{DelayThresholdMilliseconds: 250, Servers: []string{"ntp", "nts1"}}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("inject time request = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "predicted orbits request",
			test: func(t *testing.T) {
				var got LOCInjectPredictedOrbitsRequest
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x01, allowedSizes),
					tlv.Bytes(0x10, []byte{1, 4, 'x', 't', 'r', 'a'}),
					tlv.Bytes(0x11, binary.LittleEndian.AppendUint64(nil, uint64(LOCPredictedOrbitsInjectionRequested|LOCPredictedOrbitsServersUpdated))),
					tlv.Uint(0x12, uint32(LOCPredictedOrbitsRateUpdate)),
					tlv.Uint(0x13, uint32(3600)),
					tlv.Bytes(0x14, fileInfo),
				})
				if err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := LOCInjectPredictedOrbitsRequest{
					MaxFileSize:       4096,
					MaxPartSize:       1024,
					AllowedSizesKnown: true,
					Servers:           []string{"xtra"},
					ServersKnown:      true,
					ServerUpdate:      LOCPredictedOrbitsInjectionRequested | LOCPredictedOrbitsServersUpdated,
					ServerUpdateKnown: true,
					UpdateType:        LOCPredictedOrbitsRateUpdate,
					UpdateTypeKnown:   true,
					UpdatePeriod:      3600,
					UpdatePeriodKnown: true,
					File:              LOCPredictedOrbitsFileInfo{Type: LOCPredictedOrbitsFileXTRA, DownloadIntervalMinutes: 720},
					FileKnown:         true,
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("predicted orbits request = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "position request",
			test: func(t *testing.T) {
				var got LOCPositionInjectionRequest
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x01, locFloat64Bytes(30.5)),
					tlv.Bytes(0x02, locFloat64Bytes(120.75)),
					tlv.Bytes(0x03, locFloat32Bytes(500)),
					tlv.Bytes(0x04, binary.LittleEndian.AppendUint64(nil, 123456)),
				})
				if err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := LOCPositionInjectionRequest{
					Latitude: 30.5, LatitudeKnown: true,
					Longitude: 120.75, LongitudeKnown: true,
					HorizontalUncertaintyMeters: 500, HorizontalUncertaintyKnown: true,
					UTCTimestampMilliseconds: 123456, UTCTimestampKnown: true,
				}
				if got != want {
					t.Fatalf("position request = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "engine state",
			test: func(t *testing.T) {
				var got LOCEngineStateIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, binary.LittleEndian.AppendUint32(nil, uint32(LOCEngineOn)))}); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				if got.State != LOCEngineOn {
					t.Fatalf("State = %d, want %d", got.State, LOCEngineOn)
				}
			},
		},
		{
			name: "fix recurrence",
			test: func(t *testing.T) {
				var got LOCFixRecurrenceIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x10, uint32(LOCFixPeriodic))}); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				if got.Recurrence != LOCFixPeriodic {
					t.Fatalf("Recurrence = %d, want %d", got.Recurrence, LOCFixPeriodic)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestLOCAuxiliaryEventErrors(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{name: "inject time missing", call: func() error { return new(LOCInjectTimeRequest).UnmarshalTLVs(nil) }},
		{name: "inject time server truncated", call: func() error {
			return new(LOCInjectTimeRequest).UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, []byte{0, 0, 0, 0, 1})})
		}},
		{name: "predicted sizes truncated", call: func() error {
			return new(LOCInjectPredictedOrbitsRequest).UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, make([]byte, 7))})
		}},
		{name: "predicted file truncated", call: func() error {
			return new(LOCInjectPredictedOrbitsRequest).UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x14, make([]byte, 7))})
		}},
		{name: "position longitude truncated", call: func() error {
			return new(LOCPositionInjectionRequest).UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x02, make([]byte, 7))})
		}},
		{name: "engine state missing", call: func() error { return new(LOCEngineStateIndication).UnmarshalTLVs(nil) }},
		{name: "recurrence missing", call: func() error { return new(LOCFixRecurrenceIndication).UnmarshalTLVs(nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("call() error = nil, want non-nil")
			}
		})
	}
}

func TestLOCWatchPositionReports(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "register forward and release"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &locNMEAIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{check: func(req Request) { assertLOCEventMask(t, req, LOCEventPositionReport) }, resp: successResponse(MessageLOCRegisterEvents)},
				{check: func(req Request) { assertLOCEventMask(t, req, 0) }, resp: successResponse(MessageLOCRegisterEvents)},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceLOC: 7}}

			out, err := client.LOCWatchPositionReports(ctx)
			if err != nil {
				t.Fatalf("LOCWatchPositionReports() error = %v", err)
			}
			if transport.service != ServiceLOC || transport.clientID != 7 || transport.messageID != MessageLOCPositionReport {
				t.Fatalf("Indications() = service 0x%X client %d message 0x%04X", transport.service, transport.clientID, transport.messageID)
			}
			transport.emit(Indication{
				Service: ServiceLOC, ClientID: 7, MessageID: MessageLOCPositionReport,
				TLVs: tlv.TLVs{tlv.Uint(0x01, uint32(LOCSessionInProgress)), tlv.Uint(0x02, uint8(4))},
			})

			select {
			case got := <-out:
				if got.Status != LOCSessionInProgress || got.SessionID != 4 || !got.SessionIDKnown {
					t.Fatalf("report = %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for LOC position report")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func locFloat32Bytes(value float32) []byte {
	return binary.LittleEndian.AppendUint32(nil, math.Float32bits(value))
}

func locFloat64Bytes(value float64) []byte {
	return binary.LittleEndian.AppendUint64(nil, math.Float64bits(value))
}

func appendLOCFloat32(data []byte, value float32) []byte {
	return binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
}
