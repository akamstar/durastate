package paths

import (
	"path/filepath"
	"runtime"
	"testing"
)

func resolver(goos string, env map[string]string) Resolver {
	return Resolver{GOOS: goos, Getenv: func(k string) string { return env[k] }}
}

func TestConfigFileDefaults(t *testing.T) {
	tests := []struct {
		goos string
		env  map[string]string
		want string
	}{
		{"windows", nil, `C:\ProgramData\dstate\dstate.yaml`},
		{"windows", map[string]string{"ProgramData": `D:\PD`}, `D:\PD\dstate\dstate.yaml`},
		{"linux", nil, "/etc/dstate/dstate.yaml"},
		{"freebsd", nil, "/etc/dstate/dstate.yaml"},
		{"darwin", nil, "/Library/Application Support/dstate/dstate.yaml"},
	}
	for _, tt := range tests {
		if tt.goos == runtime.GOOS {
			tt.want = filepath.Clean(tt.want)
		}
		if got := resolver(tt.goos, tt.env).ConfigFile(); got != tt.want {
			t.Errorf("%s %v: got %q, want %q", tt.goos, tt.env, got, tt.want)
		}
	}
}

func TestConfigFileOverrides(t *testing.T) {
	r := resolver("linux", map[string]string{EnvHome: "/opt/dstate", EnvConfig: "/srv/infra/dstate.yaml"})
	if got := r.ConfigFile(); got != "/srv/infra/dstate.yaml" {
		t.Errorf("DSTATE_CONFIG must win, got %q", got)
	}
	r = resolver("linux", map[string]string{EnvHome: "/opt/dstate"})
	want := "/opt/dstate/dstate.yaml"
	if runtime.GOOS == "linux" {
		want = filepath.Clean(want)
	}
	if got := r.ConfigFile(); got != want {
		t.Errorf("DSTATE_HOME: got %q, want %q", got, want)
	}
	if got := r.Home(); got != "/opt/dstate" {
		t.Errorf("Home() = %q", got)
	}
}

func TestHost(t *testing.T) {
	if h := Host(); h.GOOS != runtime.GOOS || h.Getenv == nil {
		t.Errorf("Host() = %+v", h)
	}
}
