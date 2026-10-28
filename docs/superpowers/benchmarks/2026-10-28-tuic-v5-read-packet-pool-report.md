# tuic v5 read packet buffer pool

Date: 2026-10-28

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`. The
baseline is the pre-cycle tip `5b5e4316` measured in a worktree with the
identical harness (`ReadPacket` followed by a `pool.Put`, the single-owner
lifecycle the reader hands the buffer through), n=10 per side.

## Change

The udp-over-quic-stream packet reader allocated a fresh `make([]byte, c.SIZE)`
per datagram, about 1.4 KiB at the typical payload size, and the server's
`serverUDPPacket.Drop` simply dropped the reference. A single-fragment packet
has exactly one owner: the reader that walks it up to the tunnel, whose
`processPacket` consumes the data and only then drops the wrapper. That path
now draws the buffer from the pool and returns it on Drop; the fragmented
path keeps its allocation, because the de-fragmenter holds the fragment
buffers until the message completes and reuses the same `Packet` values.

## Gate

`transport/tuic/v5/packet_perf_test.go` measures the shape with the buffer
returned to the pool, matching the production lifecycle.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 318.4n ± 4% | 1428.0 | 3.000 |
| current | 125.8n ± 6% | 4.0 | 1.000 |
| delta | -60.50% (p=0.000) | -99.72% | -66.67% |

The remaining allocation is the four-byte address slice `ReadAddress` makes
per packet, which is per-packet data that changes with every datagram.

## Verification

- `go test ./transport/tuic/... ./listener/tuic/... ./listener/inbound/... -count=1` green.
- `go test -race ./transport/tuic/v5 -count=1` green.
- `go vet ./transport/tuic/v5` clean.
