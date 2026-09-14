# Tool Profiles and Groups

Date: 2026-09-14

## Problem

The server advertises 122 tools (126 with local CLI tools). Every tool schema
is sent to the model on each session, which wastes context and degrades tool
selection. Grants filter the list per caller, but changing what a client sees
means editing the ACL, and clients without a tool picker cannot narrow the
list themselves.

## Goal

Let the operator define named tool profiles in a config file and let each
client pick one by the URL it connects to, so one server can serve different
tool sets to different clients. Grants still bound everything.

## Selectors

A selector names a set of tools. The same grammar is used everywhere a tool
list appears: ACL grants, `--local-grants`, and profiles.

| Selector | Matches |
|---|---|
| `*` | every tool |
| `read:*` | every tool whose MCP annotation is read-only |
| `group:<name>` | every tool in the group |
| `group:<name>:read` | read-only tools in the group |
| `<tool name>` | exactly that tool |

Unknown group names match nothing. Selectors are matched against a tool's
metadata: its name, its read-only annotation, and its group.

## Groups

Each tool belongs to exactly one group. Generated and curated tools that hit
an API endpoint derive their group from the endpoint path so new endpoints
land in a group automatically. Tools without an endpoint declare a group.

| Group | API paths or tools |
|---|---|
| `devices` | `/device/...`, `/tailnet/{tailnet}/devices`, `/tailnet/{tailnet}/device-attributes`, `get_device_info`, `list_all_devices`, curated device tools |
| `dns` | `/tailnet/{tailnet}/dns/...` |
| `policy` | `/tailnet/{tailnet}/acl...`, curated ACL tools |
| `keys` | `/tailnet/{tailnet}/keys...` |
| `users` | `/users/...`, `/tailnet/{tailnet}/users` |
| `invites` | `/user-invites/...`, `/device-invites/...`, `/tailnet/{tailnet}/user-invites`, `/device/{id}/device-invites` |
| `webhooks` | `/webhooks/...`, `/tailnet/{tailnet}/webhooks` |
| `services` | `/tailnet/{tailnet}/services...` |
| `logs` | `/tailnet/{tailnet}/logging/...`, `/tailnet/{tailnet}/aws-external-id/...` |
| `posture` | `/posture/...`, `/tailnet/{tailnet}/posture/...` |
| `oauth` | `/tailnet/{tailnet}/oauth-apps...` |
| `contacts` | `/tailnet/{tailnet}/contacts...` |
| `settings` | `/tailnet/{tailnet}/settings`, `tailscale_status` |
| `local` | local CLI tools |

Path-to-group resolution strips the `/tailnet/{tailnet}` prefix, then keys on
the first path segment. `device-invites` under `/device/{id}/` is checked
before the `device` rule. Anything unmatched falls into `other`, and a test
asserts that no registered tool is in `other`.

`--list-groups` prints every registered tool as `group  read-only  name` and
exits, so an operator can build profiles from real names.

## Tool catalog

A `catalog` records metadata for every registered tool: name, group,
read-only. It is filled during registration and is the single source the
access check, the tool filter, and `--list-groups` read from. The read-only
flag comes from the tool's MCP annotation so it cannot drift from what
clients see.

## Config file

YAML, path from `--config` or `TS_MCP_CONFIG`. Read once at startup. Absent
means no profiles: `/mcp` serves everything the grant allows, as today.

```yaml
default: readonly
profiles:
  readonly: ["read:*"]
  dns:      ["group:dns"]
  devices:  ["group:devices:read", "tailscale_device_set_tags"]
  ops:      ["read:*", "group:policy"]
```

Validation at startup, all fatal: unreadable file, invalid YAML, `default`
naming a profile that does not exist, a profile with an empty list, an
unknown group in any selector, a tool name in any selector that is not
registered. Profile names must match `^[a-z0-9][a-z0-9_-]*$`.

## Routing

The tailnet and loopback listeners mount the MCP handler at both `/mcp` and
`/mcp/`. The path segment after `/mcp/` is the profile name; `/mcp` and
`/mcp/` mean the default profile. An unknown profile name is a 404 before
authentication. When no config file is loaded, any profile name other than
empty is a 404.

The resolved profile's selectors are placed in the request context. Stdio
takes `--profile <name>` and rejects a name that is not defined.

## Visibility

A tool is visible to a request when the profile allows it and the caller's
grant allows it. With no profile configured, only the grant applies. The
mcp-go tool filter applies this to `tools/list` and `tools/call`. The
server-side access check on call is unchanged and still enforces the grant.
Resources are not affected by profiles.

## Out of scope

Resource filtering by profile, config hot reload, per-profile grants, and
removing the `_curated` duplicate tools.

## Testing

- Selector matching for each form, including unknown group and `read` suffix.
- Path-to-group for every rule plus a fallthrough case.
- Every registered tool has a group other than `other`.
- Config parsing: valid file, each validation failure.
- Profile resolution from URL path, including default, trailing slash,
  unknown, and no-config cases.
- End to end through `HandleMessage`: `tools/list` under a profile and grant
  returns the intersection.
