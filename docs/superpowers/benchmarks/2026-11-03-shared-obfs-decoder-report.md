# Shared obfs decoder

Date: 2026-11-03

`NewShadowSocks` and `NewSnell` each built a fresh `structure.Decoder` even
though its options are immutable and identical. Both constructors now share
one package decoder, and the redundant local aliases are gone. A concurrent
Shadowsocks plugin-build test covers read-only decoder sharing.

The 800-node `BenchmarkParseConfig` pair drops allocations from 92.33k to
91.53k (-0.87%, p=0.000), bytes from 5.016Mi to 4.992Mi (-0.49%, p=0.000),
and time from 19.12ms to 16.48ms (-13.82%, p=0.029; n=10).

Verification:

- `go test ./adapter/outbound -run TestNewShadowSocksSharesPluginDecoderAcrossGoroutines`
- `go test -race ./adapter/outbound`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
