package config

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Duration is a non-negative time.Duration that also accepts a "d" (24h) unit.
type Duration time.Duration

var dayRe = regexp.MustCompile(`(\d+(?:\.\d+)?)d`)

// ParseDuration parses "30s", "1h30m", "14d" or "1d12h".
func ParseDuration(s string) (Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration")
	}
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("invalid duration %q: must not be negative", s)
	}
	var convErr error
	// Go units never contain "d", so every "d" is a day.
	expanded := dayRe.ReplaceAllStringFunc(s, func(m string) string {
		days, err := strconv.ParseFloat(strings.TrimSuffix(m, "d"), 64)
		if err != nil || days*24 > math.MaxInt64/float64(time.Hour) {
			convErr = fmt.Errorf("invalid duration %q: out of range", s)
			return m
		}
		return strconv.FormatFloat(days*24, 'f', -1, 64) + "h"
	})
	if convErr != nil {
		return 0, convErr
	}
	d, err := time.ParseDuration(expanded)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (examples: 30s, 5m, 1h30m, 14d)", s)
	}
	return Duration(d), nil
}

// String formats the duration compactly: "14d", "5m", "1h30m".
func (d Duration) String() string {
	td := time.Duration(d)
	if td == 0 {
		return "0s"
	}
	if td%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", td/(24*time.Hour))
	}
	s := td.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// Size is a byte count. Units are powers of 1024, like lumberjack.
type Size int64

var sizeRe = regexp.MustCompile(`^(\d+)\s*([KMGT]i?B|B)$`)

var sizeUnits = map[string]int64{
	"B":  1,
	"KB": 1 << 10, "KiB": 1 << 10,
	"MB": 1 << 20, "MiB": 1 << 20,
	"GB": 1 << 30, "GiB": 1 << 30,
	"TB": 1 << 40, "TiB": 1 << 40,
}

// ParseSize parses "512KB", "50MB" or "1GB". A unit is required.
func ParseSize(s string) (Size, error) {
	m := sizeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("invalid size %q (examples: 512KB, 50MB, 1GB)", s)
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	unit := sizeUnits[m[2]]
	if err != nil || n > math.MaxInt64/unit {
		return 0, fmt.Errorf("invalid size %q: out of range", s)
	}
	return Size(n * unit), nil
}

// String formats the size with the largest exact unit.
func (s Size) String() string {
	for _, u := range []struct {
		name string
		n    int64
	}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}} {
		if s != 0 && int64(s)%u.n == 0 {
			return fmt.Sprintf("%d%s", int64(s)/u.n, u.name)
		}
	}
	return fmt.Sprintf("%dB", int64(s))
}
