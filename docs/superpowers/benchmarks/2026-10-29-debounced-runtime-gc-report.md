# Debounced runtime GC

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `623387946`.

## Change

Configuration reloads now share one scheduled background collection. When a
reload lands while that collection is running, the scheduler records one
follow-up instead of immediately starting another. This removes repeated full
GCs from provider-parse reload bursts while preserving a final reclaim.

## Gate

`TestGCSchedulerCoalescesReloadBursts` schedules a 20-call burst and requires
one collection, then a single follow-up. `TestGCSchedulerKeepsWorkQueuedDuringCollection`
pins the reload that arrives while a collection is running.

## Verification

- `go test ./hub/executor -count=1`
- Full gate: see `2026-10-29-local-verification-round-report.md`.
