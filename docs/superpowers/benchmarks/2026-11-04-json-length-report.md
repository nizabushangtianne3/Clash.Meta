# JSON Safe String Length Report

Date: 2026-11-04

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. Comparisons use
`benchstat -count=10 -benchtime=1000000x`.

`StringLen` pre-sizes provider and proxy-group JSON documents. It now scans for
the first byte that JSON would rewrite, returns immediately for a fully safe
ASCII string, and resumes the encoder accounting only after that boundary.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `StringLenSafeASCII32` | 53.98ns | 17.32ns | -67.91%, p=0.000 |
| `StringLenSafeASCII256` | 382.9ns | 122.5ns | -68.01%, p=0.000 |
| `StringLenUnicode32` | 126.8ns | 133.6ns | no significant change |
| `StringLenEscaped32` | 64.97ns | 64.43ns | no significant change |

The paired `AppendString` benchmark stayed flat, so the safe append path added
earlier is unchanged:

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `AppendStringSafeASCII32` | 16.47ns | 16.89ns | no significant change |
| `AppendStringSafeASCII256` | 108.8ns | 113.8ns | no significant change |
| `AppendStringEscaped32` | 77.10ns | 77.04ns | no significant change |
| `AppendStringUnicode32` | 146.3ns | 148.2ns | no significant change |

Memory and allocations remain zero in every case.

## Verification

- `go test ./common/jsonutil`: green.
- Changed-package race: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p=2`: green.
- Android `arm64` build: green.
