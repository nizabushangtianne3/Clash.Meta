# DNS empty EDNS response fast path

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `e1d018b18`; its benchmark measures the
old complete miekg fallback for the same wire.

## Change

Real UDP DNS replies commonly add one EDNS OPT. The simple parser accepted only
zero additional records, so these replies fell back to the full parser. The
single-slab parser now accepts exactly one empty OPT, stores its `D.OPT` and
slice in the same slab, and preserves UDP size, extended flags and RCODE.
OPT records with options, non-OPT additional records, and TSIG still fall back.

## Gate

The generic-equivalence test covers an A answer plus empty OPT. A second case
keeps an OPT with one EDNS option on the full parser.
`BenchmarkUnpackSimpleUDPReplyOneAEDNS` measures that common shape.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| full fallback | 554.9n ± 3% | 304.0 | 10.000 |
| slab fast path | 168.0n ± 7% | 384.0 | 1.000 |
| delta | -69.72% (p=0.000) | +26.32% | -90.00% |

The slab reserves 80 extra bytes to turn ten allocations on a common response
shape into one; wall time falls by about 70%.

## Verification

- `go test ./dns -count=1` green.
- `go test -race ./dns -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
