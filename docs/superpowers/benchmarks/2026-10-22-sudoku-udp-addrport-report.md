# Sudoku UoT Inbound: AddrPort Targets, Flow Cache And Payload Pool

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What Changed

The sudoku UoT inbound was the last UDP relay that formatted its target into a
`host:port` string per datagram and parsed it straight back, took a fresh
`*C.Metadata` for every datagram, and read each payload into a fresh slice.

- `transport/sudoku`: the datagram address is read directly into a
  `netip.AddrPort` (`ReadAddrPortField`) and written from one (`putUoTAddress`).
  `ReadDatagram` hands back a pooled payload and `WriteDatagram` takes the
  address as an `AddrPort`. A target with no AddrPort form - a domain - is
  consumed whole and reported as `ErrAddrNotRepresentable`, so the datagram is
  dropped and the stream stays framed. The client resolves every UDP
  destination before it writes, so the wire form is IP only, the same contract
  `transport/trojan` already holds.
- `listener/sudoku`: a per-session flow cache reuses one `*C.Metadata` per
  destination, and `Drop` returns the payload to the shared pool.

The wrapper keeps its `writer` and `source` past `Drop` on purpose: a flow
answers its replies through the last packet the sender handed it, and the
write-back proxy outlives that call, which is the shape `1c277c6f8` fixed for
the packet adapter and the Snell wrapper.

Removed with the rewrite: `readDatagramHeaderAndAddress`,
`parseDatagramUDPAddr`, the variadic chunk write in `WriteDatagram`, and the
domain branch of the datagram path.

## Gate

`BenchmarkUoTPacketPath` covers one datagram end to end - encode the frame, read
it back, route it through the session - on the same tree before and after:

| Metric | Before | After | Delta |
| --- | ---: | ---: | ---: |
| sec/op | 778.0n ± 50% | 232.4n ± 44% | -70.13% (p=0.000 n=10) |
| B/op | 608 | 120 | -80.26% |
| allocs/op | 15 | 5 | -66.67% |

The host was loaded, so the timing spread is wide and the allocation count is
the gate; it and the byte count have zero variance across the ten runs.

Unit gates: `TestUoTSessionReusesMetadataAndReturnsThePayload`,
`TestUoTWarmHandleAllocatesTheWrapperOnly` (one allocation per datagram once the
flow and the payload pool are warm - the wrapper the flow must keep),
`TestUoTPacketWriteBackSurvivesDrop` (a reply still reaches its own client after
`Drop`), `TestDatagramRoundTripKeepsTheAddrPort` (IPv4 and IPv6),
`TestWriteDatagramAllocatesAtMostOnce`, and
`TestReadDatagramDropsADomainFieldWithoutBreakingFraming`, which pins that a
rejected address leaves the reader on the next frame.
