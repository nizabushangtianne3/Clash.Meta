# mKCP Zero-Allocation Segment Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=6 -benchtime=10000x` against `a5149f494`.

## Change

The remaining per-segment write-lifecycle allocations are gone:

- ACK notification rebuilds a channel only when a writer is actually waiting.
- A retired data segment returns to a segment object pool after its payload and
  final flush reference are released.
- One connection-owned batch carries ACK, data and command segments through each
  flush and is cleared after writing.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `WriteSegmentPayloadLifecycle` | 292.5ns | 200.6ns | -31.42% |
| Bytes per segment lifecycle | 192 B | 0 B | -100.00% |
| Allocations per segment lifecycle | 3 | 0 | -100.00% |

The pool boundary remains explicit: payload, segment object and flush references
are released only after ACK retirement and the final writer call. The flush batch is
cleared so it does not retain segment pointers between intervals.

## Verification

- Focused queue-signal, ACK-signal, segment-reuse and zero-allocation tests: green.
- Plain, seeded-AEAD and all header round trips: green.
- mKCP package race and inbound KCP interop race: green.
- `go vet ./...`, full tests and the `with_gvisor` suite: green.
