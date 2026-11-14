# 0008: Configuration format, version 1

- Status: accepted (2026-11-14)

## Context

`dstate.yaml` is the single source of truth for services. Once released, version 1 of the format cannot break: every rule below is part of the compatibility contract.

## Decision

**Parsing**

- YAML only, one document per file. Unknown fields are errors, with a "did you mean" hint. `null` is never accepted (`alerts:` left empty is an error).
- Every error is reported, not just the first, with file, line and column.
- Typed fields also accept quoted text (`replicas: "2"`), so that `replicas: ${N}` works.
- Anchors, aliases and merge keys (`<<`) are supported. Root keys starting with `x-` are ignored, as in Docker Compose, to hold anchors.

**Variables**

- `${VAR}` (error if unset), `${VAR:-default}`, `${VAR:?message}`, `$$` for a literal `$`. `$VAR` and `%VAR%` are left as is. Nested references are rejected.
- Only string values are interpolated, never the raw text: a value cannot change the YAML structure.
- Sources: the daemon environment, overridden by the global `.env` files. The root `env_file` is read first, with the daemon environment only.
- A service `.env` only feeds the environment of its processes, never interpolation.
- `.env` values are literal. Variable names are case-insensitive on Windows.
- Process environment, weakest first: daemon environment (if `inherit_env`), global `.env`, service `.env`, `env` block, then `DSTATE_SERVICE`, `DSTATE_INSTANCE`, `DSTATE_REPLICAS`. The `DSTATE_` prefix is reserved.

**Values**

- Durations use Go syntax plus `d` (`14d`). Sizes need a unit, in powers of 1024 (`50MB`).
- A string `command` is split like a shell, with backslashes kept literal (Windows paths) and `{{ ... }}` never split. `shell: true` requires a string.
- Relative paths are resolved from the config file directory, which is also the default `cwd`.
- Service names: lowercase letters, digits, `-` and `_`, 63 characters max; `all` and Windows device names (`con`, `nul`...) are reserved.
- `stop_signal`: `ctrl_break`, `kill` or `none` on Windows; `sigterm`, `sigint`, `kill` or `none` on Unix. `winsvc://` targets are Windows only.
- Defaults merge in three levels: service, `defaults` block, built-in values. Nested blocks merge field by field; `alerts.rules` is replaced as a whole. `defaults` cannot hold `command`, `cwd`, `env`, `env_file`, `replicas`, `autostart` or `wait_for`.
- Per-service alerts: `alerts: {enabled: false}` disables them, `alerts: {rules: [...]}` replaces the global rules.

**Templates**

- `{{.Service}}`, `{{.Instance}}`, `{{.Replicas}}` and the `add` function, rendered in `command`, `stop_command`, `cwd` and the `env` block only, never in `.env` values.

**Secrets**

- Values interpolated from a `.env` file or from a variable named like a secret (`TOKEN`, `PASSWORD`, `SECRET`, `KEY`...) are masked in every output, as are `.env` variables, bot tokens, webhook URLs and secret-named headers.

## Consequences

- Logs rotation works in whole megabytes and days: `max_size` is at least `1MB`, `max_age` is `0` or at least `1d`.
- `dstate validate` interpolates with the user environment; the daemon uses its own when applying the config.
- Known limit: a value interpolated from a `.env` file that contains `{{` in `command` is read as a template.
- Known parser quirk: inside a flow mapping, `{targets: [tcp://db:5432]}` must be quoted; block style works unquoted.
