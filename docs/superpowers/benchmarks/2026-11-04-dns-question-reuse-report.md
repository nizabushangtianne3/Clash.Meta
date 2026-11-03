# DNS simple reply question reuse

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `99c7ec230`.

## Change

The simple UDP reply parser allocated a new question-domain string even though
the request already owned it. It now matches the reply's wire labels against the
request question in place, reuses that request string, and verifies response
type/class. Case-insensitive ASCII labels are accepted; escaped or mismatched
names fall back to the complete miekg parser.

## Gate

Existing generic-equivalence tests cover compressed A and uncompressed AAAA.
The fallback test now also rejects a reply question that does not match the
requested type. `BenchmarkUnpackSimpleUDPReplyOneA` and
`BenchmarkClientExchangeUDP` measure the common response path.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline parser | 159.6n ± 4% | 336.0 | 2.000 |
| reused parser | 147.9n ± 8% | 320.0 | 1.000 |
| delta | -7.33% (p=0.000) | -4.76% | -50.00% |

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline exchange | 19.15µ ± 6% | 624.0 | 12.000 |
| reused exchange | 19.16µ ± 11% | 608.0 | 11.000 |
| delta | p=0.853 | -2.56% | -8.33% |

The end-to-end wall time is host noise; the accepted wins are one fewer
allocation and 16 fewer bytes per common exchange.

## Verification

- `go test ./dns -count=1` green.
- `go test -race ./dns -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
