# Native tproxy packet adapter

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. Both benchmark arms run on the same current tree:
`wrapped` reproduces the old allocation, while `native` exercises the new
type-asserted path.

## Change

The tproxy UDP wrapper now retains its cached metadata pointer and exposes
`Metadata`, so it satisfies `C.PacketAdapter`. The first packet stores the
metadata returned by `inbound.NewPacketFromAddrPort`; later packets carry the
per-source cached pointer. The tunnel uses the native adapter directly instead
of allocating another generic wrapper for every datagram.

## Gate

`TestTProxyPacketIsNativePacketAdapter` verifies native adapter identity,
metadata, flow key, and zero allocations. `BenchmarkTProxyPacketAdapter`
compares the old wrapped path and the new native path.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| wrapped | 23.15n ± 7% | 64.0 | 1.000 |
| native | 1.285n ± 1% | 0.0 | 0.000 |
| delta | -94.45% | -100.00% | -100.00% |

## Verification

- `go test ./listener/tproxy -count=1` green.
- `go test -race ./listener/tproxy -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
