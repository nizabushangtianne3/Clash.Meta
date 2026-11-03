# DNS CNAME + address fast path

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `261af6d58`; its benchmark measures the
old complete miekg fallback for the same wire.

## Change

IP lookups commonly receive a CNAME record followed by the requested A/AAAA
record. The fast parser accepted only one answer, so this shape always fell back
to the full parser. It now accepts exactly `CNAME + query-type address`, reuses
request and CNAME-target names through compression aliases, and keeps both
answer slots in one CNAME slab. The decoded target remains the only
variable-length string allocation. A single direct answer and empty EDNS OPT
retain the existing path.

## Gate

A generic-equivalence test covers a compressed `CNAME + A` response, including
the CNAME target and second owner name. The fallback suite still rejects a lone
CNAME and malformed address records. `BenchmarkUnpackSimpleUDPReplyCNAMEA`
measures the common two-answer shape.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| full fallback | 715.2n ± 1% | 352.0 | 11.000 |
| CNAME slab | 286.2n ± 4% | 528.0 | 3.000 |
| delta | -59.98% (p=0.000) | +50.00% | -72.73% |

The larger slab and target string buy a 60% wall-time reduction and remove eight
allocations from a common resolver response.

## Verification

- `go test ./dns -count=1` green.
- `go test -race ./dns -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
