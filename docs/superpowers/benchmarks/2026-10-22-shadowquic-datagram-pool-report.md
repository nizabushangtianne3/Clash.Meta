# ShadowQUIC: pool the native datagram frame

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

The native ShadowQUIC UDP path allocated a `2 + len(payload)` slice for every
datagram before calling quic-go's `SendDatagram`. quic-go copies the payload
before returning, so the frame now comes from the shared pool and is recycled
after the send.

`encodeDatagramInto` is the allocation-free frame writer; the wire-format test
pins the ID and payload bytes.

## Gate

`BenchmarkEncodeDatagramInto`: 0 B/op and 0 allocs/op across ten runs
(11.6-12.7 ns/op). The previous `EncodeDatagram` allocated one frame of
`2 + len(payload)` bytes per datagram.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./transport/shadowquic`
- `go vet ./...`
