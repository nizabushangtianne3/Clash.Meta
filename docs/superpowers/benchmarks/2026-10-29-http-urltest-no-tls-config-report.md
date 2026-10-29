# HTTP URLTest TLS config

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## Change

`Proxy.URLTest` unconditionally built a TLS configuration even for plain HTTP
targets. The configuration is mutated later by the TLS exchange, so it cannot
be cached safely, but plain HTTP never needs it. Build it only for HTTPS.

## Gate

`BenchmarkProxyURLTestHTTP` measures a real loopback HTTP health check. TLS
behaviour remains covered by the unified-delay HTTPS test.

## Result

The benchmark is susceptible to Windows loopback port exhaustion across repeated
runs. One valid paired sample before and after the change measured:

| benchmark | sec/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ProxyURLTestHTTP | 1.142ms -> 1.380ms | 12317 -> 11869 | 93 -> 92 |

Wall-clock is dominated by the local HTTP exchange and is not used as a gate.
The removed work is one `tls.Config` allocation and certificate-pool access per
plain HTTP health check.

## Verification

- `go test ./adapter -count=1` green.
- `go test -race ./adapter -count=1` green.
- `go vet ./adapter` green.
- Full-tree verification follows before push.
