# Email Service v1

`client.Project(projectID).Mail()` implements the management operations in the
pinned Email Service Swagger. Service identity in refs/catalog is `email`, API
generation `v1`, authentication is project IAM (`X-Auth-Token`). No SMTP send
method is implemented or invoked.

## Endpoint and scope

The current schema has no host/basePath/servers, the official API page uses
relative paths, and the official URL table contains no Email Service entry.
Additional primary-source searches did not establish a management endpoint.
This is a search limitation, not a claim that no official endpoint exists.
An exact `Config.EndpointOverrides` entry for service `email`, generation `v1`
and the explicit account/project scope is required. The SMTP relay host is a
different protocol and is never substituted. Local errors return mutation
`not_sent` receipts before credential lookup or HTTP.

`Resource(UUID)` constructs a local exact reference. Foreign service/generation,
project, endpoint or parent refs are rejected. Provider identity/project must
match the caller scope. Transfer requires the explicit target project and permits
that project in response metadata, while receipt and resource ref retain the
originating scope. It does not adopt a target-scoped ref or retry after a lost
response. No name lookup is used to recover a mutation.

## Methods and protection

The 20 source operations cover project/resource quotas; resource list, create,
get, rename, delete, password rotation and transfer; domain list, link, unlink
and verification; delivery statuses, complaints, undelivered reports and
statistics; stop-list search, add and remove. DTOs are generated locally from
the pinned source, including inline pagination objects. Optional false/zero
remain distinct from absence; numeric counters and issue IDs stay exact.

SMTP login/password, DKIM keys, domains, message/tracking identifiers, addresses,
reasons and user text use protected values. Unknown extensions are protected.
Provider-injected SDK refs are ignored. Explicit typed request values reveal
only while building body/query. Provider error text is not copied to diagnostics.
Creates scan source ID/project before decoding independent credential fields so
accepted receipts keep independently trustworthy identities on decoding errors.

All nine mutations retain receipts on every path. HTTP rejection sets are bound
per operation; a server error/lost response remains unknown and never replays.
The source's SMTP rate-limit prose does not establish a management API rejection
contract. A positive HTTP acknowledgement is not proof of application readiness.

## Bounds and source gaps

Lists use `pagination.Options` with scope/operation/revision/filter-bound cursors,
limits and exact partial-page replay. Provider envelopes retain pagination
metadata with duplicate rows removed. Strictly advancing integer offsets are
followed. Four list operations remain **partial** because the source does not
define a terminal sentinel for `next_offset`/`cursor_next`: zero, absence or
nonprogress returns `protocol_error`, preserving already issued rows and never
asserting a complete inventory. Missing arrays are not replaced by empty success.

Statistics remains **partial**: its response defines `limit` and `next_offset`,
but the request defines only date bounds. No continuation query is invented and
full-period traversal is not claimed. Statistics, delivery messages and stop-list
nested collections have `ReadBounds.MaxItems` (zero uses Config.MaxItems).
Exceeding it returns `response_limit` with a typed prefix; aggregate counters and
provider metadata remain available. Neither a bounded response nor a zero
next_offset proves full-period completeness.

The resource-quota path contains `{ses_id}` but omits its parameter declaration;
the SDK binds that explicit UUID placeholder as the documented path requires.
These gaps remain owned by issue #63. The current claim is 15 implemented and
5 partial within the enumerated 20-operation subset. Whole-service inventory is
incomplete and coverage_ratio remains null.

## Observation

Root `Resource(ref)` observes the exact email resource. `active` is succeeded;
`blocked`, `error`, `deleted` are failed, future/missing states stay unknown.
A `deleted` response does not prove absence: only exact GET NotFound satisfies
`resource_absent`. Receipt-bound create permits `resource_active`, delete permits
`resource_absent`; all goals are filtered by the originating catalog capability.
Receiptless observation supports both. Observing active cannot prove password
rotation, transfer, domain linkage or stop-list effects; those mutations expose
no invented wait goal. Status/Recover observations never authorize replay or
infer newly appeared resources.

## Evidence and example

The [snapshot](../../../catalog/sources/email-service.json) and
[source evidence](../../../catalog/evidence/email-current-sources.json) record
the revision, endpoint search and source discrepancies. Tests use the public
Client with synthetic credentials and only HTTP transport substitution. Offline
evidence does not claim live deployment compatibility.

```go
// Configure an explicit confirmed management endpoint for the exact scope first.
service := project.Mail()
ref, err := service.Resource(resourceUUID)
if err != nil { return err }
result, err := service.GetResource(ctx, ref, types.CallOptions{})
// result.Data.Resource.Password stays protected unless the caller reveals it.
```
