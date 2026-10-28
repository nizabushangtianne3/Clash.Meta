# mKCP Control Segment Pool Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=12 -benchtime=300x` against `1a5ef1400` in a separate worktree.

Outgoing ACK and ping segments now use dedicated control-segment pools, and the
ACK flush path retains its candidate-number slice on the connection. The flush
release switch returns each control object to its own pool. The allocation
assertion skips only under the race build, where instrumentation adds an
allocation; the race build still executes the same control flow.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip allocations | 176.5 | 134.0 | -24.08%, p=0.000 |
| Round-trip time | 226.5µs | 227.0µs | no significant change |
| Bytes per round trip | 88.85 KiB | 87.80 KiB | no significant change |

Focused verification: control-segment allocation test, mKCP package tests,
staticcheck and mKCP race are green.
