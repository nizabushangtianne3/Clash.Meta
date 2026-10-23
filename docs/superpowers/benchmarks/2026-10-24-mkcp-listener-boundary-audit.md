# mKCP Listener Boundary Audit

Date: 2026-10-24

The history audit found one boundary that did not meet the no-compatibility
standard: `sessionAddress` interpreted both UDP and virtual packet addresses in
one hot-path function. That mixed-domain lookup has been removed.

The listener now has two explicit entry points:

- `ListenUDP` canonicalizes `*net.UDPAddr` values to unmapped `netip.AddrPort`.
- `ListenVirtual` uses the virtual packet connection's comparable address object.

There is no generic `Listen` entry point, no address-type fallback, and no legacy
string key. Mekya calls the virtual constructor directly; the VMess inbound calls
the UDP constructor directly.

The same audit also removed MASQUE H2's lazy receive-reader branch: the buffered
varint reader is created with the stream, so `ReceiveDatagram` has one read path.

A serial `benchstat -count=10 -benchtime=300x` comparison against `61c33999e`
showed 94.34 KiB -> 91.23 KiB per 16 KiB round trip (-3.30%, p=0.000), with no
significant time or allocation-count change.

## Historical Audit

| Commit | Result |
| --- | --- |
| `fae790e2f` | Datagram pool ownership changes; no legacy path retained. |
| `d890d6724` | Segment payload lifetime ownership; no compatibility branch. |
| `a5149f494` | Removes an invalid write wakeup; no compatibility branch. |
| `48c9ae77f` | Segment and flush-batch reuse; no compatibility branch. |
| `9fc0cc580` | Mixed UDP/virtual address lookup; superseded by this local boundary refactor. |
| `95ead5d19` | Reduced key boxing but retained the mixed lookup; superseded here. |
| `61c33999e` | Idle-reader signal guard; a waiter-state check, not a fallback. |

## Verification

- Focused UDP and virtual key tests, plus mKCP, Mekya and VMess inbound package
  tests: green.
- Staticcheck and `go vet ./...`: green.
- mKCP, Mekya, MASQUE package race and inbound KCP interop race: green.
- Full tests and the `with_gvisor` suite: green.
