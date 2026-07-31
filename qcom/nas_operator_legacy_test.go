package qcom

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestOperatorNameData(t *testing.T) {
	plmnRecords := binary.LittleEndian.AppendUint16(nil, 1)
	plmnRecords = append(plmnRecords, []byte("46001F")...)
	plmnRecords = binary.LittleEndian.AppendUint16(plmnRecords, 10)
	plmnRecords = binary.LittleEndian.AppendUint16(plmnRecords, 20)
	plmnRecords = append(plmnRecords, 3)

	plmnNames := []byte{1, byte(NASNetworkDescriptionGSM7), byte(NASCountryInitialsAdd), 1, 2, 4}
	plmnNames = append(plmnNames, []byte("Long")...)
	plmnNames = append(plmnNames, 5)
	plmnNames = append(plmnNames, []byte("Short")...)
	nitz := append([]byte(nil), plmnNames[1:]...)

	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{
			name: "all legacy name sources",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, append([]byte{1}, []byte("Carrier")...)),
				tlv.Bytes(0x11, plmnRecords),
				tlv.Bytes(0x12, plmnNames),
				tlv.Bytes(0x13, []byte("Operator")),
				tlv.Bytes(0x14, nitz),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != MessageNASGetOperatorName || len(req.TLVs) != 0 {
						t.Fatalf("request = %+v, want Get Operator Name", req)
					}
				},
				resp: successResponse(MessageNASGetOperatorName, tt.tlvs...),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			got, err := client.OperatorNameData(context.Background())
			if err != nil {
				t.Fatalf("OperatorNameData() error = %v", err)
			}
			if !got.ServiceProviderKnown || string(got.ServiceProvider.Name) != "Carrier" || !got.PLMNRecordsKnown || len(got.PLMNRecords) != 1 {
				t.Fatalf("operator-name data = %+v", got)
			}
			if got.PLMNRecords[0].MCC != "460" || got.PLMNRecords[0].MNC != "01F" || got.PLMNRecords[0].NameRecordID != 3 {
				t.Fatalf("PLMN record = %+v", got.PLMNRecords[0])
			}
			if !got.PLMNNamesKnown || len(got.PLMNNames) != 1 || string(got.PLMNNames[0].Long.Data) != "Long" || string(got.PLMNNames[0].Short.Data) != "Short" {
				t.Fatalf("PLMN names = %+v", got.PLMNNames)
			}
			if !got.OperatorNameKnown || got.OperatorName != "Operator" || !got.NITZKnown || string(got.NITZ.Long.Data) != "Long" {
				t.Fatalf("operator string or NITZ = %+v", got)
			}
		})
	}
}

func TestOperatorNameDataRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{name: "service provider missing header", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}},
		{name: "PLMN record truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 0})}},
		{name: "PLMN name truncated", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0})}},
		{name: "NITZ trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x14, []byte{0, 0, 0, 0, 0, 0, 1})}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASOperatorNameData
			if err := got.UnmarshalTLVs(tt.tlvs); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want error")
			}
		})
	}
}
