# TUIC v4: packet fields without binary.Read or binary.Write

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

`v4.Packet` is the UDP datagram codec, so every packet a TUIC v4 session
carries wrote three fixed fields through `binary.Write` (association id,
payload length, address port) and the read side did the same through five
`binary.Read` calls including the address port. Each fixed-size call allocates
a scratch slice on the fast path.

Both directions now use the same byte methods the codec already exposes:
`writeUint16`/`readUint16`, `writeUint32`/`readUint32`, and `WriteByte` for
the address port. `Dissociate` uses the same helpers, and a round-trip test
pins the fields and the address.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| `BenchmarkPacketWriteTo` | 223.8 ns mean, 8 B, 3 allocs | 68.9 ns mean, 0 B, 0 allocs |
| `BenchmarkReadPacket` | 361.9 ns mean, 1424 B, 5 allocs | 327.6 ns mean, 1412 B, 2 allocs |

The two remaining read-side allocations are the address and the payload the
codec has to copy out of the stream. `TestPacketWriteToAllocatesNothing` and
`TestReadPacketAllocatesPayloadAndAddressOnly` threshold at 0.5 and 2.5 so a
`sync.Pool` refill after a GC cannot flake them; on the parent commit they
report 3 and 5 allocations. Both carry `//go:build !race` because the race
detector instruments every access and makes `AllocsPerRun` meaningless.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./transport/tuic/...`
- `go vet ./...`
