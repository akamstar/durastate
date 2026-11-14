package config

import (
	"fmt"
	"strings"
)

// lookupFunc returns the value of a variable and whether it is set.
type lookupFunc func(name string) (string, bool)

// interpolate expands variables in s:
//
//	${VAR}          value of VAR, error if unset
//	${VAR:-default} default if VAR is unset or empty
//	${VAR:?message} error with message if VAR is unset or empty
//	$$              a literal "$"
//
// Any other "$" is kept as is, so "$VAR" and "%VAR%" are not expanded.
func interpolate(s string, lookup lookupFunc) (string, error) {
	if !strings.Contains(s, "$") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' || i == len(s)-1 {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case '$':
			b.WriteByte('$')
			i++
		case '{':
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				return "", fmt.Errorf("unterminated variable reference %q", s[i:])
			}
			val, err := expandRef(s[i+2:i+2+end], lookup)
			if err != nil {
				return "", err
			}
			b.WriteString(val)
			i += 2 + end
		default:
			b.WriteByte('$')
		}
	}
	return b.String(), nil
}

// expandRef expands the inside of "${...}".
func expandRef(ref string, lookup lookupFunc) (string, error) {
	name, op, arg := ref, "", ""
	if i := strings.Index(ref, ":"); i >= 0 {
		name, op = ref[:i], ref[i:]
		if len(op) < 2 || (op[1] != '-' && op[1] != '?') {
			return "", fmt.Errorf("invalid variable reference \"${%s}\" (expected ${VAR}, ${VAR:-default} or ${VAR:?message})", ref)
		}
		op, arg = op[:2], op[2:]
		if strings.Contains(arg, "${") {
			return "", fmt.Errorf("nested variable references are not supported in \"${%s}\"", ref)
		}
	}
	if !envNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid variable name %q in \"${%s}\"", name, ref)
	}
	val, ok := lookup(name)
	switch op {
	case ":-":
		if !ok || val == "" {
			return arg, nil
		}
	case ":?":
		if !ok || val == "" {
			if arg == "" {
				arg = "is required"
			}
			return "", fmt.Errorf("variable %s %s", name, arg)
		}
	default:
		if !ok {
			return "", fmt.Errorf("variable %s is not set (use ${%s:-} to allow an empty value)", name, name)
		}
	}
	return val, nil
}
