# DNS one-A cache slabs

Date: 2026-11-03

The most common successful DNS reply holds one A record. The generic all-A
two-record slab over-allocated that shape by 80 bytes. Cache hits and writes now
use a one-record slab; all other shapes retain the existing all-A, all-AAAA or
general copying paths.

Paired n=10 benchmarks at `-cpu 1`:

- cache hit: 187.4ns/320B/1 alloc -> 168.0ns/240B/1 alloc
  (time -10.35%, bytes -25.00%, allocations unchanged, p=0.000)
- cache write: 160.5ns/320B/1 alloc -> 141.6ns/240B/1 alloc
  (time -11.80%, bytes -25.00%, allocations unchanged, p=0.000)

A new write/hit isolation test mutates the question, answer and discarded OPT
after caching and verifies the cached owner is unchanged.

Verification:

- `go test ./dns -run 'TestOneACache|TestCopyCachedMsg|TestExchangeContext|TestPutMsgToCache' -count 1`
- `go test -race ./dns -count 1`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
