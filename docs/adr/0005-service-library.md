# 0005: Windows service library

- Status: accepted (2026-11-08)
- Risk addressed: R5

## Decision

Use **`golang.org/x/sys/windows/svc`** (with `svc/mgr` and `svc/eventlog`) directly on Windows, not `kardianos/service`. Install-time configuration:

- `StartAutomatic` + `DelayedAutoStart`;
- recovery actions: restart after 5 s, 10 s, 30 s, reset after 24 h, also on non-crash failures;
- preshutdown: `AcceptPreShutdown` with a 3 min timeout (`SERVICE_CONFIG_PRESHUTDOWN_INFO` via `ChangeServiceConfig2`);
- while stopping: `StopPending` with a checkpoint every 2 s;
- Event Log source registered at install time.

`kardianos/service` remains an option for Linux and macOS (v1.1), behind the `ServiceInstaller` interface.

## Consequences

- Delayed start means the daemon starts about 2 minutes after boot. `wait_for` remains the main safeguard; provide a non-delayed install option.
- A service only sees the **system PATH**: an executable found only in the user PATH (e.g. PHP from Laragon) cannot be found. Requires an explicit error message and a `dstate doctor` check.

## Evidence (PoC, Windows 11, LocalSystem service)

| Test | Result |
|---|---|
| Install | `AUTO_START (DELAYED)`, 3 recovery actions as expected |
| Session | daemon runs in session 0 |
| Daemon killed abruptly | 0 surviving children, restarted by the SCM, services relaunched |
| `Stop-Service` | full parallel stop in 17.8 s (Laravel job completed) |
| Machine shutdown | preshutdown received, full graceful stop in 17.7 s |
| Boot | service started 2 min 24 s after boot, children running |
