# UDP adapter binary flow key

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
Baseline: `a65dbdf8a`.

## Change

`PacketAdapter` now owns a binary `FlowKey`; the old string SNAT-key method
and the tunnel-side optional type assertion are gone. Every packet adapter is
built from the same AddrPort key, and zoned `*net.UDPAddr` sources reuse the
already-parsed address instead of formatting and re-parsing its IP just to add
the zone.

## Benchmark

`go test ./constant -run '^$' -bench '^BenchmarkPacketAdapter.*Lifecycle$' -benchmem -count=8`

- Unzoned IPv4 adapter lifecycle: 35.72 ns -> 36.56 ns (within noise), 64 B/op
  and 1 alloc/op before and after.
- Zoned IPv6 adapter lifecycle: 254.7 ns / 72 B / 2 allocs ->
  168.6 ns / 64 B / 1 alloc; benchstat reports -60.61% sec/op, -11.11% B/op,
  and -50% allocs/op. Wall time is noisy on this host, while the allocation
  difference is deterministic.
- The generic flow-key-only lifecycle remains 64 B/op and 1 alloc/op (the
  adapter itself).

## Verification

- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `go test -race ./constant ./tunnel ./component/sniffer -count=1`
- New zoned and unzoned adapter flow-key tests; compatibility string-key test
  removed with its dead path.
