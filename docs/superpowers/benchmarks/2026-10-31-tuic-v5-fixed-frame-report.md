# TUIC v5 fixed-frame fast path

Date: 2026-10-31

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
The comparison covers packet write, single packet read, and four-fragment
receive; both sides ran 10 times.

| Benchmark | Baseline | Current | Result |
| --- | ---: | ---: | ---: |
| PacketWriteTo | 59.55 ns | 36.24 ns | -39.14% |
| ReadPacketInto | 624.6 ns | 610.4 ns | -2.27% |
| Fragment receive | 784.7 ns | 743.4 ns | -5.26% |
| Three-path geomean | 307.9 ns | 254.3 ns | -17.41% |

The `bytes.Buffer` write path packs the ten fixed packet-header bytes into one
write, then emits the address and payload through concrete buffer writes. The
read path peeks fixed-width headers, addresses, and ports from `bufio.Reader`
or `BufferedConn` instead of consuming each byte through an interface. Generic
readers retain the prior byte-wise behavior and generic writers retain the
prior wire path.

## Verification

- `go test ./transport/tuic/v5 -count=1`
- `go test -race ./transport/tuic/v5 -count=5`
- Paired `benchstat`, n=10
