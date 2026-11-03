# Connection JSON Shared String Encoder Report

Date: 2026-11-04

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The paired
benchmark uses `benchstat -count=10 -benchtime=1000000x`.

The connection view duplicated the JSON string encoder. It now calls the
shared `jsonutil.AppendString`, which has the safe-ASCII fast path and retains
identical HTML escaping, control escaping, Unicode line separators and invalid
UTF-8 replacement. The golden connection document and `encoding/json` parity
tests pin the wire format.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `SafeASCII256` | 301.4ns | 128.8ns | -57.26%, p=0.000 |
| `Escaped32` | 72.99ns | 84.18ns | +15.32%, p=0.001 |
| `Unicode32` | 148.8ns | 147.8ns | no significant change |

This is an accepted trade: metadata hosts and domains are commonly safe ASCII,
while escaped connection strings are shorter. Memory and allocations remain
`0 B/op / 0 allocs/op`. The change also removes 79 lines of duplicated encoder
logic.

## Verification

- Golden connection-view document: green.
- `encoding/json` escaping parity: green.
- `go test ./tunnel/statistic`: green.
- Changed-package race: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p=2`: green.
- Android `arm64` build: green.
