# Tool Profiles Configuration Guide

Profiles let one running server advertise a different set of tools to each client. A client chooses its profile by the URL it connects to. This guide covers the file format, the selector grammar, every group and the tools in it, how profiles combine with Tailscale grants, and how to debug a profile that does not behave.

## Why profiles

The server registers 110 tools, or 114 with local CLI tools enabled. Every MCP client sends every tool schema to the model on each session. That costs context before the first question and makes tool choice worse. Profiles let you publish only what a given client needs. Clients with a built-in tool picker can narrow further; clients without one just get a different URL.

## Quick start

1. Copy the example and edit it:

   ```bash
   cp profiles.example.yaml profiles.yaml
   ```

2. Start the server with it:

   ```bash
   ./ts-mcp --tls --config profiles.yaml
   ```

   The log prints `Loaded tool profiles` with the names and the default.

3. Point clients at profile URLs:

   | URL | Profile served |
   |---|---|
   | `https://ts-mcp.example.ts.net/mcp` | the one named by `default` |
   | `https://ts-mcp.example.ts.net/mcp/dns` | `dns` |
   | `https://ts-mcp.example.ts.net/mcp/nope` | none: HTTP 404 |

   The same paths work on the loopback listener when `--local-grants` is set. In `--stdio` mode pass `--profile <name>` instead.

## File format

```yaml
# Profile served at /mcp. Optional. If omitted, /mcp serves every tool the
# caller's grant allows, exactly as if no config file were loaded.
default: readonly

# Named profiles. Each is a list of selectors. Any matching selector
# includes the tool.
profiles:
  readonly: ["read:*"]
  dns:      ["group:dns"]
  devices:  ["group:devices:read", "tailscale_device_set_tags"]
  ops:      ["read:*", "group:policy"]
```

Rules:

- Profile names match `^[a-z0-9][a-z0-9_-]*$`. They appear in URLs, so keep them short.
- Every profile needs at least one selector.
- `default`, when present, must name a profile that exists.
- The file is read once at startup. Restart the server after editing it.
- Both YAML list styles work: `["a", "b"]` or one `- item` per line.

The file is passed with `--config <path>` or the `TS_MCP_CONFIG` environment variable.

## Selectors

| Selector | Matches |
|---|---|
| `*` | every tool |
| `read:*` | every tool whose MCP annotation marks it read-only |
| `group:<name>` | every tool in the group |
| `group:<name>:read` | the group's read-only tools |
| `<tool name>` | exactly that tool |

Selectors are additive within a profile. There is no exclusion syntax, so to get "everything in a group except one tool" list the tools you want by name. `--list-groups` prints the names.

Read-only comes from each tool's MCP `readOnlyHint`. GET-backed tools, the validate and preview tools, the local CLI tools, and the read-only curated wrappers carry it. Nothing that creates, updates, or deletes does.

The same grammar works in the ACL grant and in `--local-grants`, so a selector you have tested in a profile can be moved into the grant unchanged.

## Profiles and grants

A profile is a view, not a permission. Each request is authorized by the caller's Tailscale grant, and what the caller sees is the intersection:

| Profile lists | Grant allows | Client sees |
|---|---|---|
| `group:dns` (11 tools) | `*` | 11 tools |
| `group:dns` | `read:*` | the 5 read-only dns tools |
| `*` | `list_all_devices` | 1 tool |
| `read:*` | `group:devices` | the read-only devices tools |

A tool outside the intersection is neither listed nor callable; a call by name returns "tool not found". Call-time grant checks still run underneath, so a profile can never widen access.

Resources are not filtered by profile. Reading one still needs a matching resource grant.

## Choosing profiles

Some shapes that work well:

**Per task area.** One profile per group, as in the example file. Add each to your client as a separate server and enable the one you need. A client with a server toggle effectively gets group toggles.

**Per trust level.** `readonly` as the default, a `write` profile with the mutating tools you actually use, and the ACL grant deciding who may reach the write profile at all. Since grants are per user, two people can point at the same `write` URL and see different subsets.

**Per agent.** A narrow list of named tools for an automated agent, so its context holds only what its job needs:

```yaml
profiles:
  inventory: ["list_all_devices", "get_device_info", "tailscale_list_device_routes"]
```

**Escape hatch.** An `all: ["*"]` profile for when you need something rare, reachable at `/mcp/all`. Your grant still applies.

## Groups

Groups are derived from the Tailscale API path each tool calls, so a tool added upstream lands in the right group automatically. The tables below come from `ts-mcp --list-groups`. Run it yourself after upgrading to see the current set.

### `devices` (32 tools, 8 read-only)

| Tool | Read-only |
|---|---|
| `get_device_info` | yes |
| `list_all_devices` | yes |
| `tailscale_authorize_device` | no |
| `tailscale_batch_update_custom_device_posture_attributes` | no |
| `tailscale_delete_custom_device_posture_attributes` | no |
| `tailscale_delete_device` | no |
| `tailscale_device_authorize` | no |
| `tailscale_device_batch_update_posture_attributes` | no |
| `tailscale_device_deauthorize` | no |
| `tailscale_device_delete` | no |
| `tailscale_device_delete_posture_attribute` | no |
| `tailscale_device_expire_key` | no |
| `tailscale_device_posture_attributes` | yes |
| `tailscale_device_rename` | no |
| `tailscale_device_routes` | yes |
| `tailscale_device_set_ip` | no |
| `tailscale_device_set_posture_attribute` | no |
| `tailscale_device_set_routes` | no |
| `tailscale_device_set_tags` | no |
| `tailscale_device_update_key` | no |
| `tailscale_expire_device_key` | no |
| `tailscale_get_device` | yes |
| `tailscale_get_device_posture_attributes` | yes |
| `tailscale_list_device_routes` | yes |
| `tailscale_list_devices` | yes |
| `tailscale_set_custom_device_posture_attributes` | no |
| `tailscale_set_device_ip` | no |
| `tailscale_set_device_name` | no |
| `tailscale_set_device_routes` | no |
| `tailscale_set_device_tags` | no |
| `tailscale_set_devices_authorized` | no |
| `tailscale_update_device_key` | no |

### `dns` (11 tools, 5 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_get_dns_configuration` | yes |
| `tailscale_get_dns_preferences` | yes |
| `tailscale_get_split_dns` | yes |
| `tailscale_list_dns_nameservers` | yes |
| `tailscale_list_dns_search_paths` | yes |
| `tailscale_set_dns_configuration` | no |
| `tailscale_set_dns_nameservers` | no |
| `tailscale_set_dns_preferences` | no |
| `tailscale_set_dns_search_paths` | no |
| `tailscale_set_split_dns` | no |
| `tailscale_update_split_dns` | no |

### `policy` (7 tools, 5 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_get_acl` | yes |
| `tailscale_preview_acl` | yes |
| `tailscale_preview_rule_matches` | yes |
| `tailscale_set_policy_file` | no |
| `tailscale_update_acl` | no |
| `tailscale_validate_acl` | yes |
| `tailscale_validate_and_test_policy_file` | yes |

### `keys` (5 tools, 2 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_create_key` | no |
| `tailscale_delete_key` | no |
| `tailscale_get_key` | yes |
| `tailscale_list_tailnet_keys` | yes |
| `tailscale_set_key` | no |

### `users` (7 tools, 2 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_approve_user` | no |
| `tailscale_delete_user` | no |
| `tailscale_get_user` | yes |
| `tailscale_list_users` | yes |
| `tailscale_restore_user` | no |
| `tailscale_suspend_user` | no |
| `tailscale_update_user_role` | no |

### `invites` (11 tools, 4 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_accept_device_invite` | no |
| `tailscale_create_device_invites` | no |
| `tailscale_create_user_invites` | no |
| `tailscale_delete_device_invite` | no |
| `tailscale_delete_user_invite` | no |
| `tailscale_get_device_invite` | yes |
| `tailscale_get_user_invite` | yes |
| `tailscale_list_device_invites` | yes |
| `tailscale_list_user_invites` | yes |
| `tailscale_resend_device_invite` | no |
| `tailscale_resend_user_invite` | no |

### `webhooks` (7 tools, 2 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_create_webhook` | no |
| `tailscale_delete_webhook` | no |
| `tailscale_get_webhook` | yes |
| `tailscale_list_webhooks` | yes |
| `tailscale_rotate_webhook_secret` | no |
| `tailscale_test_webhook` | no |
| `tailscale_update_webhook` | no |

### `services` (7 tools, 4 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_delete_service` | no |
| `tailscale_get_service` | yes |
| `tailscale_get_service_device_approval` | yes |
| `tailscale_list_service_hosts` | yes |
| `tailscale_list_services` | yes |
| `tailscale_update_service` | no |
| `tailscale_update_service_device_approval` | no |

### `logs` (8 tools, 5 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_disable_log_streaming` | no |
| `tailscale_get_aws_external_id` | no |
| `tailscale_get_log_streaming_configuration` | yes |
| `tailscale_get_log_streaming_status` | yes |
| `tailscale_list_configuration_audit_logs` | yes |
| `tailscale_list_network_flow_logs` | yes |
| `tailscale_set_log_streaming_configuration` | no |
| `tailscale_validate_aws_external_id` | yes |

### `posture` (5 tools, 2 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_create_posture_integration` | no |
| `tailscale_delete_posture_integration` | no |
| `tailscale_get_posture_integration` | yes |
| `tailscale_get_posture_integrations` | yes |
| `tailscale_update_posture_integration` | no |

### `oauth` (5 tools, 2 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_create_oauth_app` | no |
| `tailscale_delete_oauth_app` | no |
| `tailscale_get_oauth_app` | yes |
| `tailscale_list_oauth_apps` | yes |
| `tailscale_update_oauth_app` | no |

### `contacts` (3 tools, 1 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_get_contacts` | yes |
| `tailscale_resend_contact_verification_email` | no |
| `tailscale_update_contact` | no |

### `settings` (2 tools, 1 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_status` | yes |
| `tailscale_update_tailnet_settings` | no |

### `local` (4 tools, 4 read-only)

| Tool | Read-only |
|---|---|
| `tailscale_local_status` | yes |
| `tailscale_local_version` | yes |
| `tailscale_netcheck` | yes |
| `tailscale_ping` | yes |
## Validation and errors

The server refuses to start on a bad config and says why. The message names the profile and the selector.

| Log message contains | Cause |
|---|---|
| `invalid YAML` | Syntax error in the file. Check indentation and quoting. |
| `profiles must define at least one profile` | The `profiles:` key is missing or empty. |
| `name must match` | A profile name has uppercase letters, spaces, or starts with a symbol. |
| `must list at least one selector` | A profile is `[]`. Delete it or fill it in. |
| `unknown group` | A typo in a `group:` selector. The message lists the valid names. |
| `no registered tool with that name` | A tool name selector is misspelled, or names a local CLI tool while `TAILSCALE_LOCAL_CLI` is off. |
| `only a :read suffix is supported` | Something like `group:dns:write`. Only `:read` exists. |
| `default profile "x" is not defined` | `default:` names a profile that is not under `profiles:`. |
| `Unknown --profile` | In stdio mode, `--profile` names a profile that is not defined. |

At request time, `Unknown tool profile` in the log with a 404 to the client means the URL's profile name does not exist. The log lists the known names.

## Debugging

**Client sees fewer tools than the profile lists.** The grant is narrower than the profile. Look for `Access denied` lines in the server log, or compare the grant's selectors with the profile's.

**Client sees no tools at all.** Either the caller has no grant (`No MCP capabilities found` in the log) or the profile and grant do not overlap.

**Client sees every tool.** The client is on `/mcp` and no `default` is set, or the server was started without `--config`. The startup log shows whether profiles loaded.

**Edits to the file have no effect.** The file is read once. Restart the server.

**A tool is in the wrong group.** Groups come from the API path. Open an issue with the tool name; the mapping lives in `internal/toolmeta`.
