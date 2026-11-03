# Pre-sized provider JSON

Date: 2026-11-03

Provider documents append dozens of proxy rows into a pooled response, but the
buffer was unsized. `appendProviderJSON` now computes a conservative capacity
from provider metadata, proxy names, timestamps and subscription fields, then
calls `slices.Grow` before encoding. Unusual escaping can still grow without
changing the wire bytes.

`BenchmarkGetProvidersResponse` with 8 providers and 64 proxies is unchanged in
aggregate B/op because the benchmark Recorder must copy the response, but its
memory profile moves provider and newline growth from repeated steady-state
allocations to warm-up. Provider and route tests pin the exact JSON shape.

Verification:

- `go test ./adapter/provider ./hub/route -count=1`
- `go test -race ./adapter/provider ./hub/route`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
