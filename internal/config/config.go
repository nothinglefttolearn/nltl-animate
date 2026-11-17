package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nothinglefttolearn/nltl-animate/internal/preset"
)

// DefaultBase is the preset a custom preset inherits from when it has no
// `extends` key. Fields omitted from the YAML keep the base preset's values.
const DefaultBase = "industrial"

type Config struct {
	Presets []preset.Preset
}

// presetEntry is the on-disk shape of one preset. It exists so the strict
// decoding pass knows `extends` is a valid key alongside the preset fields.
type presetEntry struct {
	Extends       string `yaml:"extends"`
	preset.Preset `yaml:",inline"`
}

// Load reads a YAML config. Unknown keys are rejected, each preset is layered
// on top of the preset it extends, and every resulting preset is validated.
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
	// Pass 1: strict decode to catch typos (e.g. zoom_amplitude) and type errors.
	var entries struct {
		Presets []presetEntry `yaml:"presets"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&entries); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	// Pass 2: decode each preset onto a copy of its base. yaml.v3 leaves
	// fields that are absent from the node untouched, which gives inheritance.
	var nodes struct {
		Presets []yaml.Node `yaml:"presets"`
	}
	if err := yaml.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	cfg := &Config{}
	var errs []error
	for i, node := range nodes.Presets {
		e := entries.Presets[i]
		where := fmt.Sprintf("preset %d (line %d)", i+1, node.Line)
		if e.Name == "" {
			errs = append(errs, fmt.Errorf("%s: name is required", where))
			continue
		}
		if _, dup := cfg.GetPreset(e.Name); dup {
			errs = append(errs, fmt.Errorf("%s: duplicate preset name %q", where, e.Name))
			continue
		}

		baseName := e.Extends
		if baseName == "" {
			baseName = DefaultBase
		}
		base, ok := cfg.GetPreset(baseName) // earlier custom presets first
		if !ok {
			base, ok = preset.Get(baseName)
		}
		if !ok {
			errs = append(errs, fmt.Errorf("%s: %q extends unknown preset %q (built-ins: %s; custom presets must be defined earlier in the file)",
				where, e.Name, baseName, strings.Join(preset.Names(), ", ")))
			continue
		}

		p := base
		p.Description = ""
		if err := node.Decode(&p); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
			continue
		}
		if err := p.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		cfg.Presets = append(cfg.Presets, p)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

func (c *Config) GetPreset(name string) (preset.Preset, bool) {
	for _, p := range c.Presets {
		if p.Name == name {
			return p, true
		}
	}
	return preset.Preset{}, false
}
