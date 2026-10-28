# TCP tracker counter array

Date: 2026-10-28

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the parent commit measured with the same benchmark harness, n=10
per side.

## Change

Each TCP tracker heap-allocated a separate `trackerCounts` object and then two
fresh slices to hold its read and write counters. The tracker now owns a
two-element counter array. `UnwrapReader` and `UnwrapWriter` return capped
disjoint slices over that array, preserving the sing counter contract and
append isolation while removing three allocations per TCP connection.

## Gate

`BenchmarkNewTCPTracker` measures creation and manager join. The statistic
tests account reads, writes, buffered reads/writes, manager traffic and the
tracker lifecycle.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 628.0n ± 6% | 832.0 | 12.000 |
| current | 580.0n ± 9% | 784.0 | 9.000 |
| delta | -7.64% (p=0.004) | -5.77% | -25.00% |

## Verification

- `go test ./tunnel/statistic -count=1` green.
- Full-package verification follows before push.
