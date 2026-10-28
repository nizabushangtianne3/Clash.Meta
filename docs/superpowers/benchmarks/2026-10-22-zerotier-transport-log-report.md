# ZeroTier Transport Log: Check The Level Before Formatting

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What Changed

The transport config's `Log` callback ran `fmt.Sprintf` and then a `log.Infoln`
or `log.Debugln` for every line the library emits, so the message was built and
the logger called before anything looked at the level. It is
`ZeroTier.logFromTransport` now, which returns before formatting when the level
is off.

`github.com/metacubex/zerotier-go` has 16 `LogDebug` and 7 `LogInfo` call sites,
so a large share of its lines are debug-only and paid the full cost with debug
logging off.

## Gate

`TestZeroTierTransportLogCostsNothingWhenTheLevelIsOff` runs the callback against
a zero-value `ZeroTier` with the level set to `SILENT`: a zero value has no
`Base`, so reaching `z.Name()` panics. The test fails with that panic when the
guard is removed, which pins that the guard returns before both the formatting
and the name lookup.

`BenchmarkZeroTierTransportLog` measures one debug line with the level off:

| Variant | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `unguarded` (removed) | 117.6n ± 3% | 64 | 2 |
| `guarded` | 2.925n ± 77% | 0 | 0 |

The rest of the tree is covered by `go test ./... -count=1 -p 2` (94 packages),
`go vet ./...` and `-race` over `adapter/outbound`.

## Left Alone

The other 19 `log.Debugln` calls in `zerotier.go` stay unguarded on purpose.
They sit on the node/peer/path event handler, on the per-operation apply path,
or behind the error slices that `runOutboundBatches` and `runInboundFrames`
only fill when a write or a frame actually fails; none of them is on a
per-packet path the way the library's own log callback was.
