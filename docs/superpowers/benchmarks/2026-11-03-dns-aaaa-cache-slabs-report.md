# DNS all-AAAA cache slabs

Date: 2026-11-03

The all-A cache-hit and cache-write slabs now have an all-AAAA counterpart.
A successful IPv6 reply with one or two direct AAAA answers, no authority
records and only discardable OPT additional records can reuse the same
caller-owned slab strategy. Mixed records, CNAME chains, larger answers,
authority sections and non-OPT additional records still use the complete copy
path.

New paired benchmarks, n=10 at `-cpu 1`, show:

- cache hit: 283.0ns/320B/3 allocs -> 222.7ns/352B/1 alloc
  (time -21.31%, allocations -66.67%, bytes +10.00%)
- cache write: 246.2ns/344B/4 allocs -> 199.8ns/352B/1 alloc
  (time -18.83%, allocations -75.00%, bytes +2.33%)

Verification:

- `go test ./dns -run 'TestCacheHitTwoAAAA|TestPutMsgToCacheClonesAAAA|TestCopyCachedMsg|TestExchangeContext' -count 1`
- `go test -race ./dns -count 1`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
