# mKCP Read Buffer Reuse Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=12 -benchtime=300x` against `300807c5d` in a separate worktree.

Draining `Conn.readBuf` no longer slices to the end of its backing array. Any
unread bytes move to the front, and a fully drained buffer resets to zero length
while retaining its capacity for the next delivered datagram.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip allocations | 109.0 | 77.0 | -29.36%, p=0.000 |
| Bytes per round trip | 87.28 KiB | 39.33 KiB | -54.93%, p=0.000 |
| Round-trip time | 235.7µs | 217.6µs | -7.66%, p=0.020 |

Focused verification: read-buffer capacity reuse, protocol round trips, mKCP
package tests, focused race runs and staticcheck are green.
