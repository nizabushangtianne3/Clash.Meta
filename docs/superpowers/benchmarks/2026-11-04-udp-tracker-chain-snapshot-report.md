# UDP tracker chain snapshot allocation

Date: 2026-11-04

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`,
`-cpu 1`, n=10 per side. The baseline is `1bd99a494` without the tracker patch.

## Change

`cloneChains` always built a combined backing array even when a flow exposed only
one chain, which is the common UDP case. `snapshotChains` now preserves that
single chain without a copy and pins the slice cap to its length. A later append
on the source therefore allocates a new array instead of growing the tracker's
snapshot in place; combined chain+provider flows still copy exactly once.

## Gate

`TestUDPTrackerChainsStayAppendIsolated` appends to the source after tracker
creation and requires the tracker chain to remain unchanged with
`cap == len`. `BenchmarkNewUDPTrackerIsolated` measures each UDP flow's tracker
creation cost without socket noise.

## Result

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| baseline | 342.3n ± 5% | 768.0 | 4.000 |
| snapshot | 293.7n ± 8% | 752.0 | 3.000 |
| delta | -14.20% (p=0.000) | -2.08% | -25.00% |

## Verification

- `go test ./tunnel/statistic -count=1` green.
- `go test -race ./tunnel/statistic -count=1` green.
- `go vet ./...` clean.
- `go test ./... -count=1 -p=2` green with the prepared V2Ray interop binary.
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` clean.
