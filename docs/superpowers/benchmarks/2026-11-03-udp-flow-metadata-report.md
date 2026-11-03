# UDP flow metadata slab

Date: 2026-11-03

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
n=10 per side. The baseline is `d490036a8`.

## Change

`newUDPFlowMetadata` previously made one clone allocation and then allocated a
second three-field origin snapshot. The origin snapshot and mutable dial copy
now live in one caller-owned `udpFlowMetadata` slab. `handleUDPConn` passes that
flow to the async dial goroutine; origin stays immutable and dial is the only
copy prepared by `preHandleMetadata`.

## Gate

`TestNewUDPFlowMetadataSharesOneClone` pins the immutable origin snapshot and
the independent dial copy. `BenchmarkNewUDPFlowMetadata` measures the allocation
shape on the first packet of every UDP flow.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 173.4n ± 5% | 832.0 | 2.000 |
| current | 155.5n ± 4% | 896.0 | 1.000 |
| delta | -10.38% (p=0.000) | +7.69% | -50.00% |

The paired benchmark keeps one allocation on this first-packet path while
trading 64 bytes of slab capacity for the lower allocation count.

## Verification

- `go test ./tunnel -count=1` green.
- `go test -race ./tunnel -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
