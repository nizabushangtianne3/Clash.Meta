# Adapter dial options cache

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `0b3596462`.

## Change

Outbound dial policy is fixed when an adapter is constructed, so `Base` now
builds its dial options once. `DialOptions` returns that immutable slice with
equal length and capacity; callers appending private options therefore allocate
a fresh array instead of overwriting the shared backing array.

## Gate

`go test ./adapter/outbound -run '^$' -bench '^BenchmarkBaseDialOptions' -benchmem -count=10`

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Empty options | 3.17n -> 0.349n (-88.98%) | 0 -> 0 | 0 -> 0 |
| Full options | 94.43n -> 0.335n (-99.65%) | 136 -> 0 (-100%) | 5 -> 0 (-100%) |

The paired captures use `benchstat`; all deltas above have p=0.000 with n=10.

## Verification

- `go test ./adapter/... -count=1`
- `go test -race ./adapter/outbound ./adapter -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 1`
- `CGO_ENABLED=0 go test .`
- `CGO_ENABLED=0 go vet . ./cmd/...`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet -tags=with_gvisor . ./mobile ./tun ./platform`
