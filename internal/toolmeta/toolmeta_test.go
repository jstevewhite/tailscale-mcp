package toolmeta

import (
	"strings"
	"testing"
)

func TestGroupForPath(t *testing.T) {
	tests := map[string]string{
		"/device/{deviceId}/routes":                 "devices",
		"/tailnet/{tailnet}/devices":                "devices",
		"/tailnet/{tailnet}/device-attributes":      "devices",
		"/device/{deviceId}/device-invites":         "invites",
		"/device-invites/{deviceInviteId}":          "invites",
		"/tailnet/{tailnet}/user-invites":           "invites",
		"/user-invites/{userInviteId}/resend":       "invites",
		"/tailnet/{tailnet}/dns/nameservers":        "dns",
		"/tailnet/{tailnet}/acl/validate":           "policy",
		"/tailnet/{tailnet}/keys/{keyId}":           "keys",
		"/users/{userId}/approve":                   "users",
		"/tailnet/{tailnet}/users":                  "users",
		"/webhooks/{endpointId}":                    "webhooks",
		"/tailnet/{tailnet}/webhooks":               "webhooks",
		"/tailnet/{tailnet}/services/{serviceName}": "services",
		"/tailnet/{tailnet}/logging/network":        "logs",
		"/tailnet/{tailnet}/aws-external-id":        "logs",
		"/posture/integrations/{id}":                "posture",
		"/tailnet/{tailnet}/posture/integrations":   "posture",
		"/tailnet/{tailnet}/oauth-apps/{appId}":     "oauth",
		"/tailnet/{tailnet}/contacts/{contactType}": "contacts",
		"/tailnet/{tailnet}/settings":               "settings",
		"/tailnet/{tailnet}/something-new":          "other",
	}
	for path, want := range tests {
		if got := GroupForPath(path); got != want {
			t.Errorf("GroupForPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestSelectorsAllows(t *testing.T) {
	reader := Meta{Name: "tailscale_get_dns_configuration", Group: "dns", ReadOnly: true}
	writer := Meta{Name: "tailscale_set_dns_configuration", Group: "dns", ReadOnly: false}
	device := Meta{Name: "list_all_devices", Group: "devices", ReadOnly: true}
	tests := []struct {
		sel  string
		meta Meta
		want bool
	}{
		{"*", writer, true},
		{"read:*", reader, true},
		{"read:*", writer, false},
		{"group:dns", writer, true},
		{"group:dns", device, false},
		{"group:dns:read", reader, true},
		{"group:dns:read", writer, false},
		{"group:nope", reader, false},
		{"tailscale_get_dns_configuration", reader, true},
		{"tailscale_get_dns_configuration", writer, false},
		{"", reader, false},
	}
	for _, tt := range tests {
		if got := (Selectors{tt.sel}).Allows(tt.meta); got != tt.want {
			t.Errorf("Selectors{%q}.Allows(%s) = %v, want %v", tt.sel, tt.meta.Name, got, tt.want)
		}
	}
	if !(Selectors{"group:devices", "group:dns:read"}).Allows(reader) {
		t.Error("any matching selector should allow")
	}
}

func TestCatalogAllIsSortedByGroupThenName(t *testing.T) {
	c := NewCatalog()
	c.Add(Meta{Name: "b", Group: "dns"})
	c.Add(Meta{Name: "a", Group: "dns"})
	c.Add(Meta{Name: "z", Group: "devices"})
	got := []string{}
	for _, m := range c.All() {
		got = append(got, m.Group+"/"+m.Name)
	}
	if strings.Join(got, ",") != "devices/z,dns/a,dns/b" {
		t.Fatalf("All() order = %v", got)
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("Get(a) should find the tool")
	}
}

func TestValidateSelectors(t *testing.T) {
	c := NewCatalog()
	c.Add(Meta{Name: "list_all_devices", Group: "devices", ReadOnly: true})
	if err := ValidateSelectors(Selectors{"*", "read:*", "group:dns", "group:dns:read", "list_all_devices"}, c); err != nil {
		t.Fatalf("valid selectors rejected: %v", err)
	}
	for _, bad := range []string{"group:nope", "group:dns:write", "no_such_tool", ""} {
		if err := ValidateSelectors(Selectors{bad}, c); err == nil {
			t.Errorf("selector %q should be rejected", bad)
		}
	}
}
