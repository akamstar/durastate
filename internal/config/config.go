// Package config loads, interpolates and validates dstate.yaml.
//
// Load returns the effective configuration: every default is applied, paths
// are resolved and commands are split, so consumers never deal with unset
// fields.
package config

// Config is the effective configuration.
type Config struct {
	File     string   // config file path, as given to Load
	Version  int      // format version, always 1 for now
	EnvFiles []string // resolved paths of the global .env files
	Alerts   Alerts
	Services []*Service // in file order
}

// Service returns the service with the given name, or nil.
func (c *Config) Service(name string) *Service {
	for _, s := range c.Services {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// RestartPolicy says when a stopped process is started again.
type RestartPolicy string

const (
	RestartAlways    RestartPolicy = "always"
	RestartOnFailure RestartPolicy = "on-failure"
	RestartNever     RestartPolicy = "never"
)

// StopSignal is the graceful stop signal sent to a process.
type StopSignal string

const (
	SignalCtrlBreak StopSignal = "ctrl_break" // Windows
	SignalTerm      StopSignal = "sigterm"    // Unix
	SignalInt       StopSignal = "sigint"     // Unix
	SignalKill      StopSignal = "kill"       // no graceful stop
	SignalNone      StopSignal = "none"       // rely on stop_command, then kill
)

// Service is one supervised command, run as Replicas instances.
type Service struct {
	Name string

	// Command is the argument list. With Shell, it holds a single element:
	// the command line passed to "cmd /C" or "sh -c".
	Command []string
	Shell   bool
	Cwd     string // absolute

	// Env holds the variables set by dstate, weakest first: global .env,
	// service .env, env block. The daemon environment is added at start
	// time when InheritEnv is true.
	Env        map[string]string
	EnvFiles   []string // resolved paths of the service .env files
	InheritEnv bool

	Replicas  int
	Autostart bool

	Restart          RestartPolicy
	SuccessExitCodes []int
	Backoff          Backoff
	MaxRestarts      int
	RestartWindow    Duration
	StableAfter      Duration
	CrashLoopRetry   CrashLoopRetry
	StartDelay       Duration
	WaitFor          WaitFor

	StopSignal  StopSignal
	StopCommand []string // nil when unset; same shape as Command
	StopTimeout Duration

	Logs   Logs
	Alerts ServiceAlerts
}

// Backoff is the delay between restarts after a crash.
type Backoff struct {
	Initial    Duration
	Max        Duration
	Multiplier float64
}

// CrashLoopRetry controls slow retries once a service is in crash loop.
type CrashLoopRetry struct {
	Enabled bool
	Initial Duration
	Max     Duration
}

// WaitFor lists conditions to meet before starting a service.
type WaitFor struct {
	Targets  []string // tcp://host:port, http(s)://url, winsvc://Name
	Timeout  Duration
	Interval Duration
}

// Logs controls the capture and rotation of a service output.
type Logs struct {
	MaxSize     Size
	MaxFiles    int
	MaxAge      Duration
	Compress    bool
	Timestamps  bool
	MergeStderr bool
}

// ServiceAlerts overrides the global alert rules for one service.
type ServiceAlerts struct {
	Enabled bool
	Rules   []Rule // nil means the global rules apply
}

// Alerts is the global alerting configuration.
type Alerts struct {
	Channels []Channel // in file order
	Rules    []Rule
	Throttle Throttle
}

// Channel is a notification target. Only the fields of its Type are set.
type Channel struct {
	Name string
	Type string // telegram, slack, discord, webhook

	BotToken   string // telegram
	ChatID     string // telegram
	WebhookURL string // slack, discord

	URL     string // webhook
	Method  string // webhook
	Headers map[string]string
	Body    string // webhook, Go template
}

// Rule sends the listed events to the listed channels.
type Rule struct {
	Events   []string
	Channels []string
}

// Throttle limits the alert rate.
type Throttle struct {
	MinInterval Duration
	GroupWindow Duration
}
