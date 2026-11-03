# DNS UDP packed buffer reuse

Date: 2026-11-03

`Msg.PackBuffer` checks the supplied slice length, not capacity. Passing the
pooled buffer as `wire[:0]` defeated the reuse and made miekg allocate a fresh
wire document for every UDP query. The client now passes the full pooled slice;
`PackBuffer` overwrites its prefix and returns only the written length.

Paired `BenchmarkClientExchangeUDP`, n=10 at `-cpu 1`, changes
632B/18 allocs to 600B/17 allocs: bytes -5.06%, allocations -5.56%
(p=0.000). Time stays inside host noise (p=0.739). Existing UDP exchange,
deadline, mismatched-ID and TCP truncation suites cover the wire shape.

Verification:

- `go test ./dns -count=1`
- `go test -race ./dns`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
