package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akamstar/durastate/internal/config"
	"github.com/akamstar/durastate/internal/paths"
)

type result struct {
	stdout, stderr string
	err            error
}

// runApp executes the CLI with a controlled environment.
func runApp(t *testing.T, env map[string]string, args ...string) result {
	t.Helper()
	var envList []string
	for k, v := range env {
		envList = append(envList, k+"="+v)
	}
	a := &app{
		paths:    paths.Resolver{GOOS: "linux", Getenv: func(k string) string { return env[k] }},
		loadOpts: config.Options{Env: envList, GOOS: "linux"},
	}
	var out, errOut bytes.Buffer
	cmd := newRootCmd(a)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return result{out.String(), errOut.String(), err}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const validSrc = "services:\n  api:\n    command: run --token ${TOKEN}\n"
const invalidSrc = "services:\n  api:\n    replicas: 0\n    restart: alway\n"

func TestValidateValid(t *testing.T) {
	p := writeFile(t, t.TempDir(), "dstate.yaml", validSrc)
	r := runApp(t, map[string]string{"TOKEN": "x"}, "validate", p)
	if r.err != nil || ExitCode(r.err) != ExitOK {
		t.Fatalf("err = %v, stderr = %s", r.err, r.stderr)
	}
	if want := p + " is valid (1 service)\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestValidateInvalid(t *testing.T) {
	p := writeFile(t, t.TempDir(), "dstate.yaml", invalidSrc)
	r := runApp(t, nil, "validate", p)
	if ExitCode(r.err) != ExitInvalidConfig || !Silent(r.err) {
		t.Fatalf("exit code = %d, silent = %v", ExitCode(r.err), Silent(r.err))
	}
	for _, want := range []string{
		p + `:4:14: services.api.restart: invalid value "alway"`,
		"invalid configuration: 1 error",
	} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr:\n%s\nwant to contain:\n%s", r.stderr, want)
		}
	}
	if r.stdout != "" {
		t.Errorf("stdout must be empty, got %q", r.stdout)
	}
}

func TestValidateErrorCount(t *testing.T) {
	p := writeFile(t, t.TempDir(), "dstate.yaml", "services:\n  api:\n    replicas: 0\n")
	r := runApp(t, nil, "validate", p)
	if !strings.Contains(r.stderr, "invalid configuration: 2 errors") {
		t.Errorf("stderr = %s", r.stderr)
	}
}

func TestValidateMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.yaml")
	r := runApp(t, nil, "validate", p)
	if ExitCode(r.err) != ExitInvalidConfig || !strings.Contains(r.stderr, "cannot read config") {
		t.Errorf("exit = %d, stderr = %s", ExitCode(r.err), r.stderr)
	}
}

func TestConfigPathPrecedence(t *testing.T) {
	dir := t.TempDir()
	arg := writeFile(t, dir, "arg.yaml", validSrc)
	flag := writeFile(t, dir, "flag.yaml", validSrc)
	envFile := writeFile(t, dir, "env.yaml", validSrc)
	env := map[string]string{"TOKEN": "x", paths.EnvConfig: envFile}

	if r := runApp(t, env, "validate", "--config", flag, arg); !strings.HasPrefix(r.stdout, arg) {
		t.Errorf("argument must win: %q", r.stdout)
	}
	if r := runApp(t, env, "validate", "-c", flag); !strings.HasPrefix(r.stdout, flag) {
		t.Errorf("--config must win over the environment: %q", r.stdout)
	}
	if r := runApp(t, env, "validate"); !strings.HasPrefix(r.stdout, envFile) {
		t.Errorf("DSTATE_CONFIG must be used: %q", r.stdout)
	}
	if r := runApp(t, nil, "validate"); !strings.Contains(r.stderr, "/etc/dstate/dstate.yaml") {
		t.Errorf("OS default must be used: %q", r.stderr)
	}
}

func TestValidateJSON(t *testing.T) {
	dir := t.TempDir()
	valid := writeFile(t, dir, "ok.yaml", validSrc)
	r := runApp(t, map[string]string{"TOKEN": "x"}, "validate", "--json", valid)
	var res validateResult
	if err := json.Unmarshal([]byte(r.stdout), &res); err != nil || r.err != nil {
		t.Fatalf("json = %q, err = %v / %v", r.stdout, err, r.err)
	}
	if !res.Valid || res.Services != 1 || res.Errors != nil || res.File != valid {
		t.Errorf("result = %+v", res)
	}

	invalid := writeFile(t, dir, "bad.yaml", invalidSrc)
	r = runApp(t, nil, "validate", "--json", invalid)
	res = validateResult{}
	if err := json.Unmarshal([]byte(r.stdout), &res); err != nil {
		t.Fatal(err)
	}
	if res.Valid || len(res.Errors) != 1 || ExitCode(r.err) != ExitInvalidConfig {
		t.Fatalf("result = %+v, exit = %d", res, ExitCode(r.err))
	}
	want := configIssue{File: invalid, Line: 4, Column: 14, Path: "services.api.restart", Message: `invalid value "alway" (expected always, on-failure, never)`}
	if res.Errors[0] != want {
		t.Errorf("issue = %+v\nwant  %+v", res.Errors[0], want)
	}
	if r.stderr != "" {
		t.Errorf("stderr must be empty in JSON mode, got %q", r.stderr)
	}
}

func TestConfigShow(t *testing.T) {
	p := writeFile(t, t.TempDir(), "dstate.yaml", validSrc)
	env := map[string]string{"TOKEN": "tok-secret"}

	r := runApp(t, env, "config", "show", p)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if strings.Contains(r.stdout, "tok-secret") || !strings.Contains(r.stdout, `- "***"`) || !strings.Contains(r.stdout, "replicas: 1") {
		t.Errorf("yaml output:\n%s", r.stdout)
	}

	r = runApp(t, env, "config", "show", "--show-secrets", p)
	if !strings.Contains(r.stdout, "tok-secret") {
		t.Errorf("--show-secrets output:\n%s", r.stdout)
	}

	r = runApp(t, env, "--config", p, "config", "show", "--json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, r.stdout)
	}
	if strings.Contains(r.stdout, "tok-secret") || !strings.HasSuffix(r.stdout, "}\n") {
		t.Errorf("json output:\n%s", r.stdout)
	}
}

func TestConfigShowInvalid(t *testing.T) {
	p := writeFile(t, t.TempDir(), "dstate.yaml", invalidSrc)
	r := runApp(t, nil, "config", "show", p)
	if ExitCode(r.err) != ExitInvalidConfig || r.stdout != "" || !strings.Contains(r.stderr, "invalid configuration") {
		t.Errorf("exit = %d, stdout = %q, stderr = %q", ExitCode(r.err), r.stdout, r.stderr)
	}
}

func TestInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "infra")
	r := runApp(t, nil, "init", "--dir", dir)
	if r.err != nil {
		t.Fatal(r.err)
	}
	for _, f := range []string{"dstate.yaml", ".env.example"} {
		if !strings.Contains(r.stdout, "created "+filepath.Join(dir, f)) {
			t.Errorf("stdout = %q, want %s listed", r.stdout, f)
		}
	}

	// The generated config must be valid as is, on every OS.
	for _, goos := range []string{"linux", "windows", "darwin"} {
		cfg, err := config.Load(filepath.Join(dir, "dstate.yaml"), config.Options{Env: []string{}, GOOS: goos})
		if err != nil {
			t.Fatalf("%s: generated config is invalid:\n%v", goos, err)
		}
		if got := cfg.Service("web").Command; strings.Join(got, " ") != "python app.py --port 8000" {
			t.Errorf("web command = %q", got)
		}
	}

	r = runApp(t, nil, "init", "--dir", dir)
	if r.err == nil || !strings.Contains(r.err.Error(), "already exists (use --force to overwrite)") {
		t.Errorf("second init must fail, got %v", r.err)
	}

	writeFile(t, dir, "dstate.yaml", "changed")
	if r = runApp(t, nil, "init", "--dir", dir, "--force"); r.err != nil {
		t.Fatal(r.err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "dstate.yaml")); !bytes.Equal(data, initConfig) {
		t.Error("--force must overwrite dstate.yaml")
	}
}

func TestInitEnvExampleIsValidDotenv(t *testing.T) {
	dir := t.TempDir()
	if r := runApp(t, nil, "init", "--dir", dir); r.err != nil {
		t.Fatal(r.err)
	}
	// Use the example as the global .env: it must parse.
	cfgPath := writeFile(t, dir, "with-env.yaml", "env_file: .env.example\n"+validSrc)
	if _, err := config.Load(cfgPath, config.Options{Env: []string{"TOKEN=x"}, GOOS: "linux"}); err != nil {
		t.Errorf(".env.example does not parse: %v", err)
	}
}

func TestExitCode(t *testing.T) {
	base := errors.New("boom")
	if ExitCode(nil) != ExitOK || ExitCode(base) != ExitError {
		t.Error("nil must be 0, plain errors 1")
	}
	e := &exitError{code: ExitInvalidConfig, err: base, silent: true}
	wrapped := errors.Join(errors.New("ctx"), e)
	if ExitCode(wrapped) != ExitInvalidConfig || !Silent(wrapped) || Silent(base) {
		t.Error("wrapped exit errors must keep their code and silence")
	}
	if e.Error() != "boom" || !errors.Is(e, base) {
		t.Error("exitError must expose the wrapped error")
	}
}

func TestPlural(t *testing.T) {
	if plural(1, "error") != "1 error" || plural(0, "error") != "0 errors" || plural(3, "service") != "3 services" {
		t.Error("plural")
	}
}
