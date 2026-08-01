# wwan-go

`wwan-go` is a Go protocol library for controlling cellular modems over QMI,
QRTR, MBIM, AT, and card-facing UICC access paths.

The repository separates modem protocols from SIM card business logic:

- Top-level protocol packages implement concrete primitives such as AT `+CSIM`, PC/SC CCID APDU transport, MBIM UICC low-level access, QMI UIM, and QRTR QMI transport.
- `apdu` implements APDU request/response encoding.
- `sim` adapts readers into higher-level SIM, USIM, and ISIM operations such as loading ICCID/IMSI identities, AKA, EAP-AKA, SMS-PP download, and SIM Toolkit.

The protocol clients do not depend on `sim`. Use `sim` only when you want card-level business behavior on top of a reader.

## Status

This is an early protocol extraction layer. The current focus is correctness, explicit transport selection, and small idiomatic APIs.

Implemented reader families:

- AT APDU transport over serial ports with `AT+CSIM`
- CCID APDU transport over PC/SC
- MBIM direct and proxy transports with UICC low-level access
- QMI direct and proxy transports
- QRTR direct Linux socket transport
- QCOM UIM primitives over QMI or QRTR
- SIM Toolkit over APDU, QMI CAT raw indications, and MBIM STK PAC

The implementation is pure Go. It does not use cgo and does not link against `libqmi`, `libmbim`, or `libqrtr-glib`.

## Requirements

- Go 1.26 or newer
- Linux for direct `/dev/cdc-wdm*` QMI/MBIM and QRTR socket support
- Linux or Windows for AT serial support
- PC/SC runtime for `ccid`

Install:

```sh
go get github.com/damonto/wwan-go
```

## Package Layout

```text
apdu                         APDU request/response encoding
at                           AT +CSIM APDU reader
ccid                         PC/SC CCID APDU reader
cdcwdm                       Linux cdc-wdm connection primitive
mbim                         MBIM protocol, proxy/direct dialers, UICC access
modem                        Linux modem discovery and unified QMI/MBIM operations
qcom                         QCOM client, QMI service primitives, constants, and transport contracts
qcom/qmi                     QMI/QMUX transport, proxy/direct dialers
qcom/qrtr                    QRTR transport for QMI services
qcom/tlv                     QCOM QMI TLV types, codecs, constructors, and lookup helpers
sim                          SIM/USIM/ISIM card loading and high-level operations
sim/card                     Card-facing interfaces consumed by sim
sim/command                  APDU command helpers used by sim
sim/simfile                  SIM file parsers
sim/stk                      SIM Toolkit commands, envelopes, terminal profile, and BIP
sim/tlv                      BER-TLV helpers
```

## Transport Model

`qmi`, `mbim`, and `qrtr` separate protocol logic from connection setup.

QMI and MBIM require an explicit dialer option:

```go
qmi.Open(ctx, qmi.WithProxy("/dev/cdc-wdm0"))
qmi.Open(ctx, qmi.WithDirect("/dev/cdc-wdm0"))

mbim.Open(ctx, mbim.WithProxy("/dev/cdc-wdm0"), mbim.WithSlot(1))
mbim.Open(ctx, mbim.WithDirect("/dev/cdc-wdm0"), mbim.WithSlot(1))
```

Proxy mode connects to the existing daemon socket protocol (`qmi-proxy` or `mbim-proxy`) and passes the device path through that proxy protocol.

Direct mode opens the device node and performs framing in Go.

QRTR is a Linux QRTR socket transport:

```go
transport, err := qrtr.Open(ctx)
```

## Modem Discovery

The `modem` package models one physical modem as a collection of ports. QMI
and MBIM are properties of individual control ports; a physical device does
not have one implicit protocol or preferred control node.

Linux discovery trusts the WWAN port type or the bound `qmi_wwan`/`cdc_mbim`
driver. It does not infer protocols from device names and does not send both
protocols to an unknown control node. `modem.Open` opens and validates only the
protocol declared by the selected port. Ports returned by `modem.Discover`
include their sysfs path; before opening, `modem.Open` checks that the kernel
still reports the same protocol and driver, and returns `modem.ErrPortChanged`
when a device node has been replaced. Explicitly constructed ports with an empty
`SysPath` skip this discovery-snapshot check.

Each network port records its owning QMI or MBIM device node in `ControlPath`.
This keeps data-interface selection unambiguous when one physical modem exposes
multiple control protocols.

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/wwan-go/modem"
)

func main() {
	ctx := context.Background()

	devices, err := modem.Discover(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, device := range devices {
		for _, port := range device.Ports {
			if port.Protocol() == modem.ProtocolUnknown {
				continue
			}
			opened, err := modem.Open(ctx, port, modem.AccessAuto)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("opened %s modem on %s", opened.Protocol(), opened.Port().Path)
			if err := opened.Close(); err != nil {
				log.Fatal(err)
			}
			return
		}
	}
	log.Fatal("no QMI or MBIM control port found")
}
```

## APDU Readers

AT and CCID expose the same APDU-style shape:

```go
type Transmitter interface {
	Transmit(ctx context.Context, req []byte) ([]byte, error)
	Close() error
}
```

### AT

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/wwan-go/at"
)

func main() {
	ctx := context.Background()

	reader, err := at.Open("/dev/ttyUSB2", 115200)
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	resp, err := reader.Transmit(ctx, []byte{0x00, 0xA4, 0x00, 0x04, 0x02, 0x3F, 0x00})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("% X", resp)
}
```

### CCID

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/wwan-go/ccid"
)

func main() {
	ctx := context.Background()

	names, err := ccid.ListReaders(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if len(names) == 0 {
		log.Fatal("no PC/SC readers")
	}

	reader, err := ccid.Open(ctx, names[0])
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	resp, err := reader.Transmit(ctx, []byte{0x00, 0xA4, 0x00, 0x04, 0x02, 0x3F, 0x00})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("% X", resp)
}
```

## QCOM Client over QMI

Use `qmi.Open` to create a QMI transport, then `qcom.NewClient` to expose UIM, CAT, WDS, NAS, DMS, WDA, and IMS services.
The client lazily allocates one client ID for each QMI service it uses and reuses that ID until `Close`.
Always close the client so its cached service client IDs are explicitly released through `qmi-proxy`.
Long-running PDN and IMS sessions own additional client IDs and must also be closed independently.

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/qmi"
)

func main() {
	ctx := context.Background()

	transport, err := qmi.Open(ctx, qmi.WithProxy("/dev/cdc-wdm0"))
	if err != nil {
		log.Fatal(err)
	}

	client, err := qcom.NewClient(transport, qcom.WithSlot(1))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	status, err := client.CardStatus(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("ready=%v", status.Ready())
}
```

Direct mode:

```go
transport, err := qmi.Open(ctx, qmi.WithDirect("/dev/cdc-wdm0"))
```

## QCOM Client over QRTR

QRTR uses the top-level `qcom/qrtr` package and is not nested under `qmi`.

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/qrtr"
)

func main() {
	ctx := context.Background()

	transport, err := qrtr.Open(ctx)
	if err != nil {
		log.Fatal(err)
	}

	client, err := qcom.NewClient(transport, qcom.WithSlot(1))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	status, err := client.CardStatus(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("ready=%v", status.Ready())
}
```

## MBIM UICC Low-Level Access

MBIM exposes UICC primitives directly from `mbim`.

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/wwan-go/mbim"
)

func main() {
	ctx := context.Background()

	client, err := mbim.Open(ctx, mbim.WithProxy("/dev/cdc-wdm0"), mbim.WithSlot(1))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	apps, err := client.ListApplications(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, app := range apps {
		log.Printf("% X %s", app.AID, app.Label)
	}
}
```

Direct mode:

```go
client, err := mbim.Open(ctx, mbim.WithDirect("/dev/cdc-wdm0"), mbim.WithSlot(1))
```

## SIM Adaptation

`sim` consumes small card-facing interfaces from `sim/card`. It can work over AT, CCID, QMI UIM, or MBIM after adaptation.

APDU transports such as AT and CCID can be wrapped directly:

```go
package main

import (
	"context"
	"log"
	"log/slog"

	"github.com/damonto/wwan-go/at"
	"github.com/damonto/wwan-go/sim"
)

func main() {
	ctx := context.Background()

	tx, err := at.Open("/dev/ttyUSB2", 115200)
	if err != nil {
		log.Fatal(err)
	}

	reader, err := sim.NewReader(tx)
	if err != nil {
		log.Fatal(err)
	}

	card, err := sim.New(ctx, reader, slog.Default())
	if err != nil {
		log.Fatal(err)
	}
	defer card.Close()

	log.Printf("ICCID=%s IMSI=%s MCC=%s MNC=%s", card.ICCID(), card.IMSI(), card.MCC(), card.MNC())
}
```

Pass a logger when the caller owns logging:

```go
card, err := sim.New(ctx, reader, logger)
```

QCOM can be adapted with `sim.NewQCOM`:

```go
transport, err := qmi.Open(ctx, qmi.WithProxy("/dev/cdc-wdm0"))
if err != nil {
	return err
}
qcomClient, err := qcom.NewClient(transport, qcom.WithSlot(1))
if err != nil {
	return err
}
reader, err := sim.NewQCOM(qcomClient)
```

MBIM can be adapted with `sim.NewMBIM`:

```go
mbimClient, err := mbim.Open(ctx, mbim.WithProxy("/dev/cdc-wdm0"), mbim.WithSlot(1))
if err != nil {
	return err
}
reader, err := sim.NewMBIM(mbimClient)
```

Once loaded, `*sim.Card` exposes identity and authentication helpers:

```go
result, err := card.AKA(ctx, rand, autn)
result, err := card.EAPAKA(ctx, rand, autn)
```

## SIM Toolkit

STK hangs off the loaded card. The transport can be APDU (`sim.Reader` over AT or CCID), QCOM UIM, or MBIM; application code uses the same `card.STK()` entry point.

STK command and response types live in `github.com/damonto/wwan-go/sim/stk`:

```go
transport, err := qmi.Open(ctx, qmi.WithProxy("/dev/cdc-wdm0"))
if err != nil {
	return err
}
qcomClient, err := qcom.NewClient(transport, qcom.WithSlot(1))
if err != nil {
	return err
}
reader, err := sim.NewQCOM(qcomClient)
if err != nil {
	return err
}
card, err := sim.New(ctx, reader, slog.Default())
if err != nil {
	return err
}
defer card.Close()

toolkit, err := card.STK()
if err != nil {
	return err
}

return toolkit.Run(ctx, sim.STKCallbacks{
	DisplayText: func(ctx context.Context, cmd stk.DisplayTextCommand) (stk.TerminalResponse, error) {
		log.Print(cmd.Text.Value)
		return stk.OK(), nil
	},
	SetupMenu: func(ctx context.Context, cmd stk.SetupMenuCommand) (stk.TerminalResponse, error) {
		for _, item := range cmd.Items {
			log.Printf("%d %s", item.Identifier, item.Text.Value)
		}
		return stk.OK(), nil
	},
})
```

The terminal profile is derived from the callbacks. Missing callbacks are reported to the card as unsupported terminal capabilities; ordinary callback errors are converted to a best-effort terminal response before the error is returned.

Menu selection and other envelopes are sent through the same STK instance:

```go
_, err = toolkit.SendEnvelope(ctx, stk.MenuSelection(itemID, false))
```

Bearer Independent Protocol is built in for TCP and UDP client channels. The STK runtime opens channels, sends and receives data, reports channel status, and closes active channels when the runtime exits. Application callbacks only need to handle user-visible behavior such as text, menu, SMS, calls, and browser launches.

Transport notes:

- APDU transports use `TERMINAL PROFILE`, `STATUS`, `FETCH`, `TERMINAL RESPONSE`, and `ENVELOPE`. Polling is used when the reader has no proactive indication path.
- QCOM uses CAT/CAT2 raw proactive-command indications and sends raw terminal responses. The high-level `sim.QCOM` adapter registers event reports for the active run and does not change persistent modem CAT configuration.
- If an operator explicitly calls QMI CAT `SetConfiguration` with a custom terminal profile, `GetConfiguration` can confirm the modem setting immediately, but the UICC may not see changed terminal-profile bits until the next UICC initialization. Some cards support additional terminal profile after activation; many real deployments still require an explicit SIM power cycle. The library does not power-cycle SIMs implicitly.
- MBIM uses STK PAC, terminal response, and envelope CIDs. The host PAC profile is cleared when `Run` exits.

### QCOM CAT2 Event Ownership

On Qualcomm modems, CAT/CAT2 event registration is owned by QMI CAT client IDs. `SET_EVENT_REPORT` cannot replace another client's raw event registration; the modem returns `InvalidOperation` with a raw error mask when another CAT2 client already owns one of the requested bits.

When the modem is shared with other host software, prefer `qmi-proxy` and use CAT service-state probing before taking over ownership. The safe takeover flow is:

1. Allocate your CAT2 client through the normal `qcom.Client`.
2. Try `SET_EVENT_REPORT` once.
3. If the response reports a raw event conflict, read CAT `GET_SERVICE_STATE`.
4. Probe candidate CAT2 CIDs with `GET_SERVICE_STATE` and check `raw_client_mask`.
5. Release only the CID whose `raw_client_mask` owns the requested raw bits.
6. Retry `SET_EVENT_REPORT` with your CAT2 client.

The helper below implements that flow:

```go
callbacks := sim.STKCallbacks{
	DisplayText: func(ctx context.Context, cmd stk.DisplayTextCommand) (stk.TerminalResponse, error) {
		log.Print(cmd.Text.Value)
		return stk.OK(), nil
	},
}
profile := sim.ProfileFromCallbacks(callbacks)

claim, err := qcom.NewCAT(qcomClient).ForceClaimEvents(ctx, qcom.CATEventClaimConfig{
	RawMask:          profile.QMIEventMask(),
	FullFunctionMask: profile.QMIFullFunctionMask(),
})
if err != nil {
	return err
}
if claim.ReleasedClientID != 0 {
	log.Printf("released CAT2 owner CID %d", claim.ReleasedClientID)
}

toolkit, err := card.STK()
if err != nil {
	return err
}
return toolkit.Run(ctx, callbacks)
```

Keep the `qcom.Client` open for as long as the STK run is active. Closing it releases the CAT2 client and drops the claimed event registration.

This is still a deliberate takeover of another CAT2 client. Releasing the owner can disrupt whatever process or daemon owns that CAT2 CID until it reallocates a client or the modem is reset.

## Testing

Run all tests:

```sh
GOCACHE=/tmp/wwan-go-build go test ./...
```

Race-test the protocol packages:

```sh
GOCACHE=/tmp/wwan-go-build go test -race ./at ./mbim ./qcom ./qcom/qmi ./qcom/qrtr
```

Cross-compile the AT package:

```sh
GOOS=windows GOCACHE=/tmp/wwan-go-build go test -c -o /tmp/wwan-go-at-windows.test.exe ./at
GOOS=darwin GOCACHE=/tmp/wwan-go-build go test -c -o /tmp/wwan-go-at-darwin.test ./at
```

Check module tidiness:

```sh
GOCACHE=/tmp/wwan-go-build go mod tidy -diff
```

## Design Notes

- Protocol clients expose transport and protocol primitives. They should not depend on `sim`.
- `sim` provides card-level adaptation and business behavior on top of readers.
- QMI and MBIM require explicit proxy or direct mode selection.
- QRTR is a top-level QCOM transport package.
- Types implement Go standard interfaces such as `encoding.BinaryMarshaler`, `encoding.BinaryUnmarshaler`, `io.ReaderFrom`, and `io.WriterTo` where those interfaces naturally fit the wire format.

## License

No license file is currently included.
