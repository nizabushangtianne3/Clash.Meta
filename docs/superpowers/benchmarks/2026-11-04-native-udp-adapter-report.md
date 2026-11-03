# Native sing packet adapter

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. Both benchmark arms run on the same current tree:
`wrapped` reproduces the old allocation, while `native` exercises the new
type-asserted path.

## Change

The sing packet already owns its metadata and flow key. It now exposes
`Metadata`, so it satisfies `C.PacketAdapter`; the tunnel uses that native
adapter directly and only allocates the generic wrapper for packets that do not
implement the interface. Existing Drop semantics are unchanged and tests pin the
zero-allocation type assertion.

## Gate

`TestSingPacketIsNativePacketAdapter` verifies native adapter identity, metadata,
flow key, and zero allocations.
`BenchmarkHandleUDPConnExistingFlowAdapter` compares the old wrapped path and the
new native path through the same existing-flow queue and sender.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| wrapped | 114.8n ± 4% | 128.0 | 2.000 |
| native | 40.8n ± 3% | 0.0 | 0.000 |
| delta | -64.46% | -100.00% | -100.00% |

## Verification

- `go test ./listener/sing ./tunnel -count=1` green.
- `go test -race ./listener/sing ./tunnel -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
