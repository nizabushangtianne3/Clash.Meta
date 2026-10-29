# queue ring buffer

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the current main (`0cdee0bf`) measured in a worktree with the same
benchmark, n=10 per side.

## Change

`Queue` implemented FIFO by advancing the slice header. Once a bounded delay
history was full, every `Put` after a `Pop` copied the remaining items into a
new array. The queue now uses a ring buffer with explicit head/count, reuses
slots across pops, clears popped slots so pointer values do not linger, and
preserves lock semantics and batch growth.

## Gate

`BenchmarkQueueSaturatedPutPop` keeps a ten-item history full and measures one
`Put` plus one `Pop` per iteration. The tests cover FIFO order, wrapping across
the backing-array boundary, copies, resize, concurrent producers/consumers and
the adapter delay-history path.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| QueueSaturatedPutPop | 109.25n -> 52.31n (-52.12%, p=0.002) | 16.0 -> 0.0 (-100.00%) | 0.000 -> 0.000 |

The baseline B/op comes from slice copying during steady-state growth; both
versions keep the fast path allocation-free.

## Verification

- `go test ./common/queue ./adapter -count=1` green.
- `go test -race ./common/queue -count=1` green.
- Full-tree tests and gVisor tests green; cross-compile matrix green.
