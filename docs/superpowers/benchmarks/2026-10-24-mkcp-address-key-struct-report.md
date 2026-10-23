# mKCP Address Key Struct Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. Comparisons use
`benchstat -count=6 -benchtime=200x`.

Storing `netip.AddrPort` in an `any` map key boxed the address on every UDP
datagram. The session key is now a dedicated struct: UDP stores `AddrPort` by
value and a virtual packet connection stores its address object. This preserves
Mekya's virtual addressing while keeping the UDP path allocation-free.

| Comparison | Time | Bytes | Allocations |
| --- | ---: | ---: | ---: |
| `9fc0cc580` -> this line | no significant time change | lower, p=0.065 | 335.5 -> 313.5, -6.56% |
| `48c9ae77f` -> this line | 325.7µs -> 248.1µs, -23.83% | 97.12 KiB -> 91.30 KiB, -6.00% | 370.0 -> 313.5, -15.27% |

Verification:

- Focused address-key test, mKCP package tests and the real echo benchmark: green.
- `go vet ./...`: green.
- Full tests and the `with_gvisor` suite: green.
- mKCP/Mekya package race and inbound KCP interop race: green.
