# Sudoku And Snell Reply Paths: Write From The AddrPort

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What Changed

Both inbound relays already hold the reply destination as a `netip.AddrPort`
and then walked back through a `net.Addr` to write it.

- Sudoku: `uotPacket.WriteBack` built a `net.UDPAddr` for `UoTPacketConn.WriteTo`
  on every datagram. `UoTPacketConn.WriteToAddrPort` is the entry point now and
  `WriteTo` delegates, so the `net.PacketConn` interface keeps its shape while
  the reply path skips the conversion.
- Snell: `WritePacketResponse` took a `net.Addr` and formatted a socks address
  out of it to lay the response header. It takes the `netip.AddrPort` now and
  writes the four or sixteen address bytes plus the big-endian port directly.
  Its only production caller already held the AddrPort, and the test that pinned
  the socks-parser argument is replaced by one that pins the wire bytes and the
  allocation count.

## Gate

`BenchmarkUoTWriteBack` and `BenchmarkWritePacketResponse` compare the removed
entry points against the new ones on this tree:

| Benchmark | Variant | sec/op | B/op | allocs/op |
| --- | --- | ---: | ---: | ---: |
| `UoTWriteBack` | `net-addr` (removed) | 81.43n ± 7% | 76 | 3 |
| `UoTWriteBack` | `addrport` | 39.49n ± 2% | 24 | 1 |
| `WritePacketResponse` | `socks-addr` (removed) | 48.53n ± 88% | 8 | 1 |
| `WritePacketResponse` | `addrport` | 35.84n ± 3% | 0 | 0 |

The sudoku entry point keeps one 24-byte allocation: the datagram frame it
hands to `io.Writer.Write` escapes. The snell reply is allocation-free, and
`TestWritePacketResponseAllocatesNothing` pins that.

Unit gates also include `TestWritePacketResponseWritesTheAddrPort` (the exact
wire bytes) and `TestWritePacketResponseRejectsAnInvalidAddrPort`. The rest of
the tree is covered by `go test ./... -count=1 -p 2` (92 packages),
`go vet ./...` and `-race` over the touched packages.
