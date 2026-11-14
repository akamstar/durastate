package config

import (
	"errors"
	"testing"
)

func TestErrorFormat(t *testing.T) {
	tests := []struct {
		err  Error
		want string
	}{
		{Error{Pos{"dstate.yaml", 3, 5}, "services.api", "boom"}, "dstate.yaml:3:5: services.api: boom"},
		{Error{Pos{".env", 2, 0}, "", "bad line"}, ".env:2: bad line"},
		{Error{Pos{"dstate.yaml", 0, 0}, "", "empty"}, "dstate.yaml: empty"},
		{Error{Pos{}, "services", "boom"}, "services: boom"},
		{Error{Pos{}, "", "boom"}, "boom"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestErrorListErr(t *testing.T) {
	var l ErrorList
	if l.Err() != nil {
		t.Fatal("empty list must return a nil error")
	}
	l.add(Pos{"b.yaml", 1, 1}, "", "c")
	l.add(Pos{"a.yaml", 9, 1}, "", "b")
	l.add(Pos{"a.yaml", 2, 7}, "", "a2")
	l.add(Pos{"a.yaml", 2, 3}, "", "a1")
	err := l.Err()
	var got ErrorList
	if !errors.As(err, &got) {
		t.Fatalf("Err() returned %T", err)
	}
	want := "a.yaml:2:3: a1\na.yaml:2:7: a2\na.yaml:9:1: b\nb.yaml:1:1: c"
	if err.Error() != want {
		t.Errorf("got:\n%s\nwant:\n%s", err, want)
	}
}
