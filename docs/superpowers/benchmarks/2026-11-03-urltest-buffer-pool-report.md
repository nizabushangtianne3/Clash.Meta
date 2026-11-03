# URL-test buffer pool

Date: 2026-11-03

Every URL test allocated a 512-byte writer and a 4096-byte reader. The
buffers belong to one short-lived exchange, so they now live in a sync.Pool
and are reset away from the connection before release. A response returned
with an error is closed before that release, so no body can retain the pooled
reader.

A deterministic `net.Pipe` benchmark covers the exchange without the Windows
ephemeral-port collision seen by the old end-to-end benchmark. Paired results
were 9.328us/7.934KiB/39 allocs before and 5.419us/3.296KiB/35 allocs after:
time -41.91%, bytes -58.46%, allocations -10.26% (p=0.000, n=10).

Verification:

- `go test ./adapter -count=1`
- `go test -race ./adapter`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
