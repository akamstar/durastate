# 0007: Windows Server 2012 R2 legacy build

- Status: proposed, **untested** (no test machine available)
- Risk addressed: R6

## Context

Since Go 1.21, official Go binaries require Windows 10 / Server 2016. Option B was chosen: an official Server 2016+ build and a best-effort Server 2012 R2 build compiled with a patched toolchain (`XTLS/go-win7`). No VM or clone of the 2012 server is available yet.

## Decision (until a test machine is available)

- v1 development is **not blocked** by this.
- Coding rule: every Windows API used must exist on Server 2012 R2, or have a fallback.
  - Already used and compatible in theory: `NtResumeProcess` (XP+), nested Job Objects (Windows 8 / 2012+), `K32GetProcessMemoryInfo` (Windows 7+), preshutdown (Vista+), `NetLocalGroupAdd`, `ImpersonateNamedPipeClient`, `AllocConsole`, `GenerateConsoleCtrlEvent`.
  - Not allowed without a fallback: `PROC_THREAD_ATTRIBUTE_JOB_LIST` and ConPTY pseudo-consoles (Windows 10 1809+).
- Later, in CI: build with the patched toolchain, to at least check that the code and its dependencies compile.

## To do once a machine is available

Replay PoC experiments E1, E2, E5 and E6 on Server 2012 R2. Experiments `e1`, `e2` and `e6` install nothing (they spawn processes for a few seconds); the service test requires an install.
