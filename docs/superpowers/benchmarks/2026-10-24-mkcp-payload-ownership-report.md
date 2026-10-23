# mKCP Payload Ownership Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=6 -benchtime=10000x` against `fae790e2f` in a separate worktree.

## Change

A sent mKCP segment now owns a pooled payload. The connection's flush path takes a
reference before releasing the connection lock; ACK or terminal-state cleanup marks
the segment retired, and the final flush reference returns the payload to the pool.
This closes the use-after-recycle window that made a direct pool insertion unsafe.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `WriteSegmentPayloadLifecycle` | 689.4ns | 364.4ns | -47.14% |
| Bytes per segment lifecycle | 1570 B | 304 B | -80.64% |
| Allocations per segment lifecycle | 5 | 4 | -20.00% |

The remaining allocations are the segment object, flush result slice, and notify
channel state; they do not retain the large payload and are not worth adding
coupled object pools without a profile showing pressure.

## Verification

- `TestWriteSegmentReleasesItsPayloadAfterAck`: green; it checks an empty send
  window, a nil payload after the final flush reference, and the allocation ceiling.
- Plain, seeded-AEAD and all header round trips: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p 2`: green.
- `go test -tags with_gvisor ./... -count=1 -p 2`: green.
- `go test -race ./transport/mkcp -count=1`: green.
- `go test -race ./listener/inbound -run '^TestInboundShadowSocks_KcpTun$' -count=1`:
  green.
