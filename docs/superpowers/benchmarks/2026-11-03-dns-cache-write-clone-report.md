# DNS cache write clone

Date: 2026-11-03

`putMsgToCache` previously used `dns.Msg.Copy`, cloned the RFC-forbidden OPT
record, and then filtered it from the cloned extra slice. Typical A replies
also paid separate allocations for each A record and payload. Cache writes now
use the cached-message slab cloner directly: OPT is skipped before cloning,
Question is cloned independently, A/AAAA payloads live in record slabs, and all
retained RR interface slices share one backing array.

Paired `BenchmarkPutMsgToCacheTypical`, n=10 at `-cpu 1`, changes
377.8ns/424B/8 allocs to 304.4ns/328B/4 allocs: time -19.43%, bytes -22.64%,
allocations -50.00% (all p=0.000). Paired `BenchmarkExchangeContextCacheHit`
stays at 304B/3 allocs; time remains within noise (p=0.172).

Verification:

- `go test ./dns -count=1`
- `go test -race ./dns -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
