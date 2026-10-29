# Inline first UDP mapping

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `4b2ebfe58`.

## Change

Most UDP flows have one origin/destination NAT mapping, but every sender
previously allocated both indexed maps at creation. The first mapping now stays
inline in `packetSender`; the maps are allocated only when a second origin
requires indexed lookup. The existing first-origin cache doubles as the inline
forward lookup, while reverse replies check the inline pair before the maps.
An unresolved first target can still be upgraded in place, preserving the old
first-write semantics.

## Benchmark

`go test ./tunnel -run '^$' -bench '^(BenchmarkPacketMappingFirstAdd|BenchmarkPacketOriginLookup|BenchmarkPacketMappingRestore)$' -benchmem -count=10`

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| FirstAdd | 910.0n -> 611.0n (-32.86%, p=0.000) | 3888 -> 2832 (-27.16%, p=0.000) | 9 -> 5 (-44.44%, p=0.000) |
| OriginLookup | 5.260n -> 5.092n (-3.18%, p=0.000) | 0 -> 0 | 0 -> 0 |
| MappingRestore | 2.471n -> 2.452n (within noise, p=0.956) | 0 -> 0 | 0 -> 0 |

## Verification

- New tests cover inline retention, promotion for multiple origins and
  unresolved-target upgrade.
- `go test ./tunnel -count=1`
- `go test -race ./tunnel -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 1` (an initial listener/inbound run failed while
  the host was under heavy listener churn; the package and the full retry both
  passed)
- `CGO_ENABLED=0 go test .`
- `CGO_ENABLED=0 go vet . ./cmd/...`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet -tags=with_gvisor . ./mobile ./tun ./platform`
