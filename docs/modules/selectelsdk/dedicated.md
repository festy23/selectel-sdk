# Dedicated V2 lifecycle

`Client.Account().Dedicated()` chooses account IAM.
`Client.Project(id)` followed by `ProjectClient.Dedicated()` chooses project
IAM. Both are published authorization schemes; methods never switch scope or
auth mode after failure. The official URL table and native SDK select
`https://api.selectel.ru/servers/v2`; the Swagger descriptive `/client/v2`
base path is not concatenated onto it.

The pinned [schema](../../../catalog/sources/dedicated.json) inventories the
whole Dedicated API. The explicit
[lifecycle selection](../../../catalog/evidence/dedicated-lifecycle-scope.json)
binds 55 operations for #57: locations/price plans, server/serverchip/custom
service catalogs and billing calculations, resource ordering/details/update/
transfer/cancel/restore/upgrade, power/console, boot/OS/custom images and task
observation. Disks, network, SSH keys and remaining groups belong to #58 and
remain unimplemented in this branch. Dedicated inventory completeness and
the global coverage denominator remain unknown.

Methods, request/query/response DTOs and catalog bindings are generated from
local pinned snapshots. Every method is typed; there is no public raw executor.
Deep allOf composition is retained. Counts, timestamps-as-epochs and prices use
exact Integer/Number values; the SDK does not invent currency scaling. Optional
nullable fields distinguish omission, null and value. Passwords, userdata,
scripts, keys, console URLs, names and descriptions are protected. Formatting
and JSON redact values; request encoding reveals them only at the HTTP boundary.

## Replies and effects

`Reply[T].HTTPStatus` preserves the status. HTTP 200/201 typed schemas and the
published generic HTTP202 envelope select different reply variants through a
status-aware internal core adapter. The original DoJSON API retains its behavior.
`Data` means a typed response, not completed work. `Queued` represents the
published HTTP202 generic envelope. Accepted receipts are established before
decoding; trustworthy task/resource IDs are retained on later protocol errors.
Source task results with no declared properties remain ProtectedJSON.

The task endpoint's published HTTP303 response is decoded in place. Its
Location is ignored and credentials are never forwarded to it. Task status is
free-form in the sources: exact `GetTask` is supported, but no terminal-status
or typed task-result completion guarantee is invented. An HTTP202 order with
only a task ID cannot bind a resource waiter until an exact resource ID is
known; there is no name lookup or adoption of a newly appeared server.

POST/PUT service billing calculations are safe reads and are distinct from
resource billing/order mutations. Mutations execute once, retain receipts and
exact request refs on errors, and never replay unknown outcomes. Cancelling a
resource is a billing/lifecycle action with an explicit `immediately` field;
it is not silently treated as physical hardware deletion.

Ref kind, generation, immutable scope and endpoint are preserved. Resource,
service catalog, location, plan, custom-image and task refs remain distinct.
Existing explicitly scoped resource parents are retained and any returned
primary_uuid must agree. Otherwise primary_uuid remains metadata: its owner
scope is not inferred from a child row.
Project assignment colliding with a project view is rejected before I/O, except
the explicit transfer operation. A project-scoped transfer retains its original
receipt/ref and target metadata without adopting the new project scope or
returning a stale resource Data.Ref. Callers explicitly select the target view
for subsequent observation.

## Source variants and bounded observation

Current `PartitionsConfigModel` uses a literal `*` property; native SDK publishes
a map of named typed nodes. `PublishedPartitions` and `NativePartitions` keep
these variants explicit and decode both proven shapes. Source evidence is
pinned in [native variants](../../../catalog/evidence/dedicated-native-variants.json).
Other literal wildcard schema properties are retained as published; additional
dynamic wire variants are not inferred from errors or from SDK omissions.

Lists make one bounded provider request and preserve returned metadata.
There is no invented collection completeness from item count or short pages.
CallTimeout and MaxJSONBytes bound every reply; documented page/limit parameters
are caller-selected. Errors from typed parameters pass the core finalizer
before credential or resource I/O. The read-only
[example](../../../examples/dedicated-page/main.go) prints counts only.

Explicit `PowerHandle`, `ResourceHandle` and `OSHandle` bind validated exact refs
and compatible revision-bound receipts. Their bounded waits use the pinned
[provider field semantics](../../../catalog/evidence/dedicated-wait-sources.json):
stable physical power on/off; resource active or non-renewing; OS reinstall0
with no processing. These goals do not establish application readiness,
password usability or an observed reboot cycle. Unknown receipts remain
indeterminate; observation does not prove authorship of an unknown write.
Both power_state and target_power_state must be present and non-null; stable power
requires an explicit empty target. OS reinstall and is_processing must both be
present and non-null; installed requires reinstall0 with explicit false processing.
Missing/null goal fields return ProtocolError; unknown flag values do not become
success. Status classifications are pending/succeeded/failed/unknown according
to these field semantics, retaining unknown raw ProviderState. A resource may
satisfy one named goal while another remains pending (active versus non-renewing).
Task404 is not completion.
Root Resume is not implemented for this service.

Offline tests use real public Client/scoped methods, synthetic credentials and
only Config.Transport substitution. No live server operation or live coverage
is claimed. Per-operation contracts pin request/response/error mappings,
status schemas, source revisions, sensitive fields and evidence dependencies.
