# VLESS: keep the two-byte read header on the packet connection

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What changed

The outbound `transport/vless` `WaitReadFrom` and the inbound
`listener/sing_vless` `ReadPacket`/`WaitReadPacket`/`ReadFrom` paths read their
two-byte length with `binary.Read`. That allocates a scratch slice per packet.
Both packet connections now own the two-byte header and pass that field to
`io.ReadFull`, leaving the payload copy as the only remaining work.

## Gate

| benchmark | before | after |
| --- | ---: | ---: |
| vless `WaitReadFrom` | 101.1 ns mean, 2 B, 1 alloc | 54.1 ns mean, 0 B, 0 allocs |
| sing_vless `ReadPacket` | 66.9 ns mean, 2 B, 1 alloc | 55.1 ns mean, 0 B, 0 allocs |

`TestPacketConnWaitReadFromAllocatesNothing` and
`TestServerPacketConnReadPacketAllocatesNothing` threshold at 0.5 and report 1
allocation on the parent commit. The outbound test also keeps the existing
`ReadFrom` framing test, whose `*net.UDPAddr` return is the only allocation the
public `net.PacketConn` shape requires.

## Verification

- `go test ./... -count=1 -p 2`
- `go test -race ./transport/vless ./listener/sing_vless`
- `go vet ./...`
