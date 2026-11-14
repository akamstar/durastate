package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// memFiles is an in-memory ReadFile. Keys use "/" and are converted to host paths.
func memFiles(files map[string]string) func(string) ([]byte, error) {
	host := map[string]string{}
	for k, v := range files {
		host[filepath.Clean(filepath.FromSlash(k))] = v
	}
	return func(name string) ([]byte, error) {
		if v, ok := host[filepath.Clean(name)]; ok {
			return []byte(v), nil
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
}

var cfgPath = filepath.FromSlash("cfg/dstate.yaml")

func load(t *testing.T, env []string, files map[string]string) (*rawConfig, string) {
	t.Helper()
	cfg, errs := loadRaw(cfgPath, Options{Env: env, ReadFile: memFiles(files), GOOS: "linux"})
	if err := errs.Err(); err != nil {
		return cfg, err.Error()
	}
	return cfg, ""
}

func mustLoad(t *testing.T, env []string, files map[string]string) *rawConfig {
	t.Helper()
	cfg, err := load(t, env, files)
	if err != "" {
		t.Fatalf("unexpected errors:\n%s", err)
	}
	return cfg
}

func TestLoadInterpolation(t *testing.T) {
	cfg := mustLoad(t, []string{"APP_DIR=/srv/app", "PORT=8000", "REPLICAS=2"}, map[string]string{
		"cfg/dstate.yaml": `
services:
  api:
    command: python app.py --port ${PORT}
    cwd: ${APP_DIR}
    replicas: ${REPLICAS}
    stop_timeout: ${STOP:-45s}
    env:
      PRICE: "$$5"
      HOME_DIR: $HOME
`,
	})
	api := cfg.Services[0]
	if api.Command.Line != "python app.py --port 8000" || *api.Cwd != "/srv/app" || *api.Replicas != 2 {
		t.Errorf("api = %+v (command %q)", api, api.Command.Line)
	}
	if api.StopTimeout.String() != "45s" {
		t.Errorf("stop_timeout = %v", api.StopTimeout)
	}
	if api.Env["PRICE"] != "$5" || api.Env["HOME_DIR"] != "$HOME" {
		t.Errorf("env = %v", api.Env)
	}
}

func TestLoadGlobalDotenv(t *testing.T) {
	cfg := mustLoad(t, []string{"TOKEN=from-daemon", "ENV_DIR=secrets", "ONLY_DAEMON=d"}, map[string]string{
		"cfg/dstate.yaml": `
env_file: [.env, "${ENV_DIR}/extra.env"]
alerts:
  channels:
    tg: {type: telegram, bot_token: "${TOKEN}", chat_id: "${CHAT}"}
services:
  api:
    command: run ${ONLY_DAEMON} ${LEVEL}
`,
		"cfg/.env":              "TOKEN=from-dotenv\nCHAT=1\nLEVEL=info\n",
		"cfg/secrets/extra.env": "LEVEL=debug\n",
	})
	tg := cfg.Alerts.Channels[0]
	if *tg.BotToken != "from-dotenv" || *tg.ChatID != "1" {
		t.Errorf("global .env must win over the daemon environment: %q / %q", *tg.BotToken, *tg.ChatID)
	}
	if got := cfg.Services[0].Command.Line; got != "run d debug" {
		t.Errorf("command = %q, later .env files must win", got)
	}
	if want := map[string]string{"TOKEN": "from-dotenv", "CHAT": "1", "LEVEL": "debug"}; !reflect.DeepEqual(cfg.dotenv, want) {
		t.Errorf("dotenv = %v, want %v", cfg.dotenv, want)
	}
	if want := []string{".env", "secrets/extra.env"}; !reflect.DeepEqual(cfg.EnvFile, want) {
		t.Errorf("env_file = %v, want %v", cfg.EnvFile, want)
	}
}

func TestLoadServiceDotenv(t *testing.T) {
	cfg := mustLoad(t, nil, map[string]string{
		"cfg/dstate.yaml": `
services:
  api:
    command: run
    env_file: api.env
  worker:
    command: run
    env_file: [/abs/w1.env, w2.env]
`,
		"cfg/api.env": "A=1\n",
		"/abs/w1.env": "W=1\nX=1\n",
		"cfg/w2.env":  "W=2\n",
	})
	if want := map[string]string{"A": "1"}; !reflect.DeepEqual(cfg.Services[0].dotenv, want) {
		t.Errorf("api dotenv = %v", cfg.Services[0].dotenv)
	}
	if want := map[string]string{"W": "2", "X": "1"}; !reflect.DeepEqual(cfg.Services[1].dotenv, want) {
		t.Errorf("worker dotenv = %v", cfg.Services[1].dotenv)
	}
}

func TestLoadServiceDotenvNotUsedForInterpolation(t *testing.T) {
	_, err := load(t, nil, map[string]string{
		"cfg/dstate.yaml": "services:\n  api:\n    command: run ${PORT}\n    env_file: api.env\n",
		"cfg/api.env":     "PORT=8000\n",
	})
	want := filepath.FromSlash("cfg/dstate.yaml") + ":3:14: services.api.command: variable PORT is not set"
	if !strings.Contains(err, want) {
		t.Errorf("errors:\n%s\nwant:\n%s", err, want)
	}
}

func TestLoadEnvFileUsesDaemonEnvOnly(t *testing.T) {
	_, err := load(t, nil, map[string]string{
		"cfg/dstate.yaml": "env_file: [.env, \"${DIR}/x.env\"]\nservices:\n  a: {command: run}\n",
		"cfg/.env":        "DIR=somewhere\n",
	})
	if !strings.Contains(err, ":1:18: env_file[1]: variable DIR is not set") {
		t.Errorf("errors:\n%s", err)
	}
	if strings.Count(err, "env_file[1]") != 1 {
		t.Errorf("env_file must be reported once:\n%s", err)
	}
}

func TestLoadErrors(t *testing.T) {
	p := func(s string) string { return filepath.FromSlash(s) }
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			"missing config",
			map[string]string{},
			[]string{p("cfg/dstate.yaml") + ": cannot read config: file does not exist"},
		},
		{
			"missing global .env",
			map[string]string{"cfg/dstate.yaml": "env_file: nope.env\nservices:\n  a: {command: run}\n"},
			[]string{p("cfg/dstate.yaml") + ":1:1: env_file: cannot read " + p("cfg/nope.env") + ": file does not exist"},
		},
		{
			"missing service .env in a list",
			map[string]string{"cfg/dstate.yaml": "services:\n  a:\n    command: run\n    env_file:\n      - ok.env\n      - nope.env\n", "cfg/ok.env": ""},
			[]string{p("cfg/dstate.yaml") + ":6:9: services.a.env_file: cannot read " + p("cfg/nope.env")},
		},
		{
			"invalid .env",
			map[string]string{"cfg/dstate.yaml": "env_file: .env\nservices:\n  a: {command: run}\n", "cfg/.env": "A=1\noops\n"},
			[]string{p("cfg/.env") + ":2: invalid line (expected KEY=value)"},
		},
		{
			"empty env_file path",
			map[string]string{"cfg/dstate.yaml": "env_file: \"\"\nservices:\n  a: {command: run}\n"},
			[]string{":1:1: env_file: empty path"},
		},
		{
			"unset variables are all reported",
			map[string]string{"cfg/dstate.yaml": "services:\n  a:\n    command: run ${A}\n    cwd: ${B:?point it to the app}\n"},
			[]string{":3:14: services.a.command: variable A is not set", ":4:10: services.a.cwd: variable B point it to the app"},
		},
		{
			"interpolated value of the wrong type",
			map[string]string{"cfg/dstate.yaml": "services:\n  a:\n    command: run\n    replicas: ${N:-many}\n"},
			[]string{`:4:15: services.a.replicas: expected an integer, got "many"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, nil, tt.files)
			for _, w := range tt.want {
				if !strings.Contains(err, w) {
					t.Errorf("errors:\n%s\nwant to contain:\n%s", err, w)
				}
			}
		})
	}
}

func TestLoadNumbersAreNotInterpolated(t *testing.T) {
	cfg := mustLoad(t, nil, map[string]string{
		"cfg/dstate.yaml": "services:\n  a:\n    command: [run, '${X:-default}']\n    env:\n      N: 10\n",
	})
	a := cfg.Services[0]
	if a.Command.Args[1] != "default" || a.Env["N"] != "10" {
		t.Errorf("a = %+v", a)
	}
}

func TestLoadWindowsCaseInsensitiveLookup(t *testing.T) {
	files := memFiles(map[string]string{"cfg/dstate.yaml": "services:\n  a:\n    command: run ${path}\n"})
	cfg, errs := loadRaw(cfgPath, Options{Env: []string{"Path=C:\\bin"}, ReadFile: files, GOOS: "windows"})
	if err := errs.Err(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Services[0].Command.Line; got != `run C:\bin` {
		t.Errorf("command = %q", got)
	}
	_, errs = loadRaw(cfgPath, Options{Env: []string{"Path=/bin"}, ReadFile: files, GOOS: "linux"})
	if errs.Err() == nil {
		t.Error("lookup must be case-sensitive on unix")
	}
}

// apiEnvFile is where the host resolves the env_file `D:\apps\api\.env` of
// testdata/full.yaml: as is on Windows, relative to cfg/ elsewhere.
func apiEnvFile() string {
	return resolvePath(filepath.Dir(cfgPath), `D:\apps\api\.env`)
}

func TestLoadFullExample(t *testing.T) {
	data, err := os.ReadFile("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, nil, map[string]string{
		"cfg/dstate.yaml": string(data),
		"cfg/.env":        fullExampleEnv,
		apiEnvFile():      "DB=x\n",
	})
	if got := cfg.Alerts.Channels[3].Headers["Authorization"]; got != "Bearer n" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestLoadDefaultOptions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dstate.yaml")
	if err := os.WriteFile(path, []byte("services:\n  a:\n    command: run ${DSTATE_TEST_VAR}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSTATE_TEST_VAR", "ok")
	cfg, errs := loadRaw(path, Options{})
	if err := errs.Err(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Services[0].Command.Line; got != "run ok" {
		t.Errorf("command = %q", got)
	}
}

func TestResolvePath(t *testing.T) {
	base := filepath.FromSlash("cfg/dir")
	tests := []struct{ in, want string }{
		{".env", filepath.Join(base, ".env")},
		{"sub/x.env", filepath.Join(base, "sub", "x.env")},
		{"../x.env", filepath.FromSlash("cfg/x.env")},
		{"/abs/x.env", filepath.FromSlash("/abs/x.env")},
	}
	for _, tt := range tests {
		if got := resolvePath(base, tt.in); got != tt.want {
			t.Errorf("resolvePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
