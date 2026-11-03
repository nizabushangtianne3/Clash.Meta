# Adapter Type Name Table Report

Date: 2026-11-04

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The paired
benchmark uses `benchstat -count=10 -benchtime=10000000x`.

`AdapterType.String()` is on every proxy JSON document. The long branch chain
is now a bounds-checked static table indexed by the contiguous enum. Unknown
and unpopulated values still return `Unknown`.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `Direct` | 1.112ns | 1.071ns | no significant change |
| `Shadowsocks` | 1.119ns | 1.064ns | -4.92%, p=0.000 |
| `URLTest` | 1.135ns | 1.060ns | -6.61%, p=0.003 |
| `GostRelay` | 1.129ns | 1.046ns | -7.39%, p=0.008 |
| geomean | 1.124ns | 1.060ns | -5.66% |

Memory and allocations remain `0 B/op / 0 allocs/op`. A new test walks every
adapter enum value and asserts invalid values stay `Unknown`.

## Verification

- `go test ./constant`: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p=2`: green.
- Android `arm64` build: green.
