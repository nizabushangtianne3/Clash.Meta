# UDP DNS simple reply fast parser

Date: 2026-11-03

The UDP client now recognizes the common reply shape: one question, one direct
A/AAAA answer, no authority or additional records and no TSIG. The returned
message and every slice target live in one ownership slab. Question-name
compression back to offset 12 reuses the already-decoded owner string; every
other shape falls back to the complete miekg parser. This keeps TSIG, EDNS,
CNAME chains, negative replies and arbitrary RR types on the existing path.

Paired `BenchmarkClientExchangeUDP` against the same server, n=10 at `-cpu 1`,
changes 20.72us/600B/17 allocs to 19.88us/624B/12 allocs: time -4.01%,
allocations -29.41%, bytes +4.00% from the single ownership slab (allocation
count p=0.000).

Verification:

- `go test ./dns -run 'TestUnpackSimpleUDPReply|TestClientExchangeUDP' -count 1`
- `go test -race ./dns -count 1`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
