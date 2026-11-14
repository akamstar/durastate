package config

import (
	"reflect"
	"testing"
)

func TestEnvFromList(t *testing.T) {
	m := envFromList("linux", []string{"A=1", "B=x=y", "=C:=C:\\", "NOEQ", "EMPTY="})
	if want := map[string]string{"A": "1", "B": "x=y", "EMPTY": ""}; !reflect.DeepEqual(m.toMap(), want) {
		t.Errorf("got %v, want %v", m.toMap(), want)
	}
}

func TestEnvCaseFolding(t *testing.T) {
	win := envFromList("windows", []string{"Path=C:\\bin"})
	if v, ok := win.lookup("PATH"); !ok || v != "C:\\bin" {
		t.Errorf("windows lookup PATH = %q, %v", v, ok)
	}
	win.set("PATH", "D:\\bin")
	if want := map[string]string{"PATH": "D:\\bin"}; !reflect.DeepEqual(win.toMap(), want) {
		t.Errorf("windows override = %v, want %v", win.toMap(), want)
	}

	unix := envFromList("linux", []string{"Path=/bin"})
	if _, ok := unix.lookup("PATH"); ok {
		t.Error("unix lookup must be case-sensitive")
	}
}

func TestLayered(t *testing.T) {
	high := newEnvMap("linux")
	high.set("A", "high")
	low := envFromList("linux", []string{"A=low", "B=low"})
	lookup := layered(high, low)
	if v, _ := lookup("A"); v != "high" {
		t.Errorf("A = %q, want high", v)
	}
	if v, _ := lookup("B"); v != "low" {
		t.Errorf("B = %q, want low", v)
	}
	if _, ok := lookup("C"); ok {
		t.Error("C must be unset")
	}
}

func TestProcessEnv(t *testing.T) {
	daemon := []string{"PATH=/bin", "A=daemon", "B=daemon", "C=daemon", "D=daemon"}
	global := map[string]string{"B": "global", "C": "global", "D": "global"}
	service := map[string]string{"C": "service", "D": "service"}
	block := map[string]string{"D": "block"}

	got := processEnv("linux", daemon, true, global, service, block)
	want := map[string]string{"PATH": "/bin", "A": "daemon", "B": "global", "C": "service", "D": "block"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("inherit: got %v, want %v", got, want)
	}

	got = processEnv("linux", daemon, false, global, nil, block)
	want = map[string]string{"B": "global", "C": "global", "D": "block"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("no inherit: got %v, want %v", got, want)
	}

	got = processEnv("windows", []string{"Path=C:\\bin"}, true, map[string]string{"PATH": "D:\\bin"})
	if want := map[string]string{"PATH": "D:\\bin"}; !reflect.DeepEqual(got, want) {
		t.Errorf("windows: got %v, want %v", got, want)
	}
}
