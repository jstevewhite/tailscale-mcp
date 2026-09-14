package main

import (
	"fmt"
	"strings"

	"github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"
)

// formatToolTable renders the catalog for --list-groups.
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
