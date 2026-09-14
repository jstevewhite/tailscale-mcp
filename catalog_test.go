package main

import (
	"testing"

	"github.com/jaxxstorm/tailscale-mcp/internal/curatedtools"
	"github.com/jaxxstorm/tailscale-mcp/internal/readapi"
	"github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"
	"go.uber.org/zap"
)

func registerEverything(t *testing.T) {
	t.Helper()
	logger = zap.NewNop()
	srv := newMCPServer()
	registerCoreMCP(srv, nil)
	readapi.RegisterTools(srv, readapi.Client{}, checkToolAccess, toolCatalog)
	curatedtools.RegisterAll(srv, curatedtools.Options{Client: readapi.Client{}, Check: checkToolAccess, LocalCLI: true, Catalog: toolCatalog})
}

func TestEveryRegisteredToolHasAGroup(t *testing.T) {
	registerEverything(t)
	all := toolCatalog.All()
	if len(all) < 120 {
		t.Fatalf("catalog has %d tools, expected the full set", len(all))
	}
	for _, m := range all {
		if m.Group == toolmeta.GroupOther || m.Group == "" {
			t.Errorf("tool %q has no group", m.Name)
		}
	}
	for name, want := range map[string]string{
		"list_all_devices":                 "devices",
		"tailscale_status":                 "settings",
		"tailscale_ping":                   "local",
		"tailscale_get_acl":                "policy",
		"tailscale_list_network_flow_logs": "logs",
		"tailscale_create_key_curated":     "keys",
	} {
		m, ok := toolCatalog.Get(name)
		if !ok || m.Group != want {
			t.Errorf("%s: group = %q (found %v), want %q", name, m.Group, ok, want)
		}
	}
	if m, _ := toolCatalog.Get("tailscale_delete_device"); m.ReadOnly {
		t.Error("delete tool recorded as read-only")
	}
	if m, _ := toolCatalog.Get("tailscale_get_dns_configuration"); !m.ReadOnly {
		t.Error("GET tool not recorded as read-only")
	}
}
