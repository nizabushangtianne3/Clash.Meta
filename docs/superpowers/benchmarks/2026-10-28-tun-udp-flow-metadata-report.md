# tun UDP listener flow metadata cache

Date: 2026-10-28

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the parent commit measured in a worktree with the identical
benchmark harness (pool-sized bufs, drop-only tunnel), n=10 per side.

## Change

The tun UDP listener rebuilt a `C.Metadata` per datagram from the listener's
single fixed target and the datagram's source, and read the socket through a
plain `ReadFrom`, which materialises a `net.UDPAddr` per datagram in the net
package before the wrapper stores it. The listener now caches one metadata
per source `AddrPort` and reads through the conn poller
(`WaitReadFrom`), so a warm flow allocates only the wrapper the write-back
contract requires; the shared `*net.UDPAddr` the wrapper keeps for replies is
the one the cache entry created.

## Gate

`listener/tunnel/udp_test.go` pins metadata identity across two datagrams of
one source, the split between two sources, and the wrapper's payload release.
`BenchmarkTunnelUDPHandle` measures the warm shape.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 182.4n ± 11% | 506.0 | 2.000 |
| current | 102.2n ± 4% | 87.0 | 1.000 |
| delta | -43.94% (p=0.000) | -82.81% | -50.00% |

## Verification

- `go test ./listener/tunnel -count=1` green.
- `go test -race ./listener/tunnel -count=1` green.
- `go vet ./listener/tunnel` clean.
- Cross-compile for `linux`, `darwin`, `windows` and `freebsd` clean.
