# Connection View Encoder

Date: 2026-10-26

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.26.0` (the ci
version), baseline and current captured back to back under one load.

## What the view paid

The connection table is polled on a timer by both router writers: the plain
GET and the websocket push. `encoding/json` walked the snapshot's struct tags
and called a marshaler per field, and each of those had to hand back a fresh
slice, so a poll of a hundred connections paid 903 allocations and 81.5 KiB
for a document the host reads and discards. The allocation profile named them:

| Site | Share | What |
| --- | ---: | --- |
| `internal/strconv.AppendUint` | 33.6% | the two counters, through `common/atomic.Int64.MarshalJSON` |
| `net/netip.Addr.MarshalText` | 20.2% | the three addresses |
| `time.Time.MarshalJSON` | 18.7% | the connection start |
| `strconv.appendQuotedWith` | 15.7% | string fields and buffer growth |
| `uuid.UUID.MarshalText` | 6.7% | the connection id |
| `constant.DNSMode.AppendText` | 4.5% | the dns mode name |

## What it pays now

`tunnel/statistic/json.go` appends the whole document into a buffer the writer
owns, so a poll costs the two allocations the snapshot has always needed: the
table slice and the tracker info it points at. The field set, their order and
the escaping are pinned by `tunnel/statistic/connection_view_test.go` against
the golden document `encoding/json` produced before this change, and every
string escape the encoder emits is checked against `encoding/json` itself over
values that cover control characters, HTML characters, the two unicode line
separators and invalid utf-8.

`hub/route/connections.go` hands the encoder a buffer it reuses between
websocket polls; the GET writer takes one right sized buffer per request. Both
paths now go through `AppendJSON`, so neither stands up an encoder.

| Metric | 100 connections | 500 connections |
| --- | ---: | ---: |
| `sec/op` | 334.42us -> 50.37us (-84.94%) | 1.8726ms -> 271.1us (-85.52%) |
| `B/op` | 83523 -> 948 (-98.86%) | 437.98Ki -> 4.248Ki (-99.03%) |
| `allocs/op` | 903 -> 2 (-99.78%) | 4504 -> 2 (-99.96%) |

benchstat p=0.008 on both sizes, n=5.

## The marshaler round this replaces

Two earlier commits reported the view going from 1222 to 603 and then to 503
allocations. Neither number reproduces: `ee9bc2d5b`, which claimed 603,
measures 903 allocations and 83.55 KiB, and `00a79dd6e`, which claimed 603 ->
503, measures 903 and 83.47 KiB at `connections=100`. The gain those commits
actually landed is the one `encoding/json` cannot avoid, the counters no longer
standing up an encoder: 1222 -> 903.

The `AppendText` half of `00a79dd6e` went further and is deleted here. It
delivered nothing because `encoding/json` v1 does not implement
`encoding.TextAppender` - only `encoding/json/v2` does, and this tree records
json v1 in its benchmark - so `MarshalText` calling `AppendText(nil)` cost the
same allocation as `[]byte(e.String())`. A standalone probe over a struct with
one `TextAppender` field and one without measured 174.9 ns / 2 allocations
against 169.7 ns / 2 allocations under go 1.26. The five methods and their test
are gone; `constant/enum_text_test.go` keeps the part that pins the wire names.

## Flaky package noted, unrelated

`listener/inbound` does not survive a loaded full-suite run on this host:
`TestInboundMieru` raised `Exception 0xc0000005` (access violation at 0x8)
inside `syscall.WSARecvFrom`, and `TestInboundVMess_MKCP` failed its response
assertion once. Both reproduced on the tree before this change, both pass when
their package is run on its own, and the same package took 122s and passed in
the full-suite run before this one. Left as a separate investigation.
