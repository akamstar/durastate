package config

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// maxAliasDepth guards against pathological alias chains.
const maxAliasDepth = 64

// decoder walks the YAML AST by hand so that every error is collected with
// its exact position, instead of stopping at the first one.
type decoder struct {
	file    string
	errs    ErrorList
	anchors map[string]ast.Node
	pos     map[string]Pos

	// expand interpolates string scalars; nil disables interpolation.
	expand lookupFunc
	// skipEnvFile ignores the root env_file, already read by a first pass.
	skipEnvFile bool
	// secretVar reports whether an interpolated variable holds a secret;
	// such values are recorded in secrets, by field path.
	secretVar func(name string) bool
	secrets   map[string][]string
}

// fieldSet maps a mapping key to the function that decodes its value.
type fieldSet map[string]func(n ast.Node, path string)

// entry is one key/value pair of a mapping, after merge keys are expanded.
type entry struct {
	key    string
	keyPos Pos
	val    ast.Node
}

func newDecoder(file string) *decoder {
	return &decoder{file: file, anchors: map[string]ast.Node{}, pos: map[string]Pos{}, secrets: map[string][]string{}}
}

// parse decodes a config file into its raw form, without interpolation.
// Errors are not sorted.
func parse(file string, data []byte) (*rawConfig, ErrorList) {
	d := newDecoder(file)
	cfg := &rawConfig{pos: d.pos}
	if body := d.document(data); body != nil {
		d.root(body, cfg)
	}
	return cfg, d.errs
}

// document parses data and returns the body of its single YAML document,
// or nil after recording an error.
func (d *decoder) document(data []byte) ast.Node {
	f, err := parser.ParseBytes(data, 0)
	if err != nil {
		d.syntaxError(err)
		return nil
	}
	var body ast.Node
	for _, doc := range f.Docs {
		if doc == nil || doc.Body == nil {
			continue
		}
		if body != nil {
			d.errs.add(d.nodePos(doc.Body), "", "multiple YAML documents are not supported")
			return nil
		}
		body = doc.Body
	}
	if body == nil {
		d.errs.add(Pos{File: d.file}, "", "empty configuration (at least one service is required)")
		return nil
	}
	d.collectAnchors(body, 0)
	return body
}

// rootEnvFile decodes only the root env_file field. Other fields, and
// structural errors, are left to root.
func (d *decoder) rootEnvFile(n ast.Node) []string {
	r, ok := n.(*ast.MappingNode)
	if !ok {
		return nil
	}
	for _, mv := range r.Values {
		if k, ok := mv.Key.(ast.ScalarNode); ok && !isNull(mv.Key) && scalarText(k) == "env_file" {
			d.pos["env_file"] = d.nodePos(mv.Key)
			return d.strList(mv.Value, "env_file")
		}
	}
	return nil
}

func (d *decoder) syntaxError(err error) {
	var se *yaml.SyntaxError
	if errors.As(err, &se) && se.Token != nil {
		d.errs.add(d.tokenPos(se), "", "%s", se.Message)
		return
	}
	d.errs.add(Pos{File: d.file}, "", "invalid YAML: %v", err)
}

func (d *decoder) tokenPos(se *yaml.SyntaxError) Pos {
	return Pos{File: d.file, Line: se.Token.Position.Line, Col: se.Token.Position.Column}
}

// extensionPrefix marks root keys ignored by dstate, as in Docker Compose.
// They are a place to define YAML anchors ("x-worker: &worker ...").
const extensionPrefix = "x-"

func (d *decoder) root(n ast.Node, cfg *rawConfig) {
	es, ok := d.entries(n, "")
	if !ok {
		return
	}
	es = slices.DeleteFunc(es, func(e entry) bool { return strings.HasPrefix(e.key, extensionPrefix) })
	d.decodeEntries(es, "", fieldSet{
		"version": func(n ast.Node, p string) {
			if v, ok := d.integer(n, p); ok {
				if v != 1 {
					d.errs.add(d.nodePos(n), p, "unsupported version %d (this dstate supports version 1)", v)
				}
				cfg.Version = &v
			}
		},
		"defaults": func(n ast.Node, p string) {
			cfg.Defaults = &rawPolicy{}
			fs := d.policyFields(cfg.Defaults)
			for _, k := range serviceOnlyFields {
				fs[k] = func(n ast.Node, p string) {
					d.errs.add(d.pos[p], p, "%q is not allowed in defaults, set it on each service", lastSegment(p))
				}
			}
			d.fields(n, p, fs)
		},
		"env_file": func(n ast.Node, p string) {
			if !d.skipEnvFile {
				cfg.EnvFile = d.strList(n, p)
			}
		},
		"alerts": func(n ast.Node, p string) { cfg.Alerts = d.alerts(n, p) },
		"services": func(n ast.Node, p string) {
			es, ok := d.entries(n, p)
			if !ok {
				return
			}
			cfg.Services = []*rawService{}
			for _, e := range es {
				sp := p + "." + e.key
				d.pos[sp] = e.keyPos
				cfg.Services = append(cfg.Services, d.service(e.key, e.val, sp))
			}
		},
	}, "")
}

func (d *decoder) service(name string, n ast.Node, path string) *rawService {
	s := &rawService{Name: name}
	fs := d.policyFields(&s.rawPolicy)
	fs["command"] = func(n ast.Node, p string) { s.Command = d.command(n, p) }
	fs["cwd"] = func(n ast.Node, p string) { s.Cwd = d.str(n, p) }
	fs["env"] = func(n ast.Node, p string) { s.Env = d.strMap(n, p) }
	fs["env_file"] = func(n ast.Node, p string) { s.EnvFile = d.strList(n, p) }
	fs["replicas"] = func(n ast.Node, p string) { s.Replicas = d.intPtr(n, p) }
	fs["autostart"] = func(n ast.Node, p string) { s.Autostart = d.boolPtr(n, p) }
	fs["wait_for"] = func(n ast.Node, p string) {
		w := &rawWaitFor{}
		d.fields(n, p, fieldSet{
			"targets":  func(n ast.Node, p string) { w.Targets = d.strList(n, p) },
			"timeout":  func(n ast.Node, p string) { w.Timeout = d.duration(n, p) },
			"interval": func(n ast.Node, p string) { w.Interval = d.duration(n, p) },
		})
		s.WaitFor = w
	}
	d.fields(n, path, fs)
	return s
}

// policyFields returns the decoders shared by "defaults" and services.
func (d *decoder) policyFields(pl *rawPolicy) fieldSet {
	return fieldSet{
		"shell":              func(n ast.Node, p string) { pl.Shell = d.boolPtr(n, p) },
		"inherit_env":        func(n ast.Node, p string) { pl.InheritEnv = d.boolPtr(n, p) },
		"restart":            func(n ast.Node, p string) { pl.Restart = d.enum(n, p, restartPolicies) },
		"success_exit_codes": func(n ast.Node, p string) { pl.SuccessExitCodes = d.intList(n, p) },
		"max_restarts":       func(n ast.Node, p string) { pl.MaxRestarts = d.intPtr(n, p) },
		"restart_window":     func(n ast.Node, p string) { pl.RestartWindow = d.duration(n, p) },
		"stable_after":       func(n ast.Node, p string) { pl.StableAfter = d.duration(n, p) },
		"start_delay":        func(n ast.Node, p string) { pl.StartDelay = d.duration(n, p) },
		"stop_signal":        func(n ast.Node, p string) { pl.StopSignal = d.enum(n, p, stopSignals) },
		"stop_command":       func(n ast.Node, p string) { pl.StopCommand = d.command(n, p) },
		"stop_timeout":       func(n ast.Node, p string) { pl.StopTimeout = d.duration(n, p) },
		"backoff": func(n ast.Node, p string) {
			b := &rawBackoff{}
			d.fields(n, p, fieldSet{
				"initial":    func(n ast.Node, p string) { b.Initial = d.duration(n, p) },
				"max":        func(n ast.Node, p string) { b.Max = d.duration(n, p) },
				"multiplier": func(n ast.Node, p string) { b.Multiplier = d.float(n, p) },
			})
			pl.Backoff = b
		},
		"crash_loop_retry": func(n ast.Node, p string) {
			c := &rawCrashLoopRetry{}
			d.fields(n, p, fieldSet{
				"initial": func(n ast.Node, p string) { c.Initial = d.duration(n, p) },
				"max":     func(n ast.Node, p string) { c.Max = d.duration(n, p) },
				"enabled": func(n ast.Node, p string) { c.Enabled = d.boolPtr(n, p) },
			})
			pl.CrashLoopRetry = c
		},
		"logs": func(n ast.Node, p string) {
			l := &rawLogs{}
			d.fields(n, p, fieldSet{
				"max_size":     func(n ast.Node, p string) { l.MaxSize = d.size(n, p) },
				"max_files":    func(n ast.Node, p string) { l.MaxFiles = d.intPtr(n, p) },
				"max_age":      func(n ast.Node, p string) { l.MaxAge = d.duration(n, p) },
				"compress":     func(n ast.Node, p string) { l.Compress = d.boolPtr(n, p) },
				"timestamps":   func(n ast.Node, p string) { l.Timestamps = d.boolPtr(n, p) },
				"merge_stderr": func(n ast.Node, p string) { l.MergeStderr = d.boolPtr(n, p) },
			})
			pl.Logs = l
		},
		"alerts": func(n ast.Node, p string) {
			a := &rawServiceAlerts{}
			d.fields(n, p, fieldSet{
				"enabled": func(n ast.Node, p string) { a.Enabled = d.boolPtr(n, p) },
				"rules":   func(n ast.Node, p string) { a.Rules = d.rules(n, p) },
			})
			pl.Alerts = a
		},
	}
}

func (d *decoder) alerts(n ast.Node, path string) *rawAlerts {
	a := &rawAlerts{}
	d.fields(n, path, fieldSet{
		"channels": func(n ast.Node, p string) {
			es, ok := d.entries(n, p)
			if !ok {
				return
			}
			a.Channels = []*rawChannel{}
			for _, e := range es {
				cp := p + "." + e.key
				d.pos[cp] = e.keyPos
				if c := d.channel(e, cp); c != nil {
					a.Channels = append(a.Channels, c)
				}
			}
		},
		"rules": func(n ast.Node, p string) { a.Rules = d.rules(n, p) },
		"throttle": func(n ast.Node, p string) {
			t := &rawThrottle{}
			d.fields(n, p, fieldSet{
				"min_interval": func(n ast.Node, p string) { t.MinInterval = d.duration(n, p) },
				"group_window": func(n ast.Node, p string) { t.GroupWindow = d.duration(n, p) },
			})
			a.Throttle = t
		},
	})
	return a
}

// channel reads "type" first, then accepts only the fields of that type.
func (d *decoder) channel(e entry, path string) *rawChannel {
	es, ok := d.entries(e.val, path)
	if !ok {
		return nil
	}
	c := &rawChannel{Name: e.key}
	i := slices.IndexFunc(es, func(x entry) bool { return x.key == "type" })
	if i < 0 {
		d.errs.add(e.keyPos, path, "missing required field \"type\" (expected %s)", strings.Join(channelTypes, ", "))
		return nil
	}
	tp := path + ".type"
	d.pos[tp] = es[i].keyPos
	t := d.enum(es[i].val, tp, channelTypes)
	if t == nil {
		return nil
	}
	c.Type = *t

	fs := fieldSet{"type": func(ast.Node, string) {}}
	str := func(dst **string) func(ast.Node, string) {
		return func(n ast.Node, p string) { *dst = d.str(n, p) }
	}
	switch c.Type {
	case "telegram":
		fs["bot_token"] = str(&c.BotToken)
		fs["chat_id"] = str(&c.ChatID)
	case "slack", "discord":
		fs["webhook_url"] = str(&c.WebhookURL)
	case "webhook":
		fs["url"] = str(&c.URL)
		fs["method"] = str(&c.Method)
		fs["body"] = str(&c.Body)
		fs["headers"] = func(n ast.Node, p string) { c.Headers = d.strMap(n, p) }
	}
	d.decodeEntries(es, path, fs, " for a "+c.Type+" channel")
	return c
}

func (d *decoder) rules(n ast.Node, path string) []*rawRule {
	rules := []*rawRule{}
	d.seq(n, path, func(n ast.Node, p string) {
		r := &rawRule{}
		d.fields(n, p, fieldSet{
			"events": func(n ast.Node, p string) {
				r.Events = d.strList(n, p)
				for i, ev := range r.Events {
					if !slices.Contains(alertEvents, ev) {
						ep := fmt.Sprintf("%s[%d]", p, i)
						d.errs.add(d.keyOrNodePos(ep, n), ep, "unknown event %q (expected %s)", ev, strings.Join(alertEvents, ", "))
					}
				}
			},
			"channels": func(n ast.Node, p string) { r.Channels = d.strList(n, p) },
		})
		rules = append(rules, r)
	})
	return rules
}

// --- structure helpers ---

// fields decodes a mapping with a fixed set of keys.
func (d *decoder) fields(n ast.Node, path string, fs fieldSet) {
	if es, ok := d.entries(n, path); ok {
		d.decodeEntries(es, path, fs, "")
	}
}

func (d *decoder) decodeEntries(es []entry, path string, fs fieldSet, context string) {
	for _, e := range es {
		p := join(path, e.key)
		d.pos[p] = e.keyPos
		fn, ok := fs[e.key]
		if !ok {
			d.errs.add(e.keyPos, path, "unknown field %q%s%s", e.key, context, suggest(e.key, fs))
			continue
		}
		fn(e.val, p)
	}
}

// entries returns the pairs of a mapping, expanding merge keys ("<<").
// Explicit keys take precedence over merged ones.
func (d *decoder) entries(n ast.Node, path string) ([]entry, bool) {
	return d.entriesDepth(n, path, 0)
}

func (d *decoder) entriesDepth(n ast.Node, path string, depth int) ([]entry, bool) {
	orig := n
	n = d.resolve(n, path)
	if n == nil {
		return nil, false
	}
	var values []*ast.MappingValueNode
	switch v := n.(type) {
	case *ast.MappingNode:
		values = v.Values
	case *ast.MappingValueNode:
		values = []*ast.MappingValueNode{v}
	default:
		d.errs.add(d.nodePos(orig), path, "expected a mapping, got %s", describe(n))
		return nil, false
	}

	var explicit, merged []entry
	for _, mv := range values {
		if _, isMerge := mv.Key.(*ast.MergeKeyNode); isMerge {
			if depth >= maxAliasDepth {
				d.errs.add(d.nodePos(mv.Key), path, "merge keys nested too deeply")
				continue
			}
			merged = append(merged, d.mergeSources(mv.Value, path, depth)...)
			continue
		}
		key, ok := d.key(mv.Key, path)
		if !ok {
			continue
		}
		explicit = append(explicit, entry{key: key, keyPos: d.nodePos(mv.Key), val: mv.Value})
	}

	seen := map[string]bool{}
	for _, e := range explicit {
		seen[e.key] = true
	}
	out := explicit
	for _, e := range merged {
		if !seen[e.key] {
			seen[e.key] = true
			out = append(out, e)
		}
	}
	return out, true
}

// mergeSources expands "<<: *a" or "<<: [*a, *b]"; earlier sources win.
func (d *decoder) mergeSources(n ast.Node, path string, depth int) []entry {
	r := d.resolve(n, path)
	if r == nil {
		return nil
	}
	if seq, ok := r.(*ast.SequenceNode); ok {
		var out []entry
		for _, item := range seq.Values {
			es, _ := d.entriesDepth(item, path, depth+1)
			out = append(out, es...)
		}
		return out
	}
	es, _ := d.entriesDepth(n, path, depth+1)
	return es
}

// key returns the text of a mapping key. The parser already rejects complex keys.
func (d *decoder) key(n ast.Node, path string) (string, bool) {
	s, ok := n.(ast.ScalarNode)
	if !ok || isNull(n) {
		d.errs.add(d.nodePos(n), path, "mapping keys must be plain values")
		return "", false
	}
	return scalarText(s), true
}

func (d *decoder) seq(n ast.Node, path string, fn func(n ast.Node, path string)) {
	orig := n
	n = d.resolve(n, path)
	if n == nil {
		return
	}
	s, ok := n.(*ast.SequenceNode)
	if !ok {
		d.errs.add(d.nodePos(orig), path, "expected a list, got %s", describe(n))
		return
	}
	for i, item := range s.Values {
		p := fmt.Sprintf("%s[%d]", path, i)
		d.pos[p] = d.itemPos(item)
		fn(item, p)
	}
}

// resolve follows anchors, aliases and tags. A null node is reported and
// returns nil, as no field of the format accepts null.
func (d *decoder) resolve(n ast.Node, path string) ast.Node {
	orig := n
	for i := 0; i < maxAliasDepth; i++ {
		switch v := n.(type) {
		case *ast.AnchorNode:
			n = v.Value
		case *ast.TagNode:
			n = v.Value
		case *ast.AliasNode:
			name := v.Value.GetToken().Value
			target, ok := d.anchors[name]
			if !ok {
				d.errs.add(d.nodePos(v), path, "unknown alias %q", name)
				return nil
			}
			n = target
		case nil, *ast.NullNode:
			d.errs.add(d.keyOrNodePos(path, orig), path, "missing value")
			return nil
		default:
			return n
		}
	}
	d.errs.add(d.nodePos(orig), path, "aliases nested too deeply")
	return nil
}

func (d *decoder) collectAnchors(n ast.Node, depth int) {
	if n == nil || depth > 1000 {
		return
	}
	switch v := n.(type) {
	case *ast.AnchorNode:
		d.anchors[v.Name.GetToken().Value] = v.Value
		d.collectAnchors(v.Value, depth+1)
	case *ast.TagNode:
		d.collectAnchors(v.Value, depth+1)
	case *ast.MappingNode:
		for _, mv := range v.Values {
			d.collectAnchors(mv, depth+1)
		}
	case *ast.MappingValueNode:
		d.collectAnchors(v.Key, depth+1)
		d.collectAnchors(v.Value, depth+1)
	case *ast.MappingKeyNode:
		d.collectAnchors(v.Value, depth+1)
	case *ast.SequenceNode:
		for _, item := range v.Values {
			d.collectAnchors(item, depth+1)
		}
	}
}

// --- scalar helpers ---

// scalar returns the text of a scalar value, whatever its YAML type, so that
// "chat_id: 0123" keeps its leading zero.
func (d *decoder) scalar(n ast.Node, path string) (string, bool) {
	orig := n
	n = d.resolve(n, path)
	if n == nil {
		return "", false
	}
	s, ok := n.(ast.ScalarNode)
	if !ok {
		d.errs.add(d.nodePos(orig), path, "expected a single value, got %s", describe(n))
		return "", false
	}
	text := scalarText(s)
	if d.expand != nil && isString(s) {
		lookup := func(name string) (string, bool) {
			v, ok := d.expand(name)
			if ok && v != "" && d.secretVar != nil && d.secretVar(name) {
				d.secrets[path] = append(d.secrets[path], v)
			}
			return v, ok
		}
		v, err := interpolate(text, lookup)
		if err != nil {
			d.errs.add(d.nodePos(orig), path, "%v", err)
			return "", false
		}
		text = v
	}
	return text, true
}

// isString reports whether n is a string scalar, the only kind interpolated.
func isString(n ast.Node) bool {
	switch n.(type) {
	case *ast.StringNode, *ast.LiteralNode:
		return true
	}
	return false
}

func scalarText(n ast.ScalarNode) string {
	switch v := n.(type) {
	case *ast.StringNode:
		return v.Value
	case *ast.LiteralNode:
		return v.Value.Value
	default:
		return n.GetToken().Value
	}
}

func (d *decoder) str(n ast.Node, path string) *string {
	if s, ok := d.scalar(n, path); ok {
		return &s
	}
	return nil
}

func (d *decoder) boolPtr(n ast.Node, path string) *bool {
	s, ok := d.scalar(n, path)
	if !ok {
		return nil
	}
	switch strings.ToLower(s) {
	case "true":
		b := true
		return &b
	case "false":
		b := false
		return &b
	}
	d.errs.add(d.nodePos(n), path, "expected true or false, got %q", s)
	return nil
}

func (d *decoder) integer(n ast.Node, path string) (int, bool) {
	s, ok := d.scalar(n, path)
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		d.errs.add(d.nodePos(n), path, "expected an integer, got %q", s)
		return 0, false
	}
	return v, true
}

func (d *decoder) intPtr(n ast.Node, path string) *int {
	if v, ok := d.integer(n, path); ok {
		return &v
	}
	return nil
}

func (d *decoder) float(n ast.Node, path string) *float64 {
	s, ok := d.scalar(n, path)
	if !ok {
		return nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		d.errs.add(d.nodePos(n), path, "expected a number, got %q", s)
		return nil
	}
	return &v
}

func (d *decoder) duration(n ast.Node, path string) *Duration {
	s, ok := d.scalar(n, path)
	if !ok {
		return nil
	}
	v, err := ParseDuration(s)
	if err != nil {
		d.errs.add(d.nodePos(n), path, "%v", err)
		return nil
	}
	return &v
}

func (d *decoder) size(n ast.Node, path string) *Size {
	s, ok := d.scalar(n, path)
	if !ok {
		return nil
	}
	v, err := ParseSize(s)
	if err != nil {
		d.errs.add(d.nodePos(n), path, "%v", err)
		return nil
	}
	return &v
}

func (d *decoder) enum(n ast.Node, path string, allowed []string) *string {
	s, ok := d.scalar(n, path)
	if !ok {
		return nil
	}
	if !slices.Contains(allowed, s) {
		d.errs.add(d.nodePos(n), path, "invalid value %q (expected %s)", s, strings.Join(allowed, ", "))
		return nil
	}
	return &s
}

// strList accepts a single value or a list of values.
func (d *decoder) strList(n ast.Node, path string) []string {
	r := d.resolve(n, path)
	if r == nil {
		return nil
	}
	if _, ok := r.(*ast.SequenceNode); !ok {
		if s, ok := d.scalar(n, path); ok {
			return []string{s}
		}
		return nil
	}
	out := []string{}
	d.seq(n, path, func(n ast.Node, p string) {
		if s, ok := d.scalar(n, p); ok {
			out = append(out, s)
		}
	})
	return out
}

// intList accepts a single integer or a list of integers.
func (d *decoder) intList(n ast.Node, path string) []int {
	r := d.resolve(n, path)
	if r == nil {
		return nil
	}
	if _, ok := r.(*ast.SequenceNode); !ok {
		if v, ok := d.integer(n, path); ok {
			return []int{v}
		}
		return nil
	}
	out := []int{}
	d.seq(n, path, func(n ast.Node, p string) {
		if v, ok := d.integer(n, p); ok {
			out = append(out, v)
		}
	})
	return out
}

func (d *decoder) strMap(n ast.Node, path string) map[string]string {
	es, ok := d.entries(n, path)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(es))
	for _, e := range es {
		p := join(path, e.key)
		d.pos[p] = e.keyPos
		if isNull(e.val) {
			d.errs.add(e.keyPos, p, "missing value (use \"\" for an empty string)")
			continue
		}
		if s, ok := d.scalar(e.val, p); ok {
			out[e.key] = s
		}
	}
	return out
}

func (d *decoder) command(n ast.Node, path string) *rawCommand {
	r := d.resolve(n, path)
	if r == nil {
		return nil
	}
	if _, ok := r.(*ast.SequenceNode); ok {
		args := d.strList(n, path)
		if len(args) == 0 {
			d.errs.add(d.nodePos(n), path, "empty command")
			return nil
		}
		return &rawCommand{Args: args, IsList: true}
	}
	s, ok := d.scalar(n, path)
	if !ok {
		return nil
	}
	if strings.TrimSpace(s) == "" {
		d.errs.add(d.nodePos(n), path, "empty command")
		return nil
	}
	return &rawCommand{Line: s}
}

// --- misc ---

func (d *decoder) nodePos(n ast.Node) Pos {
	if n == nil || n.GetToken() == nil {
		return Pos{File: d.file}
	}
	t := n.GetToken()
	return Pos{File: d.file, Line: t.Position.Line, Col: t.Position.Column}
}

// itemPos is the position of a list item. A mapping token is its first ":",
// so a mapping item points at its first key instead.
func (d *decoder) itemPos(n ast.Node) Pos {
	switch v := n.(type) {
	case *ast.MappingNode:
		if len(v.Values) > 0 {
			return d.nodePos(v.Values[0].Key)
		}
	case *ast.MappingValueNode:
		return d.nodePos(v.Key)
	}
	return d.nodePos(n)
}

// keyOrNodePos points at the key of path when known, as a missing value has
// no useful position of its own.
func (d *decoder) keyOrNodePos(path string, n ast.Node) Pos {
	if p, ok := d.pos[path]; ok {
		return p
	}
	return d.nodePos(n)
}

func isNull(n ast.Node) bool {
	_, ok := n.(*ast.NullNode)
	return n == nil || ok
}

func describe(n ast.Node) string {
	switch v := n.(type) {
	case *ast.MappingNode, *ast.MappingValueNode:
		return "a mapping"
	case *ast.SequenceNode:
		return "a list"
	case ast.ScalarNode:
		return fmt.Sprintf("%q", scalarText(v))
	default:
		return n.Type().String()
	}
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func lastSegment(path string) string {
	return path[strings.LastIndex(path, ".")+1:]
}

// suggest returns a "did you mean" hint for a misspelled key.
func suggest(key string, fs fieldSet) string {
	keys := make([]string, 0, len(fs))
	for k := range fs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	best, bestDist := "", 3
	if len(key) <= 3 {
		bestDist = 2
	}
	for _, k := range keys {
		if dist := editDistance(key, k); dist < bestDist {
			best, bestDist = k, dist
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf(" (did you mean %q?)", best)
}

// editDistance is the optimal string alignment distance: insertions,
// deletions, substitutions and adjacent transpositions all cost 1.
func editDistance(a, b string) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}
