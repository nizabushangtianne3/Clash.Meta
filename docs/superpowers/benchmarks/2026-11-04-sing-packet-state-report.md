# Sing packet write-state pointer

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `3e5d98c37`.

## Change

Every sing UDP wrapper embedded a full `packetWriterState`, even when all
datagrams on a packet connection shared one state. The wrapper now holds one
state pointer. Flow reads share the existing connection state; the standalone
`NewPacket` path uses a combined single-shot allocation, preserving its
allocation count.

## Gate

Existing tests cover flow metadata reuse, wrapper uniqueness, buffer release,
write-back close semantics, and TCP handoff. Paired benchmarks cover 100
datagrams on one flow, single-shot allocation, write-back, and TCP handoff.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| 100-datagram flow | 13.75µ ± 9% → 11.77µ ± 2% | 56.47Ki → 47.10Ki | 104 → 104 |
| single-shot packet | 1.767µ ± 4% → 1.780µ ± 5% | 16.65Ki → 16.65Ki | 3 → 3 |
| write-back | 17.20n → 17.03n | 0 | 0 |
| TCP handoff | 115.1n → 108.8n | 416 | 1 |

Flow time falls 14.38% and memory falls 16.60%. Write-back improves 0.96%,
TCP handoff 5.47%, and single-shot differences are within noise.

## Verification

- `go test ./listener/sing -count=1` green.
- `go test -race ./listener/sing -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
