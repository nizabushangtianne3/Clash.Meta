# Direct dialer cache

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `b2d8af5da`.

## Change

`Direct` now owns one dialer built with its immutable dial policy and the
direct-host resolver. TCP and UDP flows reuse it directly; the resolved
AddrPort path also avoids rebuilding the option slice. A `Dialer.DialContextAddrPort`
method carries the cached policy without converting through a variadic rebuild.

This removes repeated option application from every direct flow and keeps the
direct resolver policy in one authoritative place.

## Verification

- `go test ./adapter/... ./component/dialer -count=1`
- `go test -race ./adapter/outbound ./adapter ./component/dialer -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 1`
- `CGO_ENABLED=0 go test .`
- `CGO_ENABLED=0 go vet . ./cmd/...`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet -tags=with_gvisor . ./mobile ./tun ./platform`

This is a hot-flow simplification without a new microbenchmark: the prior
adapter-options round already gates the policy construction, and this change
removes its repeated application rather than introducing a new algorithm.
