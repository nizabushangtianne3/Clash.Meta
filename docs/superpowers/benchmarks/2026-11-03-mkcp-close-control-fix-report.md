# MKCP close control segment fix

Date: 2026-11-03

Pooled ACK and ping control segments did not reset `option` before reuse. A
control segment created while the connection was closing could put
`segmentOptionClose` back into its pool; the next active connection then sent
that stale close flag, prematurely moved the peer to close states, and under
GOMAXPROCS=1 caused truncated full-duplex reads or timeouts.

The fix resets the option on every pooled ACK and ping. Input now applies close
state after processing a segment's payload/ACK fields, so a close-marked ACK
cannot drop the send-window updates it carries. Peer termination keeps cached
out-of-order receive records until final teardown. UDP read loops also retry
non-`net.ErrClosed` failures up to 64 times with a 1ms delay instead of closing
on the first Windows ICMP reset; closed sockets exit immediately.

Stress evidence on the same host and task cache, `go test ./transport/mkcp
-count=100 -cpu=1`, reproduced 4 `TestFullDuplex` EOF failures and 2
`TestRoundTrip` timeouts on the old code. The fixed implementation passed
`count=300 -cpu=1` cleanly.

Verification:

- `go test ./transport/mkcp -count=300 -p 1 -cpu 1`
- `go test -race ./transport/mkcp -count 3`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
