# Segmented ring range

Date: 2026-11-03

`Queue.Range` evaluated a modulo index for every element. The valid ring is
contiguous except at its boundary, so it now ranges over one slice or the tail
plus head slices while preserving early-stop semantics.

The ten-row `BenchmarkQueueRangeTen` pair improves from 87.09ns to 27.48ns
(-68.45%, p=0.000, n=10); allocations stay at zero.

Verification:

- `go test ./common/queue ./adapter -count=1`
- `go test -race ./common/queue ./adapter`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
