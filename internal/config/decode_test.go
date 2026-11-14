package config

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files")

func mustParse(t *testing.T, src string) *rawConfig {
	t.Helper()
	cfg, errs := parse("dstate.yaml", []byte(src))
	if err := errs.Err(); err != nil {
		t.Fatalf("unexpected errors:\n%v", err)
	}
	return cfg
}

func parseErrors(src string) string {
	_, errs := parse("dstate.yaml", []byte(src))
	if err := errs.Err(); err != nil {
		return err.Error()
	}
	return ""
}

func ptr[T any](v T) *T { return &v }

func TestParseMinimal(t *testing.T) {
	data, err := os.ReadFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustParse(t, string(data))
	if len(cfg.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(cfg.Services))
	}
	s := cfg.Services[0]
	if s.Name != "api" || s.Command == nil || s.Command.Line != "python app.py" || s.Command.IsList {
		t.Errorf("unexpected service %+v (command %+v)", s, s.Command)
	}
	if cfg.Version != nil || cfg.Defaults != nil || cfg.Alerts != nil || cfg.EnvFile != nil {
		t.Error("absent fields must stay nil")
	}
}

func TestParseFull(t *testing.T) {
	data, err := os.ReadFile("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustParse(t, string(data))

	if cfg.Version == nil || *cfg.Version != 1 {
		t.Errorf("version = %v", cfg.Version)
	}
	d := cfg.Defaults
	if d == nil || *d.Restart != "always" || *d.StopTimeout != Duration(30*time.Second) {
		t.Fatalf("defaults = %+v", d)
	}
	if *d.Logs.MaxSize != 50<<20 || *d.Logs.MaxFiles != 10 || *d.Logs.MaxAge != Duration(14*24*time.Hour) || !*d.Logs.Compress {
		t.Errorf("defaults.logs = %+v", d.Logs)
	}
	if !reflect.DeepEqual(cfg.EnvFile, []string{".env"}) {
		t.Errorf("env_file = %v", cfg.EnvFile)
	}

	a := cfg.Alerts
	var names []string
	for _, c := range a.Channels {
		names = append(names, c.Name+"/"+c.Type)
	}
	if want := []string{"ops-telegram/telegram", "ops-slack/slack", "ops-discord/discord", "ntfy/webhook"}; !reflect.DeepEqual(names, want) {
		t.Errorf("channels = %v, want %v", names, want)
	}
	if *a.Channels[0].BotToken != "${TELEGRAM_BOT_TOKEN}" {
		t.Errorf("bot_token = %q (interpolation is not done by the decoder)", *a.Channels[0].BotToken)
	}
	ntfy := a.Channels[3]
	if *ntfy.Method != "POST" || ntfy.Headers["Authorization"] != "Bearer ${NTFY_TOKEN}" || !strings.Contains(*ntfy.Body, "{{.Service}}") {
		t.Errorf("webhook channel = %+v", ntfy)
	}
	if len(a.Rules) != 1 || !reflect.DeepEqual(a.Rules[0].Events, []string{"crashed", "crash_loop", "recovered"}) ||
		!reflect.DeepEqual(a.Rules[0].Channels, []string{"ops-telegram", "ops-slack"}) {
		t.Errorf("rules = %+v", a.Rules)
	}
	if *a.Throttle.MinInterval != Duration(5*time.Minute) || *a.Throttle.GroupWindow != Duration(30*time.Second) {
		t.Errorf("throttle = %+v", a.Throttle)
	}

	var svc []string
	for _, s := range cfg.Services {
		svc = append(svc, s.Name)
	}
	if want := []string{"api", "front", "messenger", "queue", "reports"}; !reflect.DeepEqual(svc, want) {
		t.Fatalf("services = %v, want %v (file order)", svc, want)
	}
	api, front, messenger, queue, reports := cfg.Services[0], cfg.Services[1], cfg.Services[2], cfg.Services[3], cfg.Services[4]
	if *api.Cwd != `D:\apps\api` || api.Env["APP_ENV"] != "production" || !reflect.DeepEqual(api.EnvFile, []string{`D:\apps\api\.env`}) {
		t.Errorf("api = %+v", api)
	}
	if *front.StopSignal != "ctrl_break" {
		t.Errorf("front.stop_signal = %v", *front.StopSignal)
	}
	if *messenger.Replicas != 3 || messenger.StopCommand.Line != "php bin/console messenger:stop-workers" ||
		*messenger.StopTimeout != Duration(time.Minute) {
		t.Errorf("messenger = %+v", messenger)
	}
	if !reflect.DeepEqual(messenger.WaitFor.Targets, []string{"tcp://localhost:3306", "tcp://localhost:6379"}) ||
		*messenger.WaitFor.Timeout != Duration(5*time.Minute) || messenger.WaitFor.Interval != nil {
		t.Errorf("messenger.wait_for = %+v", messenger.WaitFor)
	}
	if !reflect.DeepEqual(queue.WaitFor.Targets, []string{"winsvc://MySQL80"}) {
		t.Errorf("queue.wait_for = %+v", queue.WaitFor)
	}
	if *reports.Restart != "on-failure" || *reports.Autostart || reports.Command.Line != `scripts\build-report.bat` {
		t.Errorf("reports = %+v", reports)
	}

	if got := cfg.pos["services.messenger.replicas"]; got != (Pos{"dstate.yaml", 59, 5}) {
		t.Errorf("position of services.messenger.replicas = %v", got)
	}
}

func TestParseFieldTypes(t *testing.T) {
	cfg := mustParse(t, `
services:
  w:
    command: [php, "bin/console", messenger:consume]
    shell: true
    inherit_env: FALSE
    replicas: "2"
    success_exit_codes: [0, 3]
    max_restarts: 5
    restart_window: 10m
    stable_after: 1m
    start_delay: 0s
    stop_command: [php, artisan, queue:restart]
    backoff: {initial: 1s, max: 5m, multiplier: 1.5}
    crash_loop_retry: {initial: 5m, max: 1h, enabled: false}
    logs: {timestamps: false, merge_stderr: true}
    alerts: {enabled: false}
    env:
      PORT: 8000
      DEBUG: true
      EMPTY: ""
  x:
    command: run
    success_exit_codes: 2
    alerts:
      rules:
        - events: crashed
          channels: [a]
`)
	w, x := cfg.Services[0], cfg.Services[1]
	if !w.Command.IsList || !reflect.DeepEqual(w.Command.Args, []string{"php", "bin/console", "messenger:consume"}) {
		t.Errorf("command = %+v", w.Command)
	}
	if !*w.Shell || *w.InheritEnv || *w.Replicas != 2 || *w.MaxRestarts != 5 {
		t.Errorf("scalars = %+v", w.rawPolicy)
	}
	if !reflect.DeepEqual(w.SuccessExitCodes, []int{0, 3}) || !reflect.DeepEqual(x.SuccessExitCodes, []int{2}) {
		t.Errorf("success_exit_codes = %v / %v", w.SuccessExitCodes, x.SuccessExitCodes)
	}
	if *w.Backoff.Multiplier != 1.5 || *w.Backoff.Max != Duration(5*time.Minute) {
		t.Errorf("backoff = %+v", w.Backoff)
	}
	if *w.CrashLoopRetry.Enabled || *w.CrashLoopRetry.Max != Duration(time.Hour) {
		t.Errorf("crash_loop_retry = %+v", w.CrashLoopRetry)
	}
	if *w.Logs.Timestamps || !*w.Logs.MergeStderr || w.Logs.MaxSize != nil {
		t.Errorf("logs = %+v", w.Logs)
	}
	if *w.Alerts.Enabled || w.Alerts.Rules != nil {
		t.Errorf("alerts = %+v", w.Alerts)
	}
	if want := map[string]string{"PORT": "8000", "DEBUG": "true", "EMPTY": ""}; !reflect.DeepEqual(w.Env, want) {
		t.Errorf("env = %v, want %v", w.Env, want)
	}
	if x.Alerts.Enabled != nil || len(x.Alerts.Rules) != 1 || !reflect.DeepEqual(x.Alerts.Rules[0].Events, []string{"crashed"}) {
		t.Errorf("x.alerts = %+v", x.Alerts)
	}
}

func TestParseKeepsScalarText(t *testing.T) {
	cfg := mustParse(t, `
alerts:
  channels:
    tg: {type: telegram, bot_token: "123:abc", chat_id: 0123}
services:
  a:
    command: |
      run --flag
    env:
      RATIO: 1.10
      OCT: 0o17
`)
	tg := cfg.Alerts.Channels[0]
	if *tg.ChatID != "0123" || *tg.BotToken != "123:abc" {
		t.Errorf("channel = %q / %q", *tg.ChatID, *tg.BotToken)
	}
	a := cfg.Services[0]
	if a.Command.Line != "run --flag\n" {
		t.Errorf("literal command = %q", a.Command.Line)
	}
	if a.Env["RATIO"] != "1.10" || a.Env["OCT"] != "0o17" {
		t.Errorf("env = %v", a.Env)
	}
}

func TestParseAnchorsAndMergeKeys(t *testing.T) {
	cfg := mustParse(t, `
x-worker: &worker
  restart: on-failure
  stop_timeout: 60s
  env: &env
    APP_ENV: prod
services:
  a:
    <<: *worker
    command: run a
    stop_timeout: 10s
  b:
    <<: [*worker]
    command: run b
    env: *env
  c:
    command: !!str run c
`)
	a, b := cfg.Services[0], cfg.Services[1]
	if *a.Restart != "on-failure" || *a.StopTimeout != Duration(10*time.Second) || a.Env["APP_ENV"] != "prod" {
		t.Errorf("a = %+v", a)
	}
	if *b.StopTimeout != Duration(time.Minute) || b.Env["APP_ENV"] != "prod" {
		t.Errorf("b = %+v", b)
	}
}

func TestParseExtensionKeysOnlyAtRoot(t *testing.T) {
	got := parseErrors("services:\n  a:\n    command: x\n    x-note: hi\n")
	if !strings.Contains(got, `unknown field "x-note"`) {
		t.Errorf("x- keys must only be ignored at the root, got %q", got)
	}
}

func TestParseMergeKeys(t *testing.T) {
	cfg := mustParse(t, `
defaults: &d
  restart: on-failure
  stop_timeout: 60s
services:
  a:
    <<: *d
    command: run a
    stop_timeout: 10s
  b:
    <<: [{restart: never}, *d]
    command: [run, b]
    env: &env
      APP_ENV: prod
  c:
    command: !!str run c
    env: *env
`)
	a, b, c := cfg.Services[0], cfg.Services[1], cfg.Services[2]
	if *a.Restart != "on-failure" || *a.StopTimeout != Duration(10*time.Second) {
		t.Errorf("a: explicit keys must win over merged ones: %+v", a.rawPolicy)
	}
	if *b.Restart != "never" || *b.StopTimeout != Duration(time.Minute) {
		t.Errorf("b: earlier merge sources must win: %+v", b.rawPolicy)
	}
	if c.Command.Line != "run c" || c.Env["APP_ENV"] != "prod" {
		t.Errorf("c = %+v", c)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"empty file", "", "dstate.yaml: empty configuration (at least one service is required)"},
		{"comments only", "# nothing\n", "dstate.yaml: empty configuration (at least one service is required)"},
		{"syntax", "services:\n  a: [1\n", "dstate.yaml:"},
		{"duplicate key", "services:\n  a:\n    command: x\n    command: y\n", `dstate.yaml:4:5: mapping key "command" already defined at [3:5]`},
		{"two documents", "services: {a: {command: x}}\n---\nservices: {}\n", "dstate.yaml:3:9: multiple YAML documents are not supported"},
		{"root not a mapping", "- a\n", "dstate.yaml:1:1: expected a mapping, got a list"},
		{"unknown root field", "service:\n  a: {}\n", `dstate.yaml:1:1: unknown field "service" (did you mean "services"?)`},
		{"unknown field no hint", "zzz: 1\n", `dstate.yaml:1:1: unknown field "zzz"`},
		{"unknown service field", "services:\n  a:\n    restat: always\n", `dstate.yaml:3:5: services.a: unknown field "restat" (did you mean "restart"?)`},
		{"unsupported version", "version: 2\nservices: {a: {command: x}}\n", "dstate.yaml:1:10: version: unsupported version 2 (this dstate supports version 1)"},
		{"bad enum", "services:\n  a:\n    restart: alway\n", `dstate.yaml:3:14: services.a.restart: invalid value "alway" (expected always, on-failure, never)`},
		{"ctrl_c removed", "services:\n  a:\n    stop_signal: ctrl_c\n", `services.a.stop_signal: invalid value "ctrl_c" (expected ctrl_break, sigterm, sigint, kill, none)`},
		{"bad bool", "services:\n  a:\n    autostart: yes\n", `dstate.yaml:3:16: services.a.autostart: expected true or false, got "yes"`},
		{"bad int", "services:\n  a:\n    replicas: two\n", `dstate.yaml:3:15: services.a.replicas: expected an integer, got "two"`},
		{"bad float", "services:\n  a:\n    backoff: {multiplier: .nan}\n", `services.a.backoff.multiplier: expected a number, got ".nan"`},
		{"bad duration", "services:\n  a:\n    stop_timeout: 30\n", `dstate.yaml:3:19: services.a.stop_timeout: invalid duration "30" (examples: 30s, 5m, 1h30m, 14d)`},
		{"bad size", "services:\n  a:\n    logs: {max_size: 50}\n", `services.a.logs.max_size: invalid size "50" (examples: 512KB, 50MB, 1GB)`},
		{"null scalar", "services:\n  a:\n    cwd:\n", "dstate.yaml:3:5: services.a.cwd: missing value"},
		{"null service", "services:\n  a:\n", "services.a: missing value"},
		{"null alerts", "services:\n  a:\n    command: x\n    alerts:\n", "services.a.alerts: missing value"},
		{"mapping instead of scalar", "services:\n  a:\n    cwd: {x: 1}\n", "services.a.cwd: expected a single value, got a mapping"},
		{"list instead of mapping", "services:\n  - a\n", "dstate.yaml:2:3: services: expected a mapping, got a list"},
		{"scalar instead of list", "alerts:\n  rules: crashed\n", `alerts.rules: expected a list, got "crashed"`},
		{"empty command string", "services:\n  a:\n    command: \"  \"\n", "services.a.command: empty command"},
		{"empty command list", "services:\n  a:\n    command: []\n", "services.a.command: empty command"},
		{"null env value", "services:\n  a:\n    env:\n      FOO:\n", `services.a.env.FOO: missing value (use "" for an empty string)`},
		{"service-only field in defaults", "defaults:\n  command: x\n", `dstate.yaml:2:3: defaults.command: "command" is not allowed in defaults, set it on each service`},
		{"unknown alias", "services:\n  a: *nope\n", `services.a: unknown alias "nope"`},
		{"complex key", "services:\n  ? [a, b]\n  : x\n", "dstate.yaml:3:3: found an invalid key for this map"},
		{"channel without type", "alerts:\n  channels:\n    c: {url: x}\n", `dstate.yaml:3:5: alerts.channels.c: missing required field "type" (expected telegram, slack, discord, webhook)`},
		{"bad channel type", "alerts:\n  channels:\n    c: {type: email}\n", `alerts.channels.c.type: invalid value "email" (expected telegram, slack, discord, webhook)`},
		{"field of another channel type", "alerts:\n  channels:\n    c: {type: slack, bot_token: x}\n", `alerts.channels.c: unknown field "bot_token" for a slack channel`},
		{"unknown event", "alerts:\n  rules:\n    - events: [crashed, crach]\n", `alerts.rules[0].events[1]: unknown event "crach"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseErrors(tt.src)
			if got == "" {
				t.Fatalf("expected an error containing %q", tt.want)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("errors:\n%s\nwant to contain:\n%s", got, tt.want)
			}
		})
	}
}

func TestParseCollectsAllErrors(t *testing.T) {
	got := parseErrors("services:\n  a:\n    replicas: x\n    restart: y\n  b:\n    autostart: z\n")
	if n := strings.Count(got, "\n") + 1; n != 3 {
		t.Errorf("got %d errors, want 3:\n%s", n, got)
	}
}

// TestParseGolden checks multi-error files against their .golden output.
// Run "go test -run TestParseGolden -update" to rewrite the golden files.
func TestParseGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/invalid/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no golden test files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			_, errs := parse(filepath.Base(f), data)
			got := ""
			if err := errs.Err(); err != nil {
				got = err.Error() + "\n"
			}
			checkGolden(t, strings.TrimSuffix(f, ".yaml")+".golden", got)
		})
	}
}

// checkGolden compares got with a golden file, or rewrites it with -update.
func checkGolden(t *testing.T, golden, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSuggest(t *testing.T) {
	fs := fieldSet{"restart": nil, "replicas": nil, "env": nil, "cwd": nil}
	tests := map[string]string{
		"restat":  ` (did you mean "restart"?)`,
		"replica": ` (did you mean "replicas"?)`,
		"evn":     ` (did you mean "env"?)`,
		"xyz":     "",
		"command": "",
	}
	for in, want := range tests {
		if got := suggest(in, fs); got != want {
			t.Errorf("suggest(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, p := range []string{"testdata/minimal.yaml", "testdata/full.yaml"} {
		if data, err := os.ReadFile(p); err == nil {
			f.Add(data)
		}
	}
	f.Add([]byte("a: &x [*x]\n"))
	f.Add([]byte("services:\n  a:\n    <<: *a\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		parse("fuzz.yaml", data) // must not panic
	})
}
