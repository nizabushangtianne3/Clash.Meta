# Allocation-free DNS cache filtering

Date: 2026-11-03

`putMsgToCache` filtered OPT records with `lo.Filter`, allocating a second
extra slice, then computed the minimum TTL through `lo.Concat`, allocating a
third concatenation of every record section. The copy is now filtered in place,
and a section-aware minimum walks the three existing slices without joining
them. Manual TTL scans and clamping replace generic helpers; the arithmetic now
avoids unsigned underflow when the old minimum was below the new base TTL.

A typical two-answer A response with one OPT allocation benchmark improves from
419.8ns/440B/9 allocs to 388.4ns/424B/8 allocs: time -7.49%, bytes -3.64%,
allocations -11.11% (p=0.000, n=10). The DNS package tests cover OPT removal,
ACME bypass, TTL updates and cache mutation isolation.

Verification:

- `go test ./dns -count=1`
- `go test -race ./dns`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
