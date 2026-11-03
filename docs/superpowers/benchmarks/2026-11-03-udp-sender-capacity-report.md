# UDP flow sender queue capacity

Date: 2026-11-03

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `028548951`.

## Change

Every UDP flow paid for a 128-entry packet channel before its first datagram
was dialed. `senderCapacity` is now 16, the same queue bound scale as the
per-worker UDP queue rather than an oversized per-flow slab. `Send` remains
nonblocking and overflow still drops the packet, so the existing sender lifetime
and drop contracts are unchanged.

## Gate

`BenchmarkNewPacketSender` measures allocation on every UDP flow creation.
The sender close race tests fill and drain the queue concurrently.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| capacity 128 | 378.4n ± 2% | 2528.0 | 3.000 |
| capacity 16 | 162.7n ± 2% | 480.0 | 3.000 |
| delta | -57.00% (p=0.000) | -81.01% | p=1.000 |

Intermediate probes measured capacity 32 at 736 B and capacity 64 at
1376 B; 16 was selected because it gives the largest per-flow memory reduction
while the full tunnel suite and race close tests stay green.

## Verification

- `go test ./tunnel -count=1` green.
- `go test -race ./tunnel -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
