package config

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/goccy/go-yaml"
)

// YAML renders the effective configuration in the dstate.yaml format, with
// every default applied. Secrets are masked unless showSecrets is true.
func (c *Config) YAML(showSecrets bool) ([]byte, error) {
	return yaml.MarshalWithOptions(c.document(showSecrets), yaml.IndentSequence(true))
}

// JSON renders the effective configuration as indented JSON.
func (c *Config) JSON(showSecrets bool) ([]byte, error) {
	return json.MarshalIndent(c.document(showSecrets), "", "  ")
}

// MarshalText renders a Duration as in the config file ("5m", "14d").
func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// MarshalYAML renders a Duration as a plain string.
func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }

// MarshalText renders a Size as in the config file ("50MB").
func (s Size) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// MarshalYAML renders a Size as a plain string.
func (s Size) MarshalYAML() (any, error) { return s.String(), nil }

// The doc* types mirror the config format for output.

type docConfig struct {
	Version  int        `yaml:"version" json:"version"`
	EnvFile  []string   `yaml:"env_file,omitempty" json:"env_file,omitempty"`
	Alerts   *docAlerts `yaml:"alerts,omitempty" json:"alerts,omitempty"`
	Services orderedMap `yaml:"services" json:"services"`
}

type docService struct {
	Command          []string        `yaml:"command" json:"command"`
	Shell            bool            `yaml:"shell" json:"shell"`
	Cwd              string          `yaml:"cwd" json:"cwd"`
	Env              orderedMap      `yaml:"env,omitempty" json:"env,omitempty"`
	EnvFile          []string        `yaml:"env_file,omitempty" json:"env_file,omitempty"`
	InheritEnv       bool            `yaml:"inherit_env" json:"inherit_env"`
	Replicas         int             `yaml:"replicas" json:"replicas"`
	Autostart        bool            `yaml:"autostart" json:"autostart"`
	Restart          RestartPolicy   `yaml:"restart" json:"restart"`
	SuccessExitCodes []int           `yaml:"success_exit_codes" json:"success_exit_codes"`
	Backoff          Backoff         `yaml:"backoff" json:"backoff"`
	MaxRestarts      int             `yaml:"max_restarts" json:"max_restarts"`
	RestartWindow    Duration        `yaml:"restart_window" json:"restart_window"`
	StableAfter      Duration        `yaml:"stable_after" json:"stable_after"`
	CrashLoopRetry   CrashLoopRetry  `yaml:"crash_loop_retry" json:"crash_loop_retry"`
	StartDelay       Duration        `yaml:"start_delay" json:"start_delay"`
	WaitFor          *WaitFor        `yaml:"wait_for,omitempty" json:"wait_for,omitempty"`
	StopSignal       StopSignal      `yaml:"stop_signal" json:"stop_signal"`
	StopCommand      []string        `yaml:"stop_command,omitempty" json:"stop_command,omitempty"`
	StopTimeout      Duration        `yaml:"stop_timeout" json:"stop_timeout"`
	Logs             Logs            `yaml:"logs" json:"logs"`
	Alerts           docServiceAlert `yaml:"alerts" json:"alerts"`
}

type docServiceAlert struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Rules   []Rule `yaml:"rules,omitempty" json:"rules,omitempty"`
}

type docAlerts struct {
	Channels orderedMap `yaml:"channels,omitempty" json:"channels,omitempty"`
	Rules    []Rule     `yaml:"rules,omitempty" json:"rules,omitempty"`
	Throttle Throttle   `yaml:"throttle" json:"throttle"`
}

type docChannel struct {
	Type       string     `yaml:"type" json:"type"`
	BotToken   string     `yaml:"bot_token,omitempty" json:"bot_token,omitempty"`
	ChatID     string     `yaml:"chat_id,omitempty" json:"chat_id,omitempty"`
	WebhookURL string     `yaml:"webhook_url,omitempty" json:"webhook_url,omitempty"`
	URL        string     `yaml:"url,omitempty" json:"url,omitempty"`
	Method     string     `yaml:"method,omitempty" json:"method,omitempty"`
	Headers    orderedMap `yaml:"headers,omitempty" json:"headers,omitempty"`
	Body       string     `yaml:"body,omitempty" json:"body,omitempty"`
}

// document builds the output form of the configuration.
func (c *Config) document(showSecrets bool) *docConfig {
	sec := c.secrets
	if showSecrets {
		sec = nil
	}
	mask := func(v string, paths ...string) string { return sec.maskField(v, paths...) }
	maskAll := func(v string) string {
		if sec == nil || v == "" {
			return v
		}
		return Masked
	}

	d := &docConfig{Version: c.Version, EnvFile: c.EnvFiles}
	if len(c.Alerts.Channels) > 0 || len(c.Alerts.Rules) > 0 {
		a := &docAlerts{Rules: c.Alerts.Rules, Throttle: c.Alerts.Throttle}
		for _, ch := range c.Alerts.Channels {
			p := "alerts.channels." + ch.Name
			dc := docChannel{
				Type:       ch.Type,
				BotToken:   maskAll(ch.BotToken),
				ChatID:     mask(ch.ChatID, p+".chat_id"),
				WebhookURL: maskAll(ch.WebhookURL),
				URL:        mask(ch.URL, p+".url"),
				Method:     ch.Method,
				Body:       mask(ch.Body, p+".body"),
			}
			for _, k := range sortedKeys(ch.Headers) {
				v := mask(ch.Headers[k], p+".headers."+k)
				if isSecretName(k) {
					v = maskAll(v)
				}
				dc.Headers = append(dc.Headers, kv{k, v})
			}
			a.Channels = append(a.Channels, kv{ch.Name, dc})
		}
		d.Alerts = a
	}

	for _, s := range c.Services {
		p := "services." + s.Name
		ds := docService{
			Command:          maskList(s.Command, sec, p+".command"),
			Shell:            s.Shell,
			Cwd:              mask(s.Cwd, p+".cwd"),
			EnvFile:          s.EnvFiles,
			InheritEnv:       s.InheritEnv,
			Replicas:         s.Replicas,
			Autostart:        s.Autostart,
			Restart:          s.Restart,
			SuccessExitCodes: s.SuccessExitCodes,
			Backoff:          s.Backoff,
			MaxRestarts:      s.MaxRestarts,
			RestartWindow:    s.RestartWindow,
			StableAfter:      s.StableAfter,
			CrashLoopRetry:   s.CrashLoopRetry,
			StartDelay:       s.StartDelay,
			StopSignal:       s.StopSignal,
			StopCommand:      maskList(s.StopCommand, sec, p+".stop_command", "defaults.stop_command"),
			StopTimeout:      s.StopTimeout,
			Logs:             s.Logs,
			Alerts:           docServiceAlert{Enabled: s.Alerts.Enabled, Rules: s.Alerts.Rules},
		}
		for _, k := range sortedKeys(s.Env) {
			v := mask(s.Env[k], p+".env."+k)
			if s.secretEnv[k] {
				v = maskAll(v)
			}
			ds.Env = append(ds.Env, kv{k, v})
		}
		if len(s.WaitFor.Targets) > 0 {
			w := s.WaitFor
			w.Targets = maskList(w.Targets, sec, p+".wait_for.targets")
			ds.WaitFor = &w
		}
		d.Services = append(d.Services, kv{s.Name, ds})
	}
	return d
}

func maskList(in []string, sec *secrets, paths ...string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = sec.maskField(v, paths...)
	}
	return out
}

// kv is an entry of an orderedMap.
type kv struct {
	Key   string
	Value any
}

// orderedMap is a mapping rendered in insertion order, in YAML and JSON.
type orderedMap []kv

func (m orderedMap) MarshalYAML() (any, error) {
	out := make(yaml.MapSlice, len(m))
	for i, e := range m {
		out[i] = yaml.MapItem{Key: e.Key, Value: e.Value}
	}
	return out, nil
}

func (m orderedMap) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, e := range m {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(e.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(e.Value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
