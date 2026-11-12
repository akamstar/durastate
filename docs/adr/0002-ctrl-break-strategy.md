# 0002: Sending CTRL_BREAK, including from a service

- Status: accepted (2026-11-08)
- Risk addressed: R2

## Context

`GenerateConsoleCtrlEvent` requires the sender and the target to share a console. A Windows service has no console. Three strategies were compared:

- `shared`: the daemon owns a console (allocated and hidden if missing); children inherit it, each in its own process group;
- `helper`: each child gets its own windowless console (`CREATE_NO_WINDOW`); a helper process attaches to it to send the event;
- `helper-newconsole`: same, with `CREATE_NEW_CONSOLE` and a hidden window.

## Decision

- **`shared`** is the default:
  - at startup, if the daemon has no console: `AllocConsole`, then hide the window;
  - if `AllocConsole` returns `ERROR_ACCESS_DENIED`, the process already has a windowless console: carry on;
  - children are created with `CREATE_NEW_PROCESS_GROUP`; stop via `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)`.
- `helper` stays in the code as an internal fallback, not exposed in v1.
- Only `CTRL_BREAK` is usable (`CREATE_NEW_PROCESS_GROUP` disables `CTRL_C` in the child). `ctrl_c` is removed from the `stop_signal` options.

## Consequences

- No extra process per stop, a single conhost for the daemon.
- A Ctrl+C typed in the `dstate run` terminal does not reach the children (verified): only the daemon gets it and stops everything gracefully.

## Evidence (PoC)

All 3 strategies pass on all 7 targets, with and without a console; `shared` is validated in session 0 (real service):

| Target | Result |
|---|---|
| Go fixture, Go tree 1+2 | signal received by 3/3, clean exit |
| `node app.js` (`SIGBREAK`) | clean exit |
| `cmd /C npm start` | clean exit of node (cmd dies immediately, see ADR 0003) |
| `python app.py` (`signal.SIGBREAK`) | clean exit |
| `php app.php` (`sapi_windows_set_ctrl_handler`) | clean exit |
| `cmd /C <exe>` | clean exit |
