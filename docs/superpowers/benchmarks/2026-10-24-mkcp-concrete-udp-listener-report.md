# mKCP Concrete UDP Listener Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The benchmark is
`benchstat -count=10 -benchtime=300x` against `7e7dbd1bf`.

The shared listener was replaced with concrete `UDPListener` and `VirtualListener`
types. The UDP listener owns `*net.UDPConn`, reads through
`ReadFromUDPAddrPort`, writes through `WriteToUDPAddrPort`, and keys sessions by
unmapped `netip.AddrPort`. The virtual listener retains `net.PacketConn` and the
virtual address object for Mekya. There is no shared address-type fallback.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip allocations | 311.0 | 267.5 | -13.99%, p=0.000 |
| Round-trip time | 254.6µs | 248.3µs | no significant change |
| Profile allocations | `net.UDPConn.readFrom` was 18.86% of allocated objects | absent | removed |

The one-off byte total moved with host load; a later profile run measured 90,369
B/op with no UDP address allocation site. The durable gate is the allocation
count and removed profile site.

Verification:

- mKCP, Mekya and VMess inbound package tests: green.
- Focused UDP write allocation and context-close tests: green.
- Staticcheck and vet: green.
- mKCP/Mekya package races and inbound KCP interop race: green.
- Full tests and the `with_gvisor` suite: green.
