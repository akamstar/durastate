package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"python app.py", []string{"python", "app.py"}},
		{"  python   app.py  ", []string{"python", "app.py"}},
		{"run\t-a\n-b\r\n", []string{"run", "-a", "-b"}},
		{`node "my app.js"`, []string{"node", "my app.js"}},
		{`echo 'single $X'`, []string{"echo", "single $X"}},
		{`--name="a b"`, []string{"--name=a b"}},
		{`a"b"'c'd`, []string{"abcd"}},
		{`run "" x`, []string{"run", "", "x"}},
		{`run ''`, []string{"run", ""}},
		{`C:\Program Files\app.exe`, []string{`C:\Program`, `Files\app.exe`}},
		{`"C:\Program Files\app.exe" --dir "D:\data\"`, []string{`C:\Program Files\app.exe`, "--dir", `D:\data\`}},
		{`scripts\build-report.bat`, []string{`scripts\build-report.bat`}},
		{`say '"quoted"'`, []string{"say", `"quoted"`}},
		{`say "it's"`, []string{"say", "it's"}},
		{"serve --port {{add 8000 .Instance}}", []string{"serve", "--port", "{{add 8000 .Instance}}"}},
		{"--id=w{{ .Instance }}", []string{"--id=w{{ .Instance }}"}},
		{"a {", []string{"a", "{"}},
		{"php bin/console messenger:consume async --time-limit=3600", []string{"php", "bin/console", "messenger:consume", "async", "--time-limit=3600"}},
	}
	for _, tt := range tests {
		got, err := splitCommand(tt.in)
		if err != nil {
			t.Errorf("splitCommand(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCommand(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitCommandErrors(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "empty command"},
		{"   \n", "empty command"},
		{`node "app.js`, "unterminated quote starting at `\"app.js`"},
		{`echo 'a very long unterminated argument`, "unterminated quote starting at `'a very long untermi...`"},
		{"serve {{.Instance", `unterminated template action "{{"`},
	}
	for _, tt := range tests {
		_, err := splitCommand(tt.in)
		if err == nil || err.Error() != tt.want {
			t.Errorf("splitCommand(%q) error = %v, want %q", tt.in, err, tt.want)
		}
	}
}

func FuzzSplitCommand(f *testing.F) {
	for _, s := range []string{`a "b c" 'd'`, `x{{.Instance}}`, `"`, "{{", `C:\a\ b`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		args, err := splitCommand(s)
		if err != nil {
			return
		}
		if len(args) == 0 {
			t.Fatalf("splitCommand(%q) returned no argument and no error", s)
		}
		// Arguments without separators, quotes or braces survive a round trip.
		for _, a := range args {
			if a == "" || strings.ContainsAny(a, " \t\r\n\"'{}") {
				return
			}
		}
		again, err := splitCommand(strings.Join(args, " "))
		if err != nil || !reflect.DeepEqual(again, args) {
			t.Fatalf("round trip of %q: %q -> %q (%v)", s, args, again, err)
		}
	})
}
