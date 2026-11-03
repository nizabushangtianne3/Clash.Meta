# Proxy Empty Extra Map Report

Date: 2026-11-04

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The focused
benchmark uses `benchstat -count=10 -benchtime=100000x`; the provider API uses
`benchstat -count=10 -benchtime=200x`.

Most proxies have no per-test-URL `extra` state. `Proxy.AppendJSON` still
entered the concurrent map for every proxy, which initialized and then dropped
an empty table. Empty extras now emit `{}` directly. Non-empty extras keep the
sorted-key behavior and now preallocate the URL slice from the map size.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `ProxyAppendJSONEmptyExtra` | 144.80ns | 94.92ns | -34.44%, p=0.000 |
| `GetProvidersResponse` | 129.5µs | 103.8µs | -19.85%, p=0.000 |

Memory and allocations stay flat in both focused and API comparisons. The
existing provider contract test and proxy delay-state tests cover empty and
populated `extra` documents.

## Verification

- `go test ./adapter`: green.
- Adapter race: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p=2`: green.
- Android `arm64` build: green.
