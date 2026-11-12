# 0001: Assigning processes to a Job Object

- Status: accepted (2026-11-08)
- Risk addressed: R1 (orphan processes)

## Context

Each instance, with all its descendants, must live in a Job Object configured with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`. The process must be in the job **before** it can spawn children. `PROC_THREAD_ATTRIBUTE_JOB_LIST`, which solves this at creation time, requires Windows 10 / Server 2016 and is therefore incompatible with the legacy build (ADR 0007).

## Decision

1. Start the process through `os/exec` with `CREATE_SUSPENDED` (plus `CREATE_NEW_PROCESS_GROUP`).
2. `OpenProcess`, then `AssignProcessToJobObject`.
3. Resume it with `NtResumeProcess` (ntdll, undocumented but available since XP).
4. If assignment fails: `TerminateProcess`. A process never runs outside its job.

Fallback if `NtResumeProcess` becomes a problem: call `CreateProcess` directly and `ResumeThread` on the primary thread.

## Consequences

- We keep `os/exec` (pipes, environment, handle inheritance list managed by Go).
- Aggregated metrics come from the job: active processes, cumulative CPU (`JobObjectBasicAccountingInformation`), PIDs (`JobObjectBasicProcessIdList`), then memory per PID (`K32GetProcessMemoryInfo`).
- If the daemon dies, the OS closes the job handle and kills the tree: no orphan cleanup is needed on restart on Windows.

## Evidence (PoC)

| Test | Iterations | Outside job | Survivors | Max delay |
|---|---|---|---|---|
| `TerminateJobObject` on a 13-process tree | 100 | 0 | 0 | 35 ms |
| Supervisor killed abruptly | 100 | 0 | 0 | 24 ms |
| Real service killed (7 direct children incl. node, python, php) | 1 | 0 | 0 | < 2 s |
