package main

import (
	"fmt"
	"net"
	"strconv"

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
func newMCPServer() *server.MCPServer {
	return server.NewMCPServer(mcpServerName, buildVersion, server.WithRecovery(), server.WithResourceRecovery())
}
