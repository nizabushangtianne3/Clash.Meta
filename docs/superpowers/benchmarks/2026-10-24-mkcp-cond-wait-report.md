# mKCP Condition Wait Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=12 -benchtime=300x` against `52617e94a` in a separate worktree.

## Change

Read and write waiting now uses two condition variables bound to the connection
mutex. Each direction owns one reusable deadline timer whose callback broadcasts
the condition. This removes the close-and-replace notification channel, its waiter
counters, and the global wait-timer pool. Write timeout and close paths also return
their newly allocated payload to the byte pool.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip time | 191.4µs | 179.6µs | -6.18%, p=0.000 |
| Bytes per round trip | 3583.5 B | 826.0 B | -76.95%, p=0.000 |
| Allocations per round trip | 33.00 | 9.00 | -72.73%, p=0.000 |

## Verification

- Concurrent read wakeup, allocation-free signaling, deadline expiry, and deadline
  clearing tests: green.
- Plain, seeded-AEAD, full-duplex, and all header round trips: green.
- `go test ./transport/mkcp -count=1`: green.
- `staticcheck ./transport/mkcp`: green.
- `go test -race ./transport/mkcp -count=1`: green.
