# Inline proxy UUID

Date: 2026-11-03

`Base` formatted a fresh UUID into a string for every proxy even though `Id`
had no interface caller; `AppendJSON` immediately encoded it. `Base` now keeps
the 16-byte UUID and `jsonutil.AppendUUID` writes the canonical lower-case form
directly. A canonical-output test pins the wire format.

The 800-node `BenchmarkParseConfig` pair drops allocations from 90.74k to
89.94k (-0.88%, p=0.000) and bytes from 4.955Mi to 4.918Mi (-0.74%, p=0.000).
Time is unchanged within noise (p=0.353, n=10).

Verification:

- `go test ./common/jsonutil ./adapter ./adapter/outbound ./hub/executor -count=1`
- `go test -race ./common/jsonutil ./adapter ./adapter/outbound ./hub/executor`
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`
