# mKCP Receive Segment Pool Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=12 -benchtime=300x` against `81ee37697`.

Each receive loop now owns a data-segment pool: one for the UDP listener, one for
the virtual listener, and one for each Dial receive loop. Parsed data segments
remain owned while they sit in the out-of-order receive cache and return to their
original loop-owned pool after delivery or connection termination. Keeping pools
loop-local prevents a client and server in the same process from reusing one
segment concurrently.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip allocations | 134.0 | 108.0 | -19.40%, p=0.000 |
| Round-trip time | 227.0µs | 209.6µs | -7.69%, p=0.001 |
| Bytes per round trip | 87.80 KiB | 85.95 KiB | -2.11%, p=0.000 |

Focused verification: zero-allocation parsing, out-of-order ownership, protocol
round trips, mKCP package tests, mKCP race and staticcheck are green.
