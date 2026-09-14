package main

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"tailscale.com/tailcfg"
)

func capMapWith(entries ...string) tailcfg.PeerCapMap {
	raw := make([]tailcfg.RawMessage, 0, len(entries))
	for _, e := range entries {
		raw = append(raw, tailcfg.RawMessage(e))
	}
	return tailcfg.PeerCapMap{mcpCapabilityName: raw}
}

func TestParseMCPCapabilitiesUnionsAllGrantEntries(t *testing.T) {
	logger = zap.NewNop()
	caps, err := parseMCPCapabilities(capMapWith(
		`{"tools":["list_all_devices"],"resources":["bootstrap://status"]}`,
		`{"tools":["get_device_info"],"resources":["tailscale://devices"]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if caps == nil {
		t.Fatal("expected capabilities, got nil")
	}
	for _, tool := range []string{"list_all_devices", "get_device_info"} {
		if !caps.AllowsTool(tool) {
			t.Errorf("tool %q should be allowed by the union of grants", tool)
		}
	}
	for _, res := range []string{"bootstrap://status", "tailscale://devices"} {
		if !caps.AllowsResource(res) {
			t.Errorf("resource %q should be allowed by the union of grants", res)
		}
	}
	if caps.AllowsTool("tailscale_delete_device") {
		t.Error("ungranted tool was allowed")
	}
}

func TestParseMCPCapabilitiesReturnsNilWithoutGrant(t *testing.T) {
	logger = zap.NewNop()
	caps, err := parseMCPCapabilities(tailcfg.PeerCapMap{"example.com/cap/other": {`{}`}})
	if err != nil {
		t.Fatal(err)
	}
	if caps != nil {
		t.Fatalf("expected nil capabilities, got %#v", caps)
	}
}

func TestResourceGrantMatching(t *testing.T) {
	tests := []struct {
		grant, uri string
		want       bool
	}{
		{"*", "tailscale://anything", true},
		{"tailscale://device", "tailscale://device", true},
		{"tailscale://device", "tailscale://devices", false},
		{"tailscale://device", "tailscale://device/n1/routes", false},
		{"tailscale://device/*", "tailscale://device/n1/routes", true},
		{"tailscale://device/*", "tailscale://device", false},
		{"", "tailscale://devices", false},
	}
	for _, tt := range tests {
		if got := resourceGrantMatches(tt.grant, tt.uri); got != tt.want {
			t.Errorf("resourceGrantMatches(%q, %q) = %v, want %v", tt.grant, tt.uri, got, tt.want)
		}
	}
}

func TestCheckToolAccessDeniesWithoutCapabilitiesInContext(t *testing.T) {
	logger = zap.NewNop()
	if err := checkToolAccess(context.Background(), "list_all_devices"); err == nil {
		t.Fatal("expected denial without capabilities in context")
	}
}

func TestCheckToolAccessUsesCapabilitiesFromContext(t *testing.T) {
	logger = zap.NewNop()
	ctx := withCapabilities(context.Background(), &MCPCapability{Tools: []string{"list_all_devices"}}, "alice@example.com")
	if err := checkToolAccess(ctx, "list_all_devices"); err != nil {
		t.Fatalf("expected access, got %v", err)
	}
	if err := checkToolAccess(ctx, "get_device_info"); err == nil {
		t.Fatal("expected denial for ungranted tool")
	}
}
