package config

import (
	"strings"
	"testing"
)

func mapLookup(m map[string]string) lookupFunc {
	return func(name string) (string, bool) {
		v, ok := m[name]
		return v, ok
	}
}

func TestInterpolate(t *testing.T) {
	vars := mapLookup(map[string]string{"HOST": "db", "PORT": "3306", "EMPTY": ""})
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"${HOST}", "db"},
		{"tcp://${HOST}:${PORT}", "tcp://db:3306"},
		{"${MISSING:-fallback}", "fallback"},
		{"${EMPTY:-fallback}", "fallback"},
		{"${HOST:-fallback}", "db"},
		{"${MISSING:-}", ""},
		{"${MISSING:-a:b-c}", "a:b-c"},
		{"${HOST:?must be set}", "db"},
		{"$$", "$"},
		{"$${HOST}", "${HOST}"},
		{"cost: 5$", "cost: 5$"},
		{"$HOST", "$HOST"},
		{"%PATH%", "%PATH%"},
		{"a $ b", "a $ b"},
		{"$$$${HOST}", "$${HOST}"},
		{"pre${HOST}post", "predbpost"},
	}
	for _, tt := range tests {
		got, err := interpolate(tt.in, vars)
		if err != nil {
			t.Errorf("interpolate(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("interpolate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestInterpolateErrors(t *testing.T) {
	vars := mapLookup(map[string]string{"EMPTY": ""})
	tests := []struct{ in, want string }{
		{"${MISSING}", "variable MISSING is not set (use ${MISSING:-} to allow an empty value)"},
		{"${MISSING:?set it in .env}", "variable MISSING set it in .env"},
		{"${EMPTY:?}", "variable EMPTY is required"},
		{"${OPEN", `unterminated variable reference "${OPEN"`},
		{"${}", `invalid variable name "" in "${}"`},
		{"${1X}", `invalid variable name "1X"`},
		{"${A-B}", `invalid variable name "A-B"`},
		{"${A:+x}", `invalid variable reference "${A:+x}"`},
		{"${A:}", `invalid variable reference "${A:}"`},
		{"${A:-${B}}", "nested variable references are not supported"},
	}
	for _, tt := range tests {
		_, err := interpolate(tt.in, vars)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("interpolate(%q) error = %v, want %q", tt.in, err, tt.want)
		}
	}
}

func FuzzInterpolate(f *testing.F) {
	for _, s := range []string{"${A}", "${B:-x}", "${C:?m}", "$$", "$", "${", "a$b"} {
		f.Add(s)
	}
	lookup := mapLookup(map[string]string{"A": "1"})
	f.Fuzz(func(t *testing.T, s string) {
		got, err := interpolate(s, lookup)
		if err == nil && !strings.Contains(s, "$") && got != s {
			t.Fatalf("text without $ changed: %q -> %q", s, got)
		}
	})
}
