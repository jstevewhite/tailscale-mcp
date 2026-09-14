package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

func TestReadOnlyWildcardAllowsOnlyReadOnlyTools(t *testing.T) {
	caps := &MCPCapability{Tools: []string{readOnlyWildcard}}
	if !caps.AllowsTool("tailscale_get_dns_configuration", true) {
		t.Error("read-only tool should be allowed by read:*")
	}
	if caps.AllowsTool("tailscale_delete_device", false) {
		t.Error("mutating tool must not be allowed by read:*")
	}
	if (&MCPCapability{Tools: []string{"*"}}).AllowsTool("tailscale_delete_device", false) == false {
		t.Error("* should still allow everything")
	}
}

func noopTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("ok"), nil
}

func TestCheckToolAccessHonorsReadOnlyWildcardFromRegisteredAnnotations(t *testing.T) {
	logger = zap.NewNop()
	srv := newMCPServer()
	srv.AddTool(mcp.NewTool("reader", mcp.WithReadOnlyHintAnnotation(true)), noopTool)
	srv.AddTool(mcp.NewTool("writer", mcp.WithReadOnlyHintAnnotation(false)), noopTool)

	ctx := withCapabilities(context.Background(), &MCPCapability{Tools: []string{readOnlyWildcard}}, "alice")
	if err := checkToolAccess(ctx, "reader"); err != nil {
		t.Fatalf("reader should be allowed: %v", err)
	}
	if err := checkToolAccess(ctx, "writer"); err == nil {
		t.Fatal("writer should be denied under read:*")
	}
	if err := checkToolAccess(ctx, "unregistered"); err == nil {
		t.Fatal("an unregistered tool must not be treated as read-only")
	}
}

func listTools(t *testing.T, ctx context.Context) []string {
	t.Helper()
	srv := newMCPServer()
	srv.AddTool(mcp.NewTool("reader", mcp.WithReadOnlyHintAnnotation(true)), noopTool)
	srv.AddTool(mcp.NewTool("writer", mcp.WithReadOnlyHintAnnotation(false)), noopTool)
	srv.AddTool(mcp.NewTool("named", mcp.WithReadOnlyHintAnnotation(false)), noopTool)

	init := srv.HandleMessage(ctx, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
	if _, bad := init.(mcp.JSONRPCError); bad {
		t.Fatalf("initialize failed: %#v", init)
	}
	resp := srv.HandleMessage(ctx, json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	rpc, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("tools/list failed: %#v", resp)
	}
	result, ok := rpc.Result.(mcp.ListToolsResult)
	if !ok {
		t.Fatalf("unexpected result type %T", rpc.Result)
	}
	names := []string{}
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestToolsListIsFilteredByGrant(t *testing.T) {
	logger = zap.NewNop()

	got := listTools(t, withCapabilities(context.Background(), &MCPCapability{Tools: []string{readOnlyWildcard, "named"}}, "alice"))
	want := map[string]bool{"reader": true, "named": true}
	if len(got) != len(want) {
		t.Fatalf("tools = %v, want reader and named only", got)
	}
	for _, name := range got {
		if !want[name] {
			t.Fatalf("tools = %v, want reader and named only", got)
		}
	}

	if got := listTools(t, context.Background()); len(got) != 0 {
		t.Fatalf("tools without a grant = %v, want none", got)
	}

	if got := listTools(t, withCapabilities(context.Background(), &MCPCapability{Tools: []string{"*"}}, "alice")); len(got) != 3 {
		t.Fatalf("tools with * = %v, want all three", got)
	}
}
