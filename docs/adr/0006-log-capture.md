# 0006: Non-blocking stdout / stderr capture

- Status: accepted (2026-11-08)
- Risk addressed: R7

## Decision

- The daemon creates the pipes (`os.Pipe`) and closes its write ends after start: reading ends when the last descendant closes the pipe, independently of `cmd.Wait`.
- One reader per stream reads continuously; lines go through a **bounded queue** to a disk writer.
- Queue full: the line is **dropped and counted**; the writer inserts an "N lines dropped" marker as soon as it catches up.
- Lines longer than 64 KB are truncated (and marked); the rest is skipped until the next newline.
- An in-memory ring buffer (last lines) and non-blocking subscribers serve live tailing.

## Consequences

- A slow disk never blocks the child; in exchange, lines can be dropped under very heavy load (always reported).
- Ideas for v1: batched writes, a queue sized in bytes, cheaper timestamps; rotation through `lumberjack`.

## Evidence (PoC)

| Test | Max `write` block | Dropped lines | Daemon working set |
|---|---|---|---|
| 20 MB/s, normal disk | 20 ms | 1.1 % | 16 MB |
| 20 MB/s, 1 ms per written line | 33 ms | 98 % (counted) | 17 MB |
| 10 MB lines | 13 ms | 0 (truncated at 64 KB) | 10 MB |
