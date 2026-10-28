# Datagram Pool Optimization Report

Date: 2026-10-23

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. Every pair below is
`benchstat -count=6 -benchtime=2000x` against the unmodified `102d6923a` baseline in a
separate worktree.

## Landed

| Benchmark | Time | Bytes | Allocations |
| --- | ---: | ---: | ---: |
| Mekya `ReadPacketBundle` | -75.45% | 1282 -> 2.5 | 2 -> 0 |
| Mekya `WrappedPacketConnForward` | -30.55% | 1280 -> 0 | 1 -> 0 |
| Mekya `ServerSessionWrite` | -64.96% | 1280 -> 2 | 1 -> 0 |
| Mekya `RequestClientSessionWrite` | -59.29% | 1280 -> 2 | 1 -> 0 |
| Mekya `WritePacketBundle` | no significant time change | 2 -> 1 | 1 -> 0 |
| mKCP `WriteDataSegment/plain` | -79.96% | 2816 -> 2 | 2 -> 0 |
| mKCP `WriteDataSegment/authenticated` | -18.36% | 2816 -> 2 | 2 -> 0 |
| MASQUE `H2DatagramStreamSendDatagram` | -39.16% | 1285 -> 3 | 1 -> 0 |
| MASQUE `H2DatagramStreamReceiveDatagram` | -34.53% | 18 -> 0 | 3 -> 0 |

The Mekya bundle reader owns its packet buffers, so the server and client queues now
transfer that ownership instead of copying at each channel boundary; their read paths
recycle after copying into the caller's buffer. Its fixed two-byte length header also
uses a dedicated pool because passing a stack array to `io.ReadFull` or `io.Writer`
makes both directions allocate once per packet.

The mKCP segment serializer and outgoing frame are both dead after the synchronous
writer call, so both use the shared byte pool. The segment payload retained by the
send window is deliberately not pooled yet: flush releases the connection lock while
still referencing segments, so ACK-time recycling would need a reference-counting
ownership redesign rather than a local pool insertion.

MASQUE H2 send capsules are pooled after the pipe write. Receive now keeps one
16-byte buffered varint reader per stream; the previous path allocated a wrapper and
two one-byte read buffers per datagram. The received payload itself is still owned by
the caller and is not pooled.

## Rejected

Replacing mKCP's `hash/fnv` authenticator with a direct loop was tested and rejected:
the existing implementation already has zero allocations, while the replacement was
slower without significance (`p=0.122`). The allocation-free authenticator round-trip
test and benchmark stay as regression gates.

Staticcheck's unused entries were not treated as automatic deletion candidates. Some
are reached from other platform files, others are deliberate cache-line padding or
upstream compatibility surface. The two genuinely unused mKCP read-buffer helpers
were deleted; the changed packages now pass staticcheck.

## Verification

- Focused allocation tests and package tests for Mekya, mKCP and MASQUE: green.
- `go vet ./...`: green.
- `go test ./... -count=1 -p 2`: green.
- `go test -tags with_gvisor ./... -count=1 -p 2`: green.
- Full race passed for every package except `listener/inbound`. Its first race is
  inside `restls-client-go@0.1.9`, before this fork's call sites; `listener/inbound`
  passes without race, the affected KCP interop case passes with race on both the
  baseline and this line, and the modified packages pass with race. A fork-local
  fix would require changing or replacing the vendored dependency behavior and is
  outside this measured allocation round.
