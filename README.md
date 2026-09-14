# Tailscale MCP Server

> **This is a fork** of [jaxxstorm/tailscale-mcp](https://github.com/jaxxstorm/tailscale-mcp), maintained at [jstevewhite/tailscale-mcp](https://github.com/jstevewhite/tailscale-mcp). It fixes the build, makes the stdio and loopback paths authorizable, and adds HTTPS on the tailnet, refreshable federated credentials, `read:*` and `group:` grant selectors, grant-filtered tool lists, and named tool profiles selectable by URL. See the [Usage Guide](docs/usage.md) for details. Upstream is unchanged and credit for the original design goes to its author.

An MCP (Model Context Protocol) server for Tailscale, enabling detailed queries and operations for devices, DNS, users, invites, keys, webhooks, services, logging, policy validation, and tailnet settings. It serves MCP over Streamable HTTP on `/mcp` and uses Tailscale OAuth grants for fine-grained access control.

## Features

* **Streamable HTTP Transport**: Serves MCP on `/mcp` via Tailscale, optionally over HTTPS with `--tls`, and on localhost
* **Comprehensive Tailscale Integration**: Full mapped coverage of the vendored Tailscale OpenAPI snapshot
* **OAuth Grants Authorization**: Fine-grained MCP access control with `jaxxstorm.com/cap/mcp`
* **Single Credential Startup**: Uses `TAILSCALE_OAUTH_TOKEN` for Admin API access and tsnet startup
* **Configurable tsnet State**: Stores tsnet state on the filesystem by default, with optional Kubernetes Secret or AWS SSM state stores
* **Local Access Opt-In**: `--local-grants` authorizes stdio and loopback clients that have no Tailscale identity
* **Tool Profiles**: Named tool sets in a YAML config, selected per client by URL path such as `/mcp/dns`

## Quick Start

```bash
export TAILSCALE_OAUTH_TOKEN='{"type":"oauth","clientId":"k123...","clientSecret":"tskey-client-...","scopes":["all"]}'
export TAILSCALE_TAILNET="yourtailnet.com"
export TS_ADVERTISE_TAGS="tag:mcp-server"
./ts-mcp
```

You can also provide the OAuth client ID separately:

```bash
export TAILSCALE_OAUTH_TOKEN="tskey-client-..."
export TAILSCALE_OAUTH_CLIENT_ID="k123..."
export TAILSCALE_TAILNET="yourtailnet.com"
export TS_ADVERTISE_TAGS="tag:mcp-server"
./ts-mcp
```

The server exposes MCP at:

* `http://<hostname>.yourtailnet.ts.net:8080/mcp` for any client on the tailnet whose user has a grant
* `http://127.0.0.1:8080/mcp` for local clients, only when `--local-grants` is set (port via `--local-port`)
* `.../mcp/<profile>` on either listener for a named tool profile, when `--config` is set

Add `--tls` (or `TS_TLS=1`) to serve `https://<hostname>.yourtailnet.ts.net/mcp` on port 443 with a certificate issued through Tailscale.

## Documentation

* [Usage Guide](docs/usage.md): installation, configuration, credentials, grants, client setup, tools, resources, coverage, and troubleshooting
* [Profiles Configuration Guide](docs/profiles.md): per-client tool sets, selectors, groups with every tool listed, and troubleshooting
* [Coverage Report](coverage/mcp-coverage.md): generated Tailscale OpenAPI to MCP coverage mapping
* [Parity Backlog](coverage/parity-backlog.md): generated list of unmapped API operations

## Development

```bash
go test ./...
go build ./...
make coverage
```

## Useful Links

* [Tailscale API Documentation](https://tailscale.com/kb/1101/api/)
* [Tailscale OAuth Grants](https://tailscale.com/kb/1017/grant-access-to-apps/)
* [MCP Protocol Documentation](https://modelcontextprotocol.io/)
