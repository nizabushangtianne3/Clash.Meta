# Hot-Path Debug Logs: Guard The Argument Build

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What Changed

`log.Debugln` returns early when debug logging is off, but Go still builds the
call's variadic slice and boxes every argument that needs it before the call.
Measured on this host with debug off, one unguarded call with three run-time
integer arguments cost 8 B and 2 allocs per call, and one with two string
arguments cost 32 B and 2 allocs per call; the same call behind
`if log.DebugEnabled()` cost 0 B and 0 allocs.

The forwarding tree already used that guard in places. This cycle adds it where
the call sits on a per-write, per-read or per-flow path:

- `transport/vless/vision/padding.go`: `ApplyPadding` logs on every filtered
  write during a connection's TLS handshake.
- `transport/vless/vision/conn.go` and `filter.go`: the matching per-read and
  per-record logging of the same filter phase.
- `transport/masque/client_h2.go`: both per-packet drop paths.
- `adapter/outbound/dns.go`: the DNS hijack log, whose `metadata.RemoteAddress()`
  argument built a string of its own.
- `tunnel/match_context.go`: the DNS-resolve and find-process failure logs,
  whose `metadata.String()` argument builds a string.

Two pieces of dead code went with them: the commented-out `XTLS Vision Direct
write` block in `conn.go` and the commented-out `long padding` line in
`padding.go`. The TLS 1.3 log's full-width comma also became an ASCII comma,
the only non-ASCII byte in the file.

## Gate

`TestApplyPaddingBuildsNoLogArgumentsWithDebugOff` runs `ApplyPadding` on a
pre-grown buffer through `testing.AllocsPerRun` and requires zero allocations.
Removing the guard makes it fail with one allocation, so the test pins the
guard rather than the implementation.

The rest of the tree is covered by `go test ./... -count=1 -p 2` (92 packages),
`go vet ./...`, and `-race` over the touched packages.
