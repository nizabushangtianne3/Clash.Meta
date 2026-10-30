# TUIC v5 fragmented receive pool

Date: 2026-10-30

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
The benchmark parses four serialized 1024-byte fragments into a persistent
`deFragger`, omits the address from non-first fragments as the protocol does,
then assembles the 4096-byte datagram; each side ran 10 times.

| Metric | Baseline | Current | Result |
| --- | ---: | ---: | ---: |
| Time | 3.1515 us | 0.7615 us | -75.84% |
| Bytes | 14124 B | 4 B | -99.97% |
| Allocations | 18 | 1 | -94.44% |

Fragment bags now carry values instead of escaped packet pointers and are
reused after assembly. Fragment payloads use the shared byte pool; assembly
performs one exact pooled copy and returns all fragment buffers. A bounded
per-connection map with an oldest-entry guard replaces the generic LRU
list/entry allocations, and a fixed presence bitmap handles zero-length
fragments and duplicates. `Feed` returns a value and payload ownership is
explicit at every rejection, assembly, read-out, and `Drop`.

## Verification

- `go test ./transport/tuic/v5 -count=1`
- `go test -race ./transport/tuic/v5 -count=5`
- Paired `benchstat`, n=10
