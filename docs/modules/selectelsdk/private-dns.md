# Private DNS and PTR

`RegionalClient.PrivateDNS()` binds the project-scoped regional Private DNS v1
API. `AccountClient.PTR()` binds the separate account-scoped PTR v1 API. These
APIs do not share public DNS zones, authentication, endpoints, or resource refs.

## Sources and coverage

Pinned primary OpenAPI snapshots are in
[private-dns.json](../../../internal/privatednscontractgen/assets/private-dns.json)
and [ip-addresses.json](../../../internal/privatednscontractgen/assets/ip-addresses.json).
The [URL table](https://docs.selectel.ru/api/urls/) directly links the PTR API
reference to `https://api.selectel.ru/ipam/v1`; use its schema-relative `/` and
`/{ptr_id}` paths. Private DNS addresses end in `/private-dns/v1` in the exact
published pools (ru-6 has no published default). Explicit endpoint overrides
remain caller choices and do not establish provider availability.

Private DNS implements all 13 current remote operations: service and zone CRUD
and list, recordset list, PUT replacement, and PATCH value changes. The pinned
[published SDK](https://github.com/selectel/private-dns-go/tree/606a8a34e1607e29dc0e1e354aa58c4559cfb8fd/pkg/v1)
confirms optional recordset list `type`/`domain` filters; current OpenAPI omits
them. These filters are explicit native SDK request variants with no fallback
and no live availability claim. Current zone name update, project filters,
generated flags and value PATCH are retained even though the older SDK omits them.

PTR implements four CRUD operations. Its list binding is partial: ordinary
limit/offset/filter reads are supported, but `only-count=true` describes a count
while its response schema still declares an array of records. `Count` returns
`capability_unavailable` without I/O until a primary source supplies the count
response shape. No count is inferred from a page's length. Service inventory
remains incomplete and coverage_ratio is null. All evidence is offline, not live.

## Requests and responses

Private DNS uses project IAM `X-Auth-Token`, with explicit account, project,
region and pool. PTR uses account static `X-Token` only; no IAM fallback.
Provider `project`/`user_id` fields are metadata and do not replace authorization
scope. Exact references preserve the caller scope and endpoint. Get, mutations,
parent recordset lists and resumed observations pin that endpoint and never
send old identifiers to a newly resolved address.

Private DNS service create takes network_id UUID and optional recursor flag;
update preserves omitted vs false. Zone create accepts name/domain, optional
TTL and initial recordsets; update accepts name and TTL. ReservedBy and Generated
are provider metadata/constraints, not client-side authorization proofs.
Records.Replace is PUT with explicit set/delete collections: each set replaces
that recordset's values. Records.PatchValues is PATCH with add/delete values,
not replacement. Empty arrays and omitted arrays remain distinct. TTL zero
(zone), -1 (record inherits zone TTL), and absent defaults are distinct; SDK
never inserts provider defaults. Integer TTL/serial/IDs retain exact precision.

All record values and PTR content use ProtectedValue. Formatting and JSON are
redacted; only private request serialization or caller-directed Reveal exposes
bytes. Returned refs/owner metadata are never serialized into request DTOs.
Request and response models and source constraints are generated offline.
Documented errors map by status, numeric Private DNS machine codes remain exact,
and provider text is discarded. No provider-text classification is performed.

## Collection and mutation guarantees

Private DNS returns complete single provider pages. ListPage/Iterate/Collect
apply bounded local pages and preserve incomplete results/cursors. PTR advances
offset by observed rows and only treats an empty provider array as the end;
short pages do not imply completeness. Cursors bind scope, generation, endpoint
URL, request filters and starting offset; protected filters use private digests.

Mutations retain Receipt on validation, transport, HTTP and decode errors and
are never replayed. Trustworthy accepted provider IDs are retained before full
DTO decoding; malformed identities are not adopted. Unknown create without a
returned ID cannot recover through network/IP/name search. Known receipts may
Recover conclusively without I/O even without a target; Status/Wait then cannot
invent resource state. Root Client.Resource/Resume delegate exact references.

Successful exact GET reports ProviderState `present`, Classification `succeeded`.
Only service/zone/ptr present and absent goals exist. Create receipts allow
present; delete receipts allow absent; updates and recordset changes have no
implicit wait goal. Existence is not resolver reachability, DNS propagation,
cache expiry, delegation, or readiness of a private network consumer.

```go
project, err := client.Project("project-id")
if err != nil { return err }
regional, err := project.Location("spb", "ru-1")
if err != nil { return err }
result, err := regional.PrivateDNS().Services.Create(ctx,
    privatedns.ServiceCreate{NetworkID: "b706af9b-b048-42a5-a87d-d69a601abec8"},
    types.CallOptions{})
// Retain result.Receipt even when err != nil; never repeat an unknown mutation.
```

Shared recovery/revision behavior follows reviewed commit `379458e`; optional
numeric diagnostics follow reviewed `095faea` and retain normal status mapping,
credential refresh and safe-read retry even if the diagnostic body is unreadable.
