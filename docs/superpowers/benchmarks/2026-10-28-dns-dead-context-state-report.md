# DNS dead context state

Date: 2026-10-28

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the parent commit measured in a worktree with the same harness,
n=10 per side.

## Change

`DNSContext.Type` was written on every DNS handler path and never read, so the
type string, its constants and all writes were dead state. Removing them shrinks
the per-query context. `handleMsgWithEmptyAnswer` also allocated an empty
non-nil answer slice; DNS encoding treats nil the same way, so empty replies now
use nil.

## Gate

`BenchmarkServeMsgFakeIP` and `BenchmarkServeMsgFakeIPEmpty` cover the default
fake-ip reply and the skipped/unsupported-type empty reply. The DNS package
tests cover handler composition, cache answers and query tracing.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ServeMsgFakeIP | 221.8n -> 225.4n (~) | 260.0 -> 244.0 (-6.15%) | 5.000 -> 5.000 |
| ServeMsgFakeIPEmpty | 80.35n -> 74.39n (-7.42%, p=0.011) | 176.0 -> 160.0 (-9.09%) | 2.000 -> 2.000 |

The remaining allocations are the context wrapper, the escaped reply and the
answer storage owned by that reply.

## Verification

- `go test ./dns ./context -count=1` green.
- Full-package verification follows before push.
