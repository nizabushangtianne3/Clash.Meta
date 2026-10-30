# TUIC v4 value address receive

Date: 2026-10-31

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
The paired benchmark uses a 1350-byte IPv4 TUIC v4 packet, n=10.

| Benchmark | Baseline | Current | Result |
| --- | ---: | ---: | ---: |
| ReadPacket time | 133.6 ns | 110.2 ns | -17.51% |
| ReadPacket allocations | 1 | 0 | -100% |
| ReadPacketInto time | 113.3 ns | 117.9 ns | +4.06% |
| PacketWriteTo time | 50.65 ns | 53.02 ns | +4.66% |
| Three-path geomean | 91.54 ns | 88.33 ns | -3.51% |

TUIC v4 addresses now store IPv4/IPv6 values and allocate only for domain
targets. Fixed-width reads use buffered peeking without exposing a stack array
through `io.Reader`; writes use the concrete `bytes.Buffer` fast path when
available. The packet connection no longer carries a 256-byte address scratch,
public `ReadPacket` no longer allocates an address per packet, and unknown
address types fail explicitly instead of parsing as an empty address.

## Verification

- `go test ./transport/tuic/v4 -count=1`
- `go test -race ./transport/tuic/v4 -count=5`
- Paired `benchstat`, n=10
