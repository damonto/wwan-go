package mbim

// These wire types are shared by multiple cellular network CIDs.

type CellularClass uint32

const (
	CellularClassNone CellularClass = 0
	CellularClassGSM  CellularClass = 1 << 0
	CellularClassCDMA CellularClass = 1 << 1
)

type DataClass uint32

const (
	DataClassNone       DataClass = 0
	DataClassGPRS       DataClass = 1 << 0
	DataClassEDGE       DataClass = 1 << 1
	DataClassUMTS       DataClass = 1 << 2
	DataClassHSDPA      DataClass = 1 << 3
	DataClassHSUPA      DataClass = 1 << 4
	DataClassLTE        DataClass = 1 << 5
	DataClass5GNSA      DataClass = 1 << 6
	DataClass5GSA       DataClass = 1 << 7
	DataClass1XRTT      DataClass = 1 << 16
	DataClass1XEVDO     DataClass = 1 << 17
	DataClass1XEVDORevA DataClass = 1 << 18
	DataClass1XEVDV     DataClass = 1 << 19
	DataClass3XRTT      DataClass = 1 << 20
	DataClass1XEVDORevB DataClass = 1 << 21
	DataClassUMB        DataClass = 1 << 22
	DataClassCustom     DataClass = 1 << 31
)

// MBIMEx 3.0 renamed the 5G capability bit and moved NSA/SA details into
// DataSubclass. The aliases preserve the MBIMEx 2.0 wire values.
const (
	DataClass5G     DataClass = 1 << 6
	DataClassUnused DataClass = 1 << 7
)

type DataSubclass uint64

const (
	DataSubclassNone     DataSubclass = 0
	DataSubclass5GENDC   DataSubclass = 1 << 0
	DataSubclass5GNR     DataSubclass = 1 << 1
	DataSubclass5GNEDC   DataSubclass = 1 << 2
	DataSubclass5GELTE   DataSubclass = 1 << 3
	DataSubclass5GNGENDC DataSubclass = 1 << 4
)

type FrequencyRange uint32

const (
	FrequencyRangeUnknown FrequencyRange = 0
	FrequencyRange1       FrequencyRange = 1 << 0
	FrequencyRange2       FrequencyRange = 1 << 1
)

type PLMN struct {
	MCC uint16
	MNC uint16
}

type TrackingAreaIdentity struct {
	PLMN PLMN
	TAC  uint32
}
