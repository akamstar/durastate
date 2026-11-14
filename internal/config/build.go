package config

import (
	"path/filepath"
	"strings"
)

// Load reads the config file at path and returns the effective configuration.
// On failure the error is an ErrorList holding every problem found.
func Load(path string, opt Options) (*Config, error) {
	opt = opt.withDefaults()
	raw, errs := loadRaw(path, opt)
	if len(errs) > 0 {
		return nil, errs.Err()
	}
	cfg, errs := build(path, raw, opt)
	if len(errs) == 0 {
		errs = validate(cfg, raw, opt.GOOS)
	}
	if len(errs) > 0 {
		return nil, errs.Err()
	}
	return cfg, nil
}

// builder turns a raw config into an effective one.
type builder struct {
	raw  *rawConfig
	opt  Options
	base string // absolute directory of the config file
	errs ErrorList
}

func build(path string, raw *rawConfig, opt Options) (*Config, ErrorList) {
	b := &builder{raw: raw, opt: opt, base: absPath(filepath.Dir(path))}
	cfg := &Config{
		File:     path,
		Version:  1,
		EnvFiles: b.paths(raw.EnvFile),
		Alerts:   b.alerts(raw.Alerts),
		Services: make([]*Service, 0, len(raw.Services)),
	}
	if raw.Version != nil {
		cfg.Version = *raw.Version
	}
	builtin := builtinPolicy(opt.GOOS)
	for _, rs := range raw.Services {
		cfg.Services = append(cfg.Services, b.service(rs, mergePolicy(mergePolicy(&rs.rawPolicy, raw.Defaults), builtin)))
	}
	return cfg, b.errs
}

// service builds a service from its raw form and its fully merged policy.
func (b *builder) service(rs *rawService, pl *rawPolicy) *Service {
	path := "services." + rs.Name
	env := newEnvMap(b.opt.GOOS)
	env.merge(b.raw.dotenv)
	env.merge(rs.dotenv)
	env.merge(rs.Env)

	s := &Service{
		Name:       rs.Name,
		Shell:      *pl.Shell,
		Cwd:        b.base,
		Env:        env.toMap(),
		EnvFiles:   b.paths(rs.EnvFile),
		InheritEnv: *pl.InheritEnv,
		Replicas:   defaultReplicas,
		Autostart:  defaultAutostart,

		Restart:          RestartPolicy(*pl.Restart),
		SuccessExitCodes: pl.SuccessExitCodes,
		Backoff: Backoff{
			Initial:    *pl.Backoff.Initial,
			Max:        *pl.Backoff.Max,
			Multiplier: *pl.Backoff.Multiplier,
		},
		MaxRestarts:   *pl.MaxRestarts,
		RestartWindow: *pl.RestartWindow,
		StableAfter:   *pl.StableAfter,
		CrashLoopRetry: CrashLoopRetry{
			Enabled: *pl.CrashLoopRetry.Enabled,
			Initial: *pl.CrashLoopRetry.Initial,
			Max:     *pl.CrashLoopRetry.Max,
		},
		StartDelay: *pl.StartDelay,
		WaitFor:    WaitFor{Timeout: defaultWaitForTimeout, Interval: defaultWaitForInterval},

		StopSignal:  StopSignal(*pl.StopSignal),
		StopTimeout: *pl.StopTimeout,

		Logs: Logs{
			MaxSize:     *pl.Logs.MaxSize,
			MaxFiles:    *pl.Logs.MaxFiles,
			MaxAge:      *pl.Logs.MaxAge,
			Compress:    *pl.Logs.Compress,
			Timestamps:  *pl.Logs.Timestamps,
			MergeStderr: *pl.Logs.MergeStderr,
		},
		Alerts: ServiceAlerts{Enabled: *pl.Alerts.Enabled, Rules: rules(pl.Alerts.Rules)},
	}
	for k, v := range rs.Env {
		if strings.Contains(v, "{{") {
			if s.templatedEnv == nil {
				s.templatedEnv = map[string]bool{}
			}
			s.templatedEnv[k] = true
		}
	}
	s.Command = b.command(rs.Command, s.Shell, path+".command")
	s.StopCommand = b.command(pl.StopCommand, s.Shell, path+".stop_command")
	if rs.Cwd != nil {
		if strings.TrimSpace(*rs.Cwd) == "" {
			b.errs.add(b.raw.pos[path+".cwd"], path+".cwd", "empty path")
		} else {
			s.Cwd = absPath(resolvePath(b.base, *rs.Cwd))
		}
	}
	if rs.Replicas != nil {
		s.Replicas = *rs.Replicas
	}
	if rs.Autostart != nil {
		s.Autostart = *rs.Autostart
	}
	if w := rs.WaitFor; w != nil {
		s.WaitFor.Targets = w.Targets
		if w.Timeout != nil {
			s.WaitFor.Timeout = *w.Timeout
		}
		if w.Interval != nil {
			s.WaitFor.Interval = *w.Interval
		}
	}
	return s
}

// command returns the argument list of a command, or nil if it is unset.
func (b *builder) command(rc *rawCommand, shell bool, path string) []string {
	switch {
	case rc == nil:
		return nil
	case shell && rc.IsList:
		b.errs.add(b.raw.pos[path], path, "with shell: true, the command must be a single string")
		return nil
	case shell:
		return []string{strings.TrimSpace(rc.Line)}
	case rc.IsList:
		return rc.Args
	}
	args, err := splitCommand(rc.Line)
	if err != nil {
		b.errs.add(b.raw.pos[path], path, "%v", err)
		return nil
	}
	return args
}

func (b *builder) alerts(ra *rawAlerts) Alerts {
	a := Alerts{Throttle: Throttle{MinInterval: defaultMinInterval, GroupWindow: defaultGroupWindow}}
	if ra == nil {
		return a
	}
	for _, rc := range ra.Channels {
		c := Channel{
			Name:       rc.Name,
			Type:       rc.Type,
			BotToken:   deref(rc.BotToken),
			ChatID:     deref(rc.ChatID),
			WebhookURL: deref(rc.WebhookURL),
			URL:        deref(rc.URL),
			Method:     deref(rc.Method),
			Headers:    rc.Headers,
			Body:       deref(rc.Body),
		}
		if c.Type == "webhook" {
			c.Method = strings.ToUpper(c.Method)
			if c.Method == "" {
				c.Method = defaultWebhookMethod
			}
		}
		a.Channels = append(a.Channels, c)
	}
	a.Rules = rules(ra.Rules)
	if t := ra.Throttle; t != nil {
		if t.MinInterval != nil {
			a.Throttle.MinInterval = *t.MinInterval
		}
		if t.GroupWindow != nil {
			a.Throttle.GroupWindow = *t.GroupWindow
		}
	}
	return a
}

// paths resolves file paths against the config directory.
func (b *builder) paths(ps []string) []string {
	if ps == nil {
		return nil
	}
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = resolvePath(b.base, p)
	}
	return out
}

// rules converts raw rules; nil stays nil (meaning "not set").
func rules(rs []*rawRule) []Rule {
	if rs == nil {
		return nil
	}
	out := make([]Rule, len(rs))
	for i, r := range rs {
		out[i] = Rule{Events: orSlice(r.Events, []string{}), Channels: orSlice(r.Channels, []string{})}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// absPath makes p absolute; it only fails if the working directory is gone.
func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
