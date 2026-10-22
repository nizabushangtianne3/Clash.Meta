# Outbound Dials: Build The Destination From The AddrPort

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What Changed

Eight dial paths in `adapter/outbound` built their destination with
`M.ParseSocksaddrHostPort(metadata.String(), metadata.DstPort)`, which formats
an IP destination into a string and parses it straight back into sing's address
form. `destinationSocksaddr` resolves it directly:
`M.SocksaddrFromNetIP(metadata.AddrPort())` for an IP target, and the host
itself for a name.

The host branch is deliberately unchanged: `Metadata.String` already returns
`metadata.Host` when it is set, so the old call passed the same string. sing's
`ParseSocksaddrHostPort` still has to try `netip.ParseAddr` on a name, which is
where its one allocation on that branch comes from; that behaviour predates this
change and is untouched.

Sites: `anytls.go`, `hysteria2.go`, `singmux.go`, `shadowsocks.go` (two),
`vmess.go` (two).

## Gate

`BenchmarkDestinationSocksaddr` measures an IP destination the removed way and
the new way on this tree:

| Variant | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `host-port-string` (removed) | 161.9n ± 24% | 16 | 1 |
| `addrport` | 6.555n ± 58% | 0 | 0 |

`TestDestinationSocksaddrAllocatesNothingForAnIPDestination` pins the zero
allocation for the IP branch, and
`TestDestinationSocksaddrKeepsTheHostAndTheAddrPort` pins both result shapes.
The rest of the tree is covered by `go test ./... -count=1 -p 2` (94 packages),
`go vet ./...` and `-race` over `adapter/outbound`.
