# Buffered packet read allocation

Date: 2026-10-31

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.
The paired benchmark reads a 1500-byte datagram through a
`ReadFromWithBuffer` packet connection; each side ran 5 times. The benchmark
was added with this change.

| Metric | Baseline | Current | Result |
| --- | ---: | ---: | ---: |
| Bytes | 104 B/op | 80 B/op | -23.08% |
| Allocations | 4 | 3 | -25% |
| Time | 160.3 ns | 154.7 ns | no significant change (`p=0.421`) |

The buffer callback is now a method value tracked by the connection instead of
a fresh closure over a local variable on every datagram. The read buffer handle
is cleared on every return path so a pooled buffer is never handed out twice.

## Verification

- `go test ./common/net/packet -count=1`
- `go test -race ./common/net/packet -count=5`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- Paired `benchstat`, n=5
