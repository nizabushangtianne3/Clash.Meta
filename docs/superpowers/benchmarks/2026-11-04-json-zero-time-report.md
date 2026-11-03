# JSON Zero-Time Append Report

Date: 2026-11-04

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The paired
benchmark uses `benchstat -count=10 -benchtime=1000000x`.

Rule and API documents append untouched hit/miss timestamps repeatedly. Those
zero `time.Time` values now use one precomputed RFC3339 wire form instead of
running the full formatter for every record.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `AppendTime/zero` | 32.33ns | 3.957ns | -87.76%, p=0.000 |
| `AppendTime/value` | 49.57ns | 50.12ns | no significant change |

Bytes and allocations remain zero in both paths. A JSON contract test compares
zero, UTC, and fixed-zone values against `encoding/json`.

## Verification

- `go test ./common/jsonutil ./hub/route`: green.
- Changed-package race: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p=2`: green.
- Android `arm64` build: green.
