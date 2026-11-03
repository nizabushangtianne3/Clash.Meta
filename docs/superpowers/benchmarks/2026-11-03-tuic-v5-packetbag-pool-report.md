# TUIC v5 packet bag pool

Date: 2026-11-03

A fragmented datagram allocated one `packetBag` per message even though the
bag is reset and removed as soon as assembly completes or eviction occurs.
Bags now live in a pool. Each reset bumps an atomic generation, and a caller
must claim the captured generation under the bag lock before mutating it; a
stale caller drops only its input and never touches the pooled bag. A new
concurrency test covers stale-generation rejection, while the existing
completed-bag duplicate race stays in place.

Paired `BenchmarkFeedFragmentedReceive`, n=10 at `-cpu 1`, changes
1.1285us/288B/1 alloc to 848.5ns/0B/0 allocs: time -24.82%, bytes -100.00%,
allocations -100.00% (all p=0.000).

Verification:

- `go test ./transport/tuic/v5 -count=1`
- `go test -race ./transport/tuic/v5 -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
