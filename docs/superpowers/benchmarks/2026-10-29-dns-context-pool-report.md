# DNS context pool

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the parent commit measured in a worktree with the same harness,
n=10 per side.

## Change

Every DNS query allocated a `DNSContext` wrapper whose only remaining role is
to identify application-initiated queries. The internal handler chain is
synchronous and does not retain the context after it returns, so `ServeMsg` now
recycles that wrapper through a pool and clears its parent context before reuse.

## Gate

`BenchmarkServeMsgFakeIP` covers the default fake-ip reply.
`BenchmarkServeMsgFakeIPEmpty` covers skipped and unsupported-type replies. The
DNS package tests cover handler composition, cache answers, query tracing and
EDNS echo behaviour.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ServeMsgFakeIP | 222.2n -> 219.0n (~) | 244.0 -> 228.0 (-6.56%) | 5.000 -> 4.000 (-20.00%) |
| ServeMsgFakeIPEmpty | 77.33n -> 79.96n (~) | 160.0 -> 144.0 (-10.00%) | 2.000 -> 1.000 (-50.00%) |

Wall-clock stayed within host noise; allocation count is the gate.

## Verification

- `go test ./dns -count=1` green.
- Full-package verification follows before push.
