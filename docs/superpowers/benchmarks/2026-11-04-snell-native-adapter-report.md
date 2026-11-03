# Native snell packet adapter

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. Both benchmark arms run on the same current tree:
`wrapped` reproduces the old allocation, while `native` exercises the new
type-asserted path.

## Change

The snell UDP wrapper now retains its cached metadata pointer and exposes
`Metadata`, so it satisfies `C.PacketAdapter`. The per-flow metadata is known
before packet creation, and the tunnel uses the native adapter directly instead
of allocating another generic wrapper for every datagram.

## Gate

`TestSnellPacketIsNativePacketAdapter` verifies native adapter identity,
metadata, flow key, and zero allocations. `BenchmarkSnellPacketAdapter` compares
the old wrapped path and the new native path.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| wrapped | 26.00n ± 21% | 64.0 | 1.000 |
| native | 1.285n ± 1% | 0.0 | 0.000 |
| delta | -95.06% | -100.00% | -100.00% |

## Verification

- `go test ./listener/snell -count=1` green.
- `go test -race ./listener/snell -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
