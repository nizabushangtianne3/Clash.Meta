# UDP flow write-back slab

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `1a45547c9`.

## Change

A UDP flow allocated a separate `nat.WriteBackProxy` while already allocating a
flow-owned `udpFlowMetadata` slab. `udpFlowMetadata` now owns the exported
`nat.WriteBackProxy` value, and dialing stores the first packet directly in that
slab. The proxy remains in `component/nat`, so listener tests and any other
write-back target keep one implementation; the tunnel no longer adds a second
allocation per flow.

## Gate

`TestUDPFlowOwnsWriteBackProxy` checks that the flow-owned proxy dispatches the
latest packet target. `BenchmarkNewUDPFlowSenderAndWriteBack` compares the same
flow + sender + write-back resource set.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| separate proxy | 349.2n ± 3% | 1392.0 | 5.000 |
| flow slab | 344.5n ± 4% | 1376.0 | 4.000 |
| delta | p=0.225 | -1.15% | -20.00% |

Time stays within host noise; the accepted win is one fewer allocation per UDP
flow and 16 fewer bytes.

## Verification

- `go test ./tunnel -count=1` green.
- `go test -race ./tunnel -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
