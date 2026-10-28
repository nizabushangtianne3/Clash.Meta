# tuic v4 read packet buffer pool

Date: 2026-10-28

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the pre-cycle tip `5b5e4316` measured in a worktree with the same
new benchmark harness (`ReadPacket` followed by a `pool.Put`, the lifecycle the
reader hands the buffer through), n=10 per side.

## Change

The legacy protocol's packet reader allocated `make([]byte, c.LEN)` per datagram
and every consumer simply dropped the reference. The v4 relay readers hand the
buffer to exactly one owner as well: the native mode re-sends the raw datagram
and only needed the parse for the assoc id, the quic mode re-serializes through
`WriteTo` which copies the payload out, and the server wrapper's `Drop` runs
after the sender wrote the datagram. The read now draws the buffer from the pool
and each of those three sites returns it.

## Gate

`transport/tuic/v4/packet_perf_test.go` measures the read with the buffer
returned and pins the payload and address round trip through the pool.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 375.4n ± 11% | 1440.0 | 6.000 |
| current | 179.6n ± 35% | 16.0 | 4.000 |
| delta | -52.16% (p=0.000) | -98.89% | -33.33% |

The remaining allocations are the two `binary.Read` escapes into the assoc id
and length, the address slice `ReadAddress` makes, and the value the benchmark
holds across the call. Replacing the two `binary.Read` calls changes a public
method's partial-read behaviour, so it stayed out of this change.

## Verification

- `go test ./transport/tuic/... -count=1` green.
- `go test -race ./transport/tuic/v4 -count=1` green.
- `go vet ./transport/tuic/...` clean.
