# DNS family-specific response slabs

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `3899aedbb`.

## Change

Every fast-path response previously shared one slab containing both A and AAAA
records even though a response can only use one family. The shared header,
question and EDNS fields moved into `udpReplyBase`; direct paths now allocate an
A-only or AAAA-only slab, CNAME owns one combined slab, and multi-answer paths
allocate only the shared base.

## Gate

All existing generic-equivalence tests cover direct A/AAAA, EDNS, CNAME chains,
and multi-answer responses. The paired benchmarks cover the common direct,
EDNS, two-answer CNAME, and multi-answer shapes.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| direct A | 159.7n ± 5% → 156.2n ± 10% | 384 → 320 | 1 → 1 |
| direct A + EDNS | 178.4n ± 4% → 167.9n ± 12% | 384 → 320 | 1 → 1 |
| CNAME + A | 314.6n ± 12% → 279.9n ± 5% | 496 → 480 | 3 → 2 |
| CNAME + 4×A | 612.9n ± 11% → 579.5n ± 2% | 880 → 736 | 6 → 6 |

Memory falls on every measured shape (`-3.23%` to `-16.67%`); CNAME allocation
count falls 33%. Direct and multi-answer time differences are within host
noise; CNAME time falls 11.02% and CNAME+4×A falls 5.44%.

## Verification

- `go test ./dns -count=1` green.
- `go test -race ./dns -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
