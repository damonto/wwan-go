package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestNetworkParametersRequestData(t *testing.T) {
	tlv := TLV{Type: TLVTypeSessionID, Data: binary.LittleEndian.AppendUint32(nil, 3)}
	tlvData, err := (TLVs{tlv}).MarshalBinary()
	if err != nil {
		t.Fatalf("TLVs.MarshalBinary() error = %v", err)
	}

	tests := []struct {
		name        string
		version     uint16
		query       NetworkParametersQuery
		wantData    []byte
		wantVersion uint16
	}{
		{
			name:        "zero version defaults to MBIMEx 3",
			query:       NetworkParametersQuery{ConfigurationsNeeded: true, TLVs: TLVs{tlv}},
			wantData:    append([]byte{1, 0, 0, 0}, tlvData...),
			wantVersion: mbimExVersion30,
		},
		{
			name:        "MBIMEx 3 no groups",
			version:     mbimExVersion30,
			query:       NetworkParametersQuery{TLVs: TLVs{tlv}},
			wantData:    append([]byte{0, 0, 0, 0}, tlvData...),
			wantVersion: mbimExVersion30,
		},
		{
			name:        "MBIMEx 3 configuration and UE policy groups",
			version:     mbimExVersion30,
			query:       NetworkParametersQuery{ConfigurationsNeeded: true, UEPoliciesNeeded: true, TLVs: TLVs{tlv}},
			wantData:    append([]byte{1, 0, 1, 0}, tlvData...),
			wantVersion: mbimExVersion30,
		},
		{
			name:        "MBIMEx 4 omits group fields",
			version:     mbimExVersion40,
			query:       NetworkParametersQuery{ConfigurationsNeeded: true, TLVs: TLVs{tlv}},
			wantData:    tlvData,
			wantVersion: mbimExVersion40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &NetworkParametersRequest{
				TransactionID: 7,
				MBIMExVersion: tt.version,
				Query:         tt.query,
			}
			got := request.Request()
			command := got.Command.(*Command)
			if !bytes.Equal(command.Data, tt.wantData) {
				t.Fatalf("command data = %X, want %X", command.Data, tt.wantData)
			}
			if request.Response.MBIMExVersion != tt.wantVersion {
				t.Fatalf("response version = %#x, want %#x", request.Response.MBIMExVersion, tt.wantVersion)
			}
		})
	}
}

func TestNetworkParametersQueryValidation(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		query   NetworkParametersQuery
		wantErr bool
	}{
		{name: "MBIMEx 2 unsupported", version: mbimExVersion20, wantErr: true},
		{name: "MBIMEx 3 UE policies", version: mbimExVersion30, query: NetworkParametersQuery{UEPoliciesNeeded: true}},
		{name: "MBIMEx 4 configurations", version: mbimExVersion40, query: NetworkParametersQuery{ConfigurationsNeeded: true}},
		{name: "MBIMEx 4 UE policies", version: mbimExVersion40, query: NetworkParametersQuery{UEPoliciesNeeded: true}, wantErr: true},
		{name: "reserved TLV type", version: mbimExVersion40, query: NetworkParametersQuery{TLVs: TLVs{{Type: 0}}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.query.marshalBinary(tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("marshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkParametersInfoMarshalValidation(t *testing.T) {
	allowedTLV, err := NewNSSAIListTLV(TLVTypeAllowedNSSAI, []SNSSAI{{SliceServiceType: 1}})
	if err != nil {
		t.Fatalf("NewNSSAIListTLV() error = %v", err)
	}
	valid := NetworkParametersInfo{
		MBIMExVersion:  mbimExVersion40,
		MICOIndication: MICOIndicationRegistrationAreaAllocated,
		DRXParameters:  DRXCycle64,
	}
	tests := []struct {
		name    string
		info    NetworkParametersInfo
		wantErr bool
	}{
		{name: "valid", info: valid},
		{name: "MBIMEx 2 unsupported", info: func() NetworkParametersInfo {
			value := valid
			value.MBIMExVersion = mbimExVersion20
			return value
		}(), wantErr: true},
		{name: "reserved MICO indication", info: func() NetworkParametersInfo {
			value := valid
			value.MICOIndication = 2
			return value
		}(), wantErr: true},
		{name: "reserved DRX parameters", info: func() NetworkParametersInfo {
			value := valid
			value.DRXParameters = 6
			return value
		}(), wantErr: true},
		{name: "reserved TLV type", info: func() NetworkParametersInfo {
			value := valid
			value.TLVs = TLVs{{Type: 0}}
			return value
		}(), wantErr: true},
		{name: "MBIMEx 3 UE policies", info: func() NetworkParametersInfo {
			value := valid
			value.MBIMExVersion = mbimExVersion30
			value.TLVs = TLVs{{Type: TLVTypeUEPolicies, Data: []byte{1}}}
			return value
		}()},
		{name: "MBIMEx 4 UE policies", info: func() NetworkParametersInfo {
			value := valid
			value.TLVs = TLVs{{Type: TLVTypeUEPolicies, Data: []byte{1}}}
			return value
		}(), wantErr: true},
		{name: "malformed allowed NSSAI", info: func() NetworkParametersInfo {
			value := valid
			value.TLVs = TLVs{{Type: TLVTypeAllowedNSSAI}}
			return value
		}(), wantErr: true},
		{name: "duplicate allowed NSSAI", info: func() NetworkParametersInfo {
			value := valid
			value.TLVs = TLVs{allowedTLV, allowedTLV}
			return value
		}(), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.info.MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkParametersInfoLADNVersion(t *testing.T) {
	ladn := LADN{
		DNN: "ims",
		TAILists: []TAIList{{
			Type: TAIListTypeNonConsecutive,
			PLMN: PLMN{MCC: 0x0310, MNC: 0x0260},
			TACs: []uint32{1},
		}},
	}
	tests := []struct {
		name          string
		encodeVersion uint16
		decodeVersion uint16
		wantErr       bool
	}{
		{name: "MBIMEx 3", encodeVersion: mbimExVersion30, decodeVersion: mbimExVersion30},
		{name: "MBIMEx 4", encodeVersion: mbimExVersion40, decodeVersion: mbimExVersion40},
		{name: "MBIMEx 3 payload as MBIMEx 4", encodeVersion: mbimExVersion30, decodeVersion: mbimExVersion40, wantErr: true},
		{name: "MBIMEx 4 payload as MBIMEx 3", encodeVersion: mbimExVersion40, decodeVersion: mbimExVersion30, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ladnTLV, err := NewLADNTLV([]LADN{ladn}, tt.encodeVersion)
			if err != nil {
				t.Fatalf("NewLADNTLV() error = %v", err)
			}
			data, err := (NetworkParametersInfo{
				MBIMExVersion:  tt.encodeVersion,
				MICOIndication: MICOIndicationNotAvailable,
				DRXParameters:  DRXNotSpecified,
				TLVs:           TLVs{ladnTLV},
			}).MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}

			got := NetworkParametersInfo{MBIMExVersion: tt.decodeVersion}
			err = got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got.LADNs) != 1 || got.LADNs[0].DNN != ladn.DNN || len(got.LADNs[0].TAILists) != 1 {
				t.Fatalf("LADNs = %+v, want %+v", got.LADNs, ladn)
			}
		})
	}
}

func TestClientNetworkParameters(t *testing.T) {
	tlv := TLV{Type: TLVTypeSessionID, Data: binary.LittleEndian.AppendUint32(nil, 5)}
	tlvData, err := (TLVs{tlv}).MarshalBinary()
	if err != nil {
		t.Fatalf("TLVs.MarshalBinary() error = %v", err)
	}
	responseData, err := (NetworkParametersInfo{
		MBIMExVersion:  mbimExVersion40,
		MICOIndication: MICOIndicationNotAvailable,
		DRXParameters:  DRXNotSpecified,
	}).MarshalBinary()
	if err != nil {
		t.Fatalf("NetworkParametersInfo.MarshalBinary() error = %v", err)
	}

	tests := []struct {
		name     string
		version  uint16
		query    NetworkParametersQuery
		wantData []byte
	}{
		{
			name:     "MBIMEx 3",
			version:  mbimExVersion30,
			query:    NetworkParametersQuery{ConfigurationsNeeded: true, UEPoliciesNeeded: true, TLVs: TLVs{tlv}},
			wantData: append([]byte{1, 0, 1, 0}, tlvData...),
		},
		{
			name:     "MBIMEx 4",
			version:  mbimExVersion40,
			query:    NetworkParametersQuery{ConfigurationsNeeded: true, TLVs: TLVs{tlv}},
			wantData: tlvData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errC := make(chan error, 1)
			go func() {
				defer close(errC)
				defer serverConn.Close()
				if err := expectMBIMCommandWithService(serverConn, 1, ServiceMsBasicConnectExtensions, CIDMsNetworkParameters, CommandTypeQuery, tt.wantData); err != nil {
					errC <- err
					return
				}
				_, err := serverConn.Write(mbimCommandDone(1, ServiceMsBasicConnectExtensions, CIDMsNetworkParameters, responseData))
				errC <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn, mbimExVersion: tt.version}
			got, err := client.NetworkParameters(ctx, tt.query)
			if err != nil {
				t.Fatalf("NetworkParameters() error = %v", err)
			}
			if got.MBIMExVersion != tt.version || got.MICOIndication != MICOIndicationNotAvailable || got.DRXParameters != DRXNotSpecified {
				t.Fatalf("NetworkParameters() = %+v", got)
			}
			if err := <-errC; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientNetworkParametersValidation(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		query   NetworkParametersQuery
	}{
		{name: "MBIMEx 1", version: mbimExVersion10},
		{name: "MBIMEx 2", version: mbimExVersion20},
		{name: "MBIMEx 4 UE policies", version: mbimExVersion40, query: NetworkParametersQuery{UEPoliciesNeeded: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{mbimExVersion: tt.version}
			if _, err := client.NetworkParameters(context.Background(), tt.query); err == nil {
				t.Fatal("NetworkParameters() error = nil, want non-nil")
			}
		})
	}
}
