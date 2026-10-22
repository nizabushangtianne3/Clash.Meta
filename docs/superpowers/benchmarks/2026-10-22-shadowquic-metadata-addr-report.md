# ShadowQUIC: Build The Target Address From The AddrPort

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What Changed

`MetadataAddr` fed `socks5.ParseAddr` a `net.JoinHostPort(metadata.String(),
port)` string, so an IP target was formatted and parsed back through
`net.ParseIP` on every request the client opens. It branches now:

- An IP target goes through `socks5.AddrFromStdAddrPort(metadata.AddrPort())`.
  The `AddrPort` unmaps a 4-in-6 address, which is the same thing
  `net.ParseIP(...).To4()` did on the old path, so the wire bytes are identical.
- A named target keeps `socks5.ParseAddr(metadata.RemoteAddress())`, because
  `transport/socks5` has no builder for a name.
- An empty destination is still `errInvalidAddress`.

## Gate

`TestMetadataAddrMatchesTheSocksEncoder` compares the new construction against
`socks5.ParseAddr` for IPv4, IPv6, a 4-in-6 mapped IPv4 and a domain, so the
wire form is pinned rather than assumed.

`BenchmarkMetadataAddr` measures an IP target both ways:

| Variant | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `addr-string` (removed) | 346.1n ± 47% | 40 | 3 |
| `addrport` | 15.81n ± 56% | 8 | 1 |

The one remaining allocation is the socks address slice itself, which
`TestMetadataAddrAllocatesOnceForAnIPDestination` pins. The rest of the tree is
covered by `go test ./... -count=1 -p 2` (94 packages), `go vet ./...` and
`-race` over `transport/shadowquic`.
