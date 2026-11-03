# Proxy decoder reuse

Date: 2026-11-03

`ParseProxy` used to build a fresh `structure.Decoder` for every proxy and
again for every `smux` block. The decoder only owns immutable options, so the
proxy parser now uses one shared package-level decoder for all mappings.

The 800-node `BenchmarkParseConfig` pair drops allocations from 93.15k to
92.34k (-0.86%, p=0.000, n=10) and bytes from 5.041Mi to 5.016Mi (-0.49%,
p=0.000, n=10). Time stayed inside host noise (p=0.165). The benchmark logs
break the benchmark-name line, so both paired captures were normalized to the
same benchmark name before `benchstat`.

Verification:

- `go test ./adapter ./config -count=1`
- `go test -race ./adapter -run TestParseProxySharesDecoderAcrossGoroutines`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
