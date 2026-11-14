package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	// Names are used in log paths, instance names ("api:1") and CLI arguments.
	nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

	// Windows device names cannot be used as directory names.
	windowsReservedNames = []string{
		"con", "prn", "aux", "nul",
		"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
		"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9",
	}

	headerNameRe = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]+$")

	webhookMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
)

// maxExitCode is the largest exit code (Windows codes are 32-bit unsigned).
const maxExitCode = 1<<32 - 1

// stopSignalsFor returns the stop signals supported on goos.
func stopSignalsFor(goos string) []string {
	if goos == "windows" {
		return []string{"ctrl_break", "kill", "none"}
	}
	return []string{"sigterm", "sigint", "kill", "none"}
}

type validator struct {
	file string
	raw  *rawConfig
	goos string
	errs ErrorList
}

// validate checks the rules that the decoder cannot express: required
// fields, ranges, references and OS support. Scalar values are checked
// where they are written (defaults block or service), so an error is
// reported once, at the right line.
func validate(cfg *Config, raw *rawConfig, goos string) ErrorList {
	v := &validator{file: cfg.File, raw: raw, goos: goos}

	if len(cfg.Services) == 0 {
		v.add("services", "at least one service is required")
	}
	if raw.Defaults != nil {
		v.policy(raw.Defaults, "defaults")
	}
	channels := v.alerts(cfg.Alerts)
	for i, s := range cfg.Services {
		v.service(s, raw.Services[i], channels)
	}
	return v.errs
}

func (v *validator) service(s *Service, rs *rawService, channels map[string]bool) {
	path := "services." + s.Name
	v.name(s.Name, path, true)
	v.policy(&rs.rawPolicy, path)

	if rs.Command == nil {
		v.add(path, "missing required field \"command\"")
	}
	if s.Replicas < 1 {
		v.add(path+".replicas", "must be at least 1 (got %d), use autostart: false to define a service without starting it", s.Replicas)
	}
	for k := range rs.Env {
		if !envNameRe.MatchString(k) {
			v.add(path+".env."+k, "invalid variable name %q", k)
		} else if strings.HasPrefix(strings.ToUpper(k), reservedEnvPrefix) {
			v.add(path+".env."+k, "the %s prefix is reserved for variables set by dstate", reservedEnvPrefix)
		}
	}

	// Cross-field rules use the merged values.
	if s.Backoff.Max < s.Backoff.Initial {
		v.add(path+".backoff", "max (%s) must not be lower than initial (%s)", s.Backoff.Max, s.Backoff.Initial)
	}
	if s.CrashLoopRetry.Enabled && s.CrashLoopRetry.Max < s.CrashLoopRetry.Initial {
		v.add(path+".crash_loop_retry", "max (%s) must not be lower than initial (%s)", s.CrashLoopRetry.Max, s.CrashLoopRetry.Initial)
	}

	if w := rs.WaitFor; w != nil {
		for i, t := range w.Targets {
			if err := checkTarget(t, v.goos); err != nil {
				v.add(fmt.Sprintf("%s.wait_for.targets[%d]", path, i), "%v", err)
			}
		}
		v.positive(w.Timeout, path+".wait_for.timeout")
		v.positive(w.Interval, path+".wait_for.interval")
	}
	if s.Alerts.Rules != nil {
		v.rules(s.Alerts.Rules, path+".alerts.rules", channels)
	}

	// Templates: rendering the first and last instance catches every error.
	if s.Replicas >= 1 && len(s.Command) > 0 {
		for _, n := range []int{1, s.Replicas} {
			if _, err := s.Instance(n); err != nil {
				field, msg, _ := strings.Cut(err.Error(), ": ")
				v.add(path+"."+field, "%s", msg)
				break
			}
		}
	}
}

// policy checks the explicit values of a defaults block or a service.
func (v *validator) policy(p *rawPolicy, path string) {
	if p.SuccessExitCodes != nil {
		if len(p.SuccessExitCodes) == 0 {
			v.add(path+".success_exit_codes", "must not be empty")
		}
		for i, c := range p.SuccessExitCodes {
			if c < 0 || int64(c) > maxExitCode {
				v.add(fmt.Sprintf("%s.success_exit_codes[%d]", path, i), "invalid exit code %d", c)
			}
		}
	}
	if p.MaxRestarts != nil && *p.MaxRestarts < 1 {
		v.add(path+".max_restarts", "must be at least 1 (got %d)", *p.MaxRestarts)
	}
	v.positive(p.RestartWindow, path+".restart_window")
	v.positive(p.StopTimeout, path+".stop_timeout")
	if b := p.Backoff; b != nil {
		v.positive(b.Initial, path+".backoff.initial")
		v.positive(b.Max, path+".backoff.max")
		if b.Multiplier != nil && *b.Multiplier < 1 {
			v.add(path+".backoff.multiplier", "must be at least 1 (got %s)", strconv.FormatFloat(*b.Multiplier, 'f', -1, 64))
		}
	}
	if c := p.CrashLoopRetry; c != nil {
		v.positive(c.Initial, path+".crash_loop_retry.initial")
		v.positive(c.Max, path+".crash_loop_retry.max")
	}
	if p.StopSignal != nil {
		if ok := stopSignalsFor(v.goos); !slices.Contains(ok, *p.StopSignal) {
			v.add(path+".stop_signal", "%q is not supported on %s (expected %s)", *p.StopSignal, v.goos, strings.Join(ok, ", "))
		}
	}
	if l := p.Logs; l != nil {
		// Rotation works in whole megabytes and days (lumberjack).
		if l.MaxSize != nil && *l.MaxSize < 1<<20 {
			v.add(path+".logs.max_size", "must be at least 1MB (got %s)", *l.MaxSize)
		}
		if l.MaxFiles != nil && *l.MaxFiles < 1 {
			v.add(path+".logs.max_files", "must be at least 1 (got %d)", *l.MaxFiles)
		}
		if l.MaxAge != nil && *l.MaxAge != 0 && *l.MaxAge < Duration(24*time.Hour) {
			v.add(path+".logs.max_age", "must be 0 (no limit) or at least 1d (got %s)", *l.MaxAge)
		}
	}
	if a := p.Alerts; a != nil && a.Rules != nil && len(a.Rules) == 0 {
		v.add(path+".alerts.rules", "must not be empty, use enabled: false to disable alerts")
	}
}

// alerts checks channels and global rules, and returns the channel names.
func (v *validator) alerts(a Alerts) map[string]bool {
	names := map[string]bool{}
	for _, c := range a.Channels {
		path := "alerts.channels." + c.Name
		names[c.Name] = true
		v.name(c.Name, path, false)
		switch c.Type {
		case "telegram":
			v.required(c.BotToken, path, "bot_token")
			v.required(c.ChatID, path, "chat_id")
		case "slack", "discord":
			if v.required(c.WebhookURL, path, "webhook_url") {
				v.url(c.WebhookURL, path+".webhook_url")
			}
		case "webhook":
			if v.required(c.URL, path, "url") {
				v.url(c.URL, path+".url")
			}
			if !slices.Contains(webhookMethods, c.Method) {
				v.add(path+".method", "invalid method %q (expected %s)", c.Method, strings.Join(webhookMethods, ", "))
			}
			for h := range c.Headers {
				if !headerNameRe.MatchString(h) {
					v.add(path+".headers."+h, "invalid header name %q", h)
				}
			}
		}
	}
	v.rules(a.Rules, "alerts.rules", names)
	return names
}

func (v *validator) rules(rules []Rule, path string, channels map[string]bool) {
	for i, r := range rules {
		rp := fmt.Sprintf("%s[%d]", path, i)
		if len(r.Events) == 0 {
			v.add(rp, "missing required field \"events\"")
		}
		if len(r.Channels) == 0 {
			v.add(rp, "missing required field \"channels\"")
		}
		for j, c := range r.Channels {
			if !channels[c] {
				v.add(fmt.Sprintf("%s.channels[%d]", rp, j), "unknown channel %q%s", c, v.channelHint(channels))
			}
		}
	}
}

func (v *validator) channelHint(channels map[string]bool) string {
	if len(channels) == 0 {
		return " (no channel is defined in alerts.channels)"
	}
	names := make([]string, 0, len(channels))
	for n := range channels {
		names = append(names, n)
	}
	slices.Sort(names)
	return " (defined: " + strings.Join(names, ", ") + ")"
}

// name checks a service or channel name.
func (v *validator) name(name, path string, service bool) {
	if !nameRe.MatchString(name) {
		v.add(path, "invalid name %q (use lowercase letters, digits, - and _, starting with a letter or digit, 63 characters max)", name)
		return
	}
	if !service {
		return
	}
	if name == "all" {
		v.add(path, "%q is reserved (dstate start all)", name)
	}
	if slices.Contains(windowsReservedNames, name) {
		v.add(path, "%q is a reserved device name on Windows", name)
	}
}

func (v *validator) required(val, path, field string) bool {
	if strings.TrimSpace(val) == "" {
		v.add(path, "missing required field %q", field)
		return false
	}
	return true
}

func (v *validator) url(raw, path string) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		v.add(path, "invalid URL %q (expected http:// or https://)", raw)
	}
}

func (v *validator) positive(d *Duration, path string) {
	if d != nil && *d <= 0 {
		v.add(path, "must be greater than 0")
	}
}

func (v *validator) add(path, format string, args ...any) {
	v.errs.add(v.at(path), path, format, args...)
}

// at returns the position of path, or of its closest known parent.
func (v *validator) at(path string) Pos {
	for p := path; p != ""; {
		if pos, ok := v.raw.pos[p]; ok {
			return pos
		}
		i := strings.LastIndexAny(p, ".[")
		if i < 0 {
			break
		}
		p = p[:i]
	}
	return Pos{File: v.file}
}

// checkTarget validates a wait_for target.
func checkTarget(t, goos string) error {
	scheme, rest, ok := strings.Cut(t, "://")
	if !ok {
		return fmt.Errorf("invalid target %q (expected tcp://host:port, http(s)://url or winsvc://Name)", t)
	}
	switch scheme {
	case "tcp":
		host, port, err := net.SplitHostPort(rest)
		if err != nil || host == "" {
			return fmt.Errorf("invalid target %q (expected tcp://host:port)", t)
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid port %q in %q", port, t)
		}
	case "http", "https":
		if u, err := url.Parse(t); err != nil || u.Host == "" {
			return fmt.Errorf("invalid URL %q", t)
		}
	case "winsvc":
		if goos != "windows" {
			return fmt.Errorf("winsvc:// targets are only supported on Windows")
		}
		if rest == "" || strings.ContainsAny(rest, `/\`) {
			return fmt.Errorf("invalid Windows service name in %q", t)
		}
	default:
		return fmt.Errorf("unsupported scheme %q (expected tcp, http, https, winsvc)", scheme)
	}
	return nil
}
