# Sing buffered-copy upgrade

Date: 2026-10-29

Core dependency: `github.com/metacubex/sing v0.5.8 -> v0.5.9`.

Upstream commit `c0accf942a` fixes buffered-copy ordering: cached reader data is
now drained before reading upstream. This removes out-of-order relay data on
readers that expose cached bytes, which is a correctness fix on the forwarding
path; the second upstream commit only updates dependency metadata.

The FlClash wrapper pin was advanced to the same version before local verification.

## Verification

- `go vet ./...`
- `go test ./... -count=1 -p 1`
- `CGO_ENABLED=0 go vet . ./cmd/...`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet -tags=with_gvisor . ./mobile ./tun ./platform`
- FlClash wrapper: `go mod tidy`, `go test .`, and `go vet . ./cmd/...`
