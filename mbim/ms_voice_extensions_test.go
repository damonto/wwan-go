package mbim

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

func TestNITZInfoUnmarshalBinary(t *testing.T) {
	lte := mustDecodeHex(t, "ea070000070000001e0000000c0000002200000038000000e00100000000000020000000")
	fiveG := mutateUint32ForSARTest(lte, 32, uint32(DataClass5G))
	reserved := mutateUint32ForSARTest(lte, 32, 1<<8)
	tests := []struct {
		name    string
		version uint16
		data    []byte
		want    NITZInfo
		wantErr bool
	}{
		{
			name: "LTE",
			data: lte,
			want: NITZInfo{
				MBIMExVersion:         mbimExVersion10,
				Year:                  2026,
				Month:                 7,
				Day:                   30,
				Hour:                  12,
				Minute:                34,
				Second:                56,
				TimeZoneOffsetMinutes: 480,
				DataClass:             DataClassLTE,
			},
		},
		{
			name:    "MBIMEx 3 5G",
			version: mbimExVersion30,
			data:    fiveG,
			want: NITZInfo{
				MBIMExVersion:         mbimExVersion30,
				Year:                  2026,
				Month:                 7,
				Day:                   30,
				Hour:                  12,
				Minute:                34,
				Second:                56,
				TimeZoneOffsetMinutes: 480,
				DataClass:             DataClass5G,
			},
		},
		{name: "truncated", data: lte[:35], wantErr: true},
		{name: "trailing data", data: append(bytes.Clone(lte), 0), wantErr: true},
		{name: "reserved data class", data: reserved, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NITZInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNITZRequest(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
	}{
		{name: "default version"},
		{name: "MBIMEx 4", version: mbimExVersion40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := (&NITZRequest{TransactionID: 1, MBIMExVersion: tt.version}).Request()
			command := request.Command.(*Command)
			if command.ServiceID != ServiceMsVoiceExtensions || command.CommandID != CIDMsVoiceExtensionsNITZ || command.CommandType != CommandTypeQuery {
				t.Fatalf("command = service %x CID %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if len(command.Data) != 0 {
				t.Fatalf("command data = %x, want empty", command.Data)
			}
			wantVersion := nitzVersion(tt.version)
			if request.Response.(*NITZInfo).MBIMExVersion != wantVersion {
				t.Fatalf("response version = %#x, want %#x", request.Response.(*NITZInfo).MBIMExVersion, wantVersion)
			}
		})
	}
}

func TestNITZClientAPI(t *testing.T) {
	payload := mustDecodeHex(t, "ea070000070000001e0000000c0000002200000038000000e00100000000000040000000")
	tests := []struct {
		name string
	}{
		{name: "query"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				if err := expectMBIMCommandWithService(serverConn, 1, ServiceMsVoiceExtensions, CIDMsVoiceExtensionsNITZ, CommandTypeQuery, nil); err != nil {
					errCh <- err
					return
				}
				_, err := serverConn.Write(mbimCommandDone(1, ServiceMsVoiceExtensions, CIDMsVoiceExtensionsNITZ, payload))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn, mbimExVersion: mbimExVersion30}
			got, err := client.NITZ(ctx)
			if err != nil {
				t.Fatalf("NITZ() error = %v", err)
			}
			if got.Year != 2026 || got.DataClass != DataClass5G || got.MBIMExVersion != mbimExVersion30 {
				t.Fatalf("NITZ() = %+v", got)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNITZNotificationAPIs(t *testing.T) {
	payload := mustDecodeHex(t, "ea070000070000001e0000000c0000002200000038000000e00100000000000040000000")
	tests := []struct {
		name  string
		watch bool
	}{
		{name: "read"},
		{name: "watch", watch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				_, err := serverConn.Write(mbimIndication(ServiceMsVoiceExtensions, CIDMsVoiceExtensionsNITZ, payload))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn, mbimExVersion: mbimExVersion30}
			var got NITZInfo
			if tt.watch {
				updates, err := client.WatchNITZ(ctx)
				if err != nil {
					t.Fatalf("WatchNITZ() error = %v", err)
				}
				select {
				case got = <-updates:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			} else {
				var err error
				got, err = client.ReadNITZ(ctx)
				if err != nil {
					t.Fatalf("ReadNITZ() error = %v", err)
				}
			}
			if got.Year != 2026 || got.DataClass != DataClass5G || got.MBIMExVersion != mbimExVersion30 {
				t.Fatalf("NITZ notification = %+v", got)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}
