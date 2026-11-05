# TUIC v5 native datagram routing report

Date: 2026-11-05

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, seven paired samples.

## Change

`NATIVE` TUIC v5 client datagrams no longer round-trip through
`N.Pipe` + `BufferedConn`. `handleMessage` parses each QUIC datagram directly,
keeps coalesced frames intact, and delivers `Packet` values on a bounded native
channel. IP addresses decode in place; only the payload copies into its pooled
buffer. QUIC stream relay keeps the existing stream parser. `Close` now drains
queued packets and incomplete fragment bags.

## Verification

- `go test ./transport/tuic/v5 -count=1`
- `go test -race ./transport/tuic/v5 -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p=1`: 104 packages ok
- wrapper `go test .`, desktop/Android wrapper vets: ok

New gates cover wire/stream equivalence, IP/domain decoding, short and invalid
datagrams, zero pooled-payload allocation, native single packets, and fragmented
native packets.

## Benchmark

`BenchmarkTUICNativeDatagramRoute` measures the receive-side routing path with a
1350-byte IPv4 payload. The baseline emulates the former pipe write plus
`BufferedConn` parse; current exercises direct parse plus native channel
delivery.

| Path | Time | Memory | Allocs |
| --- | ---: | ---: | ---: |
| Baseline pipe route | 1637.0 ns ± 5% | 0 B/op | 0 |
| Direct datagram route | 216.9 ns ± 3% | 0 B/op | 0 |

benchstat: time **-86.75%**, `p=0.001`, `n=7`; memory and allocations stay at
zero.
