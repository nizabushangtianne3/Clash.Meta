# SSR: write frame fields without binary.Write

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

The SSR protocol frames used `binary.Write` for their fixed-size length, checksum,
timestamp and connection-id fields. Each call allocated a scratch slice on its
fixed-size fast path. The fields now go through small endian-specific
`bytes.Buffer.WriteByte` helpers in `transport/ssr/protocol`; the checksum and
masking behavior is unchanged.

## Gate

`BenchmarkPutAuthData`: 0 B/op and 0 allocs/op across ten runs (28-71 ns/op).
The former path had two fixed-size `binary.Write` calls in this function; the
same allocation pattern was removed from the AES-128, chain-A and SHA-1 v4
frame writers.

## Verification

- `go test ./transport/ssr/... -count=1`
- `go test ./... -count=1 -p 2`
- `go test -race ./transport/ssr/...`
- `go vet ./...`
