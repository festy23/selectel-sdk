# CDN v3

`client.Project(projectID).CDN()` uses project IAM and the documented fixed
`https://api.selectel.ru/cdn/v3` endpoint. An explicit endpoint override remains
part of every exact reference. No region or pool is inferred or required.

## Enumerated operations

The pinned public Swagger describes 19 operations: resource list/get/create,
PATCH and PUT, resource status get/batch, cache task create/get, certificate
list/create/get/update/delete, wildcard and resource Let's Encrypt orders and
order status, origin and delivery node lists. Eighteen bindings implement their
published shapes. Certificate get remains partial: `details=true` is sent and
typed `CertificateInfoWithChain` is decoded, but the prose promises PEM content
without publishing its field names or types. Unknown extensions are protected;
they are not a typed PEM contract. This gap remains owned by issue #62.
Whole CDN inventory completeness is unknown and the coverage ratio is null.

There is no DELETE resource path in the snapshot despite introductory prose
mentioning deletion. Explicit `active=false` updates deactivate a resource.
PATCH changes first-level fields; PUT replaces configuration. Both retain the
schema-required `name`. `ResourceSpec` excludes source read-only fields.
Optional pointers preserve false, zero and empty collections. The SDK does not
inject defaults, merge nested settings, or accept arbitrary JSON mutations.

## Typed payloads and protection

Models and request validation are generated from the local pinned Swagger.
Required fields, input enums, name patterns and numeric limits are checked before
credentials and HTTP. Free strings, origin credentials, headers, domains,
paths, certificate material and unknown extensions use protected values.
Explicit typed mutation payloads reveal them only while encoding the wire body;
JSON and diagnostic formatting remain protected. Integers retain exact values.
Source-defined open response states remain observable rather than fabricated.

`Resource(id)`, `Certificate(sequence)` and `CacheTask(id)` construct exact local
references without I/O. Cross-service, generation, project or endpoint references
are rejected. Returned identities must match supplied references. Creates retain
trustworthy source IDs even if later payload decoding fails. Names never create
or adopt a reference. Configuration `task_id` is metadata and is not a cache task.

## Collections, receipts and observation

Provider lists have a single fixed page. Caller pagination bounds and continuation
cursors still apply, including incomplete collections. `Listing.Items` contains
the bounded rows; `Listing.Envelopes` retains protected envelope metadata with
its duplicate `Data` removed. Status batches accept at most 50 exact resource
references and use the documented CSV query. Omitting IDs permits an empty
response, not an implicit all-resource inventory.

Every mutation returns a receipt, including local `not_sent` failures. Only
operation-specific published HTTP rejection statuses are conclusively rejected;
other outcomes remain unknown. HTTP 200 with a logical provider error remains
accepted at the HTTP boundary. Numeric logical error codes are mapped per
response model because identical numbers have different source meanings.
Errors do not expose provider messages. Unknown mutations never replay.

Exact-reference handles support `cache_processed` (`ok`, `failed`,
`in_queue`/`processing`), `configuration_observed` (`Completed`, `Error`,
`Processing`), `certificate_present`, `certificate_absent`, and
`certificate_order_processed` (`processed`, `failed`, `accepted`). Missing or
unknown goal fields cannot complete a wait. A cache rate of 98 is not proof
that all content is ready. Configuration observation describes current resource
state, not causality for a particular configuration task, DNS propagation or
application health. Let's Encrypt processing does not assert TLS readiness.
Wildcard orders return no pollable identity and do not acquire an inferred one.
Receipt-bound resume checks the original operation, contract revision and exact
reference before observation.

## Evidence

The source snapshot is [catalog/sources/cdn.json](../../../catalog/sources/cdn.json).
The [source evidence](../../../catalog/evidence/cdn-current-sources.json) records
the official API page, endpoint table, snapshot hash and discrepancies. Public
Client tests substitute only HTTP transport and synthetic credentials. Offline
verification does not assert live deployment compatibility or mutate cloud
resources.
