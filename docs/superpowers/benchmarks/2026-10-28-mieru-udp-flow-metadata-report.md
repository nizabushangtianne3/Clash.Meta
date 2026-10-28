# mieru UDP flow metadata cache

Date: 2026-10-28

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the pre-cycle tip `5b5e4316` (the `origin/main` this session started
from, three commits behind this one) measured in a worktree with the identical
benchmark harness, n=10 per side.

## Change

`handleMieruUDPPacket` decoded the socks5 header, materialised the source as a
`net.UDPAddr`, and parsed a full `C.Metadata` per datagram, with
`parseSocksAddr` converting the address through a string and a `netip.ParseAddr`
on every call. A profile of the baseline showed all eight per-datagram
allocations sitting in that parse chain. One mieru session now keeps an LRU
flow cache keyed by source address, validated against the target bytes, so a
warm flow allocates only the wrapper the write-back contract requires.

## Gate

`listener/mieru/server_test.go` pins metadata identity across two datagrams of
one flow, the split between two targets, the shared source address object and
the wrapper's buffer release. `BenchmarkMieruUDPHandle` measures the warm shape.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 400.20n ± 4% | 656.0 | 8.000 |
| current | 86.31n ± 8% | 80.0 | 1.000 |
| delta | -78.43% (p=0.000) | -87.80% | -87.50% |

## Verification

- `go test ./listener/mieru -count=1` green.
- `go test -race ./listener/mieru -count=1` green.
- `go vet ./listener/mieru` clean.
