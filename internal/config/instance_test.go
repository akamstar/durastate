package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestServiceInstance(t *testing.T) {
	cfg, err := Load("dstate.yaml", Options{Env: []string{}, GOOS: "linux", ReadFile: memFiles(map[string]string{
		"dstate.yaml": `
env_file: .env
services:
  web:
    command: serve --port {{add 8000 .Instance}} --name {{.Service}}
    stop_command: stop {{.Instance}}
    cwd: /srv/{{.Service}}-{{.Instance}}
    replicas: 3
    env:
      ID: "{{.Instance}}/{{.Replicas}}"
      PLAIN: value
`,
		".env": "SECRET=p{{ass}}\n",
	})})
	if err != nil {
		t.Fatal(err)
	}
	web := cfg.Service("web")

	in, err := web.Instance(2)
	if err != nil {
		t.Fatal(err)
	}
	if in.Name != "web:2" || in.Index != 2 {
		t.Errorf("name = %q, index = %d", in.Name, in.Index)
	}
	if want := []string{"serve", "--port", "8002", "--name", "web"}; !reflect.DeepEqual(in.Command, want) {
		t.Errorf("command = %q, want %q", in.Command, want)
	}
	if want := []string{"stop", "2"}; !reflect.DeepEqual(in.StopCommand, want) {
		t.Errorf("stop_command = %q", in.StopCommand)
	}
	if !strings.HasSuffix(in.Cwd, "web-2") {
		t.Errorf("cwd = %q", in.Cwd)
	}
	want := map[string]string{
		"ID": "2/3", "PLAIN": "value",
		"SECRET":         "p{{ass}}", // .env values are never rendered
		"DSTATE_SERVICE": "web", "DSTATE_INSTANCE": "2", "DSTATE_REPLICAS": "3",
	}
	if !reflect.DeepEqual(in.Env, want) {
		t.Errorf("env = %v, want %v", in.Env, want)
	}
	if web.Env["ID"] != "{{.Instance}}/{{.Replicas}}" {
		t.Error("Instance must not modify the service")
	}
}

func TestServiceInstanceNoTemplates(t *testing.T) {
	s := &Service{Name: "a", Command: []string{"run"}, Cwd: "/x", Replicas: 1, Env: map[string]string{}}
	in, err := s.Instance(1)
	if err != nil {
		t.Fatal(err)
	}
	if in.StopCommand != nil || !reflect.DeepEqual(in.Command, []string{"run"}) || in.Cwd != "/x" {
		t.Errorf("instance = %+v", in)
	}
}

func TestServiceInstanceErrors(t *testing.T) {
	tests := []struct {
		name string
		s    Service
		want string
	}{
		{"command", Service{Command: []string{"{{.X}}"}}, "command: invalid template: can't evaluate field X (available: .Service, .Instance, .Replicas)"},
		{"stop_command", Service{StopCommand: []string{"{{"}}, "stop_command: invalid template"},
		{"cwd", Service{Cwd: "{{.Y}}"}, "cwd: invalid template"},
		{"env", Service{Env: map[string]string{"A": "{{bad}}"}, templatedEnv: map[string]bool{"A": true}}, `env.A: invalid template: function "bad" not defined`},
	}
	for _, tt := range tests {
		_, err := tt.s.Instance(1)
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want prefix %q", tt.name, err, tt.want)
		}
	}
}
