# Segment ring copies

Date: 2026-11-03

`Queue.Copy` indexed every element with a modulo operation. The ring is
contiguous except across its boundary, so it now performs one `copy` when the
oldest-to-newest span does not wrap and two `copy` calls when it does. Range
keeps its callback shape.

The ten-row `BenchmarkQueueCopyTen` pair improves from 178.5ns to 104.2ns
(-41.60%, p=0.000, n=10); bytes and allocations stay at 320B and 1.

Verification:

- `go test ./common/queue ./adapter -count=1`
- `go test -race ./common/queue ./adapter`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
