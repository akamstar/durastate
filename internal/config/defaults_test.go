package config

import (
	"reflect"
	"testing"
	"time"
)

// assertComplete fails if any pointer reachable from v is nil, so a field
// added to rawPolicy without a built-in default is caught.
func assertComplete(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			t.Errorf("%s has no built-in default", path)
			return
		}
		assertComplete(t, v.Elem(), path)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			assertComplete(t, v.Field(i), path+"."+v.Type().Field(i).Name)
		}
	}
}

func TestBuiltinPolicyIsComplete(t *testing.T) {
	p := builtinPolicy("windows")
	// Optional by design: no stop command, and no per-service rules.
	p.StopCommand = &rawCommand{}
	p.Alerts.Rules = []*rawRule{}
	assertComplete(t, reflect.ValueOf(p), "policy")
}

func TestBuiltinStopSignal(t *testing.T) {
	if got := *builtinPolicy("windows").StopSignal; got != "ctrl_break" {
		t.Errorf("windows: %s", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := *builtinPolicy(goos).StopSignal; got != "sigterm" {
			t.Errorf("%s: %s", goos, got)
		}
	}
}

func TestMergePolicy(t *testing.T) {
	top := &rawPolicy{
		Restart: ptrTo("never"),
		Backoff: &rawBackoff{Max: ptrTo(Duration(time.Minute))},
		Logs:    &rawLogs{MaxFiles: ptrTo(3)},
		Alerts:  &rawServiceAlerts{Rules: []*rawRule{{Events: []string{"crashed"}}}},
	}
	middle := &rawPolicy{
		Restart:          ptrTo("on-failure"),
		StopTimeout:      ptrTo(Duration(time.Minute)),
		SuccessExitCodes: []int{0, 3},
		Backoff:          &rawBackoff{Initial: ptrTo(Duration(2 * time.Second)), Max: ptrTo(Duration(time.Hour))},
		Logs:             &rawLogs{MaxFiles: ptrTo(5), Compress: ptrTo(false)},
		Alerts:           &rawServiceAlerts{Enabled: ptrTo(false), Rules: []*rawRule{}},
		CrashLoopRetry:   &rawCrashLoopRetry{Enabled: ptrTo(false)},
	}
	got := mergePolicy(mergePolicy(top, middle), builtinPolicy("linux"))

	checks := []struct {
		name      string
		got, want any
	}{
		{"restart from top", *got.Restart, "never"},
		{"stop_timeout from middle", *got.StopTimeout, Duration(time.Minute)},
		{"exit codes from middle", got.SuccessExitCodes, []int{0, 3}},
		{"max_restarts from builtin", *got.MaxRestarts, 10},
		{"backoff.max from top", *got.Backoff.Max, Duration(time.Minute)},
		{"backoff.initial from middle", *got.Backoff.Initial, Duration(2 * time.Second)},
		{"backoff.multiplier from builtin", *got.Backoff.Multiplier, 2.0},
		{"logs.max_files from top", *got.Logs.MaxFiles, 3},
		{"logs.compress from middle", *got.Logs.Compress, false},
		{"logs.max_size from builtin", *got.Logs.MaxSize, Size(50 << 20)},
		{"alerts.enabled from middle", *got.Alerts.Enabled, false},
		{"alerts.rules from top", len(got.Alerts.Rules), 1},
		{"crash_loop_retry.enabled from middle", *got.CrashLoopRetry.Enabled, false},
		{"crash_loop_retry.max from builtin", *got.CrashLoopRetry.Max, Duration(time.Hour)},
		{"stop_command stays unset", got.StopCommand, (*rawCommand)(nil)},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestMergePolicyNil(t *testing.T) {
	p := &rawPolicy{Restart: ptrTo("never")}
	if mergePolicy(p, nil) != p || mergePolicy(nil, p) != p {
		t.Error("merging with nil must return the other policy")
	}
}
