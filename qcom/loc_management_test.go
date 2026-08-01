package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestLOCManagementRequestEncoding(t *testing.T) {
	timeSource := LOCInjectedTimeNTP
	latitude := 31.25
	longitude := 121.5
	horizontalUncertainty := float32(1000)
	horizontalConfidence := uint8(68)
	horizontalReliability := LOCReliabilityMedium
	altitudeEllipsoid := float32(55.5)
	altitudeSeaLevel := float32(50.25)
	verticalUncertainty := float32(20)
	verticalConfidence := uint8(75)
	verticalReliability := LOCReliabilityLow
	altitudeSource := LOCAltitudeSourceInfo{
		Source:      LOCAltitudeGPS,
		Dependency:  LOCAltitudeDependsOnLatitudeLongitude,
		Uncertainty: LOCAltitudeUncertaintyAtPoint,
	}
	utcTimestamp := uint64(1_712_345_678_901)
	timestampAge := uint32(500)
	positionSource := LOCPositionSourceOther
	rawHorizontalUncertainty := float32(1250)
	rawHorizontalConfidence := uint8(60)
	requested := true
	provider := LOCPositionProviderExternal
	gpsDateTime := LOCGPSDateTime{Weeks: 2345, TimeOfWeekMilliseconds: 456789}
	timeUncertainty := float32(250)
	speed := LOCVector3{East: 1.25, North: -2.5, Up: 0.75}
	speedUncertainty := LOCVector3{East: 0.1, North: 0.2, Up: 0.3}
	numberSatellites := uint8(2)
	deleteAll := false
	gnssDelete := LOCDeleteGPSTime | LOCDeleteUTC
	cellDelete := LOCDeleteCellID | LOCDeleteCellNeighbors
	clockDelete := LOCDeleteClockRTCTime | LOCDeleteClockWeekNumber

	altitudeSourceValue := binary.LittleEndian.AppendUint32(nil, uint32(altitudeSource.Source))
	altitudeSourceValue = binary.LittleEndian.AppendUint32(altitudeSourceValue, uint32(altitudeSource.Dependency))
	altitudeSourceValue = binary.LittleEndian.AppendUint32(altitudeSourceValue, uint32(altitudeSource.Uncertainty))
	gpsDateTimeValue := binary.LittleEndian.AppendUint16(nil, gpsDateTime.Weeks)
	gpsDateTimeValue = binary.LittleEndian.AppendUint32(gpsDateTimeValue, gpsDateTime.TimeOfWeekMilliseconds)
	satellitesUsedValue := binary.LittleEndian.AppendUint32(nil, 2)
	satellitesUsedValue = binary.LittleEndian.AppendUint16(satellitesUsedValue, 7)
	satellitesUsedValue = binary.LittleEndian.AppendUint16(satellitesUsedValue, 22)
	deleteSatellitesValue := []byte{1}
	deleteSatellitesValue = binary.LittleEndian.AppendUint16(deleteSatellitesValue, 12)
	deleteSatellitesValue = binary.LittleEndian.AppendUint32(deleteSatellitesValue, uint32(LOCSystemGPS))
	deleteSatellitesValue = append(deleteSatellitesValue, byte(LOCDeleteSatelliteEphemeris|LOCDeleteSatelliteAlmanac))
	speedValue, err := speed.MarshalBinary()
	if err != nil {
		t.Fatalf("speed MarshalBinary() error = %v", err)
	}
	speedUncertaintyValue, err := speedUncertainty.MarshalBinary()
	if err != nil {
		t.Fatalf("speed uncertainty MarshalBinary() error = %v", err)
	}

	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[uint8][]byte
	}{
		{
			name: "get predicted orbits validity",
			request: func() (Request, error) {
				return (LOCGetPredictedOrbitsValidityRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessageLOCGetPredictedOrbitsValidity,
		},
		{
			name: "inject UTC time",
			request: func() (Request, error) {
				return (LOCInjectUTCTimeRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
					Config: LOCInjectUTCTimeConfig{UTCMilliseconds: utcTimestamp, UncertaintyMilliseconds: 1000, Source: &timeSource},
				}).Request()
			},
			wantMessage: MessageLOCInjectUTCTime,
			wantTLVs: map[uint8][]byte{
				0x01: binary.LittleEndian.AppendUint64(nil, utcTimestamp),
				0x02: binary.LittleEndian.AppendUint32(nil, 1000),
				0x10: binary.LittleEndian.AppendUint32(nil, uint32(timeSource)),
			},
		},
		{
			name: "inject position",
			request: func() (Request, error) {
				return (LOCInjectPositionRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
					Config: LOCInjectPositionConfig{
						Latitude:                         &latitude,
						Longitude:                        &longitude,
						HorizontalUncertaintyCircular:    &horizontalUncertainty,
						HorizontalConfidence:             &horizontalConfidence,
						HorizontalReliability:            &horizontalReliability,
						AltitudeFromEllipsoid:            &altitudeEllipsoid,
						AltitudeFromSeaLevel:             &altitudeSeaLevel,
						VerticalUncertainty:              &verticalUncertainty,
						VerticalConfidence:               &verticalConfidence,
						VerticalReliability:              &verticalReliability,
						AltitudeSource:                   &altitudeSource,
						UTCTimestampMilliseconds:         &utcTimestamp,
						TimestampAgeMilliseconds:         &timestampAge,
						PositionSource:                   &positionSource,
						RawHorizontalUncertaintyCircular: &rawHorizontalUncertainty,
						RawHorizontalConfidence:          &rawHorizontalConfidence,
						RequestedPositionInjection:       &requested,
						PositionSourceProvider:           &provider,
						GPSDateTime:                      &gpsDateTime,
						TimeUncertaintyMilliseconds:      &timeUncertainty,
						Speed:                            &speed,
						SpeedUncertainty:                 &speedUncertainty,
						SatellitesUsed:                   []uint16{7, 22},
						NumberSatellitesInFix:            &numberSatellites,
					},
				}).Request()
			},
			wantMessage: MessageLOCInjectPosition,
			wantTLVs: map[uint8][]byte{
				0x10: locFloat64Bytes(latitude),
				0x11: locFloat64Bytes(longitude),
				0x12: locFloat32Bytes(horizontalUncertainty),
				0x13: {horizontalConfidence},
				0x14: binary.LittleEndian.AppendUint32(nil, uint32(horizontalReliability)),
				0x15: locFloat32Bytes(altitudeEllipsoid),
				0x16: locFloat32Bytes(altitudeSeaLevel),
				0x17: locFloat32Bytes(verticalUncertainty),
				0x18: {verticalConfidence},
				0x19: binary.LittleEndian.AppendUint32(nil, uint32(verticalReliability)),
				0x1A: altitudeSourceValue,
				0x1B: binary.LittleEndian.AppendUint64(nil, utcTimestamp),
				0x1C: binary.LittleEndian.AppendUint32(nil, timestampAge),
				0x1D: binary.LittleEndian.AppendUint32(nil, uint32(positionSource)),
				0x1E: locFloat32Bytes(rawHorizontalUncertainty),
				0x1F: {rawHorizontalConfidence},
				0x20: {1},
				0x21: binary.LittleEndian.AppendUint32(nil, uint32(provider)),
				0x22: gpsDateTimeValue,
				0x23: locFloat32Bytes(timeUncertainty),
				0x24: speedValue,
				0x25: speedUncertaintyValue,
				0x26: satellitesUsedValue,
				0x27: {numberSatellites},
			},
		},
		{
			name: "set engine lock",
			request: func() (Request, error) {
				return (LOCSetEngineLockRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Lock: LOCLockAll}).Request()
			},
			wantMessage: MessageLOCSetEngineLock,
			wantTLVs:    map[uint8][]byte{0x01: binary.LittleEndian.AppendUint32(nil, uint32(LOCLockAll))},
		},
		{
			name: "get engine lock",
			request: func() (Request, error) {
				return (LOCGetEngineLockRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessageLOCGetEngineLock,
		},
		{
			name: "delete assistance data",
			request: func() (Request, error) {
				return (LOCDeleteAssistanceDataRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
					Config: LOCDeleteAssistanceDataConfig{
						DeleteAll:    &deleteAll,
						Satellites:   []LOCDeleteSatellite{{ID: 12, System: LOCSystemGPS, Info: LOCDeleteSatelliteEphemeris | LOCDeleteSatelliteAlmanac}},
						GNSSData:     &gnssDelete,
						CellDatabase: &cellDelete,
						ClockInfo:    &clockDelete,
					},
				}).Request()
			},
			wantMessage: MessageLOCDeleteAssistanceData,
			wantTLVs: map[uint8][]byte{
				0x01: {0},
				0x10: deleteSatellitesValue,
				0x11: binary.LittleEndian.AppendUint64(nil, uint64(gnssDelete)),
				0x12: binary.LittleEndian.AppendUint32(nil, uint32(cellDelete)),
				0x13: binary.LittleEndian.AppendUint32(nil, uint32(clockDelete)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceLOC || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = service 0x%X client %d transaction %d message 0x%04X", got.Service, got.ClientID, got.TransactionID, got.MessageID)
			}
			if got.Timeout != 3*time.Second {
				t.Fatalf("Timeout = %v, want 3s", got.Timeout)
			}
			if len(got.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLVs))
			}
			for kind, want := range tt.wantTLVs {
				assertTLV(t, got.TLVs, kind, want)
			}
		})
	}
}

func TestLOCManagementRequestValidation(t *testing.T) {
	invalidTimeSource := LOCInjectedTimeSource(-1)
	invalidReliability := LOCReliability(5)
	invalidAltitude := LOCAltitudeSourceInfo{Source: LOCAltitudeSource(10)}
	invalidPositionSource := LOCPositionSource(11)
	invalidProvider := LOCPositionSourceProvider(2)
	tooManySatellites := make([]LOCDeleteSatellite, 256)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "UTC source below range",
			call: func() error {
				_, err := (LOCInjectUTCTimeRequest{Config: LOCInjectUTCTimeConfig{Source: &invalidTimeSource}}).Request()
				return err
			},
		},
		{
			name: "horizontal reliability above range",
			call: func() error {
				_, err := (LOCInjectPositionRequest{Config: LOCInjectPositionConfig{HorizontalReliability: &invalidReliability}}).Request()
				return err
			},
		},
		{
			name: "altitude source above range",
			call: func() error {
				_, err := (LOCInjectPositionRequest{Config: LOCInjectPositionConfig{AltitudeSource: &invalidAltitude}}).Request()
				return err
			},
		},
		{
			name: "position source above range",
			call: func() error {
				_, err := (LOCInjectPositionRequest{Config: LOCInjectPositionConfig{PositionSource: &invalidPositionSource}}).Request()
				return err
			},
		},
		{
			name: "position provider above range",
			call: func() error {
				_, err := (LOCInjectPositionRequest{Config: LOCInjectPositionConfig{PositionSourceProvider: &invalidProvider}}).Request()
				return err
			},
		},
		{
			name: "engine lock below range",
			call: func() error {
				_, err := (LOCSetEngineLockRequest{Lock: 0}).Request()
				return err
			},
		},
		{
			name: "too many delete satellites",
			call: func() error {
				_, err := (LOCDeleteAssistanceDataRequest{Config: LOCDeleteAssistanceDataConfig{Satellites: tooManySatellites}}).Request()
				return err
			},
		},
		{
			name: "delete satellite system invalid",
			call: func() error {
				_, err := (LOCDeleteAssistanceDataRequest{Config: LOCDeleteAssistanceDataConfig{
					Satellites: []LOCDeleteSatellite{{System: 0}},
				}}).Request()
				return err
			},
		},
		{
			name: "delete satellite mask invalid",
			call: func() error {
				_, err := (LOCDeleteAssistanceDataRequest{Config: LOCDeleteAssistanceDataConfig{
					Satellites: []LOCDeleteSatellite{{System: LOCSystemGPS, Info: 1 << 2}},
				}}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("call() error = nil, want non-nil")
			}
		})
	}
}

func TestLOCManagementIndicationUnmarshalTLVs(t *testing.T) {
	validity := binary.LittleEndian.AppendUint64(nil, 1_712_345_678_901)
	validity = binary.LittleEndian.AppendUint16(validity, 72)
	success := tlv.Uint(0x01, uint32(LOCIndicationSuccess))
	tests := []struct {
		name string
		test func(*testing.T)
	}{
		{
			name: "predicted orbits validity",
			test: func(t *testing.T) {
				var got LOCGetPredictedOrbitsValidityIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{success, tlv.Bytes(0x10, validity)}); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := LOCPredictedOrbitsValidity{StartUTCMilliseconds: 1_712_345_678_901, DurationHours: 72}
				if got.Validity != want || !got.ValidityKnown {
					t.Fatalf("Validity = %+v known %v, want %+v", got.Validity, got.ValidityKnown, want)
				}
			},
		},
		{
			name: "engine lock",
			test: func(t *testing.T) {
				var got LOCGetEngineLockIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{success, tlv.Uint(0x10, uint32(LOCLockMobileInitiated))}); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				if got.Lock != LOCLockMobileInitiated || !got.LockKnown {
					t.Fatalf("Lock = %d known %v", got.Lock, got.LockKnown)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestLOCManagementIndicationErrors(t *testing.T) {
	success := tlv.Uint(0x01, uint32(LOCIndicationSuccess))
	tests := []struct {
		name string
		call func() error
	}{
		{name: "validity status missing", call: func() error { return new(LOCGetPredictedOrbitsValidityIndication).UnmarshalTLVs(nil) }},
		{name: "validity truncated", call: func() error {
			return new(LOCGetPredictedOrbitsValidityIndication).UnmarshalTLVs(tlv.TLVs{success, tlv.Bytes(0x10, make([]byte, 9))})
		}},
		{name: "lock truncated", call: func() error {
			return new(LOCGetEngineLockIndication).UnmarshalTLVs(tlv.TLVs{success, tlv.Bytes(0x10, make([]byte, 3))})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("call() error = nil, want non-nil")
			}
		})
	}
}

func TestLOCManagementClientOperations(t *testing.T) {
	tests := []struct {
		name       string
		message    MessageID
		indication tlv.TLVs
		call       func(*testing.T, *Client)
	}{
		{
			name:    "predicted orbits validity",
			message: MessageLOCGetPredictedOrbitsValidity,
			indication: tlv.TLVs{
				tlv.Uint(0x01, uint32(LOCIndicationSuccess)),
				tlv.Bytes(0x10, append(binary.LittleEndian.AppendUint64(nil, 1234), 48, 0)),
			},
			call: func(t *testing.T, client *Client) {
				got, err := client.LOCPredictedOrbitsDataValidity(context.Background())
				if err != nil {
					t.Fatalf("LOCPredictedOrbitsDataValidity() error = %v", err)
				}
				if got != (LOCPredictedOrbitsValidity{StartUTCMilliseconds: 1234, DurationHours: 48}) {
					t.Fatalf("validity = %+v", got)
				}
			},
		},
		{
			name:       "set engine lock",
			message:    MessageLOCSetEngineLock,
			indication: tlv.TLVs{tlv.Uint(0x01, uint32(LOCIndicationSuccess))},
			call: func(t *testing.T, client *Client) {
				if err := client.LOCSetEngineLock(context.Background(), LOCLockAll); err != nil {
					t.Fatalf("LOCSetEngineLock() error = %v", err)
				}
			},
		},
		{
			name:       "delete assistance data status",
			message:    MessageLOCDeleteAssistanceData,
			indication: tlv.TLVs{tlv.Uint(0x01, uint32(LOCIndicationGeneralFailure))},
			call: func(t *testing.T, client *Client) {
				err := client.LOCDeleteAssistanceData(context.Background(), LOCDeleteAssistanceDataConfig{})
				if !errors.Is(err, LOCIndicationGeneralFailure) {
					t.Fatalf("LOCDeleteAssistanceData() error = %v, want status %v", err, LOCIndicationGeneralFailure)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &locAsyncTransport{
				fakeTransport: fakeTransport{t: t, calls: []transportCall{{
					check: func(req Request) {
						if req.Service != ServiceLOC || req.MessageID != tt.message {
							t.Fatalf("request = service 0x%X message 0x%04X", req.Service, req.MessageID)
						}
					},
					resp: successResponse(tt.message),
				}}},
				indication: Indication{Service: ServiceLOC, ClientID: 7, MessageID: tt.message, TLVs: tt.indication},
			}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceLOC: 7}}
			tt.call(t, client)
		})
	}
}

func TestLOCVectorBinaryCodec(t *testing.T) {
	tests := []struct {
		name string
		want LOCVector3
	}{
		{name: "signed components", want: LOCVector3{East: 1.5, North: -2.25, Up: 0.75}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := tt.want.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			var got LOCVector3
			if err := got.UnmarshalBinary(encoded); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("vector = %+v, want %+v", got, tt.want)
			}
		})
	}
}
