# TUIC: reuse the read address scratch on the UDP relay

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

The TUIC v4 and v5 QUIC UDP relays read a complete `Packet` for every
datagram. `WaitReadFrom` only needs the address as an `AddrPort`, but
`ReadPacket` allocated a slice for the address and a slice for the payload.

The v4 connection now owns a 256-byte address scratch and reads the packet
into it. v5 does the same for unfragmented packets; fragmented v5 packets keep
the allocating path because the de-fragmenter holds those packets until the
assembly completes. `ReadFrom`, which returns a `*net.UDPAddr` that outlives
the call, keeps the allocating path on both versions.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| v4 `ReadPacket` | 2 allocs, 1412 B | 1 alloc, 1408 B |
| v5 `ReadPacket` | 2 allocs, 1412 B | 1 alloc, 1408 B |

The remaining allocation is the payload itself. `TestReadPacketInto...`
correctness tests cover IPv4/IPv6, and the v5 fragment test pins that a
fragmented packet still gets an independent address slice.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./transport/tuic/...`
- `go vet ./...`
