package mbim

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestProxyVersionNotificationAPIs(t *testing.T) {
	payload := mustDecodeHex(t, "00010004")
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
				_, err := serverConn.Write(mbimIndication(ServiceMbimProxyControl, CIDProxyControlVersion, payload))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn}
			var got VersionInfo
			if tt.watch {
				updates, err := client.WatchProxyVersion(ctx)
				if err != nil {
					t.Fatalf("WatchProxyVersion() error = %v", err)
				}
				select {
				case got = <-updates:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			} else {
				var err error
				got, err = client.ReadProxyVersion(ctx)
				if err != nil {
					t.Fatalf("ReadProxyVersion() error = %v", err)
				}
			}
			if got.MBIMVersion != mbimVersion10 || got.MBIMExVersion != mbimExVersion40 {
				t.Fatalf("proxy version = %+v", got)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}
