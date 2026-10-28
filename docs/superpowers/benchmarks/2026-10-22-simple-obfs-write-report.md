# simple-obfs: write the frame length without binary.Write

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

Both `TLSObfs.write` and `TLSObfsServer.write` wrote their two-byte frame
length through `binary.Write` into the pooled buffer. The fixed-size call
allocated a scratch slice for every frame. The length now goes through the
buffer's `WriteByte`; the handshake-only `binary.Write` calls are unchanged.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| client `write` | 122.5 ns mean, 2 B, 1 alloc | 80.5 ns mean, 0 B, 0 allocs |
| server `write` | 84.6 ns mean, 2 B, 1 alloc | 0 B, 0 allocs |

`TestTLSObfsWriteAllocatesNothing` and
`TestTLSObfsServerWriteAllocatesNothing` threshold at 0.5 and report 1
allocation on the parent commit. The new frame tests pin both output shapes,
and the existing read tests keep the receive path covered.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./transport/simple-obfs`
- `go vet ./...`
