# DNS UDP wire exchange

Date: 2026-11-03

The UDP DNS path no longer calls miekg's generic read loop, which allocated
the advertised receive buffer for every exchange. The client now packs the
query through the shared pool, reads one datagram into a pooled buffer, drops
mismatched IDs in place, and unpacks only the matching response. TCP keeps the
library exchange, including its length-prefix path and TSIG handling. A UDP
reply carrying TSIG returns `dns.ErrSig`; the internal client has no signing
credentials and must not accept an unverifiable signature.

Paired `BenchmarkClientExchangeUDP`, n=10 at `-cpu 1`, changes
21.87us/4728B/19 allocs to 20.55us/632B/18 allocs: time -6.05%, bytes
-86.63%, allocations -5.26% (p=0.000). New tests cover mismatched IDs and
unsigned TSIG rejection; deadline and TCP truncation suites remain in place.

Verification:

- `go test ./dns -count=1`
- `go test -race ./dns`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
