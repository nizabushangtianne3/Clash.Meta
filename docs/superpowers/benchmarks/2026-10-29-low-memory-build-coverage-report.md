# Low-memory build coverage

Date: 2026-10-29

Environment: Windows, `windows/amd64`, Go `1.27.1`.

The previously deferred `with_low_memory` build tag is now locally verified.
The full suite ran with `go test -tags=with_low_memory ./... -count=1 -p 1`;
it exits 0 and compiles the 16 KiB relay / 8 KiB UDP buffer variant used by
low-memory builds. This closes the untested-build-tag follow-up from the
2026-10-28 scan.
