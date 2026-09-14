package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestParseLocalGrantsAcceptsCapabilityJSON(t *testing.T) {
	caps, err := parseLocalGrants(`{"tools":["*"],"resources":["bootstrap://status"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !caps.AllowsTool("anything", false) || !caps.AllowsResource("bootstrap://status") || caps.AllowsResource("tailscale://devices") {
		t.Fatalf("unexpected capabilities %#v", caps)
	}
}

func TestParseLocalGrantsEmptyMeansNone(t *testing.T) {
	caps, err := parseLocalGrants("  ")
	if err != nil || caps != nil {
		t.Fatalf("expected nil, nil; got %#v, %v", caps, err)
	}
}

func TestParseLocalGrantsRejectsInvalidJSON(t *testing.T) {
	if _, err := parseLocalGrants(`{"tools":"*"}`); err == nil {
		t.Fatal("expected error for non-array tools")
	}
	if _, err := parseLocalGrants(`{}`); err == nil {
		t.Fatal("expected error for a grant with no tools or resources")
	}
}

func TestLocalGrantMiddlewareInjectsCapabilities(t *testing.T) {
	logger = zap.NewNop()
	var gotUser string
	var gotCaps *MCPCapability
	handler := localGrantMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCaps, gotUser, _ = capabilitiesFromContext(r.Context())
	}), &MCPCapability{Tools: []string{"list_all_devices"}})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if gotUser != localGrantUser {
		t.Fatalf("user = %q, want %q", gotUser, localGrantUser)
	}
	if !gotCaps.AllowsTool("list_all_devices", false) {
		t.Fatal("capabilities not injected")
	}
}

func TestLocalGrantMiddlewareDeniesWhenUnset(t *testing.T) {
	logger = zap.NewNop()
	called := false
	handler := localGrantMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil))

	if called || rec.Code != http.StatusUnauthorized {
		t.Fatalf("called = %v, status = %d; want not called, 401", called, rec.Code)
	}
}

func TestStdioContextFuncInjectsLocalGrants(t *testing.T) {
	logger = zap.NewNop()
	ctx := stdioContextFunc(&MCPCapability{Resources: []string{"*"}})(context.Background())
	if err := checkResourceAccess(ctx, "tailscale://devices"); err != nil {
		t.Fatalf("expected access via local grant, got %v", err)
	}
	ctx = stdioContextFunc(nil)(context.Background())
	if err := checkResourceAccess(ctx, "tailscale://devices"); err == nil {
		t.Fatal("expected denial without local grant")
	}
}
