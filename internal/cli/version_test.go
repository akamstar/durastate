package cli

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func run(t *testing.T, info BuildInfo, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := NewRootCmd(info)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestVersionText(t *testing.T) {
	got, err := run(t, BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-11-14"}, "version")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dstate 1.2.3", "commit abc123", "built 2026-11-14", runtime.GOOS + "/" + runtime.GOARCH} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q does not contain %q", got, want)
		}
	}
}

func TestVersionDefaults(t *testing.T) {
	got, err := run(t, BuildInfo{}, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "dstate dev (commit none, built unknown,") {
		t.Errorf("unexpected output %q", got)
	}
}

func TestVersionJSON(t *testing.T) {
	got, err := run(t, BuildInfo{Version: "1.2.3"}, "version", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var v versionOutput
	if err := json.Unmarshal([]byte(got), &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", got, err)
	}
	want := versionOutput{Version: "1.2.3", Commit: "none", Date: "unknown", Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
	if v != want {
		t.Errorf("got %+v, want %+v", v, want)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	if _, err := run(t, BuildInfo{}, "version", "extra"); err == nil {
		t.Error("expected an error for extra arguments")
	}
}
