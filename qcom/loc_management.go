package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const locMaxTLVValueLength = 1<<16 - 1

// LOCPredictedOrbitsValidity describes the useful lifetime of injected data.
type LOCPredictedOrbitsValidity struct {
	StartUTCMilliseconds uint64
	DurationHours        uint16
}

// LOCGetPredictedOrbitsValidityRequest encodes the validity query.
type LOCGetPredictedOrbitsValidityRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a LOC request.
func (r LOCGetPredictedOrbitsValidityRequest) Request() Request {
	return locEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageLOCGetPredictedOrbitsValidity)
}

// LOCGetPredictedOrbitsValidityIndication is the asynchronous validity result.
type LOCGetPredictedOrbitsValidityIndication struct {
	Result        LOCIndicationResult
	Validity      LOCPredictedOrbitsValidity
	ValidityKnown bool
}

// UnmarshalTLVs parses a predicted-orbits validity indication.
func (i *LOCGetPredictedOrbitsValidityIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCGetPredictedOrbitsValidityIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 10 {
			return fmt.Errorf("parsing QMI LOC predicted-orbits validity: validity TLV length %d, want 10", len(value))
		}
		i.Validity = LOCPredictedOrbitsValidity{
			StartUTCMilliseconds: binary.LittleEndian.Uint64(value[0:8]),
			DurationHours:        binary.LittleEndian.Uint16(value[8:10]),
		}
		i.ValidityKnown = true
	}
	return nil
}

// LOCInjectedTimeSource identifies the origin of injected UTC time.
type LOCInjectedTimeSource int32

const (
	LOCInjectedTimeUnknown LOCInjectedTimeSource = iota
	LOCInjectedTimeApplicationProcessor
	LOCInjectedTimeNTP
	LOCInjectedTimeNTS
)

// LOCInjectUTCTimeConfig contains UTC milliseconds and uncertainty.
type LOCInjectUTCTimeConfig struct {
	UTCMilliseconds         uint64
	UncertaintyMilliseconds uint32
	Source                  *LOCInjectedTimeSource
}

// LOCInjectUTCTimeRequest encodes QMI LOC Inject UTC Time.
type LOCInjectUTCTimeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        LOCInjectUTCTimeConfig
}

// Request converts UTC time into LOC TLVs.
func (r LOCInjectUTCTimeRequest) Request() (Request, error) {
	tlvs := tlv.TLVs{
		locUint64TLV(0x01, r.Config.UTCMilliseconds),
		tlv.Uint(0x02, r.Config.UncertaintyMilliseconds),
	}
	if r.Config.Source != nil {
		if *r.Config.Source < LOCInjectedTimeUnknown || *r.Config.Source > LOCInjectedTimeNTS {
			return Request{}, fmt.Errorf("encoding QMI LOC UTC time: source %d is out of range", *r.Config.Source)
		}
		tlvs = append(tlvs, locInt32TLV(0x10, int32(*r.Config.Source)))
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCInjectUTCTime,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCAltitudeSource identifies where injected altitude was obtained.
type LOCAltitudeSource uint32

const (
	LOCAltitudeUnknown LOCAltitudeSource = iota
	LOCAltitudeGPS
	LOCAltitudeCellID
	LOCAltitudeEnhancedCellID
	LOCAltitudeWiFi
	LOCAltitudeTerrestrial
	LOCAltitudeTerrestrialHybrid
	LOCAltitudeDatabase
	LOCAltitudeBarometer
	LOCAltitudeOther
)

// LOCAltitudeDependency describes horizontal and vertical interdependence.
type LOCAltitudeDependency uint32

const (
	LOCAltitudeDependencyUnknown LOCAltitudeDependency = iota
	LOCAltitudeFullyInterdependent
	LOCAltitudeDependsOnLatitudeLongitude
	LOCAltitudeFullyIndependent
)

// LOCAltitudeUncertainty describes the region covered by altitude uncertainty.
type LOCAltitudeUncertainty uint32

const (
	LOCAltitudeUncertaintyUnknown LOCAltitudeUncertainty = iota
	LOCAltitudeUncertaintyAtPoint
	LOCAltitudeUncertaintyFullRegion
)

// LOCAltitudeSourceInfo describes the provenance of injected altitude.
type LOCAltitudeSourceInfo struct {
	Source      LOCAltitudeSource
	Dependency  LOCAltitudeDependency
	Uncertainty LOCAltitudeUncertainty
}

// LOCPositionSource identifies where an injected position was obtained.
type LOCPositionSource uint32

const (
	LOCPositionSourceGNSS LOCPositionSource = iota
	LOCPositionSourceCellID
	LOCPositionSourceEnhancedCellID
	LOCPositionSourceWiFi
	LOCPositionSourceTerrestrial
	LOCPositionSourceGNSSTerrestrialHybrid
	LOCPositionSourceOther
	LOCPositionSourceDeadReckoning
	LOCPositionSourceFusedLocation
	LOCPositionSourceNetworkLocation
	LOCPositionSourceFusedAndroidEngine
)

// LOCPositionSourceProvider identifies an internal or external provider.
type LOCPositionSourceProvider uint32

const (
	LOCPositionProviderExternal LOCPositionSourceProvider = iota
	LOCPositionProviderInternal
)

// LOCVector3 contains east, north, and up components.
type LOCVector3 struct {
	East  float32
	North float32
	Up    float32
}

// MarshalBinary encodes an east, north, and up vector.
func (vector LOCVector3) MarshalBinary() ([]byte, error) {
	value := binary.LittleEndian.AppendUint32(nil, math.Float32bits(vector.East))
	value = binary.LittleEndian.AppendUint32(value, math.Float32bits(vector.North))
	return binary.LittleEndian.AppendUint32(value, math.Float32bits(vector.Up)), nil
}

// UnmarshalBinary decodes an east, north, and up vector.
func (vector *LOCVector3) UnmarshalBinary(value []byte) error {
	if len(value) != 12 {
		return fmt.Errorf("LOC vector length %d, want 12", len(value))
	}
	*vector = LOCVector3{
		East:  math.Float32frombits(binary.LittleEndian.Uint32(value[:4])),
		North: math.Float32frombits(binary.LittleEndian.Uint32(value[4:8])),
		Up:    math.Float32frombits(binary.LittleEndian.Uint32(value[8:12])),
	}
	return nil
}

// LOCInjectPositionConfig selects standard fields in Inject Position. Nil
// pointers and a nil SatellitesUsed slice omit the corresponding TLVs.
type LOCInjectPositionConfig struct {
	Latitude                         *float64
	Longitude                        *float64
	HorizontalUncertaintyCircular    *float32
	HorizontalConfidence             *uint8
	HorizontalReliability            *LOCReliability
	AltitudeFromEllipsoid            *float32
	AltitudeFromSeaLevel             *float32
	VerticalUncertainty              *float32
	VerticalConfidence               *uint8
	VerticalReliability              *LOCReliability
	AltitudeSource                   *LOCAltitudeSourceInfo
	UTCTimestampMilliseconds         *uint64
	TimestampAgeMilliseconds         *uint32
	PositionSource                   *LOCPositionSource
	RawHorizontalUncertaintyCircular *float32
	RawHorizontalConfidence          *uint8
	RequestedPositionInjection       *bool
	PositionSourceProvider           *LOCPositionSourceProvider
	GPSDateTime                      *LOCGPSDateTime
	TimeUncertaintyMilliseconds      *float32
	Speed                            *LOCVector3
	SpeedUncertainty                 *LOCVector3
	SatellitesUsed                   []uint16
	NumberSatellitesInFix            *uint8
}

// LOCInjectPositionRequest encodes QMI LOC Inject Position.
type LOCInjectPositionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        LOCInjectPositionConfig
}

// Request converts an injected position into LOC TLVs.
func (r LOCInjectPositionRequest) Request() (Request, error) {
	if err := r.Config.validate(); err != nil {
		return Request{}, fmt.Errorf("encoding QMI LOC position: %w", err)
	}
	var tlvs tlv.TLVs
	if r.Config.Latitude != nil {
		tlvs = append(tlvs, locFloat64TLV(0x10, *r.Config.Latitude))
	}
	if r.Config.Longitude != nil {
		tlvs = append(tlvs, locFloat64TLV(0x11, *r.Config.Longitude))
	}
	if r.Config.HorizontalUncertaintyCircular != nil {
		tlvs = append(tlvs, locFloat32TLV(0x12, *r.Config.HorizontalUncertaintyCircular))
	}
	if r.Config.HorizontalConfidence != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, *r.Config.HorizontalConfidence))
	}
	if r.Config.HorizontalReliability != nil {
		tlvs = append(tlvs, tlv.Uint(0x14, uint32(*r.Config.HorizontalReliability)))
	}
	if r.Config.AltitudeFromEllipsoid != nil {
		tlvs = append(tlvs, locFloat32TLV(0x15, *r.Config.AltitudeFromEllipsoid))
	}
	if r.Config.AltitudeFromSeaLevel != nil {
		tlvs = append(tlvs, locFloat32TLV(0x16, *r.Config.AltitudeFromSeaLevel))
	}
	if r.Config.VerticalUncertainty != nil {
		tlvs = append(tlvs, locFloat32TLV(0x17, *r.Config.VerticalUncertainty))
	}
	if r.Config.VerticalConfidence != nil {
		tlvs = append(tlvs, tlv.Uint(0x18, *r.Config.VerticalConfidence))
	}
	if r.Config.VerticalReliability != nil {
		tlvs = append(tlvs, tlv.Uint(0x19, uint32(*r.Config.VerticalReliability)))
	}
	if r.Config.AltitudeSource != nil {
		value := binary.LittleEndian.AppendUint32(nil, uint32(r.Config.AltitudeSource.Source))
		value = binary.LittleEndian.AppendUint32(value, uint32(r.Config.AltitudeSource.Dependency))
		value = binary.LittleEndian.AppendUint32(value, uint32(r.Config.AltitudeSource.Uncertainty))
		tlvs = append(tlvs, tlv.Bytes(0x1A, value))
	}
	if r.Config.UTCTimestampMilliseconds != nil {
		tlvs = append(tlvs, locUint64TLV(0x1B, *r.Config.UTCTimestampMilliseconds))
	}
	if r.Config.TimestampAgeMilliseconds != nil {
		tlvs = append(tlvs, tlv.Uint(0x1C, *r.Config.TimestampAgeMilliseconds))
	}
	if r.Config.PositionSource != nil {
		tlvs = append(tlvs, tlv.Uint(0x1D, uint32(*r.Config.PositionSource)))
	}
	if r.Config.RawHorizontalUncertaintyCircular != nil {
		tlvs = append(tlvs, locFloat32TLV(0x1E, *r.Config.RawHorizontalUncertaintyCircular))
	}
	if r.Config.RawHorizontalConfidence != nil {
		tlvs = append(tlvs, tlv.Uint(0x1F, *r.Config.RawHorizontalConfidence))
	}
	if r.Config.RequestedPositionInjection != nil {
		tlvs = append(tlvs, tlv.Uint(0x20, boolByte(*r.Config.RequestedPositionInjection)))
	}
	if r.Config.PositionSourceProvider != nil {
		tlvs = append(tlvs, tlv.Uint(0x21, uint32(*r.Config.PositionSourceProvider)))
	}
	if r.Config.GPSDateTime != nil {
		value := binary.LittleEndian.AppendUint16(nil, r.Config.GPSDateTime.Weeks)
		value = binary.LittleEndian.AppendUint32(value, r.Config.GPSDateTime.TimeOfWeekMilliseconds)
		tlvs = append(tlvs, tlv.Bytes(0x22, value))
	}
	if r.Config.TimeUncertaintyMilliseconds != nil {
		tlvs = append(tlvs, locFloat32TLV(0x23, *r.Config.TimeUncertaintyMilliseconds))
	}
	if r.Config.Speed != nil {
		value, err := r.Config.Speed.MarshalBinary()
		if err != nil {
			return Request{}, fmt.Errorf("encoding QMI LOC speed: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x24, value))
	}
	if r.Config.SpeedUncertainty != nil {
		value, err := r.Config.SpeedUncertainty.MarshalBinary()
		if err != nil {
			return Request{}, fmt.Errorf("encoding QMI LOC speed uncertainty: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x25, value))
	}
	if r.Config.SatellitesUsed != nil {
		if len(r.Config.SatellitesUsed) > (locMaxTLVValueLength-4)/2 {
			return Request{}, fmt.Errorf("encoding QMI LOC position: satellites used length %d exceeds protocol limit", len(r.Config.SatellitesUsed))
		}
		value := binary.LittleEndian.AppendUint32(nil, uint32(len(r.Config.SatellitesUsed)))
		for _, satellite := range r.Config.SatellitesUsed {
			value = binary.LittleEndian.AppendUint16(value, satellite)
		}
		tlvs = append(tlvs, tlv.Bytes(0x26, value))
	}
	if r.Config.NumberSatellitesInFix != nil {
		tlvs = append(tlvs, tlv.Uint(0x27, *r.Config.NumberSatellitesInFix))
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCInjectPosition,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCLockType controls which positioning sessions the engine accepts.
type LOCLockType uint32

const (
	LOCLockNone LOCLockType = iota + 1
	LOCLockMobileInitiated
	LOCLockMobileTerminated
	LOCLockAll
)

// LOCSetEngineLockRequest encodes QMI LOC Set Engine Lock.
type LOCSetEngineLockRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Lock          LOCLockType
}

// Request converts the lock type into a LOC request.
func (r LOCSetEngineLockRequest) Request() (Request, error) {
	if err := validateLOCLockType(r.Lock); err != nil {
		return Request{}, fmt.Errorf("encoding QMI LOC engine lock: %w", err)
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCSetEngineLock,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Lock))},
	}, nil
}

// LOCGetEngineLockRequest encodes QMI LOC Get Engine Lock.
type LOCGetEngineLockRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a LOC request.
func (r LOCGetEngineLockRequest) Request() Request {
	return locEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageLOCGetEngineLock)
}

// LOCGetEngineLockIndication is the asynchronous engine-lock result.
type LOCGetEngineLockIndication struct {
	Result    LOCIndicationResult
	Lock      LOCLockType
	LockKnown bool
}

// UnmarshalTLVs parses a Get Engine Lock indication.
func (i *LOCGetEngineLockIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCGetEngineLockIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	var lock uint32
	if err := parseLOCOptionalUint32(tlvs, 0x10, &lock, &i.LockKnown); err != nil {
		return err
	}
	i.Lock = LOCLockType(lock)
	return nil
}

// LOCDeleteSatelliteInfo is one satellite-specific assistance deletion.
type LOCDeleteSatelliteInfo uint8

const (
	LOCDeleteSatelliteEphemeris LOCDeleteSatelliteInfo = 1 << iota
	LOCDeleteSatelliteAlmanac
)

// LOCDeleteSatellite selects assistance data for one GNSS satellite.
type LOCDeleteSatellite struct {
	ID     uint16
	System LOCSystem
	Info   LOCDeleteSatelliteInfo
}

// LOCDeleteGNSSData is a mask selecting global GNSS assistance data.
type LOCDeleteGNSSData uint64

const (
	LOCDeleteGPSSVDir LOCDeleteGNSSData = 1 << iota
	LOCDeleteGPSSVSteer
	LOCDeleteGPSTime
	LOCDeleteGPSAlmanacCorrection
	LOCDeleteGLONASSSVDir
	LOCDeleteGLONASSSVSteer
	LOCDeleteGLONASSTime
	LOCDeleteGLONASSAlmanacCorrection
	LOCDeleteSBASSVDir
	LOCDeleteSBASSVSteer
	LOCDeletePosition
	LOCDeleteTime
	LOCDeleteIonosphere
	LOCDeleteUTC
	LOCDeleteHealth
	LOCDeleteSAData
	LOCDeleteRTI
	LOCDeleteSVNoExist
	LOCDeleteFrequencyBiasEstimate
)

// LOCDeleteCellDatabase is a mask selecting cached cellular locations.
type LOCDeleteCellDatabase uint32

const (
	LOCDeleteCellPosition LOCDeleteCellDatabase = 1 << iota
	LOCDeleteCellLatestGPSPosition
	LOCDeleteCellOTAPosition
	LOCDeleteCellExternalReferencePosition
	LOCDeleteCellTimeTag
	LOCDeleteCellID
	LOCDeleteCellCachedID
	LOCDeleteCellLastServing
	LOCDeleteCellCurrentServing
	LOCDeleteCellNeighbors
)

// LOCDeleteClockInfo is a mask selecting cached engine clock state.
type LOCDeleteClockInfo uint32

const (
	LOCDeleteClockTimeEstimate LOCDeleteClockInfo = 1 << iota
	LOCDeleteClockFrequencyEstimate
	LOCDeleteClockWeekNumber
	LOCDeleteClockRTCTime
	LOCDeleteClockTimeTransfer
	LOCDeleteClockGPSTimeEstimate
	LOCDeleteClockGLONASSTimeEstimate
	LOCDeleteClockGLONASSDayNumber
	LOCDeleteClockGLONASSYearNumber
	LOCDeleteClockGLONASSRFGroupDelay
	LOCDeleteClockDisableTimeTransfer
)

// LOCDeleteAssistanceDataConfig selects standard assistance data to remove.
type LOCDeleteAssistanceDataConfig struct {
	DeleteAll    *bool
	Satellites   []LOCDeleteSatellite
	GNSSData     *LOCDeleteGNSSData
	CellDatabase *LOCDeleteCellDatabase
	ClockInfo    *LOCDeleteClockInfo
}

// LOCDeleteAssistanceDataRequest encodes QMI LOC Delete Assistance Data.
type LOCDeleteAssistanceDataRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        LOCDeleteAssistanceDataConfig
}

// Request converts deletion selectors into LOC TLVs.
func (r LOCDeleteAssistanceDataRequest) Request() (Request, error) {
	var tlvs tlv.TLVs
	if r.Config.DeleteAll != nil {
		tlvs = append(tlvs, tlv.Uint(0x01, boolByte(*r.Config.DeleteAll)))
	}
	if r.Config.Satellites != nil {
		if len(r.Config.Satellites) > 255 {
			return Request{}, fmt.Errorf("encoding QMI LOC assistance deletion: satellite count %d exceeds 255", len(r.Config.Satellites))
		}
		value := []byte{byte(len(r.Config.Satellites))}
		for index, satellite := range r.Config.Satellites {
			if satellite.System < LOCSystemGPS || satellite.System > LOCSystemGLONASS {
				return Request{}, fmt.Errorf("encoding QMI LOC assistance deletion: satellite %d system %d is out of range", index, satellite.System)
			}
			if satellite.Info&^(LOCDeleteSatelliteEphemeris|LOCDeleteSatelliteAlmanac) != 0 {
				return Request{}, fmt.Errorf("encoding QMI LOC assistance deletion: satellite %d info mask 0x%02X is invalid", index, satellite.Info)
			}
			value = binary.LittleEndian.AppendUint16(value, satellite.ID)
			value = binary.LittleEndian.AppendUint32(value, uint32(satellite.System))
			value = append(value, byte(satellite.Info))
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	}
	if r.Config.GNSSData != nil {
		tlvs = append(tlvs, locUint64TLV(0x11, uint64(*r.Config.GNSSData)))
	}
	if r.Config.CellDatabase != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, uint32(*r.Config.CellDatabase)))
	}
	if r.Config.ClockInfo != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, uint32(*r.Config.ClockInfo)))
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCDeleteAssistanceData,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCPredictedOrbitsDataValidity returns the modem's injected-data lifetime.
func (c *Client) LOCPredictedOrbitsDataValidity(ctx context.Context) (LOCPredictedOrbitsValidity, error) {
	req := LOCGetPredictedOrbitsValidityRequest{Timeout: DefaultRequestTimeout}.Request()
	indicationTLVs, err := c.locSingleIndication(ctx, req)
	if err != nil {
		return LOCPredictedOrbitsValidity{}, fmt.Errorf("querying QMI LOC predicted-orbits validity: %w", err)
	}
	var indication LOCGetPredictedOrbitsValidityIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return LOCPredictedOrbitsValidity{}, err
	}
	if err := indication.Result.Err(); err != nil {
		return LOCPredictedOrbitsValidity{}, fmt.Errorf("querying QMI LOC predicted-orbits validity: %w", err)
	}
	if !indication.ValidityKnown {
		return LOCPredictedOrbitsValidity{}, errors.New("querying QMI LOC predicted-orbits validity: validity TLV missing")
	}
	return indication.Validity, nil
}

// LOCInjectUTCTime supplies coarse UTC time to the location engine.
func (c *Client) LOCInjectUTCTime(ctx context.Context, config LOCInjectUTCTimeConfig) error {
	req, err := (LOCInjectUTCTimeRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.locOperation(ctx, req); err != nil {
		return fmt.Errorf("injecting QMI LOC UTC time: %w", err)
	}
	return nil
}

// LOCInjectPosition supplies a coarse external position to the engine.
func (c *Client) LOCInjectPosition(ctx context.Context, config LOCInjectPositionConfig) error {
	req, err := (LOCInjectPositionRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.locOperation(ctx, req); err != nil {
		return fmt.Errorf("injecting QMI LOC position: %w", err)
	}
	return nil
}

// LOCSetEngineLock changes which location sessions the modem accepts.
func (c *Client) LOCSetEngineLock(ctx context.Context, lock LOCLockType) error {
	req, err := (LOCSetEngineLockRequest{Timeout: DefaultRequestTimeout, Lock: lock}).Request()
	if err != nil {
		return err
	}
	if err := c.locOperation(ctx, req); err != nil {
		return fmt.Errorf("setting QMI LOC engine lock: %w", err)
	}
	return nil
}

// LOCEngineLock returns the active location-engine lock policy.
func (c *Client) LOCEngineLock(ctx context.Context) (LOCLockType, error) {
	req := LOCGetEngineLockRequest{Timeout: DefaultRequestTimeout}.Request()
	indicationTLVs, err := c.locSingleIndication(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("querying QMI LOC engine lock: %w", err)
	}
	var indication LOCGetEngineLockIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return 0, err
	}
	if err := indication.Result.Err(); err != nil {
		return 0, fmt.Errorf("querying QMI LOC engine lock: %w", err)
	}
	if !indication.LockKnown {
		return 0, errors.New("querying QMI LOC engine lock: lock TLV missing")
	}
	return indication.Lock, nil
}

// LOCDeleteAssistanceData removes selected cached location assistance data.
func (c *Client) LOCDeleteAssistanceData(ctx context.Context, config LOCDeleteAssistanceDataConfig) error {
	req, err := (LOCDeleteAssistanceDataRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.locOperation(ctx, req); err != nil {
		return fmt.Errorf("deleting QMI LOC assistance data: %w", err)
	}
	return nil
}

func (config *LOCInjectPositionConfig) validate() error {
	if config.HorizontalReliability != nil && *config.HorizontalReliability > LOCReliabilityHigh {
		return fmt.Errorf("horizontal reliability %d is out of range", *config.HorizontalReliability)
	}
	if config.VerticalReliability != nil && *config.VerticalReliability > LOCReliabilityHigh {
		return fmt.Errorf("vertical reliability %d is out of range", *config.VerticalReliability)
	}
	if config.AltitudeSource != nil {
		if config.AltitudeSource.Source > LOCAltitudeOther {
			return fmt.Errorf("altitude source %d is out of range", config.AltitudeSource.Source)
		}
		if config.AltitudeSource.Dependency > LOCAltitudeFullyIndependent {
			return fmt.Errorf("altitude dependency %d is out of range", config.AltitudeSource.Dependency)
		}
		if config.AltitudeSource.Uncertainty > LOCAltitudeUncertaintyFullRegion {
			return fmt.Errorf("altitude uncertainty %d is out of range", config.AltitudeSource.Uncertainty)
		}
	}
	if config.PositionSource != nil && *config.PositionSource > LOCPositionSourceFusedAndroidEngine {
		return fmt.Errorf("position source %d is out of range", *config.PositionSource)
	}
	if config.PositionSourceProvider != nil && *config.PositionSourceProvider > LOCPositionProviderInternal {
		return fmt.Errorf("position source provider %d is out of range", *config.PositionSourceProvider)
	}
	return nil
}

func validateLOCLockType(lock LOCLockType) error {
	if lock < LOCLockNone || lock > LOCLockAll {
		return fmt.Errorf("lock type %d is out of range", lock)
	}
	return nil
}

func locUint64TLV(kind uint8, value uint64) tlv.TLV {
	return tlv.Bytes(kind, binary.LittleEndian.AppendUint64(nil, value))
}

func locInt32TLV(kind uint8, value int32) tlv.TLV {
	return tlv.Bytes(kind, binary.LittleEndian.AppendUint32(nil, uint32(value)))
}

func locFloat32TLV(kind uint8, value float32) tlv.TLV {
	return tlv.Bytes(kind, binary.LittleEndian.AppendUint32(nil, math.Float32bits(value)))
}

func locFloat64TLV(kind uint8, value float64) tlv.TLV {
	return tlv.Bytes(kind, binary.LittleEndian.AppendUint64(nil, math.Float64bits(value)))
}
