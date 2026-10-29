# Local verification round

Date: 2026-10-29

Baseline: `a65dbdf8a`. This covers `623387946` (binary UDP adapter keys) and
the debounced runtime-GC follow-up.

- `go vet ./...`
- `go test ./... -count=1 -p 1`
- `go test -race ./constant ./tunnel ./component/sniffer ./hub/executor -count=1`
- `CGO_ENABLED=0 go test .`
- `CGO_ENABLED=0 go vet . ./cmd/...`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet -tags=with_gvisor . ./mobile ./tun ./platform`
