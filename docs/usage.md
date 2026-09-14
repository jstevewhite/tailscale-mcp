# Usage Guide

## Prerequisites

* [Tailscale](https://tailscale.com) account with an OAuth or federated credential
* Go 1.26.4 or higher if building from source

## Installation

Download the latest pre-built binary for your platform from the release page, or build from source:

```bash
git clone <repo_url>
cd <repo_dir>
go mod tidy
go build -o ts-mcp .
```

You can also install with Homebrew:

```bash
brew install jaxxstorm/tap/tailscale-mcp
```

## Configuration

Required environment variables:

```bash
export TAILSCALE_OAUTH_TOKEN='{"type":"oauth","clientId":"k123...","clientSecret":"tskey-client-...","scopes":["all"]}'
export TAILSCALE_TAILNET="yourtailnet.com"
export TS_ADVERTISE_TAGS="tag:mcp-server"
```

Optional environment variables:

```bash
export TS_HOSTNAME="ts-mcp"
export TS_PORT="8080"            # defaults to 8080, or 443 with TS_TLS
export TS_TLS="1"                # serve HTTPS on the tailnet
export TSNET_STATE="file://"
export TS_MCP_LOCAL_GRANTS='{"tools":["*"],"resources":["*"]}'   # allow stdio and loopback clients
```

`TS_ADVERTISE_TAGS` is required when `TAILSCALE_OAUTH_TOKEN` is an OAuth client secret or federated credential because tsnet mints a tagged node auth key during startup. The OAuth client or federated credential must be allowed to create auth keys for the advertised tag.

Command line options:

* `--debug` / `-d`: Enable debug logging
* `--version` / `-v`: Show version information
* `--oauth-client-id`: OAuth client ID to use when `TAILSCALE_OAUTH_TOKEN` is a raw `tskey-client-*` secret
* `--advertise-tags`: Comma-separated Tailscale tags to advertise when minting tsnet auth keys from OAuth or federated credentials
* `--state`: tsnet state location. Same as `TSNET_STATE`
* `--tls`: Serve HTTPS on the tailnet using a Tailscale-issued certificate. Same as `TS_TLS`
* `--local-grants`: JSON grant applied to stdio and loopback callers. Same as `TS_MCP_LOCAL_GRANTS`
* `--local-port`: Port for the plain-HTTP loopback listener, default 8080. Same as `TS_MCP_LOCAL_PORT`
* `--config`: YAML file defining named tool profiles. Same as `TS_MCP_CONFIG`
* `--profile`: Tool profile to serve in `--stdio` mode. Same as `TS_MCP_PROFILE`
* `--list-groups`: Print every tool with its group and read-only flag, then exit
* `--stdio`: Use deprecated stdio compatibility mode instead of Streamable HTTP

## tsnet State

`TSNET_STATE` controls where the embedded tsnet node stores its Tailscale identity and local state.

If `TSNET_STATE` is unset, the default is unchanged from earlier releases: state is stored in a hostname-specific directory under the process working directory. With the default `TS_HOSTNAME=ts-mcp`, the state file is:

```text
./tsnet-ts-mcp/tailscaled.state
```

The server logs the resolved state location at startup as `Configured tsnet state`, including the absolute filesystem path when state is file-backed.

Supported values:

* `file://`: use the default hostname-specific directory, such as `./tsnet-ts-mcp/tailscaled.state`
* `file:///var/lib/tailscale-mcp`: store filesystem state at `/var/lib/tailscale-mcp/tailscaled.state`
* `kube://tailscale-mcp-state`: store state in the Kubernetes Secret `tailscale-mcp-state`
* `aws://us-east-1/123456789012/parameter/tailscale/mcp`: store state in AWS SSM Parameter Store
* `aws://arn:aws:ssm:us-east-1:123456789012:parameter/tailscale/mcp`: store state in the given AWS SSM ARN

The native Tailscale store prefixes are also accepted for compatibility: `kube:<secret>`, `arn:aws:ssm:...`, and `mem:`.

Kubernetes Secret state requires the pod service account to have `get` and `update` on the state Secret. Grant `patch` and `create` as well so the store can efficiently update or create the Secret. Optional Kubernetes Events require `get`, `create`, and `patch` on `events`.

Example Kubernetes RBAC:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tailscale-mcp-state
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    resourceNames: ["tailscale-mcp-state"]
    verbs: ["get", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["create"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "create", "patch"]
```

When using Kubernetes state:

```bash
export TSNET_STATE="kube://tailscale-mcp-state"
```

When using AWS SSM state, the workload must have AWS credentials that can read and write the target parameter. If `kmsKey` is provided, the workload also needs permission to use that key:

```bash
export TSNET_STATE="aws://us-east-1/123456789012/parameter/tailscale/mcp?kmsKey=alias/tailscale-state"
```

When running the container image, the default state directory is relative to the working directory, which is `/` and read-only for the non-root user. Mount a volume and point `TSNET_STATE` at it:

```bash
docker run -v tailscale-mcp-state:/state -e TSNET_STATE=file:///state ...
```

## Credentials

Create a Tailscale OAuth client or federated credential that can read tailnet settings at startup and perform every Admin API operation you expose through MCP.

OAuth client JSON form:

```bash
export TAILSCALE_OAUTH_TOKEN='{"type":"oauth","clientId":"k123...","clientSecret":"tskey-client-...","scopes":["all"]}'
export TS_ADVERTISE_TAGS="tag:mcp-server"
```

OAuth client split form:

```bash
export TAILSCALE_OAUTH_TOKEN="tskey-client-..."
export TAILSCALE_OAUTH_CLIENT_ID="k123..."
export TS_ADVERTISE_TAGS="tag:mcp-server"
```

Federated JSON form:

```bash
export TAILSCALE_OAUTH_TOKEN='{"type":"federated","clientId":"k123...","idToken":"<oidc-id-token>"}'
export TS_ADVERTISE_TAGS="tag:mcp-server"
```

OIDC ID tokens usually expire within an hour. For a long-running server, point `idTokenFile` at a file that an external refresher keeps current. The file is re-read on every Admin API token exchange:

```bash
export TAILSCALE_OAUTH_TOKEN='{"type":"federated","clientId":"k123...","idTokenFile":"/var/run/secrets/tailscale/id-token"}'
```

A raw bearer/auth-key-like token is also accepted for deployments where the same token can authenticate Admin API requests and tsnet startup:

```bash
export TAILSCALE_OAUTH_TOKEN="tskey-..."
```

For full OpenAPI coverage, grant the credential scopes or permissions for devices, DNS, policy files, tailnet settings, users, invites, keys, webhooks, services, logging, OAuth apps, and posture integrations. Mutating MCP tools also require the operation-specific `confirm` argument and matching Tailscale API write permissions.

## OAuth Grants And Access Control

The server uses Tailscale grants with the custom MCP capability `jaxxstorm.com/cap/mcp`. These grants control incoming MCP user access and are separate from the server credential's Tailscale Admin API scopes.

Example ACL policy:

```json
{
  "grants": [
    {
      "src": ["user:alice@example.com"],
      "dst": ["tag:mcp-server"],
      "app": {
        "jaxxstorm.com/cap/mcp": [{
          "tools": ["*"],
          "resources": ["*"]
        }]
      }
    },
    {
      "src": ["user:bob@example.com"],
      "dst": ["tag:mcp-server"],
      "app": {
        "jaxxstorm.com/cap/mcp": [{
          "tools": ["list_all_devices"],
          "resources": ["bootstrap://status", "tailscale://devices"]
        }]
      }
    }
  ]
}
```

Tool grants:

* `get_device_info`: Allow querying specific device details
* `list_all_devices`: Allow listing all devices
* `tailscale_<operation>`: Allow a generated Tailscale API tool, for example `tailscale_get_dns_configuration`
* `read:*`: Allow every tool whose MCP annotation marks it read-only, and nothing that mutates
* `group:<name>` and `group:<name>:read`: Allow a whole group, or its read-only tools. See Tool Profiles for the group list
* `*`: Allow all tools

The server advertises only the tools a caller's grant allows. With `read:*` an agent sees about forty-five read-only tools instead of the full set of over a hundred, which keeps its context small and stops it from planning around tools it cannot call. A tool outside the grant is neither listed nor callable; calling it by name returns "tool not found". Resources are always listed, but reading one still requires a matching resource grant.

A sensible starting grant for an agent:

```json
"jaxxstorm.com/cap/mcp": [{
  "tools": ["read:*"],
  "resources": ["*"]
}]
```

Resource grants:

* `bootstrap://status`: Health check endpoint
* `tailscale://devices`: Device list resource
* `tailscale://policy`: Policy file access
* `tailscale://tailnet-settings`: Tailnet settings access
* `tailscale://device`: Individual device resource access
* `tailscale://dns/*`, `tailscale://keys`, `tailscale://webhooks`, `tailscale://services`, `tailscale://oauth-apps`, and similar read API resources
* `*`: Allow all resources

Resource grants match exactly, or hierarchically when written with a trailing `/*`. `tailscale://device/*` covers `tailscale://device/<id>/routes` and `tailscale://device/<id>/attributes`; it does not cover `tailscale://devices`. When a user matches several grant rules, the tool and resource lists from all of them are combined.

The `confirm` argument on mutating tools is a speed bump, not a safeguard: the required value is printed in the tool description, so an agent will supply it. The grant is the only enforcement. Prefer explicit tool lists over `"tools": ["*"]` for agents, because a wildcard grant includes `tailscale_create_key`, `tailscale_set_policy_file`, and `tailscale_delete_user`.

## Tool Profiles

A hundred-plus tool schemas is a lot of context for a model. Profiles let the operator publish a named subset of tools, and let each client pick one by the URL it connects to. Grants still apply: a client sees the intersection of its profile and its grant.

Define profiles in a YAML file passed with `--config` or `TS_MCP_CONFIG`. The repo ships `profiles.example.yaml` as a starting point; copy it to `profiles.yaml`, which is ignored by git:

```yaml
default: readonly          # served at /mcp; omit to serve everything the grant allows
profiles:
  readonly: ["read:*"]
  dns:      ["group:dns"]
  devices:  ["group:devices:read", "tailscale_device_set_tags"]
  ops:      ["read:*", "group:policy"]
```

Clients connect to `/mcp` for the default profile or `/mcp/<name>` for a named one. An unknown name is a 404. In `--stdio` mode pass `--profile <name>`.

Selectors, usable in profiles, ACL grants, and `--local-grants`:

| Selector | Matches |
|---|---|
| `*` | every tool |
| `read:*` | every read-only tool |
| `group:<name>` | every tool in the group |
| `group:<name>:read` | the group's read-only tools |
| `<tool name>` | one tool |

Groups: `devices`, `dns`, `policy`, `keys`, `users`, `invites`, `webhooks`, `services`, `logs`, `posture`, `oauth`, `contacts`, `settings`, `local`. Run `./ts-mcp --list-groups` to print every tool with its group and read-only flag.

The config is validated at startup: unknown groups, unknown tool names, empty profiles, and a `default` that names no profile all stop the server. It is read once; restart to pick up changes.

See the [Profiles Configuration Guide](profiles.md) for the full tool list per group, recipes, and troubleshooting.

### Local Access

Requests over stdio or the loopback listener have no Tailscale identity, so they are denied unless `--local-grants` (or `TS_MCP_LOCAL_GRANTS`) is set. The value is one entry of the grant capability, so it can be copied from the ACL:

```bash
export TS_MCP_LOCAL_GRANTS='{"tools":["list_all_devices","get_device_info"],"resources":["tailscale://devices"]}'
```

The loopback listener is only started when local grants are set. It always speaks plain HTTP on `--local-port` (default 8080), independent of the tailnet port, so `--tls` on 443 does not require binding a privileged port on the host. Anything that can reach it, or launch the binary with `--stdio`, receives this grant. Tailnet requests never use it.

## Running The Server

Streamable HTTP is the default transport:

```bash
./ts-mcp
```

The server exposes MCP at:

* `http://<hostname>.yourtailnet.ts.net:8080/mcp` for clients on the tailnet
* `http://127.0.0.1:8080/mcp` for local clients, only when `--local-grants` is set (port via `--local-port`)

With `--tls` the tailnet listener serves HTTPS on port 443 using a certificate issued through Tailscale, so the URL becomes `https://<hostname>.yourtailnet.ts.net/mcp`. This requires HTTPS certificates to be enabled in the tailnet's DNS settings. The startup log prints the resolved URL.

```bash
./ts-mcp --tls
```

Deprecated stdio compatibility mode is available for older local clients that cannot use Streamable HTTP yet. It requires `--local-grants`:

```bash
./ts-mcp --stdio --local-grants '{"tools":["*"],"resources":["*"]}'
```

## Claude Desktop Integration

Use Claude Desktop's Streamable HTTP remote MCP configuration when available. Point it at the Tailscale or localhost `/mcp` endpoint.

For older Claude Desktop versions that only support local stdio MCP servers, use the deprecated compatibility mode temporarily:

```json
{
  "mcpServers": {
    "tailscale": {
      "command": "/usr/local/bin/ts-mcp",
      "args": ["--stdio"],
      "env": {
        "TAILSCALE_OAUTH_TOKEN": "{\"type\":\"oauth\",\"clientId\":\"k123...\",\"clientSecret\":\"tskey-client-...\",\"scopes\":[\"all\"]}",
        "TAILSCALE_TAILNET": "yourtailnet.com",
        "TS_ADVERTISE_TAGS": "tag:mcp-server",
        "TS_MCP_LOCAL_GRANTS": "{\"tools\":[\"*\"],\"resources\":[\"*\"]}"
      }
    }
  }
}
```

## Tools And Resources

Core tools:

| Tool | Description | Arguments | Required Grant |
|------|-------------|-----------|----------------|
| `get_device_info` | Fetch device details by ID, IP, or hostname | `device`: Device identifier | `get_device_info` |
| `list_all_devices` | List all devices in your tailnet | None | `list_all_devices` |

Additional Tailscale API tools are generated from endpoint definitions using `tailscale_<operation>` grant names. Examples include `tailscale_get_dns_configuration`, `tailscale_list_users`, `tailscale_get_key`, `tailscale_list_webhooks`, `tailscale_list_services`, `tailscale_get_oauth_app`, and `tailscale_validate_and_test_policy_file`.

Generated tools cover the full Tailscale OpenAPI snapshot. Mutating create/update/delete tools require a `confirm` argument whose value is the OpenAPI operation ID, for example `confirm: "deleteDevice"`. This is in addition to Tailscale MCP grants and Admin API token permissions.

### Composable Endpoint Workflows

Every mapped Tailscale OpenAPI operation is available as a first-class MCP primitive, either as a tool or a resource. Agents can compose these primitives the same way an operator might compose `curl` calls: read tailnet state, inspect the JSON result, choose the next endpoint, and propose a guarded write when needed.

Generated endpoint tools include MCP safety hints for clients that support mutation gating:

* `readOnlyHint=true`: The tool is expected not to change Tailscale state. GET operations and read-like validation operations use this hint.
* `destructiveHint=true`: The tool may delete, revoke, expire, suspend, rotate, or otherwise destructively change Tailscale state.
* `idempotentHint=true`: Repeating the same tool call with the same inputs is expected not to create additional side effects.

These hints are advisory metadata for MCP clients. Server-side enforcement remains authoritative: every tool and resource still requires the configured `jaxxstorm.com/cap/mcp` grant, and mutating tools still require the exact `confirm` token for the underlying OpenAPI operation.

### Network Flow Logs

`tailscale_list_network_flow_logs` returns network flow logs in chronological windows of at most five minutes so a busy tailnet cannot overwhelm an MCP client's context. For the first call, provide RFC3339 `start` and `end` timestamps. The response contains `logs`, the effective window `start` and `end`, and `nextCursor` when more of the requested range remains. Call the tool again with that value as `cursor` until `nextCursor` is absent.

The tool remains read-only and requires the existing `tool:tailscale_list_network_flow_logs` grant.

### Curated Operator Tools

Curated tools are task-oriented wrappers around one or more generated endpoint tools. They do not replace the generated `tailscale_<operation>` tools and are not counted separately in OpenAPI coverage. Each curated tool uses its own grant name matching the tool name.

Status and ACL tools:

| Tool | Description | Required Grant |
|------|-------------|----------------|
| `tailscale_status` | Compose device/settings reads into a setup health response | `tailscale_status` |
| `tailscale_get_acl` | Read HuJSON ACL policy and ETag | `tailscale_get_acl` |
| `tailscale_validate_acl` | Validate HuJSON ACL text without applying it | `tailscale_validate_acl` |
| `tailscale_preview_acl` | Preview ACL rules for a user or IP:port | `tailscale_preview_acl` |
| `tailscale_update_acl` | Update HuJSON ACL policy with ETag and `confirm: "setPolicyFile"` | `tailscale_update_acl` |

Device workflow tools include `tailscale_list_devices`, `tailscale_get_device`, `tailscale_device_routes`, `tailscale_device_posture_attributes`, `tailscale_device_authorize`, `tailscale_device_deauthorize`, `tailscale_device_delete`, `tailscale_device_rename`, `tailscale_device_expire_key`, `tailscale_device_set_routes`, `tailscale_device_set_tags`, `tailscale_device_set_ip`, `tailscale_device_update_key`, `tailscale_device_set_posture_attribute`, `tailscale_device_delete_posture_attribute`, `tailscale_device_batch_update_posture_attributes`, and `tailscale_set_devices_authorized`.

Mutating curated tools require both the curated tool grant and an explicit `confirm` argument. Single-operation wrappers use the underlying OpenAPI operation ID as the confirmation token. Bulk/composed mutating workflows use the curated tool name as the confirmation token.

Local CLI diagnostics are disabled by default. Enable them only on hosts where the local `tailscale` binary is trusted and available:

```bash
export TAILSCALE_LOCAL_CLI=1
```

When enabled, the server registers `tailscale_local_status`, `tailscale_ping`, `tailscale_netcheck`, and `tailscale_local_version`. These tools execute the local `tailscale` binary without a shell, validate inputs, bound output size, use timeouts, and are marked read-only.

Common resources:

| URI | Description | Required Grant |
|-----|-------------|----------------|
| `bootstrap://status` | Health-check endpoint | `bootstrap://status` |
| `tailscale://devices` | Complete device list with metadata | `tailscale://devices` |
| `tailscale://policy` | Current Tailscale ACL policy file | `tailscale://policy` |
| `tailscale://tailnet-settings` | Tailnet configuration and settings | `tailscale://tailnet-settings` |
| `tailscale://device` | Individual device details | `tailscale://device` |
| `tailscale://dns/configuration` | Full DNS configuration | `tailscale://dns/configuration` |
| `tailscale://keys` | Active keys visible to the API token | `tailscale://keys` |
| `tailscale://user-invites` | Open user invites | `tailscale://user-invites` |
| `tailscale://webhooks` | Webhooks | `tailscale://webhooks` |
| `tailscale://services` | Services | `tailscale://services` |
| `tailscale://posture/integrations` | Posture integrations | `tailscale://posture/integrations` |
| `tailscale://oauth-apps` | OAuth apps | `tailscale://oauth-apps` |

## API Coverage

Full Tailscale API parity is tracked with repository tooling under `tools/coverage/`. The tooling maps each Tailscale OpenAPI operation to an MCP tool, resource, prompt workflow, or reviewed exclusion, then writes generated reports under `coverage/`.

Refresh the vendored Tailscale OpenAPI snapshot with:

```bash
make openapi-refresh
```

Run coverage generation with:

```bash
make coverage
```

Review `coverage/mcp-coverage.md` for current MCP coverage and `coverage/parity-backlog.md` for unimplemented API operations.

## Example Queries

```text
Use get_device_info to get details about device "100.101.102.103"
```

```text
List all devices in my tailnet using list_all_devices
```

```text
Show me the current Tailscale policy by reading the tailscale://policy resource
```

## Logging

Enable debug logging to see detailed protocol exchanges and OAuth grants:

```bash
./ts-mcp --debug
```

Debug mode includes MCP message flow, OAuth grants parsing, user authentication context, and access control decisions.

## Troubleshooting

**"unauthorized: local access requires --local-grants"**

* Requests over stdio or `127.0.0.1` have no Tailscale identity
* Set `--local-grants` or `TS_MCP_LOCAL_GRANTS` to authorize local clients, or connect over the tailnet instead

**"No MCP capabilities found"**

* Verify your ACL policy includes the correct grants configuration
* Check that the server node has the appropriate tags
* Ensure the user has been granted access to MCP capabilities

**"Access denied: insufficient permissions"**

* Review the grants configuration in your ACL policy
* Verify the user is listed in the `src` field of the relevant grant
* Check that the requested tool/resource is included in the capability definition

**"Failed to get Tailscale status"**

* Ensure Tailscale is running and authenticated
* Verify `TAILSCALE_OAUTH_TOKEN` can authenticate tsnet startup
* Check network connectivity to Tailscale coordination servers

**"Tailscale credential validation failed"**

* Verify `TAILSCALE_OAUTH_TOKEN` is set and valid JSON when using a JSON credential
* Verify the credential can read tailnet settings for `TAILSCALE_TAILNET`
* Add the missing OAuth scopes or federated credential permissions reported by Tailscale

**tsnet authentication fails during startup**

* OAuth credentials must include a usable `clientSecret`
* Federated credentials must include `clientId` and `idToken`
* OAuth and federated credentials must set `TS_ADVERTISE_TAGS`, for example `tag:mcp-server`
* Raw bearer tokens must be auth-key-like if they are expected to enroll the tsnet node
