package config

import "strings"

// envMap is a set of environment variables. On Windows, names are
// case-insensitive: "Path" and "PATH" are the same variable.
type envMap struct {
	fold bool
	vals map[string]envVar // keyed by normalized name
}

type envVar struct {
	name  string // spelling of the last definition
	value string
}

func newEnvMap(goos string) *envMap {
	return &envMap{fold: goos == "windows", vals: map[string]envVar{}}
}

// envFromList builds an envMap from "KEY=value" entries, as returned by os.Environ.
func envFromList(goos string, list []string) *envMap {
	m := newEnvMap(goos)
	for _, kv := range list {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" { // Windows has hidden entries like "=C:=C:\"
			continue
		}
		m.set(k, v)
	}
	return m
}

func (m *envMap) norm(name string) string {
	if m.fold {
		return strings.ToUpper(name)
	}
	return name
}

func (m *envMap) set(name, value string) {
	m.vals[m.norm(name)] = envVar{name: name, value: value}
}

func (m *envMap) lookup(name string) (string, bool) {
	v, ok := m.vals[m.norm(name)]
	return v.value, ok
}

// merge adds every variable of src, overriding existing ones.
func (m *envMap) merge(src map[string]string) {
	for k, v := range src {
		m.set(k, v)
	}
}

// toMap returns the variables keyed by their spelling.
func (m *envMap) toMap() map[string]string {
	out := make(map[string]string, len(m.vals))
	for _, v := range m.vals {
		out[v.name] = v.value
	}
	return out
}

// processEnv merges the environment of a service process, weakest first:
// daemon environment (only if inherit), then each layer in order
// (global .env, service .env, env block).
func processEnv(goos string, daemon []string, inherit bool, layers ...map[string]string) map[string]string {
	m := newEnvMap(goos)
	if inherit {
		m = envFromList(goos, daemon)
	}
	for _, l := range layers {
		m.merge(l)
	}
	return m.toMap()
}

// layered looks variables up in each map in turn; the first match wins.
func layered(maps ...*envMap) lookupFunc {
	return func(name string) (string, bool) {
		for _, m := range maps {
			if v, ok := m.lookup(name); ok {
				return v, true
			}
		}
		return "", false
	}
}
