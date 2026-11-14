package config

import (
	"reflect"
	"strings"
	"testing"
)

func loadFiles(t *testing.T, files map[string]string) *Config {
	t.Helper()
	cfg, err := Load(cfgPath, Options{Env: []string{}, GOOS: "linux", ReadFile: memFiles(files)})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func loadYAML(t *testing.T, src string) *Config {
	t.Helper()
	return loadFiles(t, map[string]string{"cfg/dstate.yaml": src})
}

const baseDiffSrc = `
services:
  api:
    command: run api
    replicas: 2
  worker:
    command: run worker
`

func TestDiffNoChange(t *testing.T) {
	p := Diff(loadYAML(t, baseDiffSrc), loadYAML(t, baseDiffSrc))
	if !p.Empty() {
		t.Errorf("plan = %+v, want empty", p)
	}
}

func TestDiffServiceChanges(t *testing.T) {
	tests := []struct {
		name   string
		api    string // replaces the api service definition
		want   Change
		scaled bool
	}{
		{"command", "command: run api --v2\n    replicas: 2", Change{Restart: true, Fields: []string{"command"}}, false},
		{"cwd", "command: run api\n    replicas: 2\n    cwd: sub", Change{Restart: true, Fields: []string{"cwd"}}, false},
		{"env block", "command: run api\n    replicas: 2\n    env: {A: \"1\"}", Change{Restart: true, Fields: []string{"env"}}, false},
		{"shell", "command: run api\n    replicas: 2\n    shell: true", Change{Restart: true, Fields: []string{"command", "shell"}}, false},
		{"inherit_env", "command: run api\n    replicas: 2\n    inherit_env: false", Change{Restart: true, Fields: []string{"inherit_env"}}, false},
		{"replicas up", "command: run api\n    replicas: 5", Change{Fields: []string{"replicas"}, NewReplicas: 5}, true},
		{"replicas down", "command: run api\n    replicas: 1", Change{Fields: []string{"replicas"}, NewReplicas: 1}, true},
		{"restart policy", "command: run api\n    replicas: 2\n    restart: never", Change{Fields: []string{"restart"}}, false},
		{"logs", "command: run api\n    replicas: 2\n    logs: {max_files: 3}", Change{Fields: []string{"logs"}}, false},
		{"stop settings", "command: run api\n    replicas: 2\n    stop_command: stop\n    stop_timeout: 1m", Change{Fields: []string{"stop_command", "stop_timeout"}}, false},
		{"wait_for", "command: run api\n    replicas: 2\n    wait_for: {targets: [\"tcp://db:5432\"]}", Change{Fields: []string{"wait_for"}}, false},
		{"alerts", "command: run api\n    replicas: 2\n    alerts: {enabled: false}", Change{Fields: []string{"alerts"}}, false},
		{"autostart", "command: run api\n    replicas: 2\n    autostart: false", Change{Fields: []string{"autostart"}}, false},
		{"command and replicas", "command: run api --v2\n    replicas: 3", Change{Restart: true, Fields: []string{"command", "replicas"}, NewReplicas: 3}, true},
	}
	old := loadYAML(t, baseDiffSrc)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := strings.Replace(baseDiffSrc, "command: run api\n    replicas: 2", tt.api, 1)
			p := Diff(old, loadYAML(t, src))
			if len(p.Changes) != 1 || p.AlertsChanged {
				t.Fatalf("plan = %+v, want one change", p)
			}
			want := tt.want
			want.Service, want.Kind, want.OldReplicas = "api", Modified, 2
			if want.NewReplicas == 0 {
				want.NewReplicas = 2
			}
			if got := p.Changes[0]; !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
			if got := p.Changes[0].Scaled(); got != tt.scaled {
				t.Errorf("Scaled() = %v, want %v", got, tt.scaled)
			}
		})
	}
}

func TestDiffAddedAndRemoved(t *testing.T) {
	old := loadYAML(t, baseDiffSrc)
	new := loadYAML(t, "services:\n  queue: {command: run queue, replicas: 2}\n  api: {command: run api, replicas: 2}\n")
	p := Diff(old, new)
	want := []Change{
		{Service: "queue", Kind: Added, NewReplicas: 2},
		{Service: "worker", Kind: Removed, OldReplicas: 1},
	}
	if !reflect.DeepEqual(p.Changes, want) {
		t.Errorf("got  %+v\nwant %+v", p.Changes, want)
	}
	if p.Changes[0].Scaled() || p.Changes[1].Scaled() {
		t.Error("added or removed services are not scaled")
	}
}

func TestDiffDotenvChangeOnly(t *testing.T) {
	src := "env_file: .env\nservices:\n  api: {command: run}\n  other: {command: run, inherit_env: false}\n"
	old := loadFiles(t, map[string]string{"cfg/dstate.yaml": src, "cfg/.env": "DB=a\n"})
	new := loadFiles(t, map[string]string{"cfg/dstate.yaml": src, "cfg/.env": "DB=b\n"})
	p := Diff(old, new)
	if len(p.Changes) != 2 {
		t.Fatalf("plan = %+v, want both services restarted", p)
	}
	for _, c := range p.Changes {
		if !c.Restart || !reflect.DeepEqual(c.Fields, []string{"env"}) {
			t.Errorf("change = %+v", c)
		}
	}
}

func TestDiffInterpolatedValueChange(t *testing.T) {
	src := "env_file: .env\nservices:\n  api: {command: \"run --port ${PORT}\"}\n"
	old := loadFiles(t, map[string]string{"cfg/dstate.yaml": src, "cfg/.env": "PORT=1\n"})
	new := loadFiles(t, map[string]string{"cfg/dstate.yaml": src, "cfg/.env": "PORT=2\n"})
	p := Diff(old, new)
	if len(p.Changes) != 1 || !p.Changes[0].Restart || !reflect.DeepEqual(p.Changes[0].Fields, []string{"command", "env"}) {
		t.Errorf("plan = %+v", p)
	}
}

func TestDiffReplicasTemplate(t *testing.T) {
	tests := []struct{ name, field string }{
		{"command", "command: run --of {{.Replicas}}"},
		{"stop_command", "command: run\n    stop_command: stop --of {{.Replicas}}"},
		{"cwd", "command: run\n    cwd: d{{.Replicas}}"},
		{"env", "command: run\n    env: {N: \"{{.Replicas}}\"}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := loadYAML(t, "services:\n  w:\n    "+tt.field+"\n    replicas: 2\n")
			new := loadYAML(t, "services:\n  w:\n    "+tt.field+"\n    replicas: 3\n")
			c := Diff(old, new).Changes[0]
			if !c.Restart || !c.Scaled() {
				t.Errorf("change = %+v, want restart and scale", c)
			}
		})
	}
	old := loadYAML(t, "services:\n  w: {command: \"run {{.Instance}}\", replicas: 2}\n")
	new := loadYAML(t, "services:\n  w: {command: \"run {{.Instance}}\", replicas: 3}\n")
	if c := Diff(old, new).Changes[0]; c.Restart {
		t.Errorf("{{.Instance}} must not force a restart on scale: %+v", c)
	}
}

func TestDiffAlerts(t *testing.T) {
	ch := "alerts:\n  channels:\n    s: {type: slack, webhook_url: \"https://hooks.slack.com/x\"}\n"
	old := loadYAML(t, ch+baseDiffSrc)
	if p := Diff(old, loadYAML(t, ch+baseDiffSrc)); !p.Empty() {
		t.Errorf("same alerts: %+v", p)
	}
	p := Diff(old, loadYAML(t, ch+"  throttle: {min_interval: 1m}\n"+baseDiffSrc))
	if !p.AlertsChanged || len(p.Changes) != 0 || p.Empty() {
		t.Errorf("plan = %+v, want alerts changed only", p)
	}
}

// TestServiceFieldsClassified fails when a Service field is added without
// deciding how a change to it is applied.
func TestServiceFieldsClassified(t *testing.T) {
	typ := reflect.TypeOf(Service{})
	exported := 0
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() && f.Name != "Name" {
			exported++
		}
	}
	if exported != len(serviceFields) {
		t.Errorf("Service has %d exported fields, serviceFields classifies %d", exported, len(serviceFields))
	}
	seen := map[string]bool{}
	for _, f := range serviceFields {
		if seen[f.key] {
			t.Errorf("duplicate key %q", f.key)
		}
		seen[f.key] = true
	}
}
