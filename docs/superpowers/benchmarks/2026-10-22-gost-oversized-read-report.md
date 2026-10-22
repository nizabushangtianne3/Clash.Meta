# gost: pool the oversized UDP read buffer

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

`relayPacketConn.ReadFrom` allocates a temporary slice when a datagram is
larger than the caller's buffer, copies it into the caller's buffer and drops
the rest. The temporary now comes from the shared pool and is returned before
the function exits.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| `ReadFrom` oversized | 1 alloc, 64 B | 0 allocs, 0 B |

`TestRelayPacketConnReadFromOversizedAllocatesNothing` thresholds at 0.5 and
reports 1 allocation on the parent commit. Existing short-buffer and framing
tests keep the drop semantics covered.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./transport/gost`
- `go vet ./...`
