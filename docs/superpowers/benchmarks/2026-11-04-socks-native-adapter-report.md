# Native socks packet adapter

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. Both benchmark arms run on the same current tree:
`wrapped` reproduces the old allocation, while `native` exercises the new
type-asserted path.

## Change

The socks UDP wrapper now retains its source as an `AddrPort`, cached metadata
pointer, and implements both `FlowKey` and `Metadata`, so it satisfies
`C.PacketAdapter`. The first packet stores the metadata returned by
`inbound.NewPacket`; later packets carry the per-source cached pointer. The
tunnel uses the native adapter directly instead of allocating another generic
wrapper for every datagram.

## Gate

`TestSocksPacketIsNativePacketAdapter` verifies native adapter identity,
metadata, flow key, and zero allocations. `BenchmarkSocksPacketAdapter` compares
the old wrapped path and the new native path.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| wrapped | 32.83n ± 8% | 64.0 | 1.000 |
| native | 2.156n ± 1% | 0.0 | 0.000 |
| delta | -93.43% | -100.00% | -100.00% |

## Verification

- `go test ./listener/socks -count=1` green.
- `go test -race ./listener/socks -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
