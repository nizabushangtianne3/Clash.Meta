# JSON Safe ASCII Fast Path Report

Date: 2026-11-04

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The paired
benchmark uses `benchstat -count=10 -benchtime=1000000x`.

API payloads are usually plain ASCII. A 128-entry ASCII escape table now
detects strings that `encoding/json` writes byte-for-byte and copies them in one
append. Escaped, Unicode, and invalid-UTF-8 strings keep the original exact
encoder path, including HTML escaping and replacement of invalid bytes.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `SafeASCII32` | 47.53ns | 16.58ns | -65.12%, p=0.000 |
| `SafeASCII256` | 296.9ns | 111.7ns | -62.37%, p=0.000 |
| `Escaped32` | 73.16ns | 74.15ns | no significant change |
| `Unicode32` | 144.2ns | 147.2ns | no significant change |

Bytes and allocations remain `0 B/op / 0 allocs/op`. Encoder parity tests add
plain ASCII and DEL, while existing cases cover quotes, HTML characters,
controls, Unicode line separators, and invalid UTF-8.

## Verification

- `go test ./common/jsonutil`: green.
- Changed-package race: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p=2`: green.
- Android `arm64` build: green.
