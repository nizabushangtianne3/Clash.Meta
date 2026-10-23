# mKCP Zero-Copy Receive Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=12 -benchtime=300x` against `490b262f8` in a separate worktree.

## Change

Receive loops now read into pooled MTU-sized buffers and data segments retain
references to those buffers. Control-only datagrams are released immediately,
multi-segment datagrams are released by their final data segment, and out-of-order
segments keep the buffer until delivery or connection cleanup. Listener shutdown
and full client queues release unparsed buffers instead of leaking them.

The GCM writer overhead now includes its nonce. Without that correction, a seeded
mKCP packet exceeds the configured MTU and an MTU-sized receive buffer truncates it.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip time | 214.4µs | 190.2µs | -11.30%, p=0.000 |
| Bytes per round trip | 39.280 KiB | 3.428 KiB | -91.27%, p=0.000 |
| Allocations per round trip | 77.00 | 33.00 | -57.14%, p=0.000 |

## Verification

- Plain, seeded-AEAD, and all header round trips: green.
- Shared-buffer, control-only release, and out-of-order ownership tests: green.
- `go test ./transport/mkcp -count=1`: green.
- `staticcheck ./transport/mkcp`: green.
- `go test -race ./transport/mkcp -count=1`: green.
