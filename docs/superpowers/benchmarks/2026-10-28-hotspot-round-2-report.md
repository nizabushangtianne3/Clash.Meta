# Hotspot round 2

Date: 2026-10-28

This round is the hotspot-slice execution wave after the six-direction scan.
Each agent had a disjoint file scope; this file records what landed, what was
verified, and what was deferred to the next directory-level sweep.

## Landed

| Area | Change | Gate |
| --- | --- | --- |
| tunnel UDP flow | precheck and dial reuse one metadata clone; UDP read cache is an inline reset field instead of an atomic pointer. | `TestNewUDPFlowMetadataSharesOneClone`, `TestPacketMappingRestoreCacheStaysInTheReadLoop` with a zero-allocation gate; `go test -race ./tunnel`. |
| listener/tunnel UDP | reads through the unified AddrPort poller, carries the source as `FlowKey`, writes back through `WriteToUDPAddrPort`. | Real stub wait-read tests, zero-length packet test, error exit test, FlowKey and write-back assertions; `go test -race ./listener/tunnel`. |
| TUIC v5 client | native and QUIC receive paths route the raw frame after the fixed routing header instead of re-parsing a full packet, and the per-datagram goroutine is gone. | New `client_test.go` covers routing header, raw frame lengths for IPv4/IPv6/domain/no-address fragments, short headers and unknown address types; `go test -race ./transport/tuic/v5`. |
| hub/route | websocket frames use a pooled single-write scratch, connection bodies are pooled, and traffic/memory/log JSON is hand-appended with golden parity. | New `server_json_test.go` pins golden documents, write counts, allocation gates and websocket handshake shape; `go test -race ./hub/route`. |
| rule indexes | suffix trie and position binary search are extracted into `component/ruleindex`, duplicated implementations removed. | New package tests plus existing tunnel and provider suites; `go test -race ./rules/provider`. |
| Android | duplicated netlink parser removed and unused `buildAndroidRules` file deleted. | `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`. |

## Verification

- `gofmt` on the changed file set
- `go vet ./...`
- `go test ./... -count=1 -p 2`
- `go test -race` on `component/ruleindex`, `hub/route`, `listener/tunnel`,
  `rules/provider`, `transport/tuic/v5`, `tunnel`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...`

## Next sweep

This was still not a full-repository partition. The next six-agent sweep should
cover directories instead of hotspots: `adapter`, `component`, `dns/config`,
  `hub`, `listener`, `transport/common`.
