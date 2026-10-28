# TrustTunnel: fixed packet fields without binary.Read or binary.Write

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

The TrustTunnel UDP codec wrote its length, ports and app-name length through
`binary.Write`, and mirrored them through `binary.Read` on the way back. Each
fixed-size call allocated a scratch slice on the fast path.

Both directions now use `buf.Buffer`'s byte methods through local uint16/uint32
helpers. The package previously had no tests; the new round-trip tests drive a
client write into a server read and a server write into a client read, pinning
the wire layout before the codec is optimized.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| client `writePacketToServer` | 202.5 ns mean, 72 B, 4 allocs | 116.9 ns mean, 64 B, 1 alloc |
| server `readPacketFromClient` | 358.9 ns mean, 101 B, 6 allocs | 284.6 ns mean, 89 B, 2 allocs |

The remaining write allocation is the caller's `buf.As` wrapper; the remaining
read allocations are that wrapper's read-side equivalent and the pooled header
buffer. `TestClientWritePacketToServerAllocatesOnlyTheBuffer` and
`TestServerReadPacketFromClientAllocatesBufferAndHeader` threshold at 1.5 and
2.5; on the parent commit they report 4 and 6 allocations. Both carry
`//go:build !race`.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./transport/trusttunnel`
- `go vet ./...`
