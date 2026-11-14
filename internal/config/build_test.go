package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fullExampleEnv is the .env file of testdata/full.yaml.
const fullExampleEnv = `TELEGRAM_BOT_TOKEN=t
TELEGRAM_CHAT_ID=42
SLACK_WEBHOOK_URL=https://hooks.slack.com/x
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/x
NTFY_TOKEN=n
`

func loadConfig(t *testing.T, goos string, env []string, files map[string]string) (*Config, error) {
	t.Helper()
	return Load(cfgPath, Options{Env: env, ReadFile: memFiles(files), GOOS: goos})
}

func mustLoadConfig(t *testing.T, goos string, files map[string]string) *Config {
	t.Helper()
	cfg, err := loadConfig(t, goos, nil, files)
	if err != nil {
		t.Fatalf("unexpected errors:\n%v", err)
	}
	return cfg
}

func cfgDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("cfg")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadMinimalDefaults(t *testing.T) {
	cfg := mustLoadConfig(t, "windows", map[string]string{"cfg/dstate.yaml": "services:\n  api:\n    command: python app.py\n"})
	if cfg.Version != 1 || cfg.File != cfgPath || cfg.EnvFiles != nil {
		t.Errorf("config = %+v", cfg)
	}
	want := &Service{
		Name:             "api",
		Command:          []string{"python", "app.py"},
		Cwd:              cfgDir(t),
		Env:              map[string]string{},
		InheritEnv:       true,
		Replicas:         1,
		Autostart:        true,
		Restart:          RestartAlways,
		SuccessExitCodes: []int{0},
		Backoff:          Backoff{Initial: Duration(time.Second), Max: Duration(5 * time.Minute), Multiplier: 2},
		MaxRestarts:      10,
		RestartWindow:    Duration(10 * time.Minute),
		StableAfter:      Duration(time.Minute),
		CrashLoopRetry:   CrashLoopRetry{Enabled: true, Initial: Duration(5 * time.Minute), Max: Duration(time.Hour)},
		WaitFor:          WaitFor{Timeout: Duration(5 * time.Minute), Interval: Duration(2 * time.Second)},
		StopSignal:       SignalCtrlBreak,
		StopTimeout:      Duration(30 * time.Second),
		Logs: Logs{
			MaxSize: Size(50 << 20), MaxFiles: 10, MaxAge: Duration(14 * 24 * time.Hour),
			Compress: true, Timestamps: true,
		},
		Alerts: ServiceAlerts{Enabled: true},
	}
	if got := cfg.Service("api"); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if cfg.Alerts.Throttle != (Throttle{MinInterval: Duration(5 * time.Minute), GroupWindow: Duration(30 * time.Second)}) {
		t.Errorf("throttle = %+v", cfg.Alerts.Throttle)
	}
	if cfg.Service("nope") != nil {
		t.Error("unknown service must be nil")
	}
}

func TestLoadUnixStopSignal(t *testing.T) {
	cfg := mustLoadConfig(t, "linux", map[string]string{"cfg/dstate.yaml": "services:\n  a:\n    command: run\n"})
	if got := cfg.Services[0].StopSignal; got != SignalTerm {
		t.Errorf("stop_signal = %s", got)
	}
}

func TestLoadFullExampleEffective(t *testing.T) {
	data, err := os.ReadFile("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(t, "windows", nil, map[string]string{
		"cfg/dstate.yaml": string(data),
		"cfg/.env":        fullExampleEnv,
		apiEnvFile():      "DB=x\nAPP_ENV=dev\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	api, messenger, reports := cfg.Service("api"), cfg.Service("messenger"), cfg.Service("reports")

	if api.StopTimeout != Duration(30*time.Second) || messenger.StopTimeout != Duration(time.Minute) {
		t.Errorf("stop_timeout: api %v, messenger %v", api.StopTimeout, messenger.StopTimeout)
	}
	if api.Logs.MaxFiles != 10 || api.Logs.MaxAge != Duration(14*24*time.Hour) || !api.Logs.Compress {
		t.Errorf("logs from defaults = %+v", api.Logs)
	}
	if !reflect.DeepEqual(api.Command, []string{"python", "-m", "uvicorn", "main:app", "--port", "8000"}) {
		t.Errorf("api.command = %q", api.Command)
	}
	if api.Env["APP_ENV"] != "production" || api.Env["DB"] != "x" || api.Env["TELEGRAM_CHAT_ID"] != "42" {
		t.Errorf("env block > service .env > global .env, got %v", api.Env)
	}
	if !reflect.DeepEqual(messenger.StopCommand, []string{"php", "bin/console", "messenger:stop-workers"}) || messenger.Replicas != 3 {
		t.Errorf("messenger = %+v", messenger)
	}
	if reports.Restart != RestartOnFailure || reports.Autostart || reports.StopCommand != nil {
		t.Errorf("reports = %+v", reports)
	}
	if !reflect.DeepEqual(cfg.EnvFiles, []string{filepath.Join(cfgDir(t), ".env")}) {
		t.Errorf("env files = %v", cfg.EnvFiles)
	}
	ntfy := cfg.Alerts.Channels[3]
	if ntfy.Method != "POST" || ntfy.Headers["Authorization"] != "Bearer n" || ntfy.BotToken != "" {
		t.Errorf("ntfy = %+v", ntfy)
	}
	if cfg.Alerts.Throttle.MinInterval != Duration(5*time.Minute) {
		t.Errorf("throttle = %+v", cfg.Alerts.Throttle)
	}
}

func TestLoadPrecedence(t *testing.T) {
	cfg := mustLoadConfig(t, "linux", map[string]string{"cfg/dstate.yaml": `
alerts:
  channels:
    x: {type: slack, webhook_url: "https://hooks.slack.com/x"}
defaults:
  restart: on-failure
  logs: {max_files: 5, compress: false}
  alerts: {enabled: false}
  stop_command: stop-all
services:
  a:
    command: run
    logs: {max_files: 2}
  b:
    command: run
    restart: never
    alerts:
      enabled: true
      rules: [{events: [crashed], channels: [x]}]
`})
	a, b := cfg.Service("a"), cfg.Service("b")
	if a.Restart != RestartOnFailure || b.Restart != RestartNever {
		t.Errorf("restart: a %s, b %s", a.Restart, b.Restart)
	}
	if a.Logs.MaxFiles != 2 || a.Logs.Compress || a.Logs.MaxSize != 50<<20 || b.Logs.MaxFiles != 5 {
		t.Errorf("logs: a %+v, b %+v", a.Logs, b.Logs)
	}
	if a.Alerts.Enabled || a.Alerts.Rules != nil {
		t.Errorf("a.alerts = %+v", a.Alerts)
	}
	if !b.Alerts.Enabled || !reflect.DeepEqual(b.Alerts.Rules, []Rule{{Events: []string{"crashed"}, Channels: []string{"x"}}}) {
		t.Errorf("b.alerts = %+v", b.Alerts)
	}
	if !reflect.DeepEqual(a.StopCommand, []string{"stop-all"}) {
		t.Errorf("stop_command from defaults = %q", a.StopCommand)
	}
}

func TestLoadServiceFields(t *testing.T) {
	cfg := mustLoadConfig(t, "linux", map[string]string{"cfg/dstate.yaml": `
services:
  rel:
    command: run
    cwd: app/{{.Instance}}
    env_file: rel.env
    replicas: 4
    wait_for: {targets: tcp://db:5432, interval: 1s}
  abs:
    command: run
    cwd: /srv/abs
  sh:
    shell: true
    command: |
      echo a | grep a
    stop_command: kill-it now
`, "cfg/rel.env": "R=1\n"})
	rel, abs, sh := cfg.Service("rel"), cfg.Service("abs"), cfg.Service("sh")
	if want := filepath.Join(cfgDir(t), "app", "{{.Instance}}"); rel.Cwd != want {
		t.Errorf("rel.cwd = %q, want %q", rel.Cwd, want)
	}
	if rel.Replicas != 4 || rel.Env["R"] != "1" || !reflect.DeepEqual(rel.EnvFiles, []string{filepath.Join(cfgDir(t), "rel.env")}) {
		t.Errorf("rel = %+v", rel)
	}
	if !reflect.DeepEqual(rel.WaitFor.Targets, []string{"tcp://db:5432"}) ||
		rel.WaitFor.Interval != Duration(time.Second) || rel.WaitFor.Timeout != Duration(5*time.Minute) {
		t.Errorf("rel.wait_for = %+v", rel.WaitFor)
	}
	if want := absPath(filepath.FromSlash("/srv/abs")); abs.Cwd != want {
		t.Errorf("abs.cwd = %q, want %q", abs.Cwd, want)
	}
	if !sh.Shell || !reflect.DeepEqual(sh.Command, []string{"echo a | grep a"}) || !reflect.DeepEqual(sh.StopCommand, []string{"kill-it now"}) {
		t.Errorf("sh = %+v", sh)
	}
}

func TestLoadBuildErrors(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"shell with a list", "services:\n  a:\n    shell: true\n    command: [echo, a]\n", ":4:5: services.a.command: with shell: true, the command must be a single string"},
		{"unterminated quote", "services:\n  a:\n    command: node \"app.js\n", "services.a.command: unterminated quote"},
		{"bad stop command", "services:\n  a:\n    command: run\n    stop_command: \"x {{\"\n", ":4:5: services.a.stop_command: unterminated template action"},
		{"empty cwd", "services:\n  a:\n    command: run\n    cwd: \"\"\n", ":4:5: services.a.cwd: empty path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(t, "linux", nil, map[string]string{"cfg/dstate.yaml": tt.src})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadReturnsErrorList(t *testing.T) {
	_, err := loadConfig(t, "linux", nil, map[string]string{"cfg/dstate.yaml": "services:\n  a:\n    replicas: x\n    restart: y\n"})
	var list ErrorList
	if !errors.As(err, &list) || len(list) != 2 {
		t.Fatalf("error = %#v, want an ErrorList of 2", err)
	}
}
