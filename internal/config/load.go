package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Options control how a configuration is loaded. Zero values use the host.
type Options struct {
	// Env is the daemon environment as "KEY=value" entries; nil means os.Environ().
	Env []string
	// ReadFile reads the config and .env files; nil means os.ReadFile.
	ReadFile func(name string) ([]byte, error)
	// GOOS selects OS-specific rules; empty means runtime.GOOS.
	GOOS string
}

func (o Options) withDefaults() Options {
	if o.Env == nil {
		o.Env = os.Environ()
	}
	if o.ReadFile == nil {
		o.ReadFile = os.ReadFile
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	return o
}

// loadRaw reads, interpolates and decodes a config file and its .env files.
//
// Interpolation uses the daemon environment and the global .env files, the
// latter taking precedence. The root env_file field is decoded first, with
// the daemon environment only. A service's .env files only feed the
// environment of its processes.
func loadRaw(path string, opt Options) (*rawConfig, ErrorList) {
	opt = opt.withDefaults()
	d := newDecoder(path)
	cfg := &rawConfig{pos: d.pos}

	data, err := opt.ReadFile(path)
	if err != nil {
		d.errs.add(Pos{File: path}, "", "cannot read config: %v", unwrapPathError(err))
		return cfg, d.errs
	}
	body := d.document(data)
	if body == nil {
		return cfg, d.errs
	}

	base := filepath.Dir(path)
	daemonEnv := envFromList(opt.GOOS, opt.Env)

	d.expand = daemonEnv.lookup
	cfg.EnvFile = d.rootEnvFile(body)
	globalEnv := newEnvMap(opt.GOOS)
	globalEnv.merge(loadDotenvFiles(d, opt, base, "env_file", cfg.EnvFile))
	cfg.dotenv = globalEnv.toMap()

	d.expand = layered(globalEnv, daemonEnv)
	d.skipEnvFile = true
	d.root(body, cfg)

	for _, s := range cfg.Services {
		if s.EnvFile != nil {
			s.dotenv = loadDotenvFiles(d, opt, base, "services."+s.Name+".env_file", s.EnvFile)
		}
	}
	return cfg, d.errs
}

// loadDotenvFiles reads .env files in order; later files win.
// Errors are recorded on d, at the position of the path in the config.
func loadDotenvFiles(d *decoder, opt Options, base, field string, paths []string) map[string]string {
	env := newEnvMap(opt.GOOS)
	for i, p := range paths {
		pos, ok := d.pos[fmt.Sprintf("%s[%d]", field, i)]
		if !ok {
			pos = d.pos[field]
		}
		if strings.TrimSpace(p) == "" {
			d.errs.add(pos, field, "empty path")
			continue
		}
		full := resolvePath(base, p)
		data, err := opt.ReadFile(full)
		if err != nil {
			d.errs.add(pos, field, "cannot read %s: %v", full, unwrapPathError(err))
			continue
		}
		vars, errs := parseDotenv(full, data)
		d.errs = append(d.errs, errs...)
		env.merge(vars)
	}
	return env.toMap()
}

// resolvePath makes p relative to base unless it is absolute or rooted.
func resolvePath(base, p string) string {
	if filepath.IsAbs(p) || filepath.VolumeName(p) != "" || strings.HasPrefix(p, `\`) || strings.HasPrefix(p, "/") {
		return filepath.Clean(p)
	}
	return filepath.Join(base, p)
}

// unwrapPathError drops the path from a *fs.PathError, as callers print it.
func unwrapPathError(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
