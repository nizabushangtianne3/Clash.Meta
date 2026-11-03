# Sing flow shared metadata pointer

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `39bda80df` (shared write-state
pointers, with metadata still embedded in every packet wrapper).

## Change

A sing UDP flow previously copied the complete `C.Metadata` into every packet
wrapper, then reused a pointer to the first wrapper as the flow template. The
wrapper now holds only the shared metadata pointer; metadata is allocated once
per destination and reused for every matching datagram. A changed destination
allocates a new metadata object, preserving the existing split semantics. The
single-shot `NewPacket` path also points to separately owned metadata.

## Gate

Existing tests pin flow metadata pointer reuse, destination splitting, wrapper
uniqueness, buffer release, write-back, and TCP handoff.
`BenchmarkNewPacketConnectionDatagramBatch` covers 100 datagrams on one flow.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| 100-datagram flow | 11.77µ ± 2% → 9.50µ ± 21% | 47.10Ki → 8.44Ki | 104 → 105 |
| single-shot packet | 1.780µ ± 5% → 1.765µ ± 4% | 16.65Ki → 16.66Ki | 3 → 4 |
| TCP handoff | 108.8n → 110.6n | 416 | 1 |

Flow time falls 19.32% and memory falls 82.09%; allocation count rises by one
because the first flow metadata is now separately owned. Single-shot and TCP
differences are within noise.

## Verification

- `go test ./listener/sing -count=1` green.
- `go test -race ./listener/sing -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
