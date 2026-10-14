# Cloud backup V2

`Client.Project(project)` followed by `ProjectClient.Location(region, pool)`
and `RegionalClient.Backups()` selects immutable project IAM scope and API
generation V2. The account is selected in client configuration; see
[interface.md](interface.md).
`Config.Transport` is the HTTP seam; the service uses the shared credential,
redirect, deadline, byte-limit, error and receipt rules.

The pinned [current schema](../../../catalog/sources/scheduled-backups.json)
implements ten operations: plans list/get/create/update/delete, checkpoints
list/get/delete, chain delete, and restore create. The [official API page](https://docs.selectel.ru/api/scheduled-backups/)
directly documents restore; its absence from the native SDK does not remove it
from API coverage. No exact restore observation or wait endpoint is published
in this snapshot, so the binding provides no restore wait or readiness claim.

## Source variants

The [native source snapshot](../../../catalog/sources/scheduled-backups-sdk.json)
is pinned to `selectel/cloudbackup-go` revision
`76292b3d4552165d97106dc6aa197c8cd7425233`. Its six methods use `/plans/`,
`/plans/{id}` and `/checkpoints/`, with retention `full_backups_amount`.
The current schema uses `/{project_id}/...` and retention `max_backups`.
Callers explicitly select `.SDKV2` for the native source contract; current
`Plans`, `Checkpoints` and `Restores` use the current schema. There is no alias,
fallback or cross-variant reference acceptance. `sdk_plan` and `sdk_checkpoint`
references remain distinct from current references. Native struct zero/empty
JSON encoding is preserved. Native SDK source establishes these wire shapes;
o live availability or withdrawal is inferred.

The [official URL table](https://docs.selectel.ru/api/urls/) supplies
`https://<pool>.cloud.api.selcloud.ru/data-protect/v2` for ru-1, ru-2, ru-3,
ru-6, ru-7, ru-8, ru-9 and gis-1, and
`https://<pool>.cloud.api.servercore.com/data-protect/v2` for uz-1, uz-2, kz-1
and ke-1. No GIS-2 address is inferred. Exact endpoint overrides retain the
same immutable scope and generation checks.

## Data and execution

Generated DTOs retain every published field. Retention counts and totals use
arbitrary precision integers. Nullable `backups.Optional[T]` separates null
from a present value; a nil pointer means omission. Explicit false, empty and
null patch values survive request encoding. Resource names and descriptions
use `types.ProtectedValue`; diagnostics, JSON and generic formatting redact
these values. `Reveal` is caller-directed. Request encoding reveals protected
values only at the HTTP boundary. RFC3339 timestamps are checked; ordinary
required timestamps normalize to UTC, optional timestamps preserve their
parsed offset.

`Plans.ListPage`, `Checkpoints.ListPage` and native page methods make one
bounded caller-selected request. They preserve provider totals and records.
Callers supply the next marker explicitly: this source does not establish a
snapshot, completion token or short-page completeness guarantee. `CallTimeout`
and `MaxJSONBytes` bound each request; no collection is claimed complete.

Exact get/update/delete methods require service, generation, scope, endpoint
and kind-compatible `ResourceRef`. Current checkpoint refs carry the plan
parent, and checkpoint item refs carry their checkpoint parent. Restore create
requires the selected item ref and checks returned project, checkpoint and
item IDs before retaining a new restore identity. Returned resource metadata
is not adopted as an independently managed Cinder resource.

Mutations run once. Accepted malformed responses retain accepted receipts and
independently trustworthy IDs. Chain deletion scans all returned identities
before reporting a malformed row, preserving other trustworthy refs.
Transport loss or undocumented rejection remains an unknown outcome; no
mutation replay, name search, adoption, root Resume or inferred authorship
is provided. Provider error bodies are discarded from diagnostic errors.

The per-operation mappings in `catalog/contracts/backups.v2.*.json` record
exact source paths, status codes, DTOs and evidence dependencies. Offline tests
use the public client with a synthetic credential source and only replace the
HTTP RoundTripper. They do not establish wire/live verification or whole-SDK
coverage; the global denominator remains incomplete.
