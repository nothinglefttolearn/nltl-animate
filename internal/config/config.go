package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

// Config holds the custom grades and motions from a config file.
type Config struct {
	Grades  []grade.Grade
	Motions []motion.Motion
}

// errPresets explains the format change to anyone with an old config.
var errPresets = errors.New(`"presets" is no longer supported: a preset is now split into a grade ` +
	`(contrast, saturation, softness, vignette, grain) under "grades:" and a motion ` +
	`(breath, breath_beats, shake, pulse, pulse_beats) under "motions:" — see nltl.yaml.example`)

// The on-disk shape of each entry. They exist so the strict decoding pass
// knows `extends` is a valid key alongside the dials.
type gradeEntry struct {
	Extends     string `yaml:"extends"`
	grade.Grade `yaml:",inline"`
}

type motionEntry struct {
	Extends       string `yaml:"extends"`
	motion.Motion `yaml:",inline"`
}

// Load reads a YAML config. Unknown keys are rejected, each grade or motion
// is layered on top of the one it extends, and every result is validated.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	cfg, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func parse(data []byte) (*Config, error) {
	// Pass 1: strict decode to catch typos (e.g. grian) and type errors.
	var strict struct {
		Grades  []gradeEntry  `yaml:"grades"`
		Motions []motionEntry `yaml:"motions"`
		Presets yaml.Node     `yaml:"presets"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&strict); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing: %w", err)
	}
	if !strict.Presets.IsZero() {
		return nil, errPresets
	}

	// Pass 2: decode each entry onto a copy of its base. yaml.v3 leaves
	// fields that are absent from the node untouched, which gives inheritance.
	var nodes struct {
		Grades  []yaml.Node `yaml:"grades"`
		Motions []yaml.Node `yaml:"motions"`
	}
	if err := yaml.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	grades, gradeErrs := layer(nodes.Grades, kind[grade.Grade]{
		noun: "grade", defaultBase: grade.Default,
		builtin: grade.Get, builtinNames: grade.Names,
		name:     func(g grade.Grade) string { return g.Name },
		reset:    func(g *grade.Grade) { g.Description = "" },
		validate: grade.Grade.Validate,
	})
	motions, motionErrs := layer(nodes.Motions, kind[motion.Motion]{
		noun: "motion", defaultBase: motion.Default,
		builtin: motion.Get, builtinNames: motion.Names,
		name:     func(m motion.Motion) string { return m.Name },
		reset:    func(m *motion.Motion) { m.Description = "" },
		validate: motion.Motion.Validate,
	})
	if errs := slices.Concat(gradeErrs, motionErrs); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &Config{Grades: grades, Motions: motions}, nil
}

// kind describes how to layer one list (grades or motions).
type kind[T any] struct {
	noun         string
	defaultBase  string
	builtin      func(string) (T, bool)
	builtinNames func() []string
	name         func(T) string
	reset        func(*T) // clear what is not inherited
	validate     func(T) error
}

// layer decodes each node onto the item it extends: an earlier custom one of
// the same kind, else a built-in. Every problem is returned, not just the
// first.
func layer[T any](nodes []yaml.Node, k kind[T]) ([]T, []error) {
	var (
		out  []T
		errs []error
	)
	find := func(name string) (T, bool) {
		for _, it := range out {
			if k.name(it) == name {
				return it, true
			}
		}
		var zero T
		return zero, false
	}
	for i, node := range nodes {
		where := fmt.Sprintf("%s %d (line %d)", k.noun, i+1, node.Line)
		var head struct {
			Name    string `yaml:"name"`
			Extends string `yaml:"extends"`
		}
		if err := node.Decode(&head); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
			continue
		}
		if head.Name == "" {
			errs = append(errs, fmt.Errorf("%s: name is required", where))
			continue
		}
		if _, dup := find(head.Name); dup {
			errs = append(errs, fmt.Errorf("%s: duplicate %s name %q", where, k.noun, head.Name))
			continue
		}

		baseName := head.Extends
		if baseName == "" {
			baseName = k.defaultBase
		}
		base, ok := find(baseName)
		if !ok {
			base, ok = k.builtin(baseName)
		}
		if !ok {
			errs = append(errs, fmt.Errorf("%s: %q extends unknown %s %q (built-ins: %s; custom ones must be defined earlier in the file)",
				where, head.Name, k.noun, baseName, strings.Join(k.builtinNames(), ", ")))
			continue
		}

		it := base
		k.reset(&it)
		if err := node.Decode(&it); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
			continue
		}
		if err := k.validate(it); err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, it)
	}
	return out, errs
}

// GetGrade returns the custom grade with that name.
func (c *Config) GetGrade(name string) (grade.Grade, bool) {
	for _, g := range c.Grades {
		if g.Name == name {
			return g, true
		}
	}
	return grade.Grade{}, false
}

// GetMotion returns the custom motion with that name.
func (c *Config) GetMotion(name string) (motion.Motion, bool) {
	for _, m := range c.Motions {
		if m.Name == name {
			return m, true
		}
	}
	return motion.Motion{}, false
}
