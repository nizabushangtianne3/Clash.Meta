# DNS uncompressed answer owner reuse

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `48097353a`.

## Change

After adopting request-question reuse, an uncompressed answer owner still called
`UnpackDomainName` and allocated a second domain string. The shared wire-name
matcher now also recognizes an uncompressed owner equal to the request name, so
both compressed-pointer and uncompressed common answers reuse the request string.
Mismatched, compressed-to-other-offset, and escaped names still fall back to
`UnpackDomainName`.

## Gate

The existing uncompressed AAAA generic-equivalence test covers correctness.
The new `BenchmarkUnpackSimpleUDPReplyOneAAAAUncompressed` measures that shape.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 226.6n ± 2% | 344.0 | 2.000 |
| reuse | 191.9n ± 4% | 320.0 | 1.000 |
| delta | -15.29% (p=0.000) | -6.98% | -50.00% |

## Verification

- `go test ./dns -count=1` green.
- `go test -race ./dns -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green with the prepared V2Ray binary.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
