# Registry management v1/v2

The existing registry lifecycle API remains compatible. This extension covers
13 remaining current management operations: v1 cluster integrations, garbage
collection start/size, repository list/get/delete/cleanup, image/tag lists,
manifest delete and layer lookup, plus v2 repository/image lists. Token lifecycle
belongs to #49; OCI data-plane transfer belongs to #50. Generations are explicit
and never switched after an error.

Registry/repository/image references retain exact account/project/location view,
endpoint and generation. Repository and opaque image refs retain their parent
chain; returned tags do not silently resolve/adopt a digest. Mutations retain
receipts and never replay unknown outcomes. GC initiation does not prove GC
completion and has no invented task ID or readiness predicate.

Source: [CRAAS v2.2](https://docs.selectel.ru/api/craas.json), SHA-256
`a6e68b67cd861617e5d66304891d4523f685593b36fbcdb4538ffc76de2bb314`,
and official [craas-go](https://github.com/selectel/craas-go/tree/8f0d0cba0b59360f344b3b2d32157b1549c2645a)
(no revision drift on 2026-10-15). Source-bound SDK-only response variants remain
explicit and do not assert current server availability. Wire/offline checks use
synthetic credentials and HTTP transport only; no live registry change is made.

## Typed calls and pagination

`V1.Registries.StartGarbageCollection` preserves optional `DeleteUntagged=false`;
`GarbageCollectionSize` returns exact Integer fields. `V1.Repositories()` exposes
`Ref`, `Get`, `Delete`, `Cleanup`, `Iterate/ListPage/Collect` and
`IterateTags/TagsPage/Tags`. Cleanup preserves omitted versus explicit empty
`digests`/`tags` and false `disable_gc`. A nonempty `failed` array returns typed
results and an Accepted receipt with `provider_error`, reason `partial_success`;
provider failure text is protected. No per-item retry occurs.

`V1.Images()` exposes opaque `Reference` (tag or digest address), explicit
`Manifest` (caller-selected digest address), `Layers`, `DeleteManifest` and
`Iterate/ListPage/Collect`. Alias reads retain the requested alias; returned
digest metadata never changes its identity. DeleteManifest requires the manifest
kind before I/O. A requested address does not prove that a resource exists.
Repository names and references are escaped once as individual path segments,
including slash, query, fragment and dot-only segments; parent chains are exact.

`V2.Repositories()` and `V2.Images()` expose generation-specific lists. v2 refs
come from the v2 registry list or explicit v2 parent constructor; no v1 adoption.
Repository lists support search, sort_field (name/updatedAt/size), sort_type,
optional short and initial offset. Short=true permits only name sorting. v2
images support search and offset. Iterators snapshot filters and parent refs
before cursor fingerprinting. v1 repository traversal ends on an empty array;
v2 uses filteredCount, preserving totalCount and filteredCount as provider
TotalCount and ReportedCount. An empty v2 page at or beyond filteredCount is
terminal, including when the count shrinks between calls. Empty pages before
filteredCount remain incomplete; nonempty pages extending beyond it return
protocol_error. No count proves a consistent mutable snapshot.
Unpaginated images/tags/clusters use bounded fixed-page replay/skip cursors,
without invented limit/offset parameters. Malformed rows retain the validated
prefix with the original error and Complete=false; terminal Next does no I/O.

`V1.Registries.IterateClusters/ClustersPage/Clusters` retains source cluster
id/name/pool and totalCount without converting these into MKS handles. A count
that differs from the unpaginated result produces an incomplete collection.

## Source variants and limits

Two repository list bindings remain partial. Current CRAAS describes short=true
as names-only but declares the same object response schema. Typed object and
name-only-object results are supported; a string-array short response is not
invented. Resolving this inconsistency is owned by #48/P1 implementation, #74
source evidence and #75 acceptance. All inventory denominators remain unknown;
coverage_ratio is null. Token operations stay in #49 and OCI transfer in #50.

`RepositoryListOptions.NativeEmpty204` explicitly selects the pinned public Go
SDK's v1 empty-list status variant. `V1.Images.NativeLayers` explicitly selects
that SDK's array-of-Layer response instead of the current OpenAPI Image object.
Neither is automatic fallback, replay, or a claim of current server availability.

Root `Resource`/`Resume` support exact v1 registry, repository, manifest and alias
observations; v2 has no source-confirmed exact read and returns unavailable.
Status reports provider_state=present and classification=succeeded as existence
only. Resource handles offer exists/absent goals; delete receipts offer absent.
GC and cleanup receipts offer no readiness goal. Unknown recovery reads the exact
address and remains indeterminate even if present/absent; it never proves causal
mutation success. Known accepted/rejected/not_sent receipt-only recovery is
conclusive without I/O, with account/schema/revision/effect validation.

Provider date-time fields require an explicit offset and normalize to UTC.
Nullable updatedAt preserves omitted/null/value. Arbitrary integers retain exact
precision; malformed fields return protocol_error. Wire/offline tests do not
establish live availability or mutation completion.
