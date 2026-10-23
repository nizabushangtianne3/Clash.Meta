# Sniffer: Keep The Candidate Set On The Stack

Date: 2026-10-22

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

## What Changed

`Dispatcher.sniffDomain` filtered the configured sniffers by network and port
for every sniffed TCP connection, and built that set with
`make([]candidate, 0, len(sd.sniffers))` - a heap allocation per connection even
though the set is only ever compacted in place from there on and never grows
past the filter's result. It lives in a four-entry stack array now, which covers
every sniffer a config can wire (quic, tls, http); a larger set still falls back
to the heap through `append`.

Escape analysis over `component/sniffer` does not report the array escaping, and
the benchmark below measures the allocation disappearing, so it stays on the
stack.

## Gate

`BenchmarkSniffDomain` drives one connection through the per-connection work -
filter the sniffers, then feed the buffered data until one names the domain —
on the same tree before and after:

| Metric | Before | After | Delta |
| --- | ---: | ---: | ---: |
| sec/op | 475.6n ± 76% | 288.0n ± 15% | -39.44% (p=0.000 n=10) |
| B/op | 768 | 656 | -14.58% |
| allocs/op | 4 | 3 | -25.00% |

The host was loaded, so the timing spread is wide and the allocation and byte
counts are the gate; both have zero variance across the ten runs.

The rest of the tree is covered by `go test ./... -count=1 -p 2` (94 packages),
`go vet ./...` and `-race` over `component/sniffer`.
