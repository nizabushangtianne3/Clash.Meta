# DNS all-A cache write slab

Date: 2026-11-03

The common cache-write shape previously cloned the message, question slice,
RR interface slice and A payload slab separately. When a reply holds one or two
A records, no authority records, one question and only RFC-forbidden OPT records
in the additional section, those caller-owned objects now share one slab. The
OPT records are dropped before caching and the slab is fully independent of the
caller, preserving mutation isolation.

Paired `BenchmarkPutMsgToCacheTypical`, n=10 at `-cpu 1`, changes
293.6ns/328B/4 allocs to 190.3ns/320B/1 alloc: time -35.16%, bytes -2.44%,
allocations -75.00% (all p=0.000). The paired cache-hit benchmark stays at
320B/1 alloc and time within noise.

Verification:

- `go test ./dns -run 'TestPutMsgToCache|TestCacheHit|TestCopyCachedMsg|TestExchangeContext' -count 1`
- `go test -race ./dns -count 1`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
