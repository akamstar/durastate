# 0003: Graceful stop sequence and PHP workers

- Status: accepted (2026-11-08)
- Risk addressed: R3

## Context

- Intermediate processes (`cmd.exe`, `npm`) die immediately on CTRL_BREAK (exit code `0xC000013A`) while the actual worker is still finishing.
- On Windows, PHP has no `pcntl`: **Symfony 7.4 (`messenger:consume`) and Laravel 12 (`queue:work`) are killed instantly by CTRL_BREAK** (10 to 15 ms), interrupting the current message.
- Once, a `stop_command` blocked a stop for 11 minutes (not reproduced, root cause unknown).

## Decision

Stop sequence for an instance:

1. `stop_command` if set, bounded by `stop_timeout` (context) **and** a 2 s `WaitDelay` (never hang on an inherited pipe);
2. `stop_signal` (`ctrl_break` by default on Windows, `none` allowed);
3. wait until **the whole job is empty**, not just the main process;
4. on `stop_timeout`: `TerminateJobObject`.

For PHP on Windows, `stop_command` is the recommended method and is provided by presets:

- `symfony-messenger`: `stop_command: php bin/console messenger:stop-workers`, `stop_signal: none`;
- `laravel-queue`: `stop_command: php artisan queue:restart`, `stop_signal: none`.

## Consequences

- To document: a message interrupted by a kill stays in the queue but is only redelivered after `redeliver_timeout` (3600 s by default, Symfony Doctrine transport) or `retry_after` (90 s by default, Laravel).
- To document: these commands rely on the application cache (a shared cache is required: filesystem or Redis for Symfony, `file`, `redis` or `database` for Laravel) and target all workers of the application at once.
- `dstate status` and logs must distinguish "graceful stop" from "forced kill on timeout".

## Evidence (PoC)

| Scenario (20 s handler) | Symfony 7.4 | Laravel 12 |
|---|---|---|
| CTRL_BREAK only | killed in 15 ms, message still queued | killed in 10 ms, message still queued |
| `stop_command` | message completed, 18 s | message completed, 18 s |
| `stop_command`, 3 replicas | 3/3 completed | 3/3 completed |
| 120 s handler, 10 s timeout | killed at 10 s, message still queued | killed at 10 s, message still queued |
| Service stop | | job completed, stop in 17.8 s |
| Machine shutdown | | job completed, stop in 17.7 s |
