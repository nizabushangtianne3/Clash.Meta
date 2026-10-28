# mKCP Read Reuse Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The benchmark is
`benchstat -count=10 -benchtime=300x` against `fd0571cbc`.

The packet reader now reuses its result slice across datagrams. ACK and command
segments use dedicated object pools, and ACK number lists retain capacity between
packets. Data segment objects are deliberately not returned to the send-segment
pool: they can remain in the out-of-order receive cache, and the race detector
found cross-connection field races when receive and send paths shared that pool.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip allocations | 236.0 | 175.0 | -25.85%, p=0.000 |
| Bytes per round trip | 89.46 KiB | 86.29 KiB | -3.55%, p=0.000 |
| Round-trip time | 270.5µs | 250.0µs | -7.55%, p=0.003 |

Focused verification: allocation parsing, out-of-order ownership, full mKCP
package tests, mKCP race and staticcheck are green.
