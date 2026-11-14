package config

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// Variables injected in every instance; they cannot be overridden.
const (
	EnvService  = "DSTATE_SERVICE"
	EnvInstance = "DSTATE_INSTANCE"
	EnvReplicas = "DSTATE_REPLICAS"
)

// reservedEnvPrefix is reserved for variables set by dstate.
const reservedEnvPrefix = "DSTATE_"

// InstanceData is the data available to templates: {{.Instance}}.
type InstanceData struct {
	Service  string
	Instance int // 1-based
	Replicas int
}

var templateFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
}

// Instance is the rendered definition of one process of a service.
type Instance struct {
	Name        string // "api:1"
	Index       int    // 1-based
	Command     []string
	StopCommand []string
	Cwd         string
	Env         map[string]string // without the daemon environment
}

// Instance renders the templates of instance n (1-based). Templates are
// rendered in command, stop_command, cwd and the env block only, never in
// values read from .env files.
func (s *Service) Instance(n int) (*Instance, error) {
	data := InstanceData{Service: s.Name, Instance: n, Replicas: s.Replicas}
	in := &Instance{Name: fmt.Sprintf("%s:%d", s.Name, n), Index: n, Env: make(map[string]string, len(s.Env)+3)}
	var err error
	if in.Command, err = renderAll(s.Command, data); err != nil {
		return nil, fmt.Errorf("command: %w", err)
	}
	if in.StopCommand, err = renderAll(s.StopCommand, data); err != nil {
		return nil, fmt.Errorf("stop_command: %w", err)
	}
	if in.Cwd, err = render(s.Cwd, data); err != nil {
		return nil, fmt.Errorf("cwd: %w", err)
	}
	for k, v := range s.Env {
		if s.templatedEnv[k] {
			if v, err = render(v, data); err != nil {
				return nil, fmt.Errorf("env.%s: %w", k, err)
			}
		}
		in.Env[k] = v
	}
	in.Env[EnvService] = s.Name
	in.Env[EnvInstance] = fmt.Sprint(n)
	in.Env[EnvReplicas] = fmt.Sprint(s.Replicas)
	return in, nil
}

func renderAll(in []string, data InstanceData) ([]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		v, err := render(s, data)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func render(s string, data InstanceData) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	t, err := template.New("").Funcs(templateFuncs).Parse(s)
	if err != nil {
		return "", templateError(err)
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", templateError(err)
	}
	return b.String(), nil
}

var (
	templatePrefixRe = regexp.MustCompile(`^template: :\d+(:\d+)?: (executing "" at <[^>]*>: )?`)
	dataTypeRe       = regexp.MustCompile(`in type \S*InstanceData`)
)

// templateError shortens text/template messages for config errors.
func templateError(err error) error {
	msg := templatePrefixRe.ReplaceAllString(err.Error(), "")
	msg = dataTypeRe.ReplaceAllString(msg, "(available: .Service, .Instance, .Replicas)")
	return fmt.Errorf("invalid template: %s", msg)
}
