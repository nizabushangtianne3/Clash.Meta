# Concrete loopback close tracking

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `e31dfd578`.

## Change

DIRECT loopback wrappers previously allocated a `sync.Once` and captured close
closure for every connection/packet conn just to delete one map key. They now
use concrete tracked-conn types that delete directly from `Close`, preserving
the wrapper lifetime and close semantics without callback indirection.

## Benchmark

`go test ./component/loopback -run '^$' -bench '^BenchmarkLoopbackNewConn$' -benchmem -count=10`

- 381.2ns -> 297.5ns, `-21.96%` (p=0.019; wall time is noisy)
- 144B -> 112B, `-22.22%` (p=0.000)
- 3 -> 2 allocs/op, `-33.33%` (p=0.000)

## Verification

- New registration/removal test for tracked TCP loopback conns.
- `go test ./component/loopback -count=1`
- `go test ./adapter/... ./component/loopback -count=1`
- `go test -race ./component/loopback ./adapter ./adapter/outbound -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 1`
- `CGO_ENABLED=0 go test .`
- `CGO_ENABLED=0 go vet . ./cmd/...`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet -tags=with_gvisor . ./mobile ./tun ./platform`
