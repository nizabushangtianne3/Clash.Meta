# Preserve pooled JSON newline capacity

Date: 2026-11-03

The pooled API writers returned JSON documents whose capacity exactly matched
the encoded document. `writeJSONDocument` then appended the protocol newline,
which grew a fresh response-sized allocation every time the capacity was full.
It now appends the newline, writes once, and returns the larger slice so the
pooled body keeps spare newline capacity for the next poll.

The 64-proxy `BenchmarkGetProxiesResponse` aggregate allocation count is
unchanged because recorder/request construction dominates it, but an
allocation profile attributes `writeJSONDocument` growth to the first warm-up
only rather than every iteration. Endpoint behavior and byte output are pinned
by existing route tests.

Verification:

- `go test ./hub/route -count=1`
- `go test -race ./hub/route`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
