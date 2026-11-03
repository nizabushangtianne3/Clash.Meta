# Single-copy delay histories

Date: 2026-11-03

`DelayHistory`, `DelayHistoryForTestUrl` and `ExtraDelayHistories` copied a
queue into an intermediate slice, then copied that slice again into the API
result. Each path now returns the queue's one defensive copy directly; a missing
per-URL state returns nil instead of allocating an empty slice.

New benchmarks with ten stored rows show paired n=10 results:
`ProxyDelayHistory` 400ns/640B/2 allocs to 221ns/320B/1 alloc, and
`ProxyExtraDelayHistories` 1.044us/1104B/4 allocs to 868ns/784B/3 allocs.
All differences are p=0.000.

Verification:

- `go test ./adapter -count=1`
- `go test -race ./adapter`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
