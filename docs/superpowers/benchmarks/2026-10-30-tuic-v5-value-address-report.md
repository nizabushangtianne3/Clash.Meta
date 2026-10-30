# TUIC v5 value address receive

Date: 2026-10-30

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
The comparison uses the same four-fragment 4096-byte receive benchmark as the
previous round, including the normal omission of the address on non-first
fragments; n=10.

| Metric | Baseline | Current | Result |
| --- | ---: | ---: | ---: |
| Fragment receive time | 754.9 ns | 773.8 ns | no significant change (`p=0.239`) |
| Fragment receive bytes | 4 B | 0 B | -100% |
| Fragment receive allocations | 1 | 0 | -100% |
| Single packet read time | 623.3 ns | 617.5 ns | no significant change (`p=0.239`) |

TUIC addresses now store IPv4/IPv6 values and only allocate a backing slice for
domain targets. The parser reads fixed-width addresses byte-by-byte so the
address does not escape through `io.Reader`, the packet connection no longer
needs address scratch, and fragment bags store payload slices plus the first
packet header instead of copying a full packet per fragment. Unknown address
types now fail explicitly instead of parsing as an empty address.

## Verification

- `go test ./transport/tuic/v5 -count=1`
- `go test -race ./transport/tuic/v5 -count=5`
- Paired `benchstat`, n=10
