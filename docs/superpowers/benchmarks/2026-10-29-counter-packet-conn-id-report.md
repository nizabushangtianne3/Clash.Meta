# Counter packet-connection IDs

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `988f920f4`.

## Change

Outbound UDP packet connections used a CSPRNG UUID string only to make their
in-process `CustomAddr` unique for QUIC connection multiplexing. They now use a
process-local atomic counter. The counter never repeats during the process, and
the adapter name remains part of the custom address.

## Benchmark

`go test ./adapter/outbound -run '^$' -bench '^BenchmarkPacketConnIDGeneration' -benchmem -count=8`

| ID source | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| UUID string | 189.8n ± 17% | 48 | 1 |
| Atomic counter | 39.12n ± 25% | 7 | 0 |

This is roughly `-79.4%` wall time and `-85.42%` bytes with deterministic zero
allocations.

## Verification

- `go test ./adapter/outbound -count=1`
- `go test -race ./adapter/outbound ./adapter -count=1`
- `go vet ./...`
- `go test ./... -count=1 -p 1`
- `CGO_ENABLED=0 go test .`
- `CGO_ENABLED=0 go vet . ./cmd/...`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet -tags=with_gvisor . ./mobile ./tun ./platform`
