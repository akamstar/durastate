package config

import (
	"regexp"
	"sort"
	"strings"
)

// Masked replaces secret values in every output.
const Masked = "***"

// minMaskLen is the shortest value Mask replaces in free text: shorter
// values ("1", "prod") would hide unrelated text.
const minMaskLen = 4

// secretNameRe matches variable or header names that hold secrets.
var secretNameRe = regexp.MustCompile(`(^|_)(TOKEN|PASSWORD|PASSWD|PASS|SECRET|CREDENTIALS?|PRIVATE|KEY|APIKEY|AUTH|AUTHORIZATION|DSN|COOKIE|SESSION)(_|$)`)

// isSecretName reports whether a variable or header name looks like a secret.
func isSecretName(name string) bool {
	return secretNameRe.MatchString(strings.ToUpper(strings.ReplaceAll(name, "-", "_")))
}

// secrets records which values must be hidden.
type secrets struct {
	// fields maps a field path to the secret values interpolated into it.
	fields map[string][]string
	// values holds every secret value long enough to mask in free text,
	// longest first so that a value containing another is replaced first.
	values []string
}

func newSecrets(fields map[string][]string, dotenvs ...map[string]string) *secrets {
	s := &secrets{fields: fields}
	seen := map[string]bool{}
	add := func(v string) {
		if len(v) >= minMaskLen && !seen[v] {
			seen[v] = true
			s.values = append(s.values, v)
		}
	}
	for _, vals := range fields {
		for _, v := range vals {
			add(v)
		}
	}
	for _, env := range dotenvs {
		for _, v := range env {
			add(v)
		}
	}
	sort.Slice(s.values, func(i, j int) bool {
		if len(s.values[i]) != len(s.values[j]) {
			return len(s.values[i]) > len(s.values[j])
		}
		return s.values[i] < s.values[j]
	})
	return s
}

// Mask hides every known secret value in s. Use it on any text that may
// contain configuration values: logs, events, alert messages.
func (c *Config) Mask(s string) string {
	if c.secrets == nil {
		return s
	}
	for _, v := range c.secrets.values {
		s = strings.ReplaceAll(s, v, Masked)
	}
	return s
}

// maskField hides the secrets interpolated into the fields at the given
// paths (and their list items), whatever their length.
func (s *secrets) maskField(value string, paths ...string) string {
	if s == nil {
		return value
	}
	var vals []string
	for p, vs := range s.fields {
		for _, prefix := range paths {
			if p == prefix || strings.HasPrefix(p, prefix+"[") || strings.HasPrefix(p, prefix+".") {
				vals = append(vals, vs...)
			}
		}
	}
	sort.Slice(vals, func(i, j int) bool { return len(vals[i]) > len(vals[j]) })
	for _, v := range vals {
		value = strings.ReplaceAll(value, v, Masked)
	}
	return value
}
