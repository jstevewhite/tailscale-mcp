package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
	"tailscale.com/tailcfg"
)

// mcpCapabilityName is the Tailscale grant application capability that
// carries MCP tool and resource allowances.
const mcpCapabilityName tailcfg.PeerCapability = "jaxxstorm.com/cap/mcp"

type ctxKey int

const (
	ctxKeyCapabilities ctxKey = iota
	ctxKeyUser
)

// withCapabilities returns a context carrying the caller's parsed MCP
// capabilities and login name. A nil caps means the caller has no grant.
func withCapabilities(ctx context.Context, caps *MCPCapability, user string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyCapabilities, caps)
	return context.WithValue(ctx, ctxKeyUser, user)
}

func capabilitiesFromContext(ctx context.Context) (*MCPCapability, string, bool) {
	caps, ok := ctx.Value(ctxKeyCapabilities).(*MCPCapability)
	if !ok {
		return nil, "unknown", false
	}
	user, _ := ctx.Value(ctxKeyUser).(string)
	if user == "" {
		user = "unknown"
	}
	return caps, user, true
}

// parseMCPCapabilities extracts every MCP grant entry from a peer's
// capability map and unions them. Tailscale merges all matching grant
// rules into one list, so a user matched by several rules must receive
// the sum of their allowances, not just the first.
func parseMCPCapabilities(capMap tailcfg.PeerCapMap) (*MCPCapability, error) {
	entries, err := tailcfg.UnmarshalCapJSON[MCPCapability](capMap, mcpCapabilityName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s grant: %w", mcpCapabilityName, err)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	merged := &MCPCapability{}
	for _, entry := range entries {
		merged.Tools = appendUnique(merged.Tools, entry.Tools)
		merged.Resources = appendUnique(merged.Resources, entry.Resources)
	}
	return merged, nil
}

func appendUnique(dst []string, src []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range src {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		dst = append(dst, v)
	}
	return dst
}

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

func isReadOnlyTool(tool mcp.Tool) bool {
	return tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint
}

// grantToolFilter trims tools/list to what the caller's grant allows, so an
// agent is not shown a hundred tools it will be denied on.
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

// AllowsResource reports whether the grant permits reading the resource URI.
func (c *MCPCapability) AllowsResource(uri string) bool {
	if c == nil {
		return false
	}
	for _, allowed := range c.Resources {
		if resourceGrantMatches(allowed, uri) {
			return true
		}
	}
	return false
}

// resourceGrantMatches matches a resource grant against a URI. A grant is
// either "*", an exact URI, or a prefix ending in "/*" that covers every
// URI beneath that path segment. Bare prefixes are not honored, so a grant
// for tailscale://device does not leak tailscale://devices.
func resourceGrantMatches(grant, uri string) bool {
	grant = strings.TrimSpace(grant)
	switch {
	case grant == "":
		return false
	case grant == "*":
		return true
	case strings.HasSuffix(grant, "/*"):
		prefix := strings.TrimSuffix(grant, "*")
		return strings.HasPrefix(uri, prefix) && len(uri) > len(prefix)
	default:
		return grant == uri
	}
}

func checkToolAccess(ctx context.Context, toolName string) error {
	allows := func(c *MCPCapability, name string) bool { return c.AllowsTool(toolMeta(name)) }
	return checkAccess(ctx, toolName, "tool", allows, func(c *MCPCapability) []string { return c.Tools })
}

func checkResourceAccess(ctx context.Context, resourceURI string) error {
	return checkAccess(ctx, resourceURI, "resource", (*MCPCapability).AllowsResource, func(c *MCPCapability) []string { return c.Resources })
}

func checkAccess(ctx context.Context, item, itemType string, allows func(*MCPCapability, string) bool, granted func(*MCPCapability) []string) error {
	caps, user, ok := capabilitiesFromContext(ctx)
	if !ok || caps == nil {
		logger.Warn("No MCP capabilities found", zap.String("user", user), zap.String(itemType, item))
		return errors.New(createPermissionErrorJSON(user, item, itemType, nil))
	}
	if allows(caps, item) {
		logger.Info("Access granted", zap.String("user", user), zap.String(itemType, item))
		return nil
	}
	logger.Warn("Access denied", zap.String("user", user), zap.String(itemType, item), zap.Strings("granted", granted(caps)))
	return errors.New(createPermissionErrorJSON(user, item, itemType, granted(caps)))
}
