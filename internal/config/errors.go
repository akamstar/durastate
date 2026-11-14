package config

import (
	"fmt"
	"sort"
	"strings"
)

// Pos is a location in a source file. Line and Col are 1-based; zero means unknown.
type Pos struct {
	File string
	Line int
	Col  int
}

func (p Pos) String() string {
	switch {
	case p.Line > 0 && p.Col > 0:
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
	case p.Line > 0:
		return fmt.Sprintf("%s:%d", p.File, p.Line)
	default:
		return p.File
	}
}

// Error is a single configuration problem.
type Error struct {
	Pos  Pos
	Path string // e.g. "services.api.restart"; empty for file-level errors
	Msg  string
}

func (e *Error) Error() string {
	var b strings.Builder
	if s := e.Pos.String(); s != "" {
		b.WriteString(s)
		b.WriteString(": ")
	}
	if e.Path != "" {
		b.WriteString(e.Path)
		b.WriteString(": ")
	}
	b.WriteString(e.Msg)
	return b.String()
}

// ErrorList collects every problem found in a configuration.
type ErrorList []*Error

func (l ErrorList) Error() string {
	msgs := make([]string, len(l))
	for i, e := range l {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

func (l *ErrorList) add(pos Pos, path, format string, args ...any) {
	*l = append(*l, &Error{Pos: pos, Path: path, Msg: fmt.Sprintf(format, args...)})
}

// Err returns nil when the list is empty, otherwise the list sorted by position.
func (l ErrorList) Err() error {
	if len(l) == 0 {
		return nil
	}
	sort.SliceStable(l, func(i, j int) bool {
		a, b := l[i].Pos, l[j].Pos
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	return l
}
