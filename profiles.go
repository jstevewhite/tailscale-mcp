package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"
	"gopkg.in/yaml.v3"
)

// ProfileConfig is the parsed --config file: named tool profiles selectable
// by URL path, plus which one /mcp serves.
type ProfileConfig struct {
	Default  string                        `yaml:"default"`
	Profiles map[string]toolmeta.Selectors `yaml:"profiles"`
}

var profileNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func loadProfileConfig(path string, catalog *toolmeta.Catalog) (*ProfileConfig, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg, err := parseProfileConfig(data, catalog)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

func parseProfileConfig(data []byte, catalog *toolmeta.Catalog) (*ProfileConfig, error) {
	var cfg ProfileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if len(cfg.Profiles) == 0 {
		return nil, errors.New("profiles must define at least one profile")
	}
	for name, selectors := range cfg.Profiles {
		if !profileNameRE.MatchString(name) {
			return nil, fmt.Errorf("profile %q: name must match %s", name, profileNameRE)
		}
		if len(selectors) == 0 {
			return nil, fmt.Errorf("profile %q: must list at least one selector", name)
		}
		if err := toolmeta.ValidateSelectors(selectors, catalog); err != nil {
			return nil, fmt.Errorf("profile %q: %w", name, err)
		}
	}
	if cfg.Default != "" {
		if _, ok := cfg.Profiles[cfg.Default]; !ok {
			return nil, fmt.Errorf("default profile %q is not defined", cfg.Default)
		}
	}
	return &cfg, nil
}

// Resolve maps a URL profile name to its selectors. "" is the default path.
// nil selectors with ok=true means "everything the grant allows".
func (c *ProfileConfig) Resolve(name string) (toolmeta.Selectors, bool) {
	if name == "" {
		if c == nil || c.Default == "" {
			return nil, true
		}
		return c.Profiles[c.Default], true
	}
	if c == nil {
		return nil, false
	}
	sel, ok := c.Profiles[name]
	return sel, ok
}

// Names returns the defined profile names, sorted, for logs.
func (c *ProfileConfig) Names() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
