package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// localGrantUser is the login name reported for callers that reach the
// server through stdio or the loopback listener, where there is no
// Tailscale identity to look up.
const localGrantUser = "local"

// parseLocalGrants parses the --local-grants value. It uses the same JSON
// shape as one entry of the jaxxstorm.com/cap/mcp grant so an operator can
// copy it straight from their ACL. An empty value means no local access.
func parseLocalGrants(raw string) (*MCPCapability, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var caps MCPCapability
	if err := json.Unmarshal([]byte(raw), &caps); err != nil {
		return nil, fmt.Errorf("invalid local grants JSON: %w", err)
	}
	caps.Tools = appendUnique(nil, caps.Tools)
	caps.Resources = appendUnique(nil, caps.Resources)
	if len(caps.Tools) == 0 && len(caps.Resources) == 0 {
		return nil, errors.New(`local grants must list at least one tool or resource, for example {"tools":["*"],"resources":["*"]}`)
	}
	return &caps, nil
}

// localGrantMiddleware authorizes loopback requests with the configured
// local grant instead of a Tailscale WhoIs lookup. With no grant configured
// every request is refused.
func localGrantMiddleware(next http.Handler, caps *MCPCapability) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if caps == nil {
			logger.Warn("Local request refused; set --local-grants to allow loopback access", zap.String("remote_addr", r.RemoteAddr))
			http.Error(w, "unauthorized: local access requires --local-grants", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(withCapabilities(r.Context(), caps, localGrantUser)))
	})
}

// stdioContextFunc attaches the local grant to every stdio request.
func stdioContextFunc(caps *MCPCapability) server.StdioContextFunc {
	return func(ctx context.Context) context.Context {
		return withCapabilities(ctx, caps, localGrantUser)
	}
}
