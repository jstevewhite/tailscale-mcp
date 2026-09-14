package main

import (
	"strings"
	"testing"

	"github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"
)

func testCatalog() *toolmeta.Catalog {
	c := toolmeta.NewCatalog()
	c.Add(toolmeta.Meta{Name: "list_all_devices", Group: "devices", ReadOnly: true})
	c.Add(toolmeta.Meta{Name: "tailscale_set_dns_configuration", Group: "dns"})
	return c
}

func TestParseProfileConfigValid(t *testing.T) {
	cfg, err := parseProfileConfig([]byte(`
default: readonly
profiles:
  readonly: ["read:*"]
  dns: ["group:dns", "list_all_devices"]
`), testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	sel, ok := cfg.Resolve("")
	if !ok || len(sel) != 1 || sel[0] != "read:*" {
		t.Fatalf("default profile = %v, %v", sel, ok)
	}
	sel, ok = cfg.Resolve("dns")
	if !ok || len(sel) != 2 {
		t.Fatalf("dns profile = %v, %v", sel, ok)
	}
	if _, ok := cfg.Resolve("nope"); ok {
		t.Fatal("unknown profile should not resolve")
	}
}

func TestParseProfileConfigNoDefaultServesEverything(t *testing.T) {
	cfg, err := parseProfileConfig([]byte("profiles:\n  dns: [\"group:dns\"]\n"), testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	sel, ok := cfg.Resolve("")
	if !ok || sel != nil {
		t.Fatalf("no default should resolve to everything (nil selectors), got %v, %v", sel, ok)
	}
}

func TestParseProfileConfigRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"invalid yaml":     "profiles: [",
		"missing default":  "default: nope\nprofiles:\n  dns: [\"group:dns\"]\n",
		"empty profile":    "profiles:\n  dns: []\n",
		"unknown group":    "profiles:\n  dns: [\"group:nope\"]\n",
		"unknown tool":     "profiles:\n  dns: [\"no_such_tool\"]\n",
		"bad profile name": "profiles:\n  \"Bad Name\": [\"*\"]\n",
		"no profiles":      "default: x\n",
	}
	for name, data := range cases {
		if _, err := parseProfileConfig([]byte(data), testCatalog()); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestNilProfileConfigResolve(t *testing.T) {
	var cfg *ProfileConfig
	if sel, ok := cfg.Resolve(""); !ok || sel != nil {
		t.Fatal("nil config must serve everything at the default path")
	}
	if _, ok := cfg.Resolve("dns"); ok {
		t.Fatal("nil config must not resolve named profiles")
	}
}

func TestLoadProfileConfigEmptyPath(t *testing.T) {
	cfg, err := loadProfileConfig("", testCatalog())
	if err != nil || cfg != nil {
		t.Fatalf("empty path should yield nil, nil; got %v, %v", cfg, err)
	}
	if _, err := loadProfileConfig("/nonexistent/ts-mcp.yaml", testCatalog()); err == nil || !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("missing file should error with the path, got %v", err)
	}
}
