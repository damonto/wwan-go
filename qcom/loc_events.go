package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const locGNSSSatelliteWireSize = 28

// LOCSessionStatus describes the state of a positioning session.
type LOCSessionStatus uint32

const (
	LOCSessionSuccess LOCSessionStatus = iota
	LOCSessionInProgress
	LOCSessionGeneralFailure
	LOCSessionTimeout
	LOCSessionUserEnded
	LOCSessionBadParameter
	LOCSessionPhoneOffline
	LOCSessionEngineLocked
)

// LOCTechnologyUsed is a mask of technologies used to calculate a position.
type LOCTechnologyUsed uint32

const (
	LOCTechnologySatellite LOCTechnologyUsed = 1 << iota
	LOCTechnologyCellular
	LOCTechnologyWiFi
	LOCTechnologySensors
	LOCTechnologyReferenceLocation
	LOCTechnologyInjectedPosition
	LOCTechnologyAFLT
	LOCTechnologyHybrid
)

// LOCReliability describes the modem's confidence in a position component.
type LOCReliability uint32

const (
	LOCReliabilityNotSet LOCReliability = iota
	LOCReliabilityVeryLow
	LOCReliabilityLow
	LOCReliabilityMedium
	LOCReliabilityHigh
)

// LOCTimeSource identifies how the engine obtained its current time.
type LOCTimeSource uint32

const (
	LOCTimeSourceInvalid LOCTimeSource = iota
	LOCTimeSourceNetworkTransfer
	LOCTimeSourceNetworkTagging
	LOCTimeSourceExternalInput
	LOCTimeSourceTOWDecode
	LOCTimeSourceTOWConfirmed
	LOCTimeSourceTOWAndWeekConfirmed
	LOCTimeSourceNavigationSolution
	LOCTimeSourceSolveForTime
	LOCTimeSourceGLONASSTOWDecode
	LOCTimeSourceTimeTransform
	LOCTimeSourceWCDMASleepTag
	LOCTimeSourceGSMSleepTag
	LOCTimeSourceUnknown
	LOCTimeSourceSystemTick
	LOCTimeSourceQZSSTOWDecode
	LOCTimeSourceBeiDouTOWDecode
)

// LOCSensorDataUsage is a mask describing sensor-assisted fix components.
type LOCSensorDataUsage uint64

const (
	LOCSensorAccelerometerUsed LOCSensorDataUsage = 1 << iota
	LOCSensorGyroscopeUsed
)

const (
	LOCSensorAidedHeading  LOCSensorDataUsage = 1 << 32
	LOCSensorAidedSpeed    LOCSensorDataUsage = 1 << 33
	LOCSensorAidedPosition LOCSensorDataUsage = 1 << 34
	LOCSensorAidedVelocity LOCSensorDataUsage = 1 << 35
)

// LOCDilutionOfPrecision contains position, horizontal, and vertical DOP.
type LOCDilutionOfPrecision struct {
	Position   float32
	Horizontal float32
	Vertical   float32
}

// LOCGPSDateTime is GPS week time carried by LOC messages.
type LOCGPSDateTime struct {
	Weeks                  uint16
	TimeOfWeekMilliseconds uint32
}

// LOCPositionReport contains one standard QMI LOC Position Report indication.
// Optional TLVs use a matching Known field so a legitimate zero remains
// distinguishable from an omitted value.
type LOCPositionReport struct {
	Status LOCSessionStatus

	SessionID      uint8
	SessionIDKnown bool

	Latitude       float64
	LatitudeKnown  bool
	Longitude      float64
	LongitudeKnown bool

	HorizontalUncertaintyCircular               float32
	HorizontalUncertaintyCircularKnown          bool
	HorizontalUncertaintyEllipticalMinor        float32
	HorizontalUncertaintyEllipticalMinorKnown   bool
	HorizontalUncertaintyEllipticalMajor        float32
	HorizontalUncertaintyEllipticalMajorKnown   bool
	HorizontalUncertaintyEllipticalAzimuth      float32
	HorizontalUncertaintyEllipticalAzimuthKnown bool
	HorizontalConfidence                        uint8
	HorizontalConfidenceKnown                   bool
	HorizontalReliability                       LOCReliability
	HorizontalReliabilityKnown                  bool
	HorizontalSpeed                             float32
	HorizontalSpeedKnown                        bool
	SpeedUncertainty                            float32
	SpeedUncertaintyKnown                       bool
	AltitudeFromEllipsoid                       float32
	AltitudeFromEllipsoidKnown                  bool
	AltitudeFromSeaLevel                        float32
	AltitudeFromSeaLevelKnown                   bool
	VerticalUncertainty                         float32
	VerticalUncertaintyKnown                    bool
	VerticalConfidence                          uint8
	VerticalConfidenceKnown                     bool
	VerticalReliability                         LOCReliability
	VerticalReliabilityKnown                    bool
	VerticalSpeed                               float32
	VerticalSpeedKnown                          bool
	Heading                                     float32
	HeadingKnown                                bool
	HeadingUncertainty                          float32
	HeadingUncertaintyKnown                     bool
	MagneticDeviation                           float32
	MagneticDeviationKnown                      bool
	Technology                                  LOCTechnologyUsed
	TechnologyKnown                             bool
	DOP                                         LOCDilutionOfPrecision
	DOPKnown                                    bool
	UTCTimestampMilliseconds                    uint64
	UTCTimestampKnown                           bool
	LeapSeconds                                 uint8
	LeapSecondsKnown                            bool
	GPSDateTime                                 LOCGPSDateTime
	GPSDateTimeKnown                            bool
	TimeUncertaintyMilliseconds                 float32
	TimeUncertaintyKnown                        bool
	TimeSource                                  LOCTimeSource
	TimeSourceKnown                             bool
	SensorDataUsage                             LOCSensorDataUsage
	SensorDataUsageKnown                        bool
	SessionFixCount                             uint32
	SessionFixCountKnown                        bool
	SatellitesUsed                              []uint16
	SatellitesUsedKnown                         bool
	AltitudeAssumed                             bool
	AltitudeAssumedKnown                        bool
}

// UnmarshalTLVs parses a QMI LOC Position Report indication.
func (r *LOCPositionReport) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = LOCPositionReport{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI LOC position report: session status TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI LOC position report: session status TLV length %d, want 4", len(value))
	}
	r.Status = LOCSessionStatus(binary.LittleEndian.Uint32(value))

	if err := parseLOCOptionalUint8(tlvs, 0x02, &r.SessionID, &r.SessionIDKnown); err != nil {
		return err
	}
	if err := parseLOCOptionalFloat64(tlvs, 0x10, &r.Latitude, &r.LatitudeKnown); err != nil {
		return err
	}
	if err := parseLOCOptionalFloat64(tlvs, 0x11, &r.Longitude, &r.LongitudeKnown); err != nil {
		return err
	}

	floatFields := []struct {
		kind  uint8
		value *float32
		known *bool
	}{
		{0x12, &r.HorizontalUncertaintyCircular, &r.HorizontalUncertaintyCircularKnown},
		{0x13, &r.HorizontalUncertaintyEllipticalMinor, &r.HorizontalUncertaintyEllipticalMinorKnown},
		{0x14, &r.HorizontalUncertaintyEllipticalMajor, &r.HorizontalUncertaintyEllipticalMajorKnown},
		{0x15, &r.HorizontalUncertaintyEllipticalAzimuth, &r.HorizontalUncertaintyEllipticalAzimuthKnown},
		{0x18, &r.HorizontalSpeed, &r.HorizontalSpeedKnown},
		{0x19, &r.SpeedUncertainty, &r.SpeedUncertaintyKnown},
		{0x1A, &r.AltitudeFromEllipsoid, &r.AltitudeFromEllipsoidKnown},
		{0x1B, &r.AltitudeFromSeaLevel, &r.AltitudeFromSeaLevelKnown},
		{0x1C, &r.VerticalUncertainty, &r.VerticalUncertaintyKnown},
		{0x1F, &r.VerticalSpeed, &r.VerticalSpeedKnown},
		{0x20, &r.Heading, &r.HeadingKnown},
		{0x21, &r.HeadingUncertainty, &r.HeadingUncertaintyKnown},
		{0x22, &r.MagneticDeviation, &r.MagneticDeviationKnown},
		{0x28, &r.TimeUncertaintyMilliseconds, &r.TimeUncertaintyKnown},
	}
	for _, field := range floatFields {
		if err := parseLOCOptionalFloat32(tlvs, field.kind, field.value, field.known); err != nil {
			return err
		}
	}
	if err := parseLOCOptionalUint8(tlvs, 0x16, &r.HorizontalConfidence, &r.HorizontalConfidenceKnown); err != nil {
		return err
	}
	if err := parseLOCOptionalUint8(tlvs, 0x1D, &r.VerticalConfidence, &r.VerticalConfidenceKnown); err != nil {
		return err
	}
	if err := parseLOCOptionalUint8(tlvs, 0x26, &r.LeapSeconds, &r.LeapSecondsKnown); err != nil {
		return err
	}

	var horizontalReliability uint32
	if err := parseLOCOptionalUint32(tlvs, 0x17, &horizontalReliability, &r.HorizontalReliabilityKnown); err != nil {
		return err
	}
	r.HorizontalReliability = LOCReliability(horizontalReliability)
	var verticalReliability uint32
	if err := parseLOCOptionalUint32(tlvs, 0x1E, &verticalReliability, &r.VerticalReliabilityKnown); err != nil {
		return err
	}
	r.VerticalReliability = LOCReliability(verticalReliability)
	var technology uint32
	if err := parseLOCOptionalUint32(tlvs, 0x23, &technology, &r.TechnologyKnown); err != nil {
		return err
	}
	r.Technology = LOCTechnologyUsed(technology)
	var timeSource uint32
	if err := parseLOCOptionalUint32(tlvs, 0x29, &timeSource, &r.TimeSourceKnown); err != nil {
		return err
	}
	r.TimeSource = LOCTimeSource(timeSource)
	if err := parseLOCOptionalUint32(tlvs, 0x2B, &r.SessionFixCount, &r.SessionFixCountKnown); err != nil {
		return err
	}
	if err := parseLOCOptionalUint64(tlvs, 0x25, &r.UTCTimestampMilliseconds, &r.UTCTimestampKnown); err != nil {
		return err
	}
	var sensorUsage uint64
	if err := parseLOCOptionalUint64(tlvs, 0x2A, &sensorUsage, &r.SensorDataUsageKnown); err != nil {
		return err
	}
	r.SensorDataUsage = LOCSensorDataUsage(sensorUsage)
	if err := parseLOCOptionalBool(tlvs, 0x2D, &r.AltitudeAssumed, &r.AltitudeAssumedKnown); err != nil {
		return err
	}

	if value, ok := tlv.Value(tlvs, 0x24); ok {
		if len(value) != 12 {
			return fmt.Errorf("parsing QMI LOC position report: DOP TLV length %d, want 12", len(value))
		}
		r.DOP = LOCDilutionOfPrecision{
			Position:   math.Float32frombits(binary.LittleEndian.Uint32(value[0:4])),
			Horizontal: math.Float32frombits(binary.LittleEndian.Uint32(value[4:8])),
			Vertical:   math.Float32frombits(binary.LittleEndian.Uint32(value[8:12])),
		}
		r.DOPKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x27); ok {
		if len(value) != 6 {
			return fmt.Errorf("parsing QMI LOC position report: GPS date/time TLV length %d, want 6", len(value))
		}
		r.GPSDateTime = LOCGPSDateTime{
			Weeks:                  binary.LittleEndian.Uint16(value[0:2]),
			TimeOfWeekMilliseconds: binary.LittleEndian.Uint32(value[2:6]),
		}
		r.GPSDateTimeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x2C); ok {
		satellites, err := decodeLOCUint16Array(value, 1)
		if err != nil {
			return err
		}
		r.SatellitesUsed = satellites
		r.SatellitesUsedKnown = true
	}
	return nil
}

// LOCSatelliteValidInformation is a mask selecting valid satellite fields.
type LOCSatelliteValidInformation uint32

const (
	LOCSatelliteValidSystem LOCSatelliteValidInformation = 1 << iota
	LOCSatelliteValidID
	LOCSatelliteValidHealth
	LOCSatelliteValidStatus
	LOCSatelliteValidNavigationData
	LOCSatelliteValidElevation
	LOCSatelliteValidAzimuth
	LOCSatelliteValidSignalToNoiseRatio
)

// LOCSystem identifies a GNSS constellation.
type LOCSystem uint32

const (
	LOCSystemGPS LOCSystem = iota + 1
	LOCSystemGalileo
	LOCSystemSBAS
	LOCSystemBeiDou
	LOCSystemGLONASS
)

// LOCSatelliteHealth reports whether a satellite is healthy.
type LOCSatelliteHealth uint8

const (
	LOCSatelliteUnhealthy LOCSatelliteHealth = iota
	LOCSatelliteHealthy
)

// LOCSatelliteStatus reports the engine's tracking state for one satellite.
type LOCSatelliteStatus uint32

const (
	LOCSatelliteIdle LOCSatelliteStatus = iota
	LOCSatelliteSearching
	LOCSatelliteTracking
)

// LOCNavigationData reports which assistance data is available for a satellite.
type LOCNavigationData uint8

const (
	LOCNavigationHasEphemeris LOCNavigationData = iota
	LOCNavigationHasAlmanac
)

// LOCGNSSSatellite contains one satellite entry in a GNSS SV Info indication.
type LOCGNSSSatellite struct {
	ValidInformation   LOCSatelliteValidInformation
	System             LOCSystem
	ID                 uint16
	Health             LOCSatelliteHealth
	Status             LOCSatelliteStatus
	NavigationData     LOCNavigationData
	ElevationDegrees   float32
	AzimuthDegrees     float32
	SignalToNoiseRatio float32
}

// LOCGNSSSatelliteInfo contains the modem's current GNSS satellite view.
type LOCGNSSSatelliteInfo struct {
	AltitudeAssumed      bool
	AltitudeAssumedKnown bool
	Satellites           []LOCGNSSSatellite
	SatellitesKnown      bool
}

// UnmarshalTLVs parses a QMI LOC GNSS SV Info indication.
func (i *LOCGNSSSatelliteInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCGNSSSatelliteInfo{}
	if err := parseLOCOptionalBool(tlvs, 0x01, &i.AltitudeAssumed, &i.AltitudeAssumedKnown); err != nil {
		return err
	}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) < 1 {
		return errors.New("parsing QMI LOC GNSS satellite list: count is missing")
	}
	count := int(value[0])
	if len(value) != 1+count*locGNSSSatelliteWireSize {
		return fmt.Errorf("parsing QMI LOC GNSS satellite list: length %d does not match count %d", len(value), count)
	}
	i.Satellites = make([]LOCGNSSSatellite, count)
	offset := 1
	for index := range count {
		entry := value[offset : offset+locGNSSSatelliteWireSize]
		i.Satellites[index] = LOCGNSSSatellite{
			ValidInformation:   LOCSatelliteValidInformation(binary.LittleEndian.Uint32(entry[0:4])),
			System:             LOCSystem(binary.LittleEndian.Uint32(entry[4:8])),
			ID:                 binary.LittleEndian.Uint16(entry[8:10]),
			Health:             LOCSatelliteHealth(entry[10]),
			Status:             LOCSatelliteStatus(binary.LittleEndian.Uint32(entry[11:15])),
			NavigationData:     LOCNavigationData(entry[15]),
			ElevationDegrees:   math.Float32frombits(binary.LittleEndian.Uint32(entry[16:20])),
			AzimuthDegrees:     math.Float32frombits(binary.LittleEndian.Uint32(entry[20:24])),
			SignalToNoiseRatio: math.Float32frombits(binary.LittleEndian.Uint32(entry[24:28])),
		}
		offset += locGNSSSatelliteWireSize
	}
	i.SatellitesKnown = true
	return nil
}

// LOCInjectTimeRequest contains time servers requested by the location engine.
type LOCInjectTimeRequest struct {
	DelayThresholdMilliseconds uint32
	Servers                    []string
}

// UnmarshalTLVs parses a QMI LOC Inject Time Request indication.
func (r *LOCInjectTimeRequest) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = LOCInjectTimeRequest{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return errors.New("parsing QMI LOC inject-time request: time server info TLV missing")
	}
	if len(value) < 5 {
		return fmt.Errorf("parsing QMI LOC inject-time request: time server info TLV length %d, want at least 5", len(value))
	}
	servers, err := decodeLOCStringList(value[4:])
	if err != nil {
		return fmt.Errorf("parsing QMI LOC inject-time request servers: %w", err)
	}
	r.DelayThresholdMilliseconds = binary.LittleEndian.Uint32(value[0:4])
	r.Servers = servers
	return nil
}

// LOCPredictedOrbitsServerUpdate selects changed predicted-orbit parameters.
type LOCPredictedOrbitsServerUpdate uint64

const (
	LOCPredictedOrbitsInjectionRequested LOCPredictedOrbitsServerUpdate = 1 << iota
	LOCPredictedOrbitsServersUpdated
	LOCPredictedOrbitsRefreshRateUpdated
)

// LOCPredictedOrbitsUpdateType identifies which periodic rate is changing.
type LOCPredictedOrbitsUpdateType uint32

const (
	LOCPredictedOrbitsRateUpdate LOCPredictedOrbitsUpdateType = iota + 1
	LOCPredictedOrbitsIntegrityRateUpdate
)

// LOCPredictedOrbitsFileType identifies a requested assistance file.
type LOCPredictedOrbitsFileType uint32

const (
	LOCPredictedOrbitsFileXTRA LOCPredictedOrbitsFileType = iota + 1
	LOCPredictedOrbitsFileNavIC
)

// LOCPredictedOrbitsFileInfo describes one requested assistance file.
type LOCPredictedOrbitsFileInfo struct {
	Type                    LOCPredictedOrbitsFileType
	DownloadIntervalMinutes uint32
}

// LOCInjectPredictedOrbitsRequest describes assistance data requested by LOC.
type LOCInjectPredictedOrbitsRequest struct {
	MaxFileSize       uint32
	MaxPartSize       uint32
	AllowedSizesKnown bool
	Servers           []string
	ServersKnown      bool
	ServerUpdate      LOCPredictedOrbitsServerUpdate
	ServerUpdateKnown bool
	UpdateType        LOCPredictedOrbitsUpdateType
	UpdateTypeKnown   bool
	UpdatePeriod      uint32
	UpdatePeriodKnown bool
	File              LOCPredictedOrbitsFileInfo
	FileKnown         bool
}

// UnmarshalTLVs parses a QMI LOC Inject Predicted Orbits Request indication.
func (r *LOCInjectPredictedOrbitsRequest) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = LOCInjectPredictedOrbitsRequest{}
	if value, ok := tlv.Value(tlvs, 0x01); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI LOC predicted-orbits request: allowed sizes TLV length %d, want 8", len(value))
		}
		r.MaxFileSize = binary.LittleEndian.Uint32(value[0:4])
		r.MaxPartSize = binary.LittleEndian.Uint32(value[4:8])
		r.AllowedSizesKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		servers, err := decodeLOCStringList(value)
		if err != nil {
			return fmt.Errorf("parsing QMI LOC predicted-orbits request servers: %w", err)
		}
		r.Servers = servers
		r.ServersKnown = true
	}
	var serverUpdate uint64
	if err := parseLOCOptionalUint64(tlvs, 0x11, &serverUpdate, &r.ServerUpdateKnown); err != nil {
		return err
	}
	r.ServerUpdate = LOCPredictedOrbitsServerUpdate(serverUpdate)
	var updateType uint32
	if err := parseLOCOptionalUint32(tlvs, 0x12, &updateType, &r.UpdateTypeKnown); err != nil {
		return err
	}
	r.UpdateType = LOCPredictedOrbitsUpdateType(updateType)
	if err := parseLOCOptionalUint32(tlvs, 0x13, &r.UpdatePeriod, &r.UpdatePeriodKnown); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI LOC predicted-orbits request: file info TLV length %d, want 8", len(value))
		}
		r.File = LOCPredictedOrbitsFileInfo{
			Type:                    LOCPredictedOrbitsFileType(binary.LittleEndian.Uint32(value[0:4])),
			DownloadIntervalMinutes: binary.LittleEndian.Uint32(value[4:8]),
		}
		r.FileKnown = true
	}
	return nil
}

// LOCPositionInjectionRequest contains a coarse position requested by LOC.
type LOCPositionInjectionRequest struct {
	Latitude                    float64
	LatitudeKnown               bool
	Longitude                   float64
	LongitudeKnown              bool
	HorizontalUncertaintyMeters float32
	HorizontalUncertaintyKnown  bool
	UTCTimestampMilliseconds    uint64
	UTCTimestampKnown           bool
}

// UnmarshalTLVs parses a QMI LOC Inject Position Request indication.
func (r *LOCPositionInjectionRequest) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = LOCPositionInjectionRequest{}
	if err := parseLOCOptionalFloat64(tlvs, 0x01, &r.Latitude, &r.LatitudeKnown); err != nil {
		return err
	}
	if err := parseLOCOptionalFloat64(tlvs, 0x02, &r.Longitude, &r.LongitudeKnown); err != nil {
		return err
	}
	if err := parseLOCOptionalFloat32(tlvs, 0x03, &r.HorizontalUncertaintyMeters, &r.HorizontalUncertaintyKnown); err != nil {
		return err
	}
	return parseLOCOptionalUint64(tlvs, 0x04, &r.UTCTimestampMilliseconds, &r.UTCTimestampKnown)
}

// LOCEngineState identifies whether the location engine is running.
type LOCEngineState int32

const (
	LOCEngineOn LOCEngineState = iota + 1
	LOCEngineOff
)

// LOCEngineStateIndication contains one engine-state change.
type LOCEngineStateIndication struct {
	State LOCEngineState
}

// UnmarshalTLVs parses a QMI LOC Engine State indication.
func (i *LOCEngineStateIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCEngineStateIndication{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI LOC engine state: state TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI LOC engine state: state TLV length %d, want 4", len(value))
	}
	i.State = LOCEngineState(int32(binary.LittleEndian.Uint32(value)))
	return nil
}

// LOCFixRecurrenceIndication contains the active fix recurrence policy.
type LOCFixRecurrenceIndication struct {
	Recurrence LOCFixRecurrence
}

// UnmarshalTLVs parses a QMI LOC Fix Recurrence Type indication.
func (i *LOCFixRecurrenceIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCFixRecurrenceIndication{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return errors.New("parsing QMI LOC fix recurrence: recurrence TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI LOC fix recurrence: recurrence TLV length %d, want 4", len(value))
	}
	i.Recurrence = LOCFixRecurrence(binary.LittleEndian.Uint32(value))
	return nil
}

// LOCWatchPositionReports subscribes to standard position reports.
func (c *Client) LOCWatchPositionReports(ctx context.Context) (<-chan LOCPositionReport, error) {
	raw, err := c.watchLOCTLVs(ctx, MessageLOCPositionReport, LOCEventPositionReport)
	if err != nil {
		return nil, fmt.Errorf("watching QMI LOC position reports: %w", err)
	}
	out := make(chan LOCPositionReport, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var report LOCPositionReport
			if err := report.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- report:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// LOCWatchGNSSSatelliteInfo subscribes to GNSS satellite-view changes.
func (c *Client) LOCWatchGNSSSatelliteInfo(ctx context.Context) (<-chan LOCGNSSSatelliteInfo, error) {
	raw, err := c.watchLOCTLVs(ctx, MessageLOCGNSSSatelliteInfo, LOCEventGNSSSatelliteInfo)
	if err != nil {
		return nil, fmt.Errorf("watching QMI LOC GNSS satellite information: %w", err)
	}
	out := make(chan LOCGNSSSatelliteInfo, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var info LOCGNSSSatelliteInfo
			if err := info.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- info:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// LOCWatchInjectTimeRequests subscribes to time-injection requests.
func (c *Client) LOCWatchInjectTimeRequests(ctx context.Context) (<-chan LOCInjectTimeRequest, error) {
	raw, err := c.watchLOCTLVs(ctx, MessageLOCInjectTimeRequest, LOCEventInjectTimeRequest)
	if err != nil {
		return nil, fmt.Errorf("watching QMI LOC inject-time requests: %w", err)
	}
	out := make(chan LOCInjectTimeRequest, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var request LOCInjectTimeRequest
			if err := request.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- request:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// LOCWatchInjectPredictedOrbitsRequests subscribes to assistance-data requests.
func (c *Client) LOCWatchInjectPredictedOrbitsRequests(ctx context.Context) (<-chan LOCInjectPredictedOrbitsRequest, error) {
	raw, err := c.watchLOCTLVs(ctx, MessageLOCInjectPredictedOrbitsRequest, LOCEventInjectPredictedOrbitsRequest)
	if err != nil {
		return nil, fmt.Errorf("watching QMI LOC predicted-orbits requests: %w", err)
	}
	out := make(chan LOCInjectPredictedOrbitsRequest, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var request LOCInjectPredictedOrbitsRequest
			if err := request.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- request:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// LOCWatchInjectPositionRequests subscribes to coarse-position requests.
func (c *Client) LOCWatchInjectPositionRequests(ctx context.Context) (<-chan LOCPositionInjectionRequest, error) {
	raw, err := c.watchLOCTLVs(ctx, MessageLOCInjectPositionRequest, LOCEventInjectPositionRequest)
	if err != nil {
		return nil, fmt.Errorf("watching QMI LOC inject-position requests: %w", err)
	}
	out := make(chan LOCPositionInjectionRequest, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var request LOCPositionInjectionRequest
			if err := request.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- request:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// LOCWatchEngineState subscribes to location-engine state changes.
func (c *Client) LOCWatchEngineState(ctx context.Context) (<-chan LOCEngineState, error) {
	raw, err := c.watchLOCTLVs(ctx, MessageLOCEngineState, LOCEventEngineState)
	if err != nil {
		return nil, fmt.Errorf("watching QMI LOC engine state: %w", err)
	}
	out := make(chan LOCEngineState, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var indication LOCEngineStateIndication
			if err := indication.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- indication.State:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// LOCWatchFixRecurrence subscribes to fix-session recurrence changes.
func (c *Client) LOCWatchFixRecurrence(ctx context.Context) (<-chan LOCFixRecurrence, error) {
	raw, err := c.watchLOCTLVs(ctx, MessageLOCFixRecurrence, LOCEventFixSessionState)
	if err != nil {
		return nil, fmt.Errorf("watching QMI LOC fix recurrence: %w", err)
	}
	out := make(chan LOCFixRecurrence, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var indication LOCFixRecurrenceIndication
			if err := indication.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- indication.Recurrence:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) watchLOCTLVs(ctx context.Context, id MessageID, mask LOCEventRegistration) (<-chan tlv.TLVs, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceLOC)
	if err != nil {
		return nil, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceLOC, clientID, id)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := c.acquireLOCEvents(ctx, mask); err != nil {
		cancel()
		return nil, err
	}
	out := make(chan tlv.TLVs, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseLOCEvents(mask)
		for indication := range indications {
			select {
			case out <- indication.TLVs:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func parseLOCOptionalUint8(tlvs tlv.TLVs, kind uint8, target *uint8, known *bool) error {
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI LOC TLV 0x%02X: length %d, want 1", kind, len(value))
	}
	*target = value[0]
	*known = true
	return nil
}

func parseLOCOptionalBool(tlvs tlv.TLVs, kind uint8, target *bool, known *bool) error {
	var value uint8
	if err := parseLOCOptionalUint8(tlvs, kind, &value, known); err != nil {
		return err
	}
	if *known {
		*target = value != 0
	}
	return nil
}

func parseLOCOptionalUint32(tlvs tlv.TLVs, kind uint8, target *uint32, known *bool) error {
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI LOC TLV 0x%02X: length %d, want 4", kind, len(value))
	}
	*target = binary.LittleEndian.Uint32(value)
	*known = true
	return nil
}

func parseLOCOptionalUint64(tlvs tlv.TLVs, kind uint8, target *uint64, known *bool) error {
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	if len(value) != 8 {
		return fmt.Errorf("parsing QMI LOC TLV 0x%02X: length %d, want 8", kind, len(value))
	}
	*target = binary.LittleEndian.Uint64(value)
	*known = true
	return nil
}

func parseLOCOptionalFloat32(tlvs tlv.TLVs, kind uint8, target *float32, known *bool) error {
	var bits uint32
	if err := parseLOCOptionalUint32(tlvs, kind, &bits, known); err != nil {
		return err
	}
	if *known {
		*target = math.Float32frombits(bits)
	}
	return nil
}

func parseLOCOptionalFloat64(tlvs tlv.TLVs, kind uint8, target *float64, known *bool) error {
	var bits uint64
	if err := parseLOCOptionalUint64(tlvs, kind, &bits, known); err != nil {
		return err
	}
	if *known {
		*target = math.Float64frombits(bits)
	}
	return nil
}

func decodeLOCUint16Array(value []byte, prefixSize int) ([]uint16, error) {
	if len(value) < prefixSize {
		return nil, errors.New("parsing QMI LOC uint16 array: count is truncated")
	}
	var count int
	switch prefixSize {
	case 1:
		count = int(value[0])
	case 4:
		count = int(binary.LittleEndian.Uint32(value[0:4]))
	default:
		return nil, fmt.Errorf("parsing QMI LOC uint16 array: unsupported count size %d", prefixSize)
	}
	if count > (len(value)-prefixSize)/2 || len(value) != prefixSize+count*2 {
		return nil, fmt.Errorf("parsing QMI LOC uint16 array: length %d does not match count %d", len(value), count)
	}
	items := make([]uint16, count)
	for index := range count {
		offset := prefixSize + index*2
		items[index] = binary.LittleEndian.Uint16(value[offset : offset+2])
	}
	return items, nil
}
