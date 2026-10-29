# Sing new-packet coverage

Date: 2026-10-30

Environment: Windows, `windows/amd64`, Intel Core i9-9900, Go `1.27.1`.

Adds a focused benchmark and behavior test for the sing UDP `NewPacket` entry.
The benchmark is CPU1-pinned (`-cpu 1`) after an unpinned paired run showed
system-scheduler noise exceeding the measured path. Baseline on this commit is
1.810us ±1%, 17078B, 6 allocs/op. The behavior test pins source/destination and
payload handling.

The benchmark provides a regression gate for future work on the unavoidable
per-datagram wrapper and metadata path.
