# direct AddrPort dialer

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## Change

The DIRECT outbound formatted a resolved destination into `host:port`, then the
dialer split that string, parsed the host back to `netip.Addr`, and wrapped the
address in a singleton slice. `DialContextAddrPort` now dials from the metadata
`AddrPort` directly, and the internal dial path carries an `AddrPort` instead of
a separate IP list plus port string. The string path remains for destinations
that still need DNS.

The change also removes `fmt.Sprintf` from forced IPv4/IPv6 network selection
and gives IP4P conversion an AddrPort helper.

## Gates

- The existing DIRECT end-to-end setup benchmark exercises the new path.
- Component, adapter and tunnel tests cover successful dials, failures, relay
  ownership and lifecycle.
- Cross-compile coverage verifies the platform bind paths still build.

## Result

The integrated benchmark is dominated by Windows socket setup and the echo
relay, and it intermittently exhausts loopback ports under repeated `-count`
runs both with and without this change, so its wall-clock and allocation totals
were not used as a gate. Code inspection of `BenchmarkTCPConnSetupDirect` shows
those costs belong to the listener, relay goroutine and standard library dial
path rather than this address conversion.

The conversion work removed is one `Metadata.RemoteAddress` plus
`net.SplitHostPort` and one `netip.ParseAddr`/singleton slice per DIRECT TCP
connection.

## Verification

- `go test ./component/dialer ./component/resolver ./adapter/... ./tunnel -count=1` green.
- `go test -race ./component/dialer ./component/resolver ./adapter/outbound ./tunnel -count=1` green.
- Full-tree tests and gVisor tests green; cross-compile matrix green.
