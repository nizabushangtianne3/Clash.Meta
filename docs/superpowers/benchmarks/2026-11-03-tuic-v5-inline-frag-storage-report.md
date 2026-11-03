# TUIC v5 inline fragment storage

Date: 2026-11-03

A fragmented TUIC v5 datagram allocated both its bag and a separate slice
header/backing array. Most datagrams fit four fragments, so `packetBag` now
holds those entries inline and keeps one slice view for callers; totals above
four still use an overflow slice.

Paired `BenchmarkFeedFragmentedReceive`, n=10 at `-cpu 1`, changes
1.155us/288B/2 allocs to 1.131us/288B/1 alloc: allocations -50.00%, time
-2.04%, bytes unchanged (all p<=0.015). A five-fragment test covers overflow
and exact assembly; concurrent duplicate and completed-bag safety suites stay
in place.

Verification:

- `go test ./transport/tuic/v5 -count=1`
- `go test -race ./transport/tuic/v5`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
