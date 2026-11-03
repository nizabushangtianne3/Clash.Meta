# Packet conn local address cache

Date: 2026-11-03

`packetConn.LocalAddr` rebuilt the adapter's custom address on every call even
though the packet conn, adapter name, process-local ID and upstream local
address are immutable. The wrapper is now built once in `NewPacketConn` and
returned by reference on later reads. This keeps the stable quic-go conn
multiplexer identity while removing a repeated heap allocation.

Paired `BenchmarkPacketConnLocalAddr`, n=10 at `-cpu 1`, changes
44.07ns/48B/1 alloc to 1.32ns/0B/0 allocs: time -97.01%, bytes -100.00%,
allocations -100.00% (all p=0.000).

Verification:

- `go test ./adapter/outbound -count=1`
- `go test -race ./adapter/outbound -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
