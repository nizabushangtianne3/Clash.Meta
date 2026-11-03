# Bounded queue history writes

Date: 2026-11-03

The only production queue use is bounded delay history. The old `Put` variadic
API made callers follow it with `Len` and `Pop`, taking the queue lock three
times per record and keeping an unbounded insertion method alive for one
bounded caller. `PutBounded` now inserts and trims under one lock; `Pop` shares
a locked helper and the unused variadic surface is gone.

The bounded-record benchmark compares the old `Put`/`Len`/`Pop` sequence with
the new method over a full ten-row queue. It improves from 67.85ns to 48.68ns
(-28.26%, p=0.000); allocations stay at zero. Queue tests now cover drop on
overflow, non-positive limits, ring ordering and concurrent bounded writers.

Verification:

- `go test ./common/queue ./adapter -count=1`
- `go test -race ./common/queue ./adapter`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
