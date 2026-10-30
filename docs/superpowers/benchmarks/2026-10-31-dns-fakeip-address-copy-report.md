# DNS fakeip address allocation

Date: 2026-10-31

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
The paired benchmark answers an A-record query through the fake-ip service;
each side ran 10 times.

| Metric | Baseline | Current | Result |
| --- | ---: | ---: | ---: |
| Bytes | 228 B/op | 224 B/op | -1.75% |
| Allocations | 4 | 3 | -25% |
| Time | 293.1 ns | 258.8 ns | no significant change (`p=0.393`) |

Fake-ip A/AAAA records now create their answer record first and copy the fixed
`netip.Addr` bytes directly into the DNS RR array. This avoids the intermediate
`Addr.AsSlice()` heap allocation on every query while preserving wire behavior.

## Verification

- `go test ./dns -count=1`
- Full-tree verification follows before push.
