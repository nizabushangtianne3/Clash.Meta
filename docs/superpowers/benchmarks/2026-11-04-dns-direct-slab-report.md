# DNS direct answer slab tightening

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `3f8c1d95e`.

## Change

When EDNS and multi-answer paths were added, the common direct-answer slab grew
two interface slots. Only a direct one-address response uses that slab; the
`CNAME + address` path already owns its own two-slot slab. This removes the
unused second slot without changing ownership or output.

## Gate

Existing generic-equivalence tests cover direct A, uncompressed AAAA, empty
EDNS OPT, CNAME chains, and multi-answer responses. The paired benchmark covers
direct answers with and without EDNS and confirms CNAME memory also drops
because every `udpReplySlab` is smaller.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline direct A | 173.6n ± 6% | 416.0 | 1.000 |
| tightened direct A | 164.8n ± 5% | 384.0 | 1.000 |
| delta | -5.07% (p=0.007) | -7.69% | p=1.000 |

EDNS time is within noise (`p=0.101`); its memory falls `-7.69%`. CNAME time is
within noise (`p=0.404`); its memory falls `-6.06%`.

## Verification

- `go test ./dns -count=1` green.
- `go test -race ./dns -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
