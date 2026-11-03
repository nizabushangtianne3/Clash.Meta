# DNS all-A cache hit slab

Date: 2026-11-03

A common successful DNS cache hit holds one or two A records, no authority or
additional records. Its returned copy previously allocated the message, an RR
interface slice and one payload slab separately. Those caller-owned objects now
live in a single two-record slab; larger, mixed, AAAA, authority and additional
answers retain the existing copying path.

Paired `BenchmarkExchangeContextCacheHit`, n=10 at `-cpu 1`, changes
277.4ns/304B/3 allocs to 217.1ns/320B/1 alloc: time -21.72%, bytes +5.26%,
allocations -66.67% (time and allocations p=0.000). The +16 bytes are the
oversized slab capacity that makes caller mutation impossible against the
cached message.

Verification:

- `go test ./dns -run 'TestCacheHitThreeARecords|TestCopyCachedMsg|TestExchangeContext' -count 1`
- `go test -race ./dns -count 1`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
