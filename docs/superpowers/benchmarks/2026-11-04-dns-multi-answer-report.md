# DNS multi-answer fast path

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `28b54bd19`; its benchmark measures the
old complete miekg fallback for the same wire.

## Change

The fast parser accepted only a direct address or exactly `CNAME + address`.
Real answers often contain several addresses. It now accepts up to eight direct
addresses, or one CNAME followed by up to eight addresses of the requested
family. Variable slices hold the concrete records and one packed address buffer,
so allocations do not grow per record. Larger answers still use the full parser.

## Gate

Generic-equivalence tests cover direct four-A and `CNAME + four-A` responses,
including alias owner names and packed address slices. A nine-answer response
covers the fast-path bound. `BenchmarkUnpackSimpleUDPReplyCNAMEFourA` measures
the common multi-answer shape.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| full fallback | 1427.0n ± 4% | 920.0 | 24.000 |
| multi slab | 603.0n ± 3% | 912.0 | 6.000 |
| delta | -57.74% (p=0.000) | -0.87% | -75.00% |

## Verification

- `go test ./dns -count=1` green.
- `go test -race ./dns -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
