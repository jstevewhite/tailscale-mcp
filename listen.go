package main

import (
	"fmt"
	"net"
	"strconv"

	"github.com/jaxxstorm/tailscale-mcp/internal/toolmeta"
	"github.com/mark3labs/mcp-go/server"
)

// resolveListenPort picks the listen port: an explicit port wins, otherwise
// 443 with TLS and 8080 without.
func resolveListenPort(port int, tls bool) int {
	if port > 0 {
		return port
	}
	if tls {
		return 443
	}
	return 8080
}

// resolveLocalPort picks the loopback listener port. It is independent of
// the tailnet port so --tls on 443 does not require root for 127.0.0.1.
func resolveLocalPort(port int) int {
	if port > 0 {
		return port
	}
	return 8080
}

// endpointURL renders the MCP endpoint for logs, omitting the port when it
// is the scheme default.
func endpointURL(host string, port int, tls bool) string {
	scheme := "http"
	defaultPort := 80
	if tls {
		scheme = "https"
		defaultPort = 443
	}
	if port == defaultPort {
		return fmt.Sprintf("%s://%s%s", scheme, host, mcpEndpointPath)
	}
	return fmt.Sprintf("%s://%s%s", scheme, net.JoinHostPort(host, strconv.Itoa(port)), mcpEndpointPath)
}

// newMCPServer builds the MCP server with panic recovery for tool and
// resource handlers so a bad argument cannot take down a session.
// toolCatalog records every registered tool; newMCPServer resets it.
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
