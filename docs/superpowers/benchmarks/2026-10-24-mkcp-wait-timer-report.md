# mKCP Wait Timer Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The benchmark is
`benchstat -count=10 -benchtime=300x` against `e1db05ec9`.

`waitNotify` no longer constructs a `time.Timer` for every blocked read or write.
It acquires a stopped timer from a pool, resets it, and returns it after draining
any pending firing. The timer-fired result is tracked so a consumed timer channel
is not drained twice.

| Metric | Before | After | Result |
| --- | ---: | ---: | --- |
| Round-trip allocations | 267.5 | 236.0 | -11.78%, p=0.000 |
| Bytes per round trip | 94.36 KiB | 89.46 KiB | -5.19%, p=0.000 |
| Round-trip time | 248.3µs | 270.5µs | no significant change under host noise |

Focused verification: the timer wait allocation test, mKCP package tests, mKCP
race and staticcheck are green.
