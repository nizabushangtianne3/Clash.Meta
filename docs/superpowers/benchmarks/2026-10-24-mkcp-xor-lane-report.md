# mKCP XOR Lane Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. Benchmarks use
`-benchtime=200000x -count=6` on a 1356-byte payload. The old implementation is
kept in the test as the reference function, so both versions run in the same
process.

## Change

The simple authenticator XOR transforms are a four-lane dependent mix. Forward and
backward now process four bytes per step with `encoding/binary` lane loads and
stores, then handle any tail bytes with the original loop. This keeps byte order
independent and preserves the exact wire transformation.

| Benchmark | Old median | New median | Result |
| --- | ---: | ---: | ---: |
| XOR forward | 606.9ns | 157.4ns | about -74.1% |
| XOR backward | 467.1ns | 134.2ns | about -71.3% |
| XOR allocations | 0 | 0 | unchanged |

## Verification

- Forward and backward equivalence tests over empty, sub-lane, lane, and non-lane
  sizes: green.
- Forward/backward round-trip test: green.
- Simple authenticator allocation-free round trip: green.
- Plain, seeded-AEAD, and all header round trips: green.
- `go test ./transport/mkcp -count=1`: green.
- `staticcheck ./transport/mkcp`: green.
- `go test -race ./transport/mkcp -count=1`: green.
- End-to-end `BenchmarkRoundTrip` rerun stayed within host noise and kept zero
  deterministic allocation growth.
