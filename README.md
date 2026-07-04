# uicc-go

`uicc-go` is a Go protocol library for working with UICC, USIM, ISIM, and SIM access paths.

The repository separates protocol packages from USIM business logic:

- Top-level protocol packages implement concrete primitives such as AT `+CSIM`, PC/SC CCID APDU transport, MBIM UICC low-level access, QMI UIM, and QRTR QMI transport.
- `apdu` implements APDU request/response encoding.
- `usim` adapts readers into higher-level USIM and ISIM operations such as loading ICCID/IMSI identities, AKA, EAP-AKA, SMS-PP download, and SIM Toolkit.

The protocol readers do not depend on `usim`. Use `usim` only when you want card-level business behavior on top of a reader.

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
go get github.com/damonto/uicc-go
```

## Package Layout

```text
apdu                         APDU request/response encoding
at                           AT +CSIM APDU reader
ccid                         PC/SC CCID APDU reader
cdcwdm                       Linux cdc-wdm connection primitive
mbim                         MBIM protocol, proxy/direct dialers, UICC access
qcom                         Shared QCOM QMI/QMUX constants and transport contracts
qcom/qmi                     QMI/QMUX transport, proxy/direct dialers
qcom/qrtr                    QRTR transport for QMI services
qcom/tlv                     QCOM QMI TLV types, codecs, constructors, and lookup helpers
qcom/uim                     QMI UIM primitives
usim                         USIM/ISIM card loading and high-level operations
usim/card                    Card-facing interfaces consumed by usim
usim/command                 APDU command helpers used by usim
usim/simfile                 SIM file parsers
usim/stk                     SIM Toolkit commands, envelopes, terminal profile, and BIP
usim/tlv                     BER-TLV helpers
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

	"github.com/damonto/uicc-go/at"
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

	"github.com/damonto/uicc-go/ccid"
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

## QCOM UIM over QMI

Use `qmi.Open` to create a QMI transport, then `uim.New` to allocate a UIM client and expose QMI UIM primitives.

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/uicc-go/qcom/qmi"
	"github.com/damonto/uicc-go/qcom/uim"
)

func main() {
	ctx := context.Background()

	transport, err := qmi.Open(ctx, qmi.WithProxy("/dev/cdc-wdm0"))
	if err != nil {
		log.Fatal(err)
	}

	reader, err := uim.New(ctx, transport, uim.WithSlot(1))
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	status, err := reader.CardStatus(ctx)
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

## QCOM UIM over QRTR

QRTR uses the top-level `qcom/qrtr` package and is not nested under `qmi`.

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/uicc-go/qcom/qrtr"
	"github.com/damonto/uicc-go/qcom/uim"
)

func main() {
	ctx := context.Background()

	transport, err := qrtr.Open(ctx)
	if err != nil {
		log.Fatal(err)
	}

	reader, err := uim.New(ctx, transport, uim.WithSlot(1))
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	status, err := reader.CardStatus(ctx)
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

	"github.com/damonto/uicc-go/mbim"
)

func main() {
	ctx := context.Background()

	reader, err := mbim.Open(ctx, mbim.WithProxy("/dev/cdc-wdm0"), mbim.WithSlot(1))
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	apps, err := reader.ListApplications(ctx)
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
reader, err := mbim.Open(ctx, mbim.WithDirect("/dev/cdc-wdm0"), mbim.WithSlot(1))
```

## USIM Adaptation

`usim` consumes small card-facing interfaces from `usim/card`. It can work over AT, CCID, QMI UIM, or MBIM after adaptation.

APDU transports such as AT and CCID can be wrapped directly:

```go
package main

import (
	"context"
	"log"

	"github.com/damonto/uicc-go/at"
	"github.com/damonto/uicc-go/usim"
)

func main() {
	ctx := context.Background()

	tx, err := at.Open("/dev/ttyUSB2", 115200)
	if err != nil {
		log.Fatal(err)
	}

	reader, err := usim.NewReader(tx)
	if err != nil {
		log.Fatal(err)
	}

	card, err := usim.New(ctx, reader)
	if err != nil {
		log.Fatal(err)
	}
	defer card.Close()

	log.Printf("ICCID=%s IMSI=%s MCC=%s MNC=%s", card.ICCID(), card.IMSI(), card.MCC(), card.MNC())
}
```

Pass a logger when the caller owns logging:

```go
card, err := usim.New(ctx, reader, logger)
```

QMI UIM can be adapted with `usim.NewQCOM`:

```go
transport, err := qmi.Open(ctx, qmi.WithProxy("/dev/cdc-wdm0"))
if err != nil {
	return err
}
uimReader, err := uim.New(ctx, transport, uim.WithSlot(1))
if err != nil {
	return err
}
reader, err := usim.NewQCOM(uimReader)
```

MBIM can be adapted with `usim.NewMBIM`:

```go
mbimReader, err := mbim.Open(ctx, mbim.WithProxy("/dev/cdc-wdm0"), mbim.WithSlot(1))
if err != nil {
	return err
}
reader, err := usim.NewMBIM(mbimReader)
```

Once loaded, `*usim.Card` exposes identity and authentication helpers:

```go
result, err := card.AKA(ctx, rand, autn)
result, err := card.EAPAKA(ctx, rand, autn)
```

## SIM Toolkit

STK hangs off the loaded card. The transport can be APDU (`usim.Reader` over AT or CCID), QCOM UIM, or MBIM; application code uses the same `card.STK()` entry point.

STK command and response types live in `github.com/damonto/uicc-go/usim/stk`:

```go
transport, err := qmi.Open(ctx, qmi.WithProxy("/dev/cdc-wdm0"))
if err != nil {
	return err
}
uimReader, err := uim.New(ctx, transport, uim.WithSlot(1))
if err != nil {
	return err
}
reader, err := usim.NewQCOM(uimReader)
if err != nil {
	return err
}
card, err := usim.New(ctx, reader)
if err != nil {
	return err
}
defer card.Close()

toolkit, err := card.STK()
if err != nil {
	return err
}

return toolkit.Run(ctx, usim.STKCallbacks{
	DisplayText: func(ctx context.Context, cmd stk.DisplayTextCommand) (stk.TerminalResponse, error) {
		log.Print(cmd.Text.String)
		return stk.OK(), nil
	},
	SetupMenu: func(ctx context.Context, cmd stk.SetupMenuCommand) (stk.TerminalResponse, error) {
		for _, item := range cmd.Items {
			log.Printf("%d %s", item.Identifier, item.Text.String)
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
- QCOM uses CAT/CAT2 raw proactive-command indications and sends raw terminal responses. The high-level `usim.QCOM` adapter registers event reports for the active run and does not change persistent modem CAT configuration.
- If an operator explicitly calls QMI CAT `SetConfiguration` with a custom terminal profile, `GetConfiguration` can confirm the modem setting immediately, but the UICC may not see changed terminal-profile bits until the next UICC initialization. Some cards support additional terminal profile after activation; many real deployments still require an explicit SIM power cycle. The library does not power-cycle SIMs implicitly.
- MBIM uses STK PAC, terminal response, and envelope CIDs. The host PAC profile is cleared when `Run` exits.

### QCOM CAT2 Event Ownership

On Qualcomm modems, CAT/CAT2 event registration is owned by QMI CAT client IDs. `SET_EVENT_REPORT` cannot replace another client's raw event registration; the modem returns `InvalidOperation` with a raw error mask when another CAT2 client already owns one of the requested bits.

When the modem is shared with other host software, prefer `qmi-proxy` and use CAT service-state probing before taking over ownership. The safe takeover flow is:

1. Allocate your CAT2 client through the normal `uim.Reader`.
2. Try `SET_EVENT_REPORT` once.
3. If the response reports a raw event conflict, read CAT `GET_SERVICE_STATE`.
4. Probe candidate CAT2 CIDs with `GET_SERVICE_STATE` and check `raw_client_mask`.
5. Release only the CID whose `raw_client_mask` owns the requested raw bits.
6. Retry `SET_EVENT_REPORT` with your CAT2 client.

The helper below implements that flow:

```go
callbacks := usim.STKCallbacks{
	DisplayText: func(ctx context.Context, cmd stk.DisplayTextCommand) (stk.TerminalResponse, error) {
		log.Print(cmd.Text.String)
		return stk.OK(), nil
	},
}
profile := usim.ProfileFromCallbacks(callbacks)

claim, err := uim.NewCAT(uimReader).ForceClaimEvents(ctx, uim.CATEventClaimConfig{
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

Keep the `uim.Reader` open for as long as the STK run is active. Closing it releases the CAT2 client and drops the claimed event registration.

Do not scan CAT2 CIDs up to 255. The Qualcomm reference stack keeps CAT client IDs in a small service-local range (`1..5` on the tested CAT2 firmware), and invalid high CIDs may time out instead of returning `InvalidClientId`. The default helper probes only that small range and only releases a CID after `GET_SERVICE_STATE` proves it owns the requested raw bits.

This is still a deliberate takeover of another CAT2 client. Releasing the owner can disrupt whatever process or daemon owns that CAT2 CID until it reallocates a client or the modem is reset.

## Testing

Run all tests:

```sh
GOCACHE=/tmp/uicc-go-build go test ./...
```

Race-test the protocol packages:

```sh
GOCACHE=/tmp/uicc-go-build go test -race ./at ./mbim ./qcom ./qcom/qmi ./qcom/qrtr ./qcom/uim
```

Cross-compile the AT package:

```sh
GOOS=windows GOCACHE=/tmp/uicc-go-build go test -c -o /tmp/uicc-go-at-windows.test.exe ./at
GOOS=darwin GOCACHE=/tmp/uicc-go-build go test -c -o /tmp/uicc-go-at-darwin.test ./at
```

Check module tidiness:

```sh
GOCACHE=/tmp/uicc-go-build go mod tidy -diff
```

## Design Notes

- Protocol readers expose transport and protocol primitives. They should not depend on `usim`.
- `usim` provides card-level adaptation and business behavior on top of readers.
- QMI and MBIM require explicit proxy or direct mode selection.
- QRTR is a top-level QCOM transport package.
- Types implement Go standard interfaces such as `encoding.BinaryMarshaler`, `encoding.BinaryUnmarshaler`, `io.ReaderFrom`, and `io.WriterTo` where those interfaces naturally fit the wire format.

## License

No license file is currently included.
