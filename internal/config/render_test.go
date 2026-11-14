package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadShow(t *testing.T) *Config {
	t.Helper()
	data, err := os.ReadFile("testdata/render/show.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath, Options{
		GOOS: "windows",
		Env:  []string{"DB_PASSWORD=s3cr3t-pw", "LEVEL=info", "PATH=C:\\bin"},
		ReadFile: memFiles(map[string]string{
			"cfg/dstate.yaml": string(data),
			"cfg/.env":        "CHAT_ID=42\nNTFY_TOPIC=my-private-topic\nNTFY_TOKEN=tok-123456\n",
			"cfg/api.env":     "SMTP_HOST=mail.local\nAPP_ENV=dev\n",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// portable replaces machine-specific paths so golden files match on every OS.
func portable(t *testing.T, s string) string {
	t.Helper()
	dir := cfgDir(t)
	escaped := strings.ReplaceAll(dir, "\\", "\\\\") // as written in YAML and JSON strings
	s = strings.ReplaceAll(s, escaped, "<dir>")
	s = strings.ReplaceAll(s, dir, "<dir>")
	s = strings.ReplaceAll(s, "\\\\", "/")
	return strings.ReplaceAll(s, "\\", "/")
}

func TestRenderGolden(t *testing.T) {
	cfg := loadShow(t)
	y, err := cfg.YAML(false)
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "testdata/render/show.yaml.golden", portable(t, string(y)))

	j, err := cfg.JSON(false)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(j) {
		t.Fatalf("invalid JSON:\n%s", j)
	}
	checkGolden(t, "testdata/render/show.json.golden", portable(t, string(j))+"\n")
}

func TestRenderMasksSecrets(t *testing.T) {
	cfg := loadShow(t)
	for _, format := range []string{"yaml", "json"} {
		out := renderAs(t, cfg, format, false)
		for _, secret := range []string{"s3cr3t-pw", "literal-token-in-yaml", "my-private-topic", "tok-123456", "inline-key", "mail.local"} {
			if strings.Contains(out, secret) {
				t.Errorf("%s output leaks %q", format, secret)
			}
		}
		for _, visible := range []string{"production", "info", "dstate", "postgres://app:***@db/app"} {
			if !strings.Contains(out, visible) {
				t.Errorf("%s output must show %q", format, visible)
			}
		}
	}
}

func TestRenderShowSecrets(t *testing.T) {
	cfg := loadShow(t)
	out := renderAs(t, cfg, "yaml", true)
	for _, secret := range []string{"s3cr3t-pw", "literal-token-in-yaml", "tok-123456", "inline-key", "mail.local"} {
		if !strings.Contains(out, secret) {
			t.Errorf("showSecrets output must contain %q", secret)
		}
	}
	if strings.Contains(out, Masked) {
		t.Error("showSecrets output must not mask anything")
	}
}

func TestRenderRoundTrip(t *testing.T) {
	cfg := loadShow(t)
	out := renderAs(t, cfg, "yaml", true)
	dir := t.TempDir()
	path := filepath.Join(dir, "dstate.yaml")
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path, Options{GOOS: "windows", Env: []string{}, ReadFile: func(name string) ([]byte, error) {
		if name == path {
			return []byte(out), nil
		}
		return []byte{}, nil // .env files: content already merged in env
	}})
	if err != nil {
		t.Fatalf("rendered config does not load:\n%v\n%s", err, out)
	}
	if again.Service("worker").Replicas != 2 || again.Service("api").Env["SMTP_HOST"] != "mail.local" {
		t.Errorf("round trip lost values")
	}
}

func TestMask(t *testing.T) {
	cfg := loadShow(t)
	got := cfg.Mask("connect with s3cr3t-pw to my-private-topic, chat 42, env dev")
	want := "connect with *** to ***, chat 42, env dev"
	if got != want {
		t.Errorf("Mask = %q, want %q", got, want)
	}
	if (&Config{}).Mask("x") != "x" {
		t.Error("Mask without secrets must return its input")
	}
}

func TestIsSecretName(t *testing.T) {
	for _, n := range []string{"TOKEN", "NTFY_TOKEN", "DB_PASSWORD", "api_key", "APIKEY", "SECRET_KEY_BASE", "Authorization", "X-Api-Key", "DATABASE_DSN", "AWS_ACCESS_KEY_ID", "MYSQL_ROOT_PASS"} {
		if !isSecretName(n) {
			t.Errorf("%s must be a secret name", n)
		}
	}
	for _, n := range []string{"PATH", "APP_ENV", "PORT", "MONKEY", "KEYBOARD", "TOKENIZER_MODE", "X-Title", "PWD", "AUTHOR"} {
		if isSecretName(n) {
			t.Errorf("%s must not be a secret name", n)
		}
	}
}

func renderAs(t *testing.T, cfg *Config, format string, showSecrets bool) string {
	t.Helper()
	var (
		b   []byte
		err error
	)
	if format == "json" {
		b, err = cfg.JSON(showSecrets)
	} else {
		b, err = cfg.YAML(showSecrets)
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
