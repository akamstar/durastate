package config

import "time"

// Built-in defaults for service-only fields.
const (
	defaultReplicas        = 1
	defaultAutostart       = true
	defaultWaitForTimeout  = Duration(5 * time.Minute)
	defaultWaitForInterval = Duration(2 * time.Second)
	defaultWebhookMethod   = "POST"
	defaultMinInterval     = Duration(5 * time.Minute)
	defaultGroupWindow     = Duration(30 * time.Second)
)

// builtinPolicy returns the defaults of every policy field.
// Every pointer is set, so a merged policy is always complete.
func builtinPolicy(goos string) *rawPolicy {
	stop := string(SignalTerm)
	if goos == "windows" {
		stop = string(SignalCtrlBreak)
	}
	return &rawPolicy{
		Shell:            ptrTo(false),
		InheritEnv:       ptrTo(true),
		Restart:          ptrTo(string(RestartAlways)),
		SuccessExitCodes: []int{0},
		Backoff: &rawBackoff{
			Initial:    ptrTo(Duration(time.Second)),
			Max:        ptrTo(Duration(5 * time.Minute)),
			Multiplier: ptrTo(2.0),
		},
		MaxRestarts:   ptrTo(10),
		RestartWindow: ptrTo(Duration(10 * time.Minute)),
		StableAfter:   ptrTo(Duration(time.Minute)),
		CrashLoopRetry: &rawCrashLoopRetry{
			Initial: ptrTo(Duration(5 * time.Minute)),
			Max:     ptrTo(Duration(time.Hour)),
			Enabled: ptrTo(true),
		},
		StartDelay:  ptrTo(Duration(0)),
		StopSignal:  &stop,
		StopTimeout: ptrTo(Duration(30 * time.Second)),
		Logs: &rawLogs{
			MaxSize:     ptrTo(Size(50 << 20)),
			MaxFiles:    ptrTo(10),
			MaxAge:      ptrTo(Duration(14 * 24 * time.Hour)),
			Compress:    ptrTo(true),
			Timestamps:  ptrTo(true),
			MergeStderr: ptrTo(false),
		},
		Alerts: &rawServiceAlerts{Enabled: ptrTo(true)},
	}
}

// mergePolicy returns top with its unset fields taken from bottom.
// Nested blocks are merged field by field.
func mergePolicy(top, bottom *rawPolicy) *rawPolicy {
	if top == nil {
		return bottom
	}
	if bottom == nil {
		return top
	}
	return &rawPolicy{
		Shell:            or(top.Shell, bottom.Shell),
		InheritEnv:       or(top.InheritEnv, bottom.InheritEnv),
		Restart:          or(top.Restart, bottom.Restart),
		SuccessExitCodes: orSlice(top.SuccessExitCodes, bottom.SuccessExitCodes),
		Backoff:          mergeBackoff(top.Backoff, bottom.Backoff),
		MaxRestarts:      or(top.MaxRestarts, bottom.MaxRestarts),
		RestartWindow:    or(top.RestartWindow, bottom.RestartWindow),
		StableAfter:      or(top.StableAfter, bottom.StableAfter),
		CrashLoopRetry:   mergeCrashLoopRetry(top.CrashLoopRetry, bottom.CrashLoopRetry),
		StartDelay:       or(top.StartDelay, bottom.StartDelay),
		StopSignal:       or(top.StopSignal, bottom.StopSignal),
		StopCommand:      or(top.StopCommand, bottom.StopCommand),
		StopTimeout:      or(top.StopTimeout, bottom.StopTimeout),
		Logs:             mergeLogs(top.Logs, bottom.Logs),
		Alerts:           mergeServiceAlerts(top.Alerts, bottom.Alerts),
	}
}

func mergeBackoff(top, bottom *rawBackoff) *rawBackoff {
	if top == nil || bottom == nil {
		return or(top, bottom)
	}
	return &rawBackoff{
		Initial:    or(top.Initial, bottom.Initial),
		Max:        or(top.Max, bottom.Max),
		Multiplier: or(top.Multiplier, bottom.Multiplier),
	}
}

func mergeCrashLoopRetry(top, bottom *rawCrashLoopRetry) *rawCrashLoopRetry {
	if top == nil || bottom == nil {
		return or(top, bottom)
	}
	return &rawCrashLoopRetry{
		Initial: or(top.Initial, bottom.Initial),
		Max:     or(top.Max, bottom.Max),
		Enabled: or(top.Enabled, bottom.Enabled),
	}
}

func mergeLogs(top, bottom *rawLogs) *rawLogs {
	if top == nil || bottom == nil {
		return or(top, bottom)
	}
	return &rawLogs{
		MaxSize:     or(top.MaxSize, bottom.MaxSize),
		MaxFiles:    or(top.MaxFiles, bottom.MaxFiles),
		MaxAge:      or(top.MaxAge, bottom.MaxAge),
		Compress:    or(top.Compress, bottom.Compress),
		Timestamps:  or(top.Timestamps, bottom.Timestamps),
		MergeStderr: or(top.MergeStderr, bottom.MergeStderr),
	}
}

// mergeServiceAlerts merges "enabled"; "rules" is replaced as a whole.
func mergeServiceAlerts(top, bottom *rawServiceAlerts) *rawServiceAlerts {
	if top == nil || bottom == nil {
		return or(top, bottom)
	}
	return &rawServiceAlerts{
		Enabled: or(top.Enabled, bottom.Enabled),
		Rules:   orSlice(top.Rules, bottom.Rules),
	}
}

func or[T any](top, bottom *T) *T {
	if top != nil {
		return top
	}
	return bottom
}

func orSlice[T any](top, bottom []T) []T {
	if top != nil {
		return top
	}
	return bottom
}

func ptrTo[T any](v T) *T { return &v }
