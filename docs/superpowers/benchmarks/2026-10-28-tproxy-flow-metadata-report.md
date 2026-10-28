# tproxy UDP flow metadata cache

Date: 2026-10-28

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the parent commit measured in a worktree with the identical
benchmark harness (pool-sized bufs, drop-only tunnel), n=10 per side.

## Change

`handlePacketConn` rebuilt a `C.Metadata` per datagram. On a warm flow the
tunnel reads it for nothing but the type check and the flow key lookup, which
the packet wrapper answers on its own, so the whole struct was dead weight per
datagram. The listener now caches one metadata per source `AddrPort` and reuses
it while the datagram repeats that destination; the reply reader on the local
conn shares the same cache because its packets carry the same source and
destination. The dscp stays on the metadata of the flow's first datagram.

## Gate

`listener/tproxy/udp_test.go` pins metadata identity across three datagrams of
one flow, the split between two destinations, the 4in6 source unmap, and the
wrapper's buffer release. `BenchmarkTproxyUDPHandle` measures the warm shape.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 178.4n ± 3% | 556.0 | 2.000 |
| current | 123.1n ± 2% | 123.0 | 1.000 |
| delta | -31.00% (p=0.000) | -77.88% | -50.00% |

The single remaining allocation is the per-datagram wrapper, which the flow
keeps as its write-back target until it ends; recycling it was reverted in
`5fce390e9` for that reason and is not attempted again here.

## Verification

- `go test ./listener/tproxy -count=1` green.
- `go test -race ./listener/tproxy -count=1` green.
- `GOOS=linux GOARCH=amd64 go build ./listener/tproxy` clean.
