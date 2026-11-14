package config

import (
	"bytes"
	"regexp"
	"strings"
)

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseDotenv parses a .env file. Values are literal: no interpolation.
//
//	KEY=value            # unquoted, trimmed, inline comment after " #"
//	KEY="a\tb\n"         # double quotes: \n \r \t \" \\ escapes
//	KEY='raw $value'     # single quotes: no escapes
//	export KEY=value     # "export" is ignored
//
// Later definitions of the same key win.
func parseDotenv(file string, data []byte) (map[string]string, ErrorList) {
	var errs ErrorList
	vars := map[string]string{}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // BOM written by some Windows editors

	for i, line := range strings.Split(string(data), "\n") {
		pos := Pos{File: file, Line: i + 1}
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(rest)
		}
		key, raw, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok {
			errs.add(pos, "", "invalid line (expected KEY=value)")
			continue
		}
		if !envNameRe.MatchString(key) {
			errs.add(pos, "", "invalid variable name %q", key)
			continue
		}
		val, err := dotenvValue(strings.TrimSpace(raw))
		if err != "" {
			errs.add(pos, key, "%s", err)
			continue
		}
		vars[key] = val
	}
	return vars, errs
}

// dotenvValue decodes the right-hand side of a line. It returns an error message, if any.
func dotenvValue(raw string) (string, string) {
	if raw == "" {
		return "", ""
	}
	switch q := raw[0]; q {
	case '"', '\'':
		end := closingQuote(raw, q)
		if end < 0 {
			return "", "unterminated quoted value"
		}
		if rest := strings.TrimSpace(raw[end+1:]); rest != "" && !strings.HasPrefix(rest, "#") {
			return "", "unexpected characters after the closing quote"
		}
		body := raw[1:end]
		if q == '\'' {
			return body, ""
		}
		return unescapeDouble(body), ""
	}
	if i := strings.Index(raw, " #"); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.Index(raw, "\t#"); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw), ""
}

func closingQuote(s string, q byte) int {
	for i := 1; i < len(s); i++ {
		switch {
		case q == '"' && s[i] == '\\':
			i++ // skip the escaped character
		case s[i] == q:
			return i
		}
	}
	return -1
}

func unescapeDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i == len(s)-1 {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '"', '\\':
			b.WriteByte(s[i])
		default: // unknown escape: keep it as is (Windows paths)
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
