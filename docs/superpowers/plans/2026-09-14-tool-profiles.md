# Tool Profiles and Groups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an operator define named tool profiles in a YAML file and let each MCP client pick one by URL path, so one server advertises different tool sets to different clients, always bounded by the caller's grant.

**Architecture:** A new `internal/toolmeta` package owns tool metadata (name, group, read-only), path-to-group derivation, and selector matching. Every registration path records into a catalog. Grants and profiles both become selector lists evaluated against catalog metadata. A profile middleware resolves `/mcp/<name>` into selectors on the request context, and the existing mcp-go tool filter intersects profile and grant.

**Tech Stack:** Go 1.26, mark3labs/mcp-go v0.56, gopkg.in/yaml.v3 (already a dependency), kong for flags.

**Spec:** `docs/superpowers/specs/2026-09-14-tool-profiles-design.md`

## Global Constraints

- Selector grammar: `*`, `read:*`, `group:<name>`, `group:<name>:read`, exact tool name. Same grammar in grants, `--local-grants`, and profiles.
- Group names: `devices dns policy keys users invites webhooks services logs posture oauth contacts settings local`; fallthrough is `other` and no registered tool may be `other`.
- Profile names match `^[a-z0-9][a-z0-9_-]*$`.
- No config file means today's behavior; `/mcp/<anything>` is then a 404.
- Unknown profile is a 404 before authentication.
- Config is read once at startup; every validation failure is fatal.
- Run `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...` before every commit. Commit messages end with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

---

## File Structure

- Create `internal/toolmeta/toolmeta.go`: `Meta`, `Catalog`, `GroupForPath`, `Selectors`, `ValidateSelectors`.
- Create `internal/toolmeta/toolmeta_test.go`.
- Modify `internal/readapi/register.go`: `RegisterTools` gains a `*toolmeta.Catalog` parameter and records each tool.
- Modify `internal/readapi/network_flow_logs.go`: `registerNetworkFlowLogTool` records too.
- Modify `internal/readapi/register_test.go`: pass a catalog.
- Modify `internal/curatedtools/curatedtools.go`: `Options.Catalog`, `toolDef.Group`, record in `registerTool`.
- Modify `internal/curatedtools/curatedtools_test.go`: pass a catalog.
- Modify `grants.go`: `MCPCapability.AllowsTool(meta toolmeta.Meta)` via `Selectors`; filter intersects profile.
- Modify `listen.go`: `newMCPServer` creates a fresh catalog.
- Create `profiles.go`: config loading, validation, profile resolution, middleware, `ctxKeyProfile`.
- Create `profiles_test.go`.
- Modify `main.go`: flags `--config`, `--profile`, `--list-groups`; drop `required` on tailnet and credential and validate manually; core tools record into catalog; mount handlers at `/mcp` and `/mcp/`.
- Modify `docs/usage.md`, `README.md`.

---

### Task 1: toolmeta package

**Files:**
- Create: `internal/toolmeta/toolmeta.go`
- Test: `internal/toolmeta/toolmeta_test.go`

**Interfaces:**
- Produces:
  - `type Meta struct { Name, Group string; ReadOnly bool }`
  - `type Catalog struct` with `func NewCatalog() *Catalog`, `func (c *Catalog) Add(m Meta)`, `func (c *Catalog) Get(name string) (Meta, bool)`, `func (c *Catalog) All() []Meta` (sorted by group then name)
  - `func GroupForPath(path string) string`
  - `type Selectors []string` with `func (s Selectors) Allows(m Meta) bool`
  - `func ValidateSelectors(s Selectors, c *Catalog) error`
  - `var Groups = []string{...}` and `const GroupOther = "other"`

- [ ] **Step 1: Write the failing tests**

```go
package toolmeta

import (
	"strings"
	"testing"
)

func TestGroupForPath(t *testing.T) {
	tests := map[string]string{
		"/device/{deviceId}/routes":                        "devices",
		"/tailnet/{tailnet}/devices":                       "devices",
		"/tailnet/{tailnet}/device-attributes":             "devices",
		"/device/{deviceId}/device-invites":                "invites",
		"/device-invites/{deviceInviteId}":                 "invites",
		"/tailnet/{tailnet}/user-invites":                  "invites",
		"/user-invites/{userInviteId}/resend":              "invites",
		"/tailnet/{tailnet}/dns/nameservers":               "dns",
		"/tailnet/{tailnet}/acl/validate":                  "policy",
		"/tailnet/{tailnet}/keys/{keyId}":                  "keys",
		"/users/{userId}/approve":                          "users",
		"/tailnet/{tailnet}/users":                         "users",
		"/webhooks/{endpointId}":                           "webhooks",
		"/tailnet/{tailnet}/webhooks":                      "webhooks",
		"/tailnet/{tailnet}/services/{serviceName}":        "services",
		"/tailnet/{tailnet}/logging/network":               "logs",
		"/tailnet/{tailnet}/aws-external-id":               "logs",
		"/posture/integrations/{id}":                       "posture",
		"/tailnet/{tailnet}/posture/integrations":          "posture",
		"/tailnet/{tailnet}/oauth-apps/{appId}":            "oauth",
		"/tailnet/{tailnet}/contacts/{contactType}":        "contacts",
		"/tailnet/{tailnet}/settings":                      "settings",
		"/tailnet/{tailnet}/something-new":                 "other",
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
		if got := Selectors{tt.sel}.Allows(tt.meta); got != tt.want {
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
	all := c.All()
	got := []string{}
	for _, m := range all {
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/toolmeta/`
Expected: build failure, undefined `GroupForPath`, `Selectors`, `NewCatalog`, `ValidateSelectors`.

- [ ] **Step 3: Implement**

```go
// Package toolmeta describes registered MCP tools (name, group, read-only)
// and evaluates the selector grammar shared by grants and profiles.
package toolmeta

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Meta is what selectors are matched against.
type Meta struct {
	Name     string
	Group    string
	ReadOnly bool
}

const (
	GroupDevices  = "devices"
	GroupDNS      = "dns"
	GroupPolicy   = "policy"
	GroupKeys     = "keys"
	GroupUsers    = "users"
	GroupInvites  = "invites"
	GroupWebhooks = "webhooks"
	GroupServices = "services"
	GroupLogs     = "logs"
	GroupPosture  = "posture"
	GroupOAuth    = "oauth"
	GroupContacts = "contacts"
	GroupSettings = "settings"
	GroupLocal    = "local"
	GroupOther    = "other"
)

// Groups lists every group a selector may name.
var Groups = []string{GroupDevices, GroupDNS, GroupPolicy, GroupKeys, GroupUsers, GroupInvites, GroupWebhooks, GroupServices, GroupLogs, GroupPosture, GroupOAuth, GroupContacts, GroupSettings, GroupLocal}

func knownGroup(name string) bool {
	for _, g := range Groups {
		if g == name {
			return true
		}
	}
	return false
}

// GroupForPath derives a tool's group from the Tailscale API path it calls.
func GroupForPath(path string) string {
	p := strings.TrimPrefix(path, "/tailnet/{tailnet}")
	p = strings.TrimPrefix(p, "/")
	if strings.Contains(p, "device-invites") {
		return GroupInvites
	}
	first := p
	if i := strings.Index(p, "/"); i >= 0 {
		first = p[:i]
	}
	switch first {
	case "device", "devices", "device-attributes":
		return GroupDevices
	case "dns":
		return GroupDNS
	case "acl":
		return GroupPolicy
	case "keys":
		return GroupKeys
	case "users":
		return GroupUsers
	case "user-invites":
		return GroupInvites
	case "webhooks":
		return GroupWebhooks
	case "services":
		return GroupServices
	case "logging", "aws-external-id":
		return GroupLogs
	case "posture":
		return GroupPosture
	case "oauth-apps":
		return GroupOAuth
	case "contacts":
		return GroupContacts
	case "settings":
		return GroupSettings
	default:
		return GroupOther
	}
}

// Catalog records every registered tool.
type Catalog struct {
	mu    sync.RWMutex
	tools map[string]Meta
}

func NewCatalog() *Catalog {
	return &Catalog{tools: map[string]Meta{}}
}

func (c *Catalog) Add(m Meta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools[m.Name] = m
}

func (c *Catalog) Get(name string) (Meta, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.tools[name]
	return m, ok
}

// All returns every tool sorted by group then name.
func (c *Catalog) All() []Meta {
	c.mu.RLock()
	defer c.mu.RUnlock()
	all := make([]Meta, 0, len(c.tools))
	for _, m := range c.tools {
		all = append(all, m)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Group != all[j].Group {
			return all[i].Group < all[j].Group
		}
		return all[i].Name < all[j].Name
	})
	return all
}

// Selectors is a list of tool selectors; any match allows the tool.
type Selectors []string

// Allows reports whether any selector matches the tool.
func (s Selectors) Allows(m Meta) bool {
	for _, sel := range s {
		if matches(strings.TrimSpace(sel), m) {
			return true
		}
	}
	return false
}

func matches(sel string, m Meta) bool {
	switch {
	case sel == "":
		return false
	case sel == "*":
		return true
	case sel == "read:*":
		return m.ReadOnly
	case strings.HasPrefix(sel, "group:"):
		rest := strings.TrimPrefix(sel, "group:")
		if strings.HasSuffix(rest, ":read") {
			return m.ReadOnly && m.Group == strings.TrimSuffix(rest, ":read")
		}
		return m.Group == rest
	default:
		return sel == m.Name
	}
}

// ValidateSelectors rejects selectors that can never match: unknown groups,
// unknown suffixes, and tool names not in the catalog.
func ValidateSelectors(s Selectors, c *Catalog) error {
	for _, sel := range s {
		sel = strings.TrimSpace(sel)
		switch {
		case sel == "":
			return fmt.Errorf("empty selector")
		case sel == "*" || sel == "read:*":
		case strings.HasPrefix(sel, "group:"):
			name := strings.TrimSuffix(strings.TrimPrefix(sel, "group:"), ":read")
			if strings.Contains(name, ":") {
				return fmt.Errorf("selector %q: only a :read suffix is supported", sel)
			}
			if !knownGroup(name) {
				return fmt.Errorf("selector %q: unknown group %q (known: %s)", sel, name, strings.Join(Groups, ", "))
			}
		default:
			if _, ok := c.Get(sel); !ok {
				return fmt.Errorf("selector %q: no registered tool with that name", sel)
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/toolmeta/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/toolmeta
git commit -m "feat: toolmeta package with groups, catalog, and selector matching"
```

---

### Task 2: Record every registered tool in a catalog

**Files:**
- Modify: `internal/readapi/register.go` (`RegisterTools`, add catalog param)
- Modify: `internal/readapi/network_flow_logs.go` (`registerNetworkFlowLogTool`)
- Modify: `internal/readapi/register_test.go`
- Modify: `internal/curatedtools/curatedtools.go` (`Options`, `toolDef`, `registerTool`, `statusTools`, `localCLITools`)
- Modify: `internal/curatedtools/curatedtools_test.go`
- Modify: `listen.go` (`newMCPServer` creates catalog), `main.go` (`registerCoreMCP` records)
- Test: `catalog_test.go` (package main)

**Interfaces:**
- Consumes: `toolmeta.Catalog`, `toolmeta.Meta`, `toolmeta.GroupForPath`.
- Produces: `readapi.RegisterTools(mcpServer, client, check, catalog *toolmeta.Catalog)`; `curatedtools.Options.Catalog *toolmeta.Catalog`; package-level `toolCatalog *toolmeta.Catalog` in main, reset by `newMCPServer()`; `registerCoreMCP(mcpServer, client)` records its two tools into `toolCatalog`.

- [ ] **Step 1: Write the failing test in package main**

```go
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
		"list_all_devices":                "devices",
		"tailscale_status":                "settings",
		"tailscale_ping":                  "local",
		"tailscale_get_acl":               "policy",
		"tailscale_list_network_flow_logs": "logs",
		"tailscale_create_key_curated":    "keys",
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./ -run TestEveryRegisteredToolHasAGroup`
Expected: build failure on `toolCatalog`, `Options.Catalog`, and the four-argument `RegisterTools`.

- [ ] **Step 3: Implement**

In `internal/readapi/register.go`:

```go
import "github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"

func RegisterTools(mcpServer *server.MCPServer, client Client, check AccessChecker, catalog *toolmeta.Catalog) {
	var networkFlowLogs Endpoint
	for _, endpoint := range ToolEndpoints() {
		endpoint := endpoint
		if endpoint.OperationID == "listNetworkFlowLogs" {
			networkFlowLogs = endpoint
			continue
		}
		// ... existing option building unchanged ...
		mcpServer.AddTool(mcp.NewTool(endpoint.ToolName, options...), /* existing handler */)
		record(catalog, endpoint)
	}
	if networkFlowLogs.OperationID != "" {
		registerNetworkFlowLogTool(mcpServer, client, check, networkFlowLogs)
		record(catalog, networkFlowLogs)
	}
}

func record(catalog *toolmeta.Catalog, endpoint Endpoint) {
	if catalog == nil {
		return
	}
	catalog.Add(toolmeta.Meta{Name: endpoint.ToolName, Group: toolmeta.GroupForPath(endpoint.Path), ReadOnly: endpoint.ToolHints().ReadOnly})
}
```

Update `internal/readapi/register_test.go` call: `RegisterTools(mcpServer, Client{}, func(context.Context, string) error { return nil }, nil)`.

In `internal/curatedtools/curatedtools.go`:

```go
type Options struct {
	Client   readapi.Client
	Check    readapi.AccessChecker
	LocalCLI bool
	Catalog  *toolmeta.Catalog
}

type toolDef struct {
	// ... existing fields ...
	Group string // set when there is no Endpoint to derive it from
}

func registerTool(s *server.MCPServer, opts Options, def toolDef) {
	// ... existing body ...
	if opts.Catalog != nil {
		group := def.Group
		if group == "" {
			group = toolmeta.GroupForPath(def.Endpoint.Path)
		}
		opts.Catalog.Add(toolmeta.Meta{Name: def.Name, Group: group, ReadOnly: def.ReadOnly})
	}
}
```

Set `Group: toolmeta.GroupSettings` on `tailscale_status` in `statusTools`; `Group: toolmeta.GroupPolicy` on the four ACL tools in `aclTools` (they use `DoRaw` with inline endpoints, so `def.Endpoint.Path` is empty); `Group: toolmeta.GroupLocal` on all four in `localCLITools`; `Group: toolmeta.GroupDevices` on `tailscale_set_devices_authorized` (handler-only, no Endpoint).

In `listen.go`:

```go
var toolCatalog = toolmeta.NewCatalog()

func newMCPServer() *server.MCPServer {
	toolCatalog = toolmeta.NewCatalog()
	srv := server.NewMCPServer(mcpServerName, buildVersion,
		server.WithRecovery(),
		server.WithResourceRecovery(),
		server.WithToolFilter(grantToolFilter),
	)
	toolIsReadOnly = func(name string) bool {
		tool := srv.GetTool(name)
		return tool != nil && isReadOnlyTool(tool.Tool)
	}
	return srv
}
```

In `main.go` `registerCoreMCP`, after each `AddTool`:

```go
toolCatalog.Add(toolmeta.Meta{Name: "get_device_info", Group: toolmeta.GroupDevices, ReadOnly: true})
toolCatalog.Add(toolmeta.Meta{Name: "list_all_devices", Group: toolmeta.GroupDevices, ReadOnly: true})
```

In `main()` pass `toolCatalog` to `readapi.RegisterTools` and `curatedtools.Options{... Catalog: toolCatalog}`.

- [ ] **Step 4: Run to verify pass**

Run: `go test -count=1 ./...`
Expected: all PASS, including the new group test.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: record every registered tool's group and read-only flag in a catalog"
```

---

### Task 3: `--list-groups` and flags that work without credentials

**Files:**
- Modify: `main.go` (CLI struct, `main()` startup ordering)
- Test: `catalog_test.go`

**Interfaces:**
- Produces: `func formatToolTable(all []toolmeta.Meta) string` in main; `--list-groups` flag; `--version` and `--list-groups` no longer require tailnet or credential.

- [ ] **Step 1: Write the failing test**

```go
func TestFormatToolTableListsGroupReadOnlyAndName(t *testing.T) {
	out := formatToolTable([]toolmeta.Meta{
		{Name: "list_all_devices", Group: "devices", ReadOnly: true},
		{Name: "tailscale_delete_device", Group: "devices", ReadOnly: false},
	})
	want := "GROUP     READ  TOOL\ndevices   yes   list_all_devices\ndevices   no    tailscale_delete_device\n"
	if out != want {
		t.Fatalf("table =\n%s\nwant\n%s", out, want)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./ -run TestFormatToolTable`
Expected: undefined `formatToolTable`.

- [ ] **Step 3: Implement**

Add to `catalog.go` (new file, package main):

```go
package main

import (
	"fmt"
	"strings"

	"github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"
)

func formatToolTable(all []toolmeta.Meta) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-9s %-5s %s\n", "GROUP", "READ", "TOOL")
	for _, m := range all {
		ro := "no"
		if m.ReadOnly {
			ro = "yes"
		}
		fmt.Fprintf(&b, "%-9s %-5s %s\n", m.Group, ro, m.Name)
	}
	return b.String()
}
```

In `main.go` CLI struct: remove `required:""` from `Tailnet` and `Credential`; add

```go
ListGroups bool `name:"list-groups" help:"Print every tool with its group and read-only flag, then exit"`
```

In `main()`, right after `cli.Port = resolveListenPort(...)` is fine but before credential parsing, add:

```go
	if cli.ListGroups {
		srv := newMCPServer()
		registerCoreMCP(srv, nil)
		readapi.RegisterTools(srv, readapi.Client{}, checkToolAccess, toolCatalog)
		curatedtools.RegisterAll(srv, curatedtools.Options{Client: readapi.Client{}, Check: checkToolAccess, LocalCLI: true, Catalog: toolCatalog})
		fmt.Print(formatToolTable(toolCatalog.All()))
		return
	}
	if strings.TrimSpace(cli.Tailnet) == "" {
		logger.Fatal("TAILSCALE_TAILNET is required")
	}
```

(`ParseTailscaleCredentialWithClientID` already fatals on an empty credential.)

- [ ] **Step 4: Run to verify pass**

Run: `go test -count=1 ./ && go build -o bin/ts-mcp . && ./bin/ts-mcp --list-groups | head -5 && ./bin/ts-mcp --version`
Expected: PASS; table prints starting with `GROUP`; version prints without env vars set.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: --list-groups, and let --version run without credentials"
```

---

### Task 4: Grants use selectors

**Files:**
- Modify: `grants.go` (`AllowsTool`, `grantToolFilter`, `checkToolAccess`)
- Modify: `grants_test.go`, `localgrants_test.go`, `toolfilter_test.go` (call sites)
- Modify: `localgrants.go` (validate selectors? no: local grants may be set before tools register, so only syntax is checked there)

**Interfaces:**
- Produces: `func (c *MCPCapability) AllowsTool(m toolmeta.Meta) bool`; `readOnlyWildcard` constant removed (grammar lives in toolmeta); `toolIsReadOnly` removed, replaced by catalog lookup `toolMeta(name string) toolmeta.Meta`.

- [ ] **Step 1: Write the failing test** (append to `toolfilter_test.go`)

```go
func TestGroupSelectorsWorkInGrants(t *testing.T) {
	logger = zap.NewNop()
	srv := newMCPServer()
	registerCoreMCP(srv, nil)
	readapi.RegisterTools(srv, readapi.Client{}, checkToolAccess, toolCatalog)

	ctx := withCapabilities(context.Background(), &MCPCapability{Tools: []string{"group:dns:read"}}, "alice")
	if err := checkToolAccess(ctx, "tailscale_get_dns_configuration"); err != nil {
		t.Fatalf("dns read tool should be allowed: %v", err)
	}
	if err := checkToolAccess(ctx, "tailscale_set_dns_configuration"); err == nil {
		t.Fatal("dns write tool should be denied under group:dns:read")
	}
	if err := checkToolAccess(ctx, "list_all_devices"); err == nil {
		t.Fatal("devices tool should be denied under group:dns:read")
	}
}
```

Also change existing call sites: `caps.AllowsTool(name, ro)` becomes `caps.AllowsTool(toolmeta.Meta{Name: name, ReadOnly: ro})`, and `readOnlyWildcard` becomes the literal `"read:*"`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./ -run TestGroupSelectorsWorkInGrants`
Expected: FAIL, `tailscale_get_dns_configuration` denied because `group:` is not understood.

- [ ] **Step 3: Implement**

In `grants.go`, replace `AllowsTool`, `toolIsReadOnly`, `grantToolFilter`, `checkToolAccess` with:

```go
// AllowsTool reports whether the grant permits the tool described by m.
func (c *MCPCapability) AllowsTool(m toolmeta.Meta) bool {
	if c == nil {
		return false
	}
	return toolmeta.Selectors(c.Tools).Allows(m)
}

// toolMeta looks a registered tool up in the catalog. An unregistered name
// yields zero metadata, which no selector but "*" or the exact name matches.
func toolMeta(name string) toolmeta.Meta {
	if m, ok := toolCatalog.Get(name); ok {
		return m
	}
	return toolmeta.Meta{Name: name}
}

func grantToolFilter(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
	caps, _, _ := capabilitiesFromContext(ctx)
	if caps == nil {
		return nil
	}
	allowed := make([]mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		m := toolMeta(tool.Name)
		m.ReadOnly = isReadOnlyTool(tool) // annotations are authoritative
		if caps.AllowsTool(m) {
			allowed = append(allowed, tool)
		}
	}
	return allowed
}

func checkToolAccess(ctx context.Context, toolName string) error {
	allows := func(c *MCPCapability, name string) bool { return c.AllowsTool(toolMeta(name)) }
	return checkAccess(ctx, toolName, "tool", allows, func(c *MCPCapability) []string { return c.Tools })
}
```

Remove `readOnlyWildcard` and `toolIsReadOnly`, and the assignment to `toolIsReadOnly` in `newMCPServer`. Keep `isReadOnlyTool`.

Note: `TestCheckToolAccessHonorsReadOnlyWildcardFromRegisteredAnnotations` adds tools straight to the server without the catalog. Update it to also call `toolCatalog.Add(toolmeta.Meta{Name: "reader", ReadOnly: true})` and `toolCatalog.Add(toolmeta.Meta{Name: "writer"})` after registering, since the catalog is now the source for the call-time check.

- [ ] **Step 4: Run to verify pass**

Run: `go test -count=1 ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: group selectors in grants; access checks read the tool catalog"
```

---

### Task 5: Config file loading and validation

**Files:**
- Create: `profiles.go`
- Test: `profiles_test.go`

**Interfaces:**
- Produces:
  - `type ProfileConfig struct { Default string; Profiles map[string]toolmeta.Selectors }`
  - `func loadProfileConfig(path string, catalog *toolmeta.Catalog) (*ProfileConfig, error)` (empty path returns `nil, nil`)
  - `func parseProfileConfig(data []byte, catalog *toolmeta.Catalog) (*ProfileConfig, error)`
  - `func (c *ProfileConfig) Resolve(name string) (toolmeta.Selectors, bool)`; on a nil receiver, `""` resolves to `nil, true` (everything) and any other name to `nil, false`.

- [ ] **Step 1: Write the failing tests**

```go
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
		"invalid yaml":          "profiles: [",
		"missing default":       "default: nope\nprofiles:\n  dns: [\"group:dns\"]\n",
		"empty profile":         "profiles:\n  dns: []\n",
		"unknown group":         "profiles:\n  dns: [\"group:nope\"]\n",
		"unknown tool":          "profiles:\n  dns: [\"no_such_tool\"]\n",
		"bad profile name":      "profiles:\n  \"Bad Name\": [\"*\"]\n",
		"no profiles":           "default: x\n",
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./ -run 'ProfileConfig'`
Expected: undefined `parseProfileConfig`, `loadProfileConfig`, `ProfileConfig`.

- [ ] **Step 3: Implement** (`profiles.go`)

```go
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
	Default  string                       `yaml:"default"`
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
```

- [ ] **Step 4: Run to verify pass**

Run: `go test -count=1 ./ -run 'ProfileConfig'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add profiles.go profiles_test.go
git commit -m "feat: load and validate the tool profile config file"
```

---

### Task 6: Profile routing, context, and filter intersection

**Files:**
- Modify: `profiles.go` (middleware, context), `grants.go` (`grantToolFilter` intersects), `main.go` (flags `--config`, `--profile`; mount at `/mcp` and `/mcp/`; stdio profile), `localgrants.go` (`stdioContextFunc` takes selectors)
- Test: `profiles_test.go`, `toolfilter_test.go`

**Interfaces:**
- Produces:
  - `ctxKeyProfile` in the `ctxKey` enum; `func withProfile(ctx, sel toolmeta.Selectors) context.Context`; `func profileFromContext(ctx) toolmeta.Selectors` (nil means unrestricted)
  - `func profileFromPath(path string) (name string, ok bool)`: `/mcp` and `/mcp/` give `"", true`; `/mcp/dns` gives `"dns", true`; `/mcp/a/b` and `/other` give `"", false`
  - `func profileMiddleware(next http.Handler, cfg *ProfileConfig) http.Handler`
  - `func stdioContextFunc(caps *MCPCapability, profile toolmeta.Selectors) server.StdioContextFunc`

- [ ] **Step 1: Write the failing tests** (append to `profiles_test.go`)

```go
func TestProfileFromPath(t *testing.T) {
	tests := []struct {
		path string
		name string
		ok   bool
	}{
		{"/mcp", "", true},
		{"/mcp/", "", true},
		{"/mcp/dns", "dns", true},
		{"/mcp/dns/", "dns", true},
		{"/mcp/a/b", "", false},
		{"/other", "", false},
	}
	for _, tt := range tests {
		name, ok := profileFromPath(tt.path)
		if name != tt.name || ok != tt.ok {
			t.Errorf("profileFromPath(%q) = %q, %v; want %q, %v", tt.path, name, ok, tt.name, tt.ok)
		}
	}
}

func TestProfileMiddlewareResolvesAndRejects(t *testing.T) {
	logger = zap.NewNop()
	cfg, err := parseProfileConfig([]byte("default: ro\nprofiles:\n  ro: [\"read:*\"]\n  dns: [\"group:dns\"]\n"), testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	var seen toolmeta.Selectors
	handler := profileMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = profileFromContext(r.Context())
	}), cfg)

	for path, want := range map[string]string{"/mcp": "read:*", "/mcp/dns": "group:dns"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://h"+path, nil))
		if rec.Code != http.StatusOK || len(seen) != 1 || seen[0] != want {
			t.Errorf("%s: status %d, selectors %v; want 200 and [%s]", path, rec.Code, seen, want)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://h/mcp/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown profile: status %d, want 404", rec.Code)
	}

	// No config: named profiles are 404, default path is unrestricted.
	handler = profileMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = profileFromContext(r.Context())
	}), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://h/mcp/dns", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no config, named profile: status %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://h/mcp", nil))
	if rec.Code != http.StatusOK || seen != nil {
		t.Fatalf("no config, default path: status %d selectors %v; want 200 and nil", rec.Code, seen)
	}
}
```

Append to `toolfilter_test.go` (uses the existing `listTools` helper, which registers `reader`, `writer`, `named` on a fresh server; extend the helper to also `toolCatalog.Add` each of the three with `Group: "dns"` for `reader` and `writer`, and `Group: "devices"` for `named`):

```go
func TestToolsListIsIntersectionOfProfileAndGrant(t *testing.T) {
	logger = zap.NewNop()
	grant := &MCPCapability{Tools: []string{"*"}}

	ctx := withProfile(withCapabilities(context.Background(), grant, "alice"), toolmeta.Selectors{"group:dns:read"})
	if got := listTools(t, ctx); len(got) != 1 || got[0] != "reader" {
		t.Fatalf("profile group:dns:read with grant * = %v, want [reader]", got)
	}

	narrow := &MCPCapability{Tools: []string{"named"}}
	ctx = withProfile(withCapabilities(context.Background(), narrow, "alice"), toolmeta.Selectors{"*"})
	if got := listTools(t, ctx); len(got) != 1 || got[0] != "named" {
		t.Fatalf("profile * with grant named = %v, want [named]", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./ -run 'ProfileFromPath|ProfileMiddleware|Intersection'`
Expected: undefined `profileFromPath`, `profileMiddleware`, `withProfile`, `profileFromContext`.

- [ ] **Step 3: Implement**

In `grants.go` add `ctxKeyProfile` to the `ctxKey` const block, and change `grantToolFilter`:

```go
func grantToolFilter(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
	caps, _, _ := capabilitiesFromContext(ctx)
	if caps == nil {
		return nil
	}
	profile := profileFromContext(ctx)
	allowed := make([]mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		m := toolMeta(tool.Name)
		m.ReadOnly = isReadOnlyTool(tool)
		if profile != nil && !profile.Allows(m) {
			continue
		}
		if caps.AllowsTool(m) {
			allowed = append(allowed, tool)
		}
	}
	return allowed
}
```

Append to `profiles.go`:

```go
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
```

Add imports `context`, `net/http`, `go.uber.org/zap` to `profiles.go`.

In `localgrants.go`:

```go
func stdioContextFunc(caps *MCPCapability, profile toolmeta.Selectors) server.StdioContextFunc {
	return func(ctx context.Context) context.Context {
		return withProfile(withCapabilities(ctx, caps, localGrantUser), profile)
	}
}
```

Update `TestStdioContextFuncInjectsLocalGrants` to pass `nil` as the second argument.

In `main.go`:

CLI struct additions:

```go
Config  string `env:"TS_MCP_CONFIG" help:"YAML file defining named tool profiles selectable at /mcp/<name>"`
Profile string `env:"TS_MCP_PROFILE" help:"Tool profile to serve in --stdio mode (requires --config)"`
```

After all registration (`curatedtools.RegisterAll`), before the stdio branch:

```go
	profileConfig, err := loadProfileConfig(cli.Config, toolCatalog)
	if err != nil {
		logger.Fatal("Invalid tool profile config", zap.Error(err))
	}
	if profileConfig != nil {
		logger.Info("Loaded tool profiles", zap.Strings("profiles", profileConfig.Names()), zap.String("default", profileConfig.Default))
	}
	stdioProfile, ok := profileConfig.Resolve(cli.Profile)
	if !ok {
		logger.Fatal("Unknown --profile", zap.String("profile", cli.Profile), zap.Strings("known", profileConfig.Names()))
	}
```

Stdio call: `server.WithStdioContextFunc(stdioContextFunc(localGrants, stdioProfile))`.

Handler wiring, replacing the two `mux.Handle(mcpEndpointPath, ...)` lines:

```go
	tailnetHandler := loggingMiddleware(profileMiddleware(allowOriginMiddleware(grantMiddleware(streamable, tsServer)), profileConfig))
	tailnetMux := http.NewServeMux()
	tailnetMux.Handle(mcpEndpointPath, tailnetHandler)
	tailnetMux.Handle(mcpEndpointPath+"/", tailnetHandler)
```

and inside the `if localGrants != nil` block:

```go
		localHandler := loggingMiddleware(profileMiddleware(allowOriginMiddleware(localGrantMiddleware(streamable, localGrants)), profileConfig))
		localMux := http.NewServeMux()
		localMux.Handle(mcpEndpointPath, localHandler)
		localMux.Handle(mcpEndpointPath+"/", localHandler)
```

Delete `streamableHTTPHandler` from `main.go` since it is no longer used (check with `go vet`).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -l . ; go vet ./... && go test -count=1 ./...`
Expected: no gofmt output, PASS everywhere.

- [ ] **Step 5: Manual smoke**

Create `/tmp/claude-1000/-mnt-asus-code/0f5a7980-f2cf-43cd-8134-f2bdd8a4d2ec/scratchpad/profiles.yaml`:

```yaml
default: readonly
profiles:
  readonly: ["read:*"]
  dns: ["group:dns"]
```

Run stdio with the config and a local grant, listing tools under the `dns` profile:

```bash
./bin/ts-mcp --stdio --local-grants '{"tools":["*"],"resources":["*"]}' --config <scratchpad>/profiles.yaml --profile dns <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
EOF
```

Expected: the `tools/list` result contains only tools whose names contain `dns` (11 generated plus curated `tailscale_get_dns_configuration_curated` and `tailscale_set_dns_configuration_curated`). Requires `TAILSCALE_OAUTH_TOKEN` and `TAILSCALE_TAILNET` in the environment; the operator runs this step.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: tool profiles selectable by URL path and --profile for stdio"
```

---

### Task 7: Documentation

**Files:**
- Modify: `docs/usage.md`, `README.md`

- [ ] **Step 1: README**

Add to Features: `* **Tool Profiles**: Named tool sets in a YAML config, selected per client by URL path such as \`/mcp/dns\``. Under "The server exposes MCP at", add: `* \`https://<hostname>.yourtailnet.ts.net/mcp/<profile>\` for a named tool profile when \`--config\` is set`.

- [ ] **Step 2: usage.md**

Add `--config`, `--profile`, and `--list-groups` to the command line options list, matching the existing bullet style.

Add a section after "OAuth Grants And Access Control":

````markdown
## Tool Profiles

A hundred-plus tool schemas is a lot of context for a model. Profiles let the operator publish a named subset of tools, and let each client pick one by the URL it connects to. Grants still apply: a client sees the intersection of its profile and its grant.

Define profiles in a YAML file passed with `--config` or `TS_MCP_CONFIG`:

```yaml
default: readonly          # served at /mcp; omit to serve everything the grant allows
profiles:
  readonly: ["read:*"]
  dns:      ["group:dns"]
  devices:  ["group:devices:read", "tailscale_device_set_tags"]
  ops:      ["read:*", "group:policy"]
```

Clients connect to `/mcp` for the default profile or `/mcp/<name>` for a named one. An unknown name is a 404. In `--stdio` mode pass `--profile <name>`.

Selectors, usable in profiles, ACL grants, and `--local-grants`:

| Selector | Matches |
|---|---|
| `*` | every tool |
| `read:*` | every read-only tool |
| `group:<name>` | every tool in the group |
| `group:<name>:read` | the group's read-only tools |
| `<tool name>` | one tool |

Groups: `devices`, `dns`, `policy`, `keys`, `users`, `invites`, `webhooks`, `services`, `logs`, `posture`, `oauth`, `contacts`, `settings`, `local`. Run `./ts-mcp --list-groups` to print every tool with its group and read-only flag.

The config is validated at startup: unknown groups, unknown tool names, empty profiles, and a `default` that names no profile all stop the server. It is read once; restart to pick up changes.
````

Update the Tool grants bullet list to add `* \`group:<name>\` and \`group:<name>:read\`: Allow a whole group, see Tool Profiles for the group list`.

- [ ] **Step 3: Verify and commit**

Run: `go test -count=1 ./...`
Expected: PASS (docs only, but keep the habit).

```bash
git add README.md docs/usage.md
git commit -m "docs: tool profiles, groups, selectors, and --list-groups"
git push fork fixes
```

---

## Self-review

- Spec coverage: selectors (T1, T4), groups and `other` assertion (T1, T2), catalog (T2), `--list-groups` (T3), config and every validation rule (T5), routing incl. trailing slash, unknown, no-config (T6), stdio `--profile` (T6), visibility intersection and unchanged call check (T6), docs (T7). Resource filtering and hot reload are explicitly out of scope.
- Types: `toolmeta.Selectors` used consistently; `ProfileConfig.Resolve` returns `(toolmeta.Selectors, bool)` everywhere; `stdioContextFunc(caps, profile)` matches the T6 call site; `RegisterTools` four-argument form used in T2, T3, T4.
- No placeholders remain.
