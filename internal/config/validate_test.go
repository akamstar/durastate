package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const slack = "    s: {type: slack, webhook_url: \"https://hooks.slack.com/x\"}\n"

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name, goos, src, want string
	}{
		// Services
		{"no services key", "linux", "version: 1\n", "dstate.yaml: services: at least one service is required"},
		{"empty services", "linux", "services: {}\n", ":1:1: services: at least one service is required"},
		{"missing command", "linux", "services:\n  a:\n    replicas: 2\n", `:2:3: services.a: missing required field "command"`},
		{"uppercase name", "linux", "services:\n  Api: {command: run}\n", `:2:3: services.Api: invalid name "Api"`},
		{"name with a dot", "linux", "services:\n  a.b: {command: run}\n", `services.a.b: invalid name "a.b"`},
		{"name too long", "linux", "services:\n  " + strings.Repeat("a", 64) + ": {command: run}\n", "invalid name"},
		{"reserved name all", "linux", "services:\n  all: {command: run}\n", `services.all: "all" is reserved`},
		{"windows device name", "linux", "services:\n  nul: {command: run}\n", `services.nul: "nul" is a reserved device name on Windows`},
		{"replicas zero", "linux", "services:\n  a: {command: run, replicas: 0}\n", "services.a.replicas: must be at least 1 (got 0)"},
		{"bad env name", "linux", "services:\n  a:\n    command: run\n    env: {BAD-NAME: x}\n", `:4:11: services.a.env.BAD-NAME: invalid variable name "BAD-NAME"`},
		{"reserved env prefix", "linux", "services:\n  a:\n    command: run\n    env: {DSTATE_INSTANCE: x}\n", "services.a.env.DSTATE_INSTANCE: the DSTATE_ prefix is reserved"},
		{"reserved env prefix any case", "windows", "services:\n  a:\n    command: run\n    env: {dstate_x: x}\n", "the DSTATE_ prefix is reserved"},

		// Policy values
		{"empty exit codes", "linux", "services:\n  a: {command: run, success_exit_codes: []}\n", "services.a.success_exit_codes: must not be empty"},
		{"negative exit code", "linux", "services:\n  a:\n    command: run\n    success_exit_codes: [0, -1]\n", ":4:29: services.a.success_exit_codes[1]: invalid exit code -1"},
		{"max_restarts zero", "linux", "services:\n  a: {command: run, max_restarts: 0}\n", "services.a.max_restarts: must be at least 1 (got 0)"},
		{"zero restart_window", "linux", "services:\n  a: {command: run, restart_window: 0s}\n", "services.a.restart_window: must be greater than 0"},
		{"zero stop_timeout", "linux", "services:\n  a: {command: run, stop_timeout: 0s}\n", "services.a.stop_timeout: must be greater than 0"},
		{"zero backoff.initial", "linux", "services:\n  a: {command: run, backoff: {initial: 0s}}\n", "services.a.backoff.initial: must be greater than 0"},
		{"low multiplier", "linux", "services:\n  a: {command: run, backoff: {multiplier: 0.5}}\n", "services.a.backoff.multiplier: must be at least 1 (got 0.5)"},
		{"backoff max below initial", "linux", "services:\n  a:\n    command: run\n    backoff: {initial: 10m}\n", ":4:5: services.a.backoff: max (5m) must not be lower than initial (10m)"},
		{"crash loop max below initial", "linux", "services:\n  a:\n    command: run\n    crash_loop_retry: {initial: 2h}\n", "services.a.crash_loop_retry: max (1h) must not be lower than initial (2h)"},
		{"zero crash loop initial", "linux", "services:\n  a: {command: run, crash_loop_retry: {initial: 0s}}\n", "services.a.crash_loop_retry.initial: must be greater than 0"},
		{"ctrl_break on unix", "linux", "services:\n  a: {command: run, stop_signal: ctrl_break}\n", `services.a.stop_signal: "ctrl_break" is not supported on linux (expected sigterm, sigint, kill, none)`},
		{"sigterm on windows", "windows", "services:\n  a: {command: run, stop_signal: sigterm}\n", `"sigterm" is not supported on windows (expected ctrl_break, kill, none)`},
		{"small max_size", "linux", "services:\n  a: {command: run, logs: {max_size: 512KB}}\n", "services.a.logs.max_size: must be at least 1MB (got 512KB)"},
		{"zero max_files", "linux", "services:\n  a: {command: run, logs: {max_files: 0}}\n", "services.a.logs.max_files: must be at least 1 (got 0)"},
		{"max_age in hours", "linux", "services:\n  a: {command: run, logs: {max_age: 12h}}\n", "services.a.logs.max_age: must be 0 (no limit) or at least 1d (got 12h)"},
		{"empty service rules", "linux", "services:\n  a: {command: run, alerts: {rules: []}}\n", "services.a.alerts.rules: must not be empty, use enabled: false"},
		{"error in defaults reported once", "linux", "defaults:\n  stop_timeout: 0s\nservices:\n  a: {command: run}\n  b: {command: run}\n", ":2:3: defaults.stop_timeout: must be greater than 0"},

		// wait_for
		{"tcp without port", "linux", "services:\n  a: {command: run, wait_for: {targets: [tcp://db]}}\n", `services.a.wait_for.targets[0]: invalid target "tcp://db" (expected tcp://host:port)`},
		{"tcp bad port", "linux", "services:\n  a: {command: run, wait_for: {targets: [\"tcp://db:99999\"]}}\n", `invalid port "99999"`},
		{"http without host", "linux", "services:\n  a: {command: run, wait_for: {targets: [\"http://\"]}}\n", `invalid URL "http://"`},
		{"unknown scheme", "linux", "services:\n  a: {command: run, wait_for: {targets: [\"redis://db:6379\"]}}\n", `unsupported scheme "redis" (expected tcp, http, https, winsvc)`},
		{"no scheme", "linux", "services:\n  a: {command: run, wait_for: {targets: [\"db:3306\"]}}\n", `invalid target "db:3306"`},
		{"winsvc on unix", "linux", "services:\n  a: {command: run, wait_for: {targets: [winsvc://MySQL80]}}\n", "winsvc:// targets are only supported on Windows"},
		{"winsvc without name", "windows", "services:\n  a: {command: run, wait_for: {targets: [\"winsvc://\"]}}\n", "invalid Windows service name"},
		{"zero wait timeout", "linux", "services:\n  a: {command: run, wait_for: {timeout: 0s}}\n", "services.a.wait_for.timeout: must be greater than 0"},

		// Templates
		{"unknown template field", "linux", "services:\n  a:\n    command: run --id {{.Instanc}}\n", `:3:5: services.a.command: invalid template: can't evaluate field Instanc (available: .Service, .Instance, .Replicas)`},
		{"template syntax", "linux", "services:\n  a:\n    command: run\n    cwd: \"x{{.Instance\"\n", "services.a.cwd: invalid template"},
		{"unknown template function", "linux", "services:\n  a:\n    command: run\n    env: {PORT: \"{{sub 8000 .Instance}}\"}\n", `services.a.env.PORT: invalid template: function "sub" not defined`},
		{"stop command template", "linux", "services:\n  a:\n    command: run\n    stop_command: stop {{.Nope}}\n", "services.a.stop_command: invalid template"},

		// Alerts
		{"bad channel name", "linux", "alerts:\n  channels:\n    Ops: {type: slack, webhook_url: \"https://x.io\"}\nservices:\n  a: {command: run}\n", `:3:5: alerts.channels.Ops: invalid name "Ops"`},
		{"telegram missing token", "linux", "alerts:\n  channels:\n    t: {type: telegram, chat_id: \"1\"}\nservices:\n  a: {command: run}\n", `alerts.channels.t: missing required field "bot_token"`},
		{"telegram empty chat", "linux", "alerts:\n  channels:\n    t: {type: telegram, bot_token: x, chat_id: \"\"}\nservices:\n  a: {command: run}\n", `alerts.channels.t: missing required field "chat_id"`},
		{"slack missing url", "linux", "alerts:\n  channels:\n    s: {type: slack}\nservices:\n  a: {command: run}\n", `alerts.channels.s: missing required field "webhook_url"`},
		{"discord bad url", "linux", "alerts:\n  channels:\n    d: {type: discord, webhook_url: \"ftp://x\"}\nservices:\n  a: {command: run}\n", `alerts.channels.d.webhook_url: invalid URL "ftp://x"`},
		{"webhook bad method", "linux", "alerts:\n  channels:\n    w: {type: webhook, url: \"https://x.io\", method: SEND}\nservices:\n  a: {command: run}\n", `alerts.channels.w.method: invalid method "SEND"`},
		{"webhook bad header", "linux", "alerts:\n  channels:\n    w:\n      type: webhook\n      url: \"https://x.io\"\n      headers: {\"Bad Header\": x}\nservices:\n  a: {command: run}\n", `alerts.channels.w.headers.Bad Header: invalid header name`},
		{"rule without events", "linux", "alerts:\n  channels:\n" + slack + "  rules:\n    - channels: [s]\nservices:\n  a: {command: run}\n", `:5:7: alerts.rules[0]: missing required field "events"`},
		{"rule without channels", "linux", "alerts:\n  channels:\n" + slack + "  rules:\n    - events: [crashed]\nservices:\n  a: {command: run}\n", `alerts.rules[0]: missing required field "channels"`},
		{"unknown channel", "linux", "alerts:\n  channels:\n" + slack + "  rules:\n    - {events: [crashed], channels: [s, x]}\nservices:\n  a: {command: run}\n", `alerts.rules[0].channels[1]: unknown channel "x" (defined: s)`},
		{"service rule unknown channel", "linux", "services:\n  a:\n    command: run\n    alerts: {rules: [{events: [crashed], channels: [x]}]}\n", `services.a.alerts.rules[0].channels[0]: unknown channel "x" (no channel is defined in alerts.channels)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load("dstate.yaml", Options{Env: []string{}, ReadFile: memFiles(map[string]string{"dstate.yaml": tt.src}), GOOS: tt.goos})
			if err == nil {
				t.Fatalf("expected an error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("errors:\n%v\nwant to contain:\n%s", err, tt.want)
			}
		})
	}
}

func TestValidateErrorInDefaultsReportedOnce(t *testing.T) {
	_, err := Load("dstate.yaml", Options{Env: []string{}, GOOS: "linux", ReadFile: memFiles(map[string]string{
		"dstate.yaml": "defaults:\n  stop_timeout: 0s\nservices:\n  a: {command: run}\n  b: {command: run}\n",
	})})
	if err == nil || strings.Count(err.Error(), "\n") != 0 {
		t.Errorf("want exactly one error, got:\n%v", err)
	}
}

func TestValidateAccepts(t *testing.T) {
	src := `
alerts:
  channels:
    tg: {type: telegram, bot_token: x, chat_id: "-100123"}
    hook:
      type: webhook
      url: https://ntfy.sh/x
      method: put
      headers: {X-Token: abc}
      body: '{"msg":"{{.Message}}"}'
  rules:
    - {events: [crashed, recovered], channels: [tg, hook]}
services:
  web-1:
    command: serve --port {{add 8000 .Instance}}
    replicas: 3
    cwd: data/{{.Service}}
    env: {ID: "w{{.Instance}}of{{.Replicas}}"}
    success_exit_codes: [0, 3221225786]
    logs: {max_size: 1MB, max_age: 0s}
    stop_signal: kill
    wait_for:
      targets: ["tcp://[::1]:5432", "https://example.com/health"]
    alerts: {enabled: false}
  worker_2:
    command: run
    alerts:
      rules: [{events: [crash_loop], channels: [tg]}]
`
	for _, goos := range []string{"linux", "windows"} {
		cfg, err := Load("dstate.yaml", Options{Env: []string{}, GOOS: goos, ReadFile: memFiles(map[string]string{"dstate.yaml": src})})
		if err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		if got := cfg.Alerts.Channels[1].Method; got != "PUT" {
			t.Errorf("method = %q, want PUT", got)
		}
	}
}

func TestFullExampleOnEachOS(t *testing.T) {
	data, err := os.ReadFile("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"cfg/dstate.yaml": string(data), "cfg/.env": fullExampleEnv, apiEnvFile(): ""}

	if _, err := Load(cfgPath, Options{Env: []string{}, GOOS: "windows", ReadFile: memFiles(files)}); err != nil {
		t.Errorf("windows: %v", err)
	}

	// Only the Windows-specific parts fail elsewhere.
	_, err = Load(cfgPath, Options{Env: []string{}, GOOS: "linux", ReadFile: memFiles(files)})
	want := []string{
		filepath.FromSlash("cfg/dstate.yaml") + `:54:5: services.front.stop_signal: "ctrl_break" is not supported on linux`,
		filepath.FromSlash("cfg/dstate.yaml") + ":75:17: services.queue.wait_for.targets[0]: winsvc:// targets are only supported on Windows",
	}
	if err == nil {
		t.Fatal("linux: expected errors")
	}
	lines := strings.Split(err.Error(), "\n")
	if len(lines) != len(want) {
		t.Fatalf("linux: got %d errors, want %d:\n%v", len(lines), len(want), err)
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w) {
			t.Errorf("linux error %d:\n%s\nwant prefix:\n%s", i, lines[i], w)
		}
	}
}

func TestValidateGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/invalid-semantic/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden files (%v)", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			name := filepath.Base(f)
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			_, lerr := Load(name, Options{Env: []string{}, GOOS: "windows", ReadFile: memFiles(map[string]string{name: string(data)})})
			got := ""
			if lerr != nil {
				got = lerr.Error() + "\n"
			}
			checkGolden(t, strings.TrimSuffix(f, ".yaml")+".golden", got)
		})
	}
}

func TestCheckTarget(t *testing.T) {
	valid := []string{"tcp://localhost:3306", "tcp://10.0.0.1:1", "tcp://[::1]:65535", "http://x", "https://x.io/health?a=1", "winsvc://MySQL80", "winsvc://My Service"}
	for _, tgt := range valid {
		if err := checkTarget(tgt, "windows"); err != nil {
			t.Errorf("checkTarget(%q): %v", tgt, err)
		}
	}
	invalid := []string{"tcp://:3306", "tcp://db:0", "tcp://db:x", "winsvc://a/b", "mysql://db", "db"}
	for _, tgt := range invalid {
		if err := checkTarget(tgt, "windows"); err == nil {
			t.Errorf("checkTarget(%q) must fail", tgt)
		}
	}
}
