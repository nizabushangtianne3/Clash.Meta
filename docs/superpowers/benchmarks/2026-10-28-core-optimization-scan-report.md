# Core optimization scan report

Date: 2026-10-28

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `51656836e`. Six independent review tracks covered network hot
paths, DNS/rules/providers, transports/ledger drift, tunnel/statistic/API,
concurrency/lifecycle, and test/toolchain coverage.

## Shipped this round

| Area | Change | Gate |
| --- | --- | --- |
| TUIC | The read scratch is used by `WaitReadFrom`, while `ReadFrom` returns independently owned addresses instead of aliasing scratch. | New `TestReadFromReturnsIndependentAddresses`; TUIC v4/v5 race clean. |
| Hysteria XPlus | Common key+salt digests use a stack buffer instead of allocating a new buffer on every datagram. Long keys still use the digest stream. | New zero-allocation and digest-equivalence tests; `BenchmarkXPlusObfuscate` is 0 B/op and 0 allocs/op. |
| Hysteria | `defragger.Feed` returns a value, fragment storage no longer takes per-fragment pointers, and UDP session channels carry values instead of pointers. | New unfragmented Feed allocation gate; `BenchmarkQuicPktConnReadFrom` is 0 B/op and 0 allocs/op. |
| Outbound SOCKS address | `serializesSocksAddr` writes one fixed stack-backed buffer instead of five small slices joined per handshake. | New wire-shape tests and a one-allocation gate. |
| Statistic | Connection snapshots use `RangeRelaxed`; TCP tracker reader/writer counters are built once; memory uses an atomic. | `BenchmarkManagerSnapshot` improved from 1.94 us to 0.89 us at 100 connections and 16.24 us to 6.89 us at 500; `BenchmarkTCPTrackerUnwrap` is 0 B/op and 0 allocs/op; statistic race clean. |
| NAT lifecycle | Local connection and lock state now live with the flow entry instead of a second lifetime-coupled map. | New concurrent delete/add test; NAT race clean. |
| Small concurrency fixes | Queue length checks moved inside their locks and provider subscription info uses an atomic pointer. | queue and provider race clean. |
| Statistic JSON | Invalid UTF-8 now matches `encoding/json` by writing the U+FFFD bytes. | Existing `TestAppendJSONStringMatchesEncodingJSON`. |

The diff over the baseline is 21 files, +268/-123 lines before this report.

## Verification

- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `go test -race` on the changed packages: adapter/provider, component/nat,
  common/queue, transport/tuic/v4, transport/tuic/v5, tunnel/statistic
- Platform builds for linux amd64/arm64/386, windows amd64/arm64,
  darwin amd64/arm64, freebsd amd64, android arm64, and linux amd64 with
  `with_gvisor`
- Paired `benchstat` for the statistic snapshot path.

The first full-repository race run exposed a race inside
`restls-client-go` while the whole `listener/inbound` suite is loaded. It is
not in a file touched by this round; focused TUIC and changed-package race
runs pass. This remains a follow-up dependency audit item.

## Next candidates

1. TUIC v5 client receive path still fully parses and starts a goroutine per
   datagram before the packet conn parses again; peek/route the native path.
2. `listener/tunnel` still reads UDP through the generic address path instead
   of the unified AddrPort poller.
3. Hub route can pool connection-response buffers, hand-write traffic/log JSON,
   and combine websocket headers with payload writes.
4. Executor applies can debounce the asynchronous runtime GC.
5. Rule-provider and tunnel rule indexes still duplicate domain-suffix indexing.
6. Android `anet` still carries a second netlink parser and an unused TUN rules
   helper.
7. `with_low_memory` is an untested build tag in CI.
8. The external RestLS race needs a dependency upgrade or upstream fix.
