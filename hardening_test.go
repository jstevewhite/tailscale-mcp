package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

func TestAllowOriginMiddlewareRejectsSuffixedHostOrigin(t *testing.T) {
	logger = zap.NewNop()
	called := false
	handler := allowOriginMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodPost, "http://ts-mcp.example/mcp", nil)
	req.Header.Set("Origin", "http://ts-mcp.example.evil")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called || rec.Code != http.StatusForbidden {
		t.Fatalf("called = %v, status = %d; want not called, 403", called, rec.Code)
	}
}

func TestAllowOriginMiddlewareRejectsOriginWithDifferentPort(t *testing.T) {
	logger = zap.NewNop()
	handler := allowOriginMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	req := httptest.NewRequest(http.MethodPost, "http://ts-mcp.example:8080/mcp", nil)
	req.Header.Set("Origin", "http://ts-mcp.example:9090")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func fullAccessContext() context.Context {
	return withCapabilities(context.Background(), &MCPCapability{Tools: []string{"*"}, Resources: []string{"*"}}, "tester")
}

func TestGetDeviceInfoRejectsNonStringDeviceArgument(t *testing.T) {
	logger = zap.NewNop()
	mcpServer := newMCPServer()
	registerCoreMCP(mcpServer, nil)
	tool := mcpServer.GetTool("get_device_info")

	result, err := tool.Handler(fullAccessContext(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"device": 123}}})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("expected error result for non-string device, got %#v", result)
	}
}

func TestListAllDevicesDenialIsErrorResult(t *testing.T) {
	logger = zap.NewNop()
	mcpServer := newMCPServer()
	registerCoreMCP(mcpServer, nil)
	tool := mcpServer.GetTool("list_all_devices")

	result, err := tool.Handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("permission denial should be an error result, got %#v", result)
	}
}

func TestMCPServerRecoversFromPanickingTool(t *testing.T) {
	logger = zap.NewNop()
	mcpServer := newMCPServer()
	mcpServer.AddTool(mcp.NewTool("boom"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("kaboom")
	})

	ctx := fullAccessContext()
	init := mcpServer.HandleMessage(ctx, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
	if _, ok := init.(mcp.JSONRPCError); ok {
		t.Fatalf("initialize failed: %#v", init)
	}
	resp := mcpServer.HandleMessage(ctx, json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"boom","arguments":{}}}`))
	rpcErr, ok := resp.(mcp.JSONRPCError)
	if !ok {
		t.Fatalf("expected JSON-RPC error after panic, got %#v", resp)
	}
	if !strings.Contains(rpcErr.Error.Message, "kaboom") && !strings.Contains(rpcErr.Error.Message, "panic") {
		t.Fatalf("unexpected error message %q", rpcErr.Error.Message)
	}
}
