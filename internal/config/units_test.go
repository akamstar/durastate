package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"0", 0},
		{"0s", 0},
		{"30s", 30 * time.Second},
		{"5m", 5 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"500ms", 500 * time.Millisecond},
		{"14d", 14 * 24 * time.Hour},
		{"1d12h", 36 * time.Hour},
		{"1.5d", 36 * time.Hour},
		{" 2m ", 2 * time.Minute},
	}
	for _, tt := range tests {
		got, err := ParseDuration(tt.in)
		if err != nil {
			t.Errorf("ParseDuration(%q): %v", tt.in, err)
			continue
		}
		if time.Duration(got) != tt.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", tt.in, time.Duration(got), tt.want)
		}
	}
}

func TestParseDurationErrors(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "empty duration"},
		{"30", "invalid duration \"30\""},
		{"-5s", "must not be negative"},
		{"5x", "invalid duration \"5x\""},
		{"abc", "invalid duration"},
		{"99999999999d", "out of range"},
		{"d", "invalid duration"},
	}
	for _, tt := range tests {
		_, err := ParseDuration(tt.in)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("ParseDuration(%q) error = %v, want %q", tt.in, err, tt.want)
		}
	}
}

func TestDurationString(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h30m"},
		{2 * time.Hour, "2h"},
		{14 * 24 * time.Hour, "14d"},
		{36 * time.Hour, "36h"},
		{1500 * time.Millisecond, "1.5s"},
	}
	for _, tt := range tests {
		if got := Duration(tt.in).String(); got != tt.want {
			t.Errorf("Duration(%v).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"0B", 0},
		{"100B", 100},
		{"512KB", 512 << 10},
		{"50MB", 50 << 20},
		{"50MiB", 50 << 20},
		{"1GB", 1 << 30},
		{"2TB", 2 << 40},
		{"10 MB", 10 << 20},
	}
	for _, tt := range tests {
		got, err := ParseSize(tt.in)
		if err != nil {
			t.Errorf("ParseSize(%q): %v", tt.in, err)
			continue
		}
		if int64(got) != tt.want {
			t.Errorf("ParseSize(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseSizeErrors(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "invalid size"},
		{"50", "invalid size \"50\""},
		{"1.5GB", "invalid size"},
		{"-1MB", "invalid size"},
		{"50mb", "invalid size"},
		{"10PB", "invalid size"},
		{"99999999999TB", "out of range"},
	}
	for _, tt := range tests {
		_, err := ParseSize(tt.in)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("ParseSize(%q) error = %v, want %q", tt.in, err, tt.want)
		}
	}
}

func TestSizeString(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{100, "100B"},
		{1536, "1536B"},
		{512 << 10, "512KB"},
		{50 << 20, "50MB"},
		{1 << 30, "1GB"},
		{3 << 40, "3TB"},
	}
	for _, tt := range tests {
		if got := Size(tt.in).String(); got != tt.want {
			t.Errorf("Size(%d).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func FuzzParseDuration(f *testing.F) {
	for _, s := range []string{"30s", "14d", "1d12h", "1.5d", "-1s", "d", "1h30m"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := ParseDuration(s)
		if err != nil {
			return
		}
		if d < 0 {
			t.Fatalf("ParseDuration(%q) = %v, negative", s, d)
		}
		back, err := ParseDuration(d.String())
		if err != nil || back != d {
			t.Fatalf("round trip %q -> %q -> %v (%v)", s, d.String(), back, err)
		}
	})
}

func FuzzParseSize(f *testing.F) {
	for _, s := range []string{"50MB", "1GB", "0B", "512 KiB"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := ParseSize(s)
		if err != nil {
			return
		}
		back, err := ParseSize(v.String())
		if err != nil || back != v {
			t.Fatalf("round trip %q -> %q -> %v (%v)", s, v.String(), back, err)
		}
	})
}
