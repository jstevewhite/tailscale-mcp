package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"
	"go.uber.org/zap"
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

func withProfile(ctx context.Context, sel toolmeta.Selectors) context.Context {
	return context.WithValue(ctx, ctxKeyProfile, sel)
}

// profileFromContext returns the request's profile selectors; nil means the
// profile does not restrict anything.
func profileFromContext(ctx context.Context) toolmeta.Selectors {
	sel, _ := ctx.Value(ctxKeyProfile).(toolmeta.Selectors)
	return sel
}

// profileFromPath extracts the profile name from /mcp or /mcp/<name>.
func profileFromPath(path string) (string, bool) {
	if path == mcpEndpointPath || path == mcpEndpointPath+"/" {
		return "", true
	}
	rest, ok := strings.CutPrefix(path, mcpEndpointPath+"/")
	if !ok {
		return "", false
	}
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}

// profileMiddleware resolves the URL's profile before authentication so an
// unknown profile is a plain 404.
func profileMiddleware(next http.Handler, cfg *ProfileConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := profileFromPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		sel, ok := cfg.Resolve(name)
		if !ok {
			logger.Warn("Unknown tool profile", zap.String("profile", name), zap.Strings("known", cfg.Names()))
			http.Error(w, "unknown tool profile", http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r.WithContext(withProfile(r.Context(), sel)))
	})
}
