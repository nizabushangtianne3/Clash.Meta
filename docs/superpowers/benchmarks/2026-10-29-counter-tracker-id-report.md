# Counter tracker IDs

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `464c37aa7`.

## Change

TCP and UDP trackers previously generated a CSPRNG UUID for every connection.
These UUIDs are local API handles mapped in the statistic manager, not security
tokens. They now use a process-local atomic counter while retaining valid
version/variant bits, so the manager map and external API still see unique UUIDs
without a kernel random read per connection.

## Benchmark

`go test ./tunnel/statistic -run '^$' -bench '^BenchmarkNewTCPTracker$' -benchmem -count=10`

- NewTCPTracker: 739.0ns -> 516.3ns, `-30.14%` (p=0.000)
- Allocations remain 9 allocs/op; the reduction removes CSPRNG latency from
  connection setup.
- Join/Leave showed a noisy p=0.019 regression in the paired run; rerun below is
  the authoritative check for that unchanged path.

## Rerun

`go test ./tunnel/statistic -run '^$' -bench '^BenchmarkTrackerJoinLeave$' -benchmem -count=10`

Before rerun median samples: 100.3n, 102.4n, 110.3n, 110.5n, 121.2n, 121.7n,
121.6n, 133.0n, 146.5n, 168.7n (median 121.45n).
After rerun samples: 100.8n, 101.6n, 102.3n, 103.6n, 105.1n, 105.1n, 105.5n,
105.6n, 107.3n, 110.1n (median 105.3n), compared with the paired before median
121.45n.

## Verification

- New uniqueness/version/variant test for tracker IDs.
- `go test ./tunnel/statistic -count=1`
- `go test -race ./tunnel/statistic -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 1`
- `CGO_ENABLED=0 go test .`
- `CGO_ENABLED=0 go vet . ./cmd/...`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet -tags=with_gvisor . ./mobile ./tun ./platform`
