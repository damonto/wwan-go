# Repository guidance

These instructions apply to the whole repository. Follow more specific instructions
in nested `AGENTS.md` files when present. Keep this file focused on durable project
conventions; `CLAUDE.md` imports it rather than maintaining a second copy.

Work as a pragmatic Go maintainer: make small, readable changes, preserve package
boundaries, and explain non-obvious decisions in plain language.

## Project and layout

`github.com/damonto/wwan-go` is a Go library for cellular modem protocols and SIM
operations. Keep changes in the owning packages and retain the existing library
structure; add application scaffolding only when the task calls for it. `go.mod` is
authoritative for the minimum Go version, currently **Go 1.26.3**. Use language and
standard-library features supported by that version.

| Path | Responsibility |
| --- | --- |
| `apdu/` | APDU request/response codecs and status errors |
| `at/`, `ccid/` | APDU readers over AT serial and PC/SC |
| `cdcwdm/` | Linux cdc-wdm connection, deadlines, and disconnect handling |
| `qcom/` | QMI service requests, responses, typed codecs, and client lifecycle |
| `qcom/qmi/` | QMI/QMUX framing and proxy/direct transports |
| `qcom/qrtr/` | QRTR transport and Linux socket support |
| `qcom/tlv/` | QMI TLV encoding and decoding |
| `mbim/` | MBIM/MBIMEx codecs, clients, indications, and proxy/direct access |
| `modem/` | Discovery, port selection, and unified modem operations |
| `modem/contract/` | Protocol-neutral types, errors, and session contracts |
| `modem/qmi/`, `modem/mbim/` | Backends adapting protocol clients to modem contracts |
| `modem/sms/` | SMS PDU/text codecs and multipart assembly |
| `sim/`, `sim/card/` | SIM/USIM/ISIM operations, adapters, and reader contracts |
| `sim/command/`, `sim/simfile/` | APDU commands and SIM file parsers |
| `sim/stk/`, `sim/tlv/` | SIM Toolkit types and BER/comprehension TLV codecs |

Tests live beside the code in `*_test.go` files. Read `README.md` for usage and
transport behavior, then inspect the nearest implementation and tests before editing.
Check `git status` first and preserve unrelated user changes.

## Architectural constraints

- Keep protocol primitives in their protocol packages. Low-level clients and
  transports must not import `sim` or `modem`; SIM adaptation belongs in `sim`,
  and unified modem behavior belongs in `modem` and its backends.
- Put protocol-neutral modem types in `modem/contract`. Preserve the public type
  aliases in `modem` and the backends instead of duplicating or coupling these types.
- Keep the distinct TLV formats in `qcom/tlv`, `sim/tlv`, and `mbim` separate.
  Similar names do not imply compatible wire formats.
- Preserve explicit transport configuration. QMI and MBIM support proxy, direct,
  and explicitly requested access auto-detection. Auto-detection falls back only
  when connecting to the proxy fails; a proxy that rejects the device must not
  trigger a direct retry. Cancellation must not trigger fallback either.
- Discovery classifies QMI/MBIM ports from kernel metadata. Do not guess from
  device names or probe an unknown node with both protocols. Preserve sysfs
  snapshot validation and each network port's owning `ControlPath`.
- Keep the implementation free of cgo and native libqmi/libmbim/libqrtr-glib
  bindings. `ccid` still requires the operating system's PC/SC runtime.
- Keep OS-specific behavior behind the existing file suffixes and build tags.
  Direct cdc-wdm and QRTR access require Linux; AT serial supports Linux and Windows.
  Preserve unsupported-platform behavior.

## Go style and API design

Follow the [Google Go Style Guide](https://google.github.io/styleguide/go/), with
explicit task requirements and repository compatibility constraints taking priority.
Use `gofmt` and follow neighboring naming and import conventions. Code, comments,
and repository documentation use English.

- Prefer the standard library, including `slices`, `maps`, and `encoding/binary`.
  Add dependencies only for a concrete need. If dependencies change, run
  `go mod tidy`, review both module files, and tell the user about the change.
- Keep files grouped by protocol service or feature, such as `qcom/dms_info.go`
  and `mbim/connect.go`, with corresponding test files. Avoid generic utility packages.
- Use concrete request/response types and small interfaces at consumption boundaries.
  Existing transport, dialer, reader, and backend interfaces provide real boundaries
  and test seams; do not remove them merely because one implementation is visible.
- Reuse existing functional options and request/config structs. Do not introduce
  abstractions or change public signatures to satisfy arbitrary parameter counts.
  Preserve exported names, aliases, error identities, and documented behavior unless
  the task explicitly requires a breaking change.
- Use standard codec interfaces where applicable: `encoding.BinaryMarshaler`,
  `encoding.BinaryUnmarshaler`, `io.ReaderFrom`, and `io.WriterTo`. Follow the
  existing `Request()`, `MarshalTLVs`, and `UnmarshalTLVs` patterns for QMI services.
- Return early on errors. Use `errors.New` for fixed errors, `%w` when adding useful
  operation context, and `errors.Is`/`errors.As` for classification. Preserve typed
  protocol errors and partial-result contracts; do not classify errors by their text.
  Error text should describe the operation without redundant "failed to" prefixes.
- Pass `ctx context.Context` first and propagate caller cancellation. Do not store
  request contexts in structs. Prefer concrete types; use generics where the existing
  stream or codec abstraction benefits from them. Keep `unsafe` confined to necessary
  OS interoperability, and avoid reflection-based codec machinery.
- Document exported contracts and explain wire quirks, ownership, and compatibility
  decisions. Do not narrate obvious implementation steps or claim unverified behavior.

## Protocol correctness and lifecycle

- Verify message IDs, field widths, byte order, lengths, offsets, alignment, and
  service/version gates against specifications or reference implementations.
  Never infer a wire layout from a similarly named command.
- Validate device data before slicing or allocating. Reject truncated payloads,
  invalid counts, overflow, and invalid references without panicking. Preserve each
  message's rules for optional fields, unknown TLVs, padding, and trailing data.
- Preserve absence versus zero values using the existing pointers and `Known`/`Has`
  flags. Decoded values that retain bytes must own their data; clone slices rather
  than retaining mutable receive buffers or exposing shared internal state.
- Keep request/response correlation, transaction ID limits, fragmentation, and
  indication ordering intact. Subscribe before issuing requests that can immediately
  produce indications. Do not silently drop messages covered by an ordered contract.
- Every goroutine needs a defined exit path. Cancellation and `Close` must unblock
  pending I/O and channel sends. Preserve lock ownership, idempotent cleanup, and
  shared connection leases so closing one client does not break another.
- Release owned QMI client IDs, logical channels, sessions, and event registrations.
  Cleanup must have a bounded budget and still release local resources after remote
  cleanup fails. Explain deliberately ignored cleanup errors in a short comment.
- Keep disruptive operations explicit. Do not add implicit modem resets, SIM power
  cycles, persistent CAT configuration changes, or takeover of another client's events.
  Qualcomm BIP channels remain modem-owned; shared STK types do not imply a host
  TCP/UDP runtime.

## Sources and tools

Use repository code and tests to establish existing behavior. For unfamiliar external
APIs, use Context7 when available and verify against the dependency version in
`go.mod`; official documentation and installed module source are valid fallbacks.
Do not invent APIs or protocol details.

An ignored `refs/` directory may be available locally. Start with `refs/README.md`
and `refs/qcom/README.md` for source provenance. Prefer the newer Qualcomm host/IDL
definitions over legacy AMSS definitions; use the MBIM/MBIMEx and 3GPP/ETSI documents
for their respective protocols. These references are optional research material,
not build dependencies. Do not modify, vendor, or commit reference trees as part
of ordinary code changes. Record the source/version near non-obvious protocol fixes.

## Tests and verification

Use the standard `testing` package and table-driven tests with named `t.Run` cases
for related inputs. Keep tests beside the implementation and use local fake
connections/transports, `net.Pipe`, and temporary sysfs trees as appropriate. Fakes
must honor the real contract, including cancellation, ordering, ownership, and errors.
Normal tests must not require a live modem or proxy daemon; isolate kernel-dependent
checks and skip with a reason when the required facility is unavailable.

For protocol changes, test exact wire bytes and malformed/boundary inputs as well as
round trips: an encoder and decoder can share the same bug. Cover absent optional
fields and input-buffer ownership when relevant. For lifecycle changes, cover
cancellation, concurrent close, resource release, and blocked readers/subscribers.
Prefer explicit synchronization to sleep-based timing assumptions.

Run commands from the repository root. Format changed Go files with `gofmt -w`,
run focused tests during development, then validate Go changes with:

```sh
GOCACHE=/tmp/wwan-go-build go test ./...
GOCACHE=/tmp/wwan-go-build go vet ./...
GOCACHE=/tmp/wwan-go-build go mod tidy -diff
```

For transport or concurrency changes, race-test the affected packages. For example:

```sh
GOCACHE=/tmp/wwan-go-build go test -race ./at ./mbim ./qcom/... ./modem/...
```

When changing AT platform code, also compile its tests for the other supported
targets and the unsupported-platform stub:

```sh
GOOS=windows GOCACHE=/tmp/wwan-go-build go test -c -o /tmp/wwan-go-at-windows.test.exe ./at
GOOS=darwin GOCACHE=/tmp/wwan-go-build go test -c -o /tmp/wwan-go-at-darwin.test ./at
```

For documentation-only changes, check accuracy, links, and the diff; rerun Go checks
only when needed to validate a documented command or claim. Review the final diff for
unrelated edits. Report what changed, which checks actually ran, and any skipped or
blocked validation. Update `README.md` when public usage or behavior changes.
