---
name: selectelsdk
description: Use the repository Go selectelsdk to inspect or manage Selectel resources, billing, telemetry and secrets. Discover supported operations, establish explicit access and scope, execute typed service methods, and handle pagination, receipts and waits. Use for Selectel SDK cloud operations; Terraform state changes remain in the infrastructure workflow.
---

# Selectel SDK

Use the Go module in [this repository](../../../README.md).
Read its current README and the selected service method before building a call.
The SDK is a library: run a small Go program from that module directory, with
the temporary source file outside the repository. A CLI or MCP is not required.
Read [usage.md](references/usage.md) for the runnable discovery/auth/read pattern.

## Discover the operation

Start with `catalog.Default().List` or `Describe`; these perform no I/O and need
no credentials. Read the capability's GoMethod/InputType/OutputType and its
request/response artifacts through `Catalog.Artifact(ref)`. The manifest and
service implementations in this checkout are the current capability source.

- `unimplemented` or a missing GoMethod means the call is unavailable.
- `partial` supports only its covered schema; inspect ExcludedVariants before use.
- `offline` proves local tests for that revision, not successful access to Selectel.
- Full API coverage requires `inventory_complete=true`; an unknown denominator
  has `coverage_ratio=null`.

For a new operation, follow the [extension contract](../../../docs/modules/selectelsdk/services.md#расширение-покрытия).
Implement and verify its binding before adding it to supported coverage. The capability catalog and typed bindings are the runtime authority.

## Choose access and target

Keep account, project and location explicit. Choose Account(), Project(id) and
Location(region,pool) for the capability's ScopeRequirements. Pool and segment
are distinct; derive a region from a pool only through the SDK location registry.
Refs returned by the SDK retain their scope, endpoint, generation and parent.

Use an explicitly selected CredentialSource: environment prefix, named profile
or a supplied keyring Store. Follow [access.md](../../../docs/modules/selectelsdk/access.md)
for auth schemes and source bindings. New and scope factories do no I/O; service
calls authorize lazily. Authenticate is useful when diagnosing access, not a
mandatory separate call before every request. IAM password material refreshes
automatically; external tokens must be renewed by their configured source.

Use existing access and authorization from the session. Inspect credential
availability without printing values; ask for missing source/target information
only when needed. Reading credentials for SDK use is distinct from permission to
change cloud resources. Execute mutations only for the action and target already
authorized by the user; this skill does not authorize provisioning or deletion.
Use the capability's auth scheme; a 403 keeps the same target and rights.

## Execute and interpret the result

Use typed methods with context deadlines. For inventory/list calls, set finite
MaxItems/MaxPages and inspect Complete, StopReason and NextCursor. Preserve
partial data and branch errors; an empty result after an access error is not a
successful empty inventory. Bound logs and range metrics by the requested time.

For a mutation, retain its Receipt even on error. `accepted` means accepted by
the provider; completion is a separate allowed Wait goal. `unknown` means it
may have applied: use Resume/Recover only with exact known refs and report
indeterminate evidence. The SDK does not replay unknown mutations. Detailed
rules are in [operations.md](../../../docs/modules/selectelsdk/operations.md).

For MKS nodegroup creation, 204 returns no new group/task ID. Keep the known
parent ref; use explicitly identified child refs later. Parent readiness does
not prove that the new groups finished. Active goals confirm current provider
state; they do not confirm application health or a particular resize count.

ProtectedValue/ProtectedJSON format and serialize as redaction. Reveal/CopyTo
only for the requested use, such as handing kubeconfig to a selected process.
Keep credentials, secret values, kubeconfig, user-data and raw log/error payloads
out of Git, console output and reports. NodegroupCreateSpec.UserData is already
base64 text; Nova UserData is raw bytes encoded once by its own binding.

Report scope, operation IDs, outcomes, completeness and verification level.
Distinguish offline evidence from calls actually completed against Selectel.
Use [verification.md](references/verification.md) when assessing or extending
test coverage; local green tests do not establish live permissions or service
availability.
