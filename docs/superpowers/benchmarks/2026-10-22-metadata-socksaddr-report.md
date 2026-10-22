# Metadata.Socksaddr: One Conversion For Every Dial Site

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What Changed

The previous cycle gave `adapter/outbound` a local `destinationSocksaddr`
helper for the eight dial sites that formatted an IP destination into a string
only to parse it back. The same conversion is needed elsewhere, so it lives on
the metadata now:

```go
func (m *Metadata) Socksaddr() M.Socksaddr
```

`adapter/outbound`'s helper is gone and its sites call the method, and
`listener/sing`'s `ConvertMetadata` uses it for the destination — one line above
its own source, which already took the string-free `M.SocksaddrFrom`. The host
branch is unchanged in behaviour: `Metadata.String` returns `metadata.Host`
whenever it is set, so the old call passed the same string.

`constant` now imports `github.com/metacubex/sing/common/metadata`. It already
depends on that package transitively through `common/net`, and `common/net`
cannot host the helper because `constant` imports it.

## Gate

`BenchmarkMetadataSocksaddr` measures an IP destination the removed way and the
new way:

| Variant | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `host-port-string` (removed) | 120.7n ± 44% | 16 | 1 |
| `addrport` | 6.038n ± 61% | 0 | 0 |

`TestMetadataSocksaddrAllocatesNothingForAnIPDestination` pins the zero
allocation for the IP branch, and
`TestMetadataSocksaddrKeepsTheHostAndTheAddrPort` pins both result shapes. The
rest of the tree is covered by `go test ./... -count=1 -p 2` (94 packages),
`go vet ./...` and `-race` over `constant`, `adapter/outbound` and
`listener/sing`.
