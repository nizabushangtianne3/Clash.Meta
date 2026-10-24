# mKCP Write Error Path Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`.

## Change

`Conn.writeSegments` used to retry every failed segment write five times, sleeping
100ms between attempts. That duplicated mKCP's own retransmission responsibility and
could block the flush loop for about 500ms per segment when a UDP writer failed
persistently.

It now writes each segment once and ignores the writer error. Unacknowledged segments
remain in the send window and are retransmitted by the normal KCP timeout path.

| Metric | Before | After | Result |
| --- | ---: | ---: | ---: |
| Write attempts on persistent failure | 5 | 1 | -80.00% |
| Fixed sleep on persistent failure | 400ms | 0ms | -100.00% |

## Verification

- New persistent-failure test asserts one attempt and no sleep: green.
- Plain, seeded-AEAD, and all header round trips: green.
- `go test ./transport/mkcp -count=1`: green.
- `staticcheck ./transport/mkcp`: green.
- `go vet ./...`: green.
- `go test -race ./transport/mkcp -count=1`: green.
