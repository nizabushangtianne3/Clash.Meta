# Direct proxy options

Date: 2026-11-03

`ParseProxy` used a variadic option slice plus functional builders even though
the repository only set API dialer, tunnel and provider fields. That forced an
options-slice allocation for every parsed proxy. The option functions are
replaced by one `ProxyOptions` pointer; a nil pointer means the defaults used
by conversion tests, while config and providers pass one stack value.

The 800-node `BenchmarkParseConfig` pair drops allocations from 91.53k to
90.74k (-0.87%, p=0.000) and bytes from 4.992Mi to 4.955Mi (-0.73%, p=0.000).
Paired time is unchanged within noise (p=0.353, n=10). A value-shaped option
regressed time by 3.42% (p=0.029) in a first capture and was replaced by this
pointer shape rather than shipped.

Verification:

- focused adapter/provider/convert/config/executor tests
- `go test -race ./adapter ./adapter/provider ./common/convert ./config ./hub/executor`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
