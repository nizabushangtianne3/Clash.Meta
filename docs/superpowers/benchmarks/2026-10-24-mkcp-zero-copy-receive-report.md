# mKCP Zero-Copy Receive Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=12 -benchtime=300x` against `aa86230ab` in a separate worktree.

## Change

Receive loops now read into pooled MTU-plus-AEAD-nonce buffers and data segments
retain references to those buffers. Control-only datagrams are released immediately,
multi-segment datagrams are released by their final data segment, and out-of-order
segments keep the buffer until delivery or connection cleanup. Listener shutdown
and full client queues release unparsed buffers instead of leaking them.

The GCM writer overhead includes its nonce, while the receive buffer covers a V2Ray
peer that counts that nonce outside its configured MTU.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip time | 227.3us | 211.2us | -7.08%, p=0.000 |
| Round-trip throughput | 68.75 MiB/s | 73.99 MiB/s | +7.62%, p=0.000 |
| Bytes per round trip | 39.447 KiB | 3.685 KiB | -90.66%, p=0.000 |
| Allocations per round trip | 77.50 | 34.00 | -56.13%, p=0.000 |

## Verification

- Plain, seeded-AEAD, and all header round trips: green.
- V2Ray seeded mKCP interoperability: green after sizing receive buffers to 1362
  bytes for the default 1350-byte MTU.
- Shared-buffer, control-only release, and out-of-order ownership tests: green.
- `go test ./transport/mkcp -count=1`: green.
- `staticcheck ./transport/mkcp`: green.
- `go test -race ./transport/mkcp -count=1`: green.
- `go test -race ./transport/mekya -count=1`: green.
- `go test -race ./listener/inbound -run '^TestInboundShadowSocks_KcpTun$' -count=1`:
  green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p 1 -parallel 1`: green.
- `go test -tags with_gvisor ./... -count=1 -p 1 -parallel 1`: green.
