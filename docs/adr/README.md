# Architecture Decision Records

One decision per file: context, decision, consequences, evidence.
Evidence comes from the Windows proof of concept (`poc/`, kept locally, not versioned); key figures are copied here.

| # | Decision | Status |
|---|---|---|
| [0001](0001-job-object-assignation.md) | Assigning processes to a Job Object | Accepted |
| [0002](0002-ctrl-break-strategy.md) | Sending CTRL_BREAK, including from a service | Accepted |
| [0003](0003-graceful-stop.md) | Graceful stop sequence and PHP workers | Accepted |
| [0004](0004-ipc-pipe-security.md) | Named pipe security and caller identification | Accepted (non-admin matrix pending) |
| [0005](0005-service-library.md) | Windows service library | Accepted |
| [0006](0006-log-capture.md) | Non-blocking stdout / stderr capture | Accepted |
| [0007](0007-windows-legacy-build.md) | Windows Server 2012 R2 legacy build | Proposed (untested) |
