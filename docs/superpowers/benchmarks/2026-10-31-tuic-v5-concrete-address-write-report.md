# TUIC v5 concrete address write

Date: 2026-10-31

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
The paired benchmark writes a 1350-byte IPv4 TUIC v5 packet through
`bytes.Buffer`; the fragment receive benchmark confirms the read path is
unchanged. Each side ran 10 times.

| Benchmark | Baseline | Current | Result |
| --- | ---: | ---: | ---: |
| PacketWriteTo time | 59.55 ns | 53.56 ns | -10.06% |
| Fragment receive time | 784.7 ns | 784.4 ns | no significant change |
| Three-path geomean | 307.9 ns | 298.1 ns | -3.18% |

TUIC v5 now writes fixed-width addresses and ports to a concrete `bytes.Buffer`
with slice-backed writes instead of six or eighteen interface `WriteByte` calls.
A generic-buffer fallback preserves the same wire shape; behavioral tests pin
IPv4, IPv6, domain, no-address, concrete, and fallback outputs.

## Verification

- `go test ./transport/tuic/v5 -count=1`
- `go test -race ./transport/tuic/v5 -count=5`
- Paired `benchstat`, n=10
