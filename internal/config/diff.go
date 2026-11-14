package config

import (
	"reflect"
	"strings"
)

// ChangeKind says what happens to a service when a new config is applied.
type ChangeKind string

const (
	Added    ChangeKind = "added"
	Removed  ChangeKind = "removed"
	Modified ChangeKind = "modified"
)

// Change describes how one service differs between two configs.
type Change struct {
	Service string
	Kind    ChangeKind
	// Restart is true when the process definition changed (command, cwd,
	// environment...): running instances must be restarted, one by one.
	Restart bool
	// Fields lists the changed fields of a modified service, in config order.
	Fields      []string
	OldReplicas int
	NewReplicas int
}

// Scaled reports whether the number of instances changed.
func (c Change) Scaled() bool {
	return c.Kind == Modified && c.OldReplicas != c.NewReplicas
}

// Plan is the difference between two configs.
type Plan struct {
	Changes       []Change // new config order, then removed services
	AlertsChanged bool     // channels, rules or throttle changed
}

// Empty reports whether applying the new config changes nothing.
func (p Plan) Empty() bool { return len(p.Changes) == 0 && !p.AlertsChanged }

// fieldClass says how a changed field is applied.
type fieldClass int

const (
	classProcess fieldClass = iota // restart the instances
	classScale                     // add or remove instances
	classHot                       // apply without restart
)

// serviceFields classifies every field of Service except Name.
// TestServiceFieldsClassified fails when a new field is missing here.
var serviceFields = []struct {
	key   string
	class fieldClass
	get   func(*Service) any
}{
	{"command", classProcess, func(s *Service) any { return s.Command }},
	{"shell", classProcess, func(s *Service) any { return s.Shell }},
	{"cwd", classProcess, func(s *Service) any { return s.Cwd }},
	{"env", classProcess, func(s *Service) any { return s.Env }},
	{"env_file", classHot, func(s *Service) any { return s.EnvFiles }}, // content is compared through env
	{"inherit_env", classProcess, func(s *Service) any { return s.InheritEnv }},
	{"replicas", classScale, func(s *Service) any { return s.Replicas }},
	{"autostart", classHot, func(s *Service) any { return s.Autostart }},
	{"restart", classHot, func(s *Service) any { return s.Restart }},
	{"success_exit_codes", classHot, func(s *Service) any { return s.SuccessExitCodes }},
	{"backoff", classHot, func(s *Service) any { return s.Backoff }},
	{"max_restarts", classHot, func(s *Service) any { return s.MaxRestarts }},
	{"restart_window", classHot, func(s *Service) any { return s.RestartWindow }},
	{"stable_after", classHot, func(s *Service) any { return s.StableAfter }},
	{"crash_loop_retry", classHot, func(s *Service) any { return s.CrashLoopRetry }},
	{"start_delay", classHot, func(s *Service) any { return s.StartDelay }},
	{"wait_for", classHot, func(s *Service) any { return s.WaitFor }},
	{"stop_signal", classHot, func(s *Service) any { return s.StopSignal }},
	{"stop_command", classHot, func(s *Service) any { return s.StopCommand }},
	{"stop_timeout", classHot, func(s *Service) any { return s.StopTimeout }},
	{"logs", classHot, func(s *Service) any { return s.Logs }},
	{"alerts", classHot, func(s *Service) any { return s.Alerts }},
}

// Diff compares two configs. Unchanged services are not listed.
func Diff(old, new *Config) Plan {
	var p Plan
	for _, ns := range new.Services {
		os := old.Service(ns.Name)
		if os == nil {
			p.Changes = append(p.Changes, Change{Service: ns.Name, Kind: Added, NewReplicas: ns.Replicas})
			continue
		}
		if c, changed := diffService(os, ns); changed {
			p.Changes = append(p.Changes, c)
		}
	}
	for _, os := range old.Services {
		if new.Service(os.Name) == nil {
			p.Changes = append(p.Changes, Change{Service: os.Name, Kind: Removed, OldReplicas: os.Replicas})
		}
	}
	p.AlertsChanged = !reflect.DeepEqual(old.Alerts, new.Alerts)
	return p
}

func diffService(old, new *Service) (Change, bool) {
	c := Change{Service: new.Name, Kind: Modified, OldReplicas: old.Replicas, NewReplicas: new.Replicas}
	for _, f := range serviceFields {
		if reflect.DeepEqual(f.get(old), f.get(new)) {
			continue
		}
		c.Fields = append(c.Fields, f.key)
		if f.class == classProcess {
			c.Restart = true
		}
	}
	// Templates using .Replicas render differently for existing instances.
	if c.Scaled() && usesReplicas(new) {
		c.Restart = true
	}
	return c, len(c.Fields) > 0
}

// usesReplicas reports whether a rendered field depends on {{.Replicas}}.
func usesReplicas(s *Service) bool {
	fields := append(append([]string{s.Cwd}, s.Command...), s.StopCommand...)
	for k := range s.templatedEnv {
		fields = append(fields, s.Env[k])
	}
	for _, f := range fields {
		if strings.Contains(f, "{{") && strings.Contains(f, ".Replicas") {
			return true
		}
	}
	return false
}
