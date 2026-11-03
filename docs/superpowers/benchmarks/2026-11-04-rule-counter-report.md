# Rule Counter Atomic Result Report

Date: 2026-11-04

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. Each comparison
uses `benchstat -count=10 -benchtime=1000000x`.

`atomic.Uint64.Add` returns the new value, so the rule wrapper no longer
follows every counter increment with a second atomic load before deciding
whether to refresh the folded timestamp.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `RuleWrapperHit` | 19.36ns | 9.093ns | -53.03%, p=0.000 |
| `RuleWrapperMiss` | 13.775ns | 8.795ns | -36.15%, p=0.000 |

Memory and allocations stay at `0 B/op / 0 allocs/op`. The existing folded-clock
tests prove the refresh boundary remains event 1 then every 16 events.

## Verification

- `go test ./rules/wrapper`: green.
- Changed-package race: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p=2`: green.
- Android `arm64` build: green.
