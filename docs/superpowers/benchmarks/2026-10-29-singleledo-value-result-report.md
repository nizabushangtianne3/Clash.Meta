# singledo value result

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the current main (`bbb6537b`) measured in a worktree with the same
harness, n=10 per side.

## Change

`singledo` stored its cache result behind a pointer, so every expired `Do`
allocated both the coordinating call and the result. The result is now a value
with a validity flag; the coordinating call remains pointer-allocated so a
`Reset` during a running callback cannot overwrite storage that concurrent
waiters still reference.

## Gate

The URLTest benchmarks exercise a cold selection over a 4096-member group and
the cached hit path. The singledo tests cover concurrency, result sharing,
expiry and reset semantics.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| URLTestSelectedProxyLargeGroup | 265.6n -> 234.6n (~, p=0.054) | 112.0 -> 48.0 (-57.14%) | 2.000 -> 1.000 (-50.00%) |
| URLTestSelectedProxyCached | 24.75n -> 25.18n (~) | 0.0 -> 0.0 | 0.000 -> 0.000 |

The remaining allocation is the coordinating call, which concurrent waiters
may reference independently of the cache result.

## Verification

- `go test ./common/singledo ./adapter/... -count=1` green.
- `go test -race ./common/singledo -count=1` green.
- Full-package verification follows before push.
