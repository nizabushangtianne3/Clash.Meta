# mKCP FNV Hash Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=20 -benchtime=10000x` against `66b2c21fd` in a separate worktree.

## Change

The simple authenticator computes FNV-32a directly instead of constructing the
standard-library hash object and calling its write interface for every segment.
The implementation is checked byte-for-byte against `hash/fnv` over empty, short,
and segment-sized inputs.

| Benchmark | Before | After | Result |
| --- | ---: | ---: | ---: |
| Simple authenticator round trip | 3.622µs | 3.514µs | -2.98%, p=0.000 |
| Authenticator bytes per operation | 0 | 0 | unchanged |
| Authenticator allocations | 0 | 0 | unchanged |

The full mKCP round-trip time showed no significant change (`p=0.551`, `n=12`),
so the focused gain does not introduce an end-to-end regression.

## Verification

- FNV reference-equivalence and authenticator round-trip tests: green.
- `go test ./transport/mkcp -count=1`: green.
- `staticcheck ./transport/mkcp`: green.
- `go test -race ./transport/mkcp -count=1`: green.
