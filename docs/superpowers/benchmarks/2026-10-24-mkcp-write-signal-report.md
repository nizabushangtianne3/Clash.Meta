# mKCP Write Signal Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=6 -benchtime=10000x` against `d890d6724`.

Queueing another segment consumes send-window space and cannot unblock a writer
waiting for space. The signal therefore only ran after ACK or state cleanup removed
segments. Keeping the existing notification channel in place removes one channel
allocation and one scheduler wakeup per segment.

| Benchmark | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `WriteSegmentPayloadLifecycle` | 364.4ns | 292.5ns | -19.74% |
| Bytes per segment lifecycle | 304 B | 192 B | -36.84% |
| Allocations per segment lifecycle | 4 | 3 | -25.00% |

Verification:

- Focused write-signal, ACK payload-ownership and protocol round trips: green.
- `go vet ./...`, full tests and the `with_gvisor` suite: green.
- mKCP package race and inbound KCP interop race: green.
