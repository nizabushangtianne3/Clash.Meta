# TUIC v4 fixed-frame fast path

Date: 2026-10-31

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
The paired benchmark writes and reads a 1350-byte IPv4 TUIC v4 packet; n=10.

| Benchmark | Baseline | Current | Result |
| --- | ---: | ---: | ---: |
| PacketWriteTo | 53.27 ns | 35.69 ns | -32.99% |
| ReadPacket | 112.4 ns | 105.9 ns | -5.74% |
| ReadPacketInto | 117.4 ns | 113.0 ns | -3.71% |
| Three-path geomean | 88.90 ns | 75.32 ns | -15.27% |

The `bytes.Buffer` write path packs the eight fixed packet-header bytes into
one write and emits the address and payload through concrete buffer writes.
The read path peeks the fixed association/length header and address port from
`bufio.Reader` or `BufferedConn`. Generic readers and writers retain the
prior byte-wise wire path; behavior is pinned for concrete and fallback writers.

## Verification

- `go test ./transport/tuic/v4 -count=1`
- `go test -race ./transport/tuic/v4 -count=5`
- Paired `benchstat`, n=10
