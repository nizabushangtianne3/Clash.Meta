# mKCP Plain Write Direct Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`. The comparison is
`benchstat -count=10 -benchtime=100000x` against `ca658c539`.

## Change

The unauthenticated mKCP write path used to serialize each segment into one pooled
buffer and then copy that buffer into the frame sent to the writer. It now serializes
the segment directly into the frame and sends that frame. The authenticated path still
creates the plaintext buffer because the AEAD needs plaintext and ciphertext storage
that cannot safely overlap.

| Benchmark | Before | After | Result |
| --- | ---: | ---: | --- |
| Plain segment write | 83.32ns | 41.43ns | -50.28%, p=0.000 |
| Plain write throughput | 15.29 GiB/s | 30.75 GiB/s | +101.11%, p=0.000 |
| Authenticated segment write | 1.544us | 1.540us | no significant change, p=0.224 |
| Plain/authenticated allocations | 0 | 0 | unchanged |

## Verification

- Plain and authenticated allocation-free write tests: green.
- Plain, seeded-AEAD, and all header round trips: green.
- `go test ./transport/mkcp -count=1`: green.
- `staticcheck ./transport/mkcp`: green.
- `go test -race ./transport/mkcp -count=1`: green.
- `go vet ./...`: green.
