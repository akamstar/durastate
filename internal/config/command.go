package config

import (
	"errors"
	"strings"
)

// splitCommand splits a command line into arguments.
//
//   - Spaces, tabs and newlines separate arguments.
//   - "double" and 'single' quotes group text; quotes may be adjacent to
//     other text (--name="a b" gives --name=a b). "" is an empty argument.
//   - Backslashes are always literal, so Windows paths need no escaping.
//     To pass a double quote, wrap it in single quotes: '"'.
//   - A template action like {{add 8000 .Instance}} is never split.
func splitCommand(s string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		inArg   bool
		quote   byte
		quoteAt int
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		switch {
		case c == '"' || c == '\'':
			quote, quoteAt, inArg = c, i, true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		case c == '{' && strings.HasPrefix(s[i:], "{{"):
			end := strings.Index(s[i+2:], "}}")
			if end < 0 {
				return nil, errors.New(`unterminated template action "{{"`)
			}
			cur.WriteString(s[i : i+2+end+2])
			i += 2 + end + 1
			inArg = true
		default:
			cur.WriteByte(c)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote starting at " + quoteContext(s, quoteAt))
	}
	if inArg {
		args = append(args, cur.String())
	}
	if len(args) == 0 {
		return nil, errors.New("empty command")
	}
	return args, nil
}

func quoteContext(s string, at int) string {
	const max = 20
	end := min(len(s), at+max)
	ctx := s[at:end]
	if end < len(s) {
		ctx += "..."
	}
	return "`" + ctx + "`"
}
