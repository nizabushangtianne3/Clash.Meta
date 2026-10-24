# mKCP Condition Wait Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=6 -benchtime=2000x` against `4b4314306` in a separate worktree.

## Change

Read and write waiting now uses two condition variables bound to the connection
mutex. Each direction owns one reusable deadline timer whose callback broadcasts the
condition. The old close-and-replace notification channel, waiter bookkeeping channel
handoff, and global wait-timer pool are gone. A waiter now checks the deadline after
being woken, so both normal signals and deadline expiry converge on the same wake path.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip time | 198.3us | 196.5us | no significant change, p=0.394 |
| Round-trip throughput | 78.80 MiB/s | 79.52 MiB/s | no significant change, p=0.394 |
| Bytes per round trip | 3215.5 B | 392.5 B | -87.79%, p=0.002 |
| Allocations per round trip | 32.00 | 9.00 | -71.88%, p=0.002 |

## Verification

- Concurrent read wakeup, allocation-free signaling, deadline expiry, and deadline
  clearing tests: green.
- Plain, seeded-AEAD, and all header round trips: green.
- Shared-buffer and out-of-order receive tests: green.
- `go test ./transport/mkcp -count=1`: green.
- `staticcheck ./transport/mkcp`: green.
- `go vet ./...`: green.
- `go test -race ./transport/mkcp -count=1`: green.
- `go test -race ./transport/mekya -count=1`: green.
- `go test -race ./listener/inbound -run '^TestInboundShadowSocks_KcpTun$' -count=1`:
  green.
- `go test ./... -count=1 -p 1 -parallel 1`: green.
- `go test -tags with_gvisor ./... -count=1 -p 1 -parallel 1`: green.
