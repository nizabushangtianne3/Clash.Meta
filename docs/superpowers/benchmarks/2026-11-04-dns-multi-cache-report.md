# DNS Multi-Address Cache Slab Report

Date: 2026-11-04

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. Comparisons use
`benchstat` with 10 samples.

## Change

Cache hits for 3-8 all-A or all-AAAA answers now use one slab instead of one
message allocation plus separate per-record payload allocations. The fixed
8-record bound deliberately keeps larger and mixed responses on the generic
copy path. Cache writes still discard OPT and clone the question before the
slab is stored.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `CacheHitFourA` | 233.6ns, 464 B, 3 allocs | 183.2ns, 896 B, 1 alloc | -21.58% time, -66.67% allocs |
| `CacheHitFourAAAA` | 226.5ns, 496 B, 3 allocs | 166.5ns, 896 B, 1 alloc | -26.49% time, -66.67% allocs |

Bytes per four-address hit increase because the slab reserves eight slots.
That is the deterministic trade for a single allocation and lower latency.

## Verification

- `go test -race ./dns`: green.
- `go vet ./...`: green.
- `go test ./... -count=1`: green.
- Android `arm64` build: green.
- Focused four-record A/AAAA cache-hit independence and discard-OPT cases: green.
