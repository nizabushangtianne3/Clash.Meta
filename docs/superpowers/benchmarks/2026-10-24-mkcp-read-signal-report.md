# mKCP Read Signal Report

Date: 2026-10-24

Environment: Windows `amd64`, Intel Core i9-9900, Go `1.27.1`.

The read path now counts waiters and replaces its notification channel only when a
reader can consume the wakeup. The focused no-waiter signal is allocation-free;
the real echo benchmark has blocked readers on both ends, so its end-to-end time
and allocation totals are unchanged within noise, as expected.

Verification:

- Focused channel/allocation test and mKCP package tests: green.
- `go vet ./...`: green.
- Full tests and the `with_gvisor` suite: green.
- mKCP package race and inbound KCP interop race: green.
