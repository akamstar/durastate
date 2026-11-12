# 0004: Named pipe security and caller identification

- Status: accepted; non-admin matrix pending (manual test)
- Risk addressed: R4

## Decision

- The pipe is created through `go-winio` with the SDDL `D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;<dstate-users SID>)`.
- The local group `dstate-users` is created at install time with `NetLocalGroupAdd`.
- Identification happens **on the server side only**, never from the JSON payload:
  1. after the first read on the pipe: `ImpersonateNamedPipeClient` on a locked OS thread, `OpenThreadToken`, `RevertToSelf`;
  2. the role comes from the token: `admin` if effective member of Administrators or SYSTEM, `operator` if member of `dstate-users`, otherwise denied.
- The client connects with the **Identification** impersonation level (`DialPipeAccessImpLevel`): the server can read its token but cannot act on its behalf. By default `go-winio` connects as Anonymous, which makes identification impossible.
- Fallback when a client stays Anonymous: the client process token (`GetNamedPipeClientProcessId`, `OpenProcessToken`, `DuplicateTokenEx`).

## Consequences

- A **non-elevated** administrator (UAC) has Administrators as "deny only" and is treated as non-admin. Error message to provide: "run from an elevated terminal or add the user to the dstate-users group".
- Membership in `dstate-users` only takes effect at the next logon (to document).
- To develop without elevation, a dev option adds the current user to the ACL (never in production).

## Evidence (PoC)

| Caller | Result |
|---|---|
| Elevated admin, daemon running as LocalSystem service | `role=admin`, admin action allowed |
| Non-elevated admin (dev mode) | `admin=false`, `role=operator`, admin action denied |
| Client at Anonymous level | anonymous token, denied |
| Non-admin member of `dstate-users` | to test |
| User outside the group | to test |
