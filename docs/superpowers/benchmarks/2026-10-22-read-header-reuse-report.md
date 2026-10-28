# Reuse the two-byte read header on gost and simple-obfs

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

Three read loops put a two-byte length in a stack array and passed it through a
`net.Conn` interface, which makes the array escape and allocates once per
frame. The connection now owns the two-byte header:

- `gost.relayPacketConn.ReadFrom`
- `simple-obfs.TLSObfs.read`
- `simple-obfs.TLSObfsServer.read`

The existing short-buffer and remainder handling stays where it was; only the
header storage moved from the call frame to the connection.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| gost `ReadFrom` | 116.3 ns mean, 2 B, 1 alloc | 38.5 ns mean, 0 B, 0 allocs |
| simple-obfs client `Read` | 77.2 ns mean, 2 B, 1 alloc | 36.1 ns mean, 0 B, 0 allocs |
| simple-obfs server `Read` | 92.4 ns mean, 2 B, 1 alloc | 0 B, 0 allocs |

`TestRelayPacketConnReadFromAllocatesNothing`,
`TestTLSObfsReadAllocatesNothing` and
`TestTLSObfsServerReadAllocatesNothing` threshold at 0.5 so a `sync.Pool`
refill after a GC cannot flake them; on the parent commit they report 1
allocation each. `TestRelayPacketConnReadFromShortBufferDropsTheRest` pins the
short-buffer behavior that the reused header must preserve. All three carry
`//go:build !race` because the race detector makes `AllocsPerRun` meaningless.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./transport/gost ./transport/simple-obfs ./transport/tuic/...`
- `go vet ./...`
