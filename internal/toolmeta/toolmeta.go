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
