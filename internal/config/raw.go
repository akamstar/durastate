package config

// Allowed values for enumerated fields.
var (
	restartPolicies = []string{"always", "on-failure", "never"}
	stopSignals     = []string{"ctrl_break", "sigterm", "sigint", "kill", "none"}
	channelTypes    = []string{"telegram", "slack", "discord", "webhook"}
	alertEvents     = []string{
		"crashed", "crash_loop", "recovered", "start_failed",
		"config_rejected", "wait_timeout", "daemon_started", "daemon_stopped",
	}
)

// The raw* types mirror the YAML file as written. A nil pointer or slice means
// the field is absent, so defaults can be applied afterwards.

type rawConfig struct {
	Version  *int
	Defaults *rawPolicy
	EnvFile  []string
	Alerts   *rawAlerts
	Services []*rawService // in file order

	// pos maps a field path ("services.api.restart") to the position of its key.
	pos map[string]Pos
	// dotenv holds the variables of the global .env files, merged in order.
	dotenv map[string]string
}

// rawPolicy holds the fields allowed both in "defaults" and in a service.
type rawPolicy struct {
	Shell            *bool
	InheritEnv       *bool
	Restart          *string
	SuccessExitCodes []int
	Backoff          *rawBackoff
	MaxRestarts      *int
	RestartWindow    *Duration
	StableAfter      *Duration
	CrashLoopRetry   *rawCrashLoopRetry
	StartDelay       *Duration
	StopSignal       *string
	StopCommand      *rawCommand
	StopTimeout      *Duration
	Logs             *rawLogs
	Alerts           *rawServiceAlerts
}

type rawService struct {
	Name string
	rawPolicy

	Command   *rawCommand
	Cwd       *string
	Env       map[string]string
	EnvFile   []string
	Replicas  *int
	Autostart *bool
	WaitFor   *rawWaitFor

	// dotenv holds the variables of the service .env files, merged in order.
	dotenv map[string]string
}

// serviceOnlyFields cannot appear in "defaults".
var serviceOnlyFields = []string{"command", "cwd", "env", "env_file", "replicas", "autostart", "wait_for"}

// rawCommand is either a command line (split later) or an explicit argument list.
type rawCommand struct {
	Line   string
	Args   []string
	IsList bool
}

type rawBackoff struct {
	Initial    *Duration
	Max        *Duration
	Multiplier *float64
}

type rawCrashLoopRetry struct {
	Initial *Duration
	Max     *Duration
	Enabled *bool
}

type rawLogs struct {
	MaxSize     *Size
	MaxFiles    *int
	MaxAge      *Duration
	Compress    *bool
	Timestamps  *bool
	MergeStderr *bool
}

type rawWaitFor struct {
	Targets  []string
	Timeout  *Duration
	Interval *Duration
}

type rawAlerts struct {
	Channels []*rawChannel // in file order
	Rules    []*rawRule
	Throttle *rawThrottle
}

type rawChannel struct {
	Name       string
	Type       string
	BotToken   *string
	ChatID     *string
	WebhookURL *string
	URL        *string
	Method     *string
	Headers    map[string]string
	Body       *string
}

type rawRule struct {
	Events   []string
	Channels []string
}

type rawThrottle struct {
	MinInterval *Duration
	GroupWindow *Duration
}

// rawServiceAlerts overrides the global alert rules for one service.
type rawServiceAlerts struct {
	Enabled *bool
	Rules   []*rawRule
}
