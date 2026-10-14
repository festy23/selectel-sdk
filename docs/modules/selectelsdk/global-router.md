# Global Router V1

`Client.Account().GlobalRouter()` uses account IAM at the native SDK's published
`https://api.selectel.ru/naas/v1` endpoint. The account comes from client
configuration. Exact endpoint overrides remain bound to account and generation.
The pinned [current schema](../../../catalog/sources/global-router.json) binds
24 remote operations: router/network/subnet/static-route CRUD and four
paginated directory reads (zones, services, zone groups and quotas).

`Routers`, `Networks`, `Subnets` and `StaticRoutes` expose typed methods. `Get`
returns provider states including failure, without treating accepted creation
or ACTIVE as application readiness. Router `GetOptions.Nested` requests the
published nested tree. Other Get methods reject an undocumented nested option.
No automated waiter or root Resume is claimed.

## Attachments and source identities

Global Router network creation attaches or establishes a connection for the
selected service/zone; it does not imply an independently adopted Neutron
network. Native SDK VPC request structs additionally publish `project_id`,
which is absent from current NetworkCreate/SubnetCreate schemas. The pinned
[supplemental native source](../../../catalog/evidence/global-router-sdk-models.json)
records this discrepancy at revision
`f4e78a2c9d3435a9caa32f92a83910e9fab00fd4`. `Networks.AttachVPC` and
`Subnets.AttachVPC` take explicit original service refs, preserving their
project/location/generation/endpoint in the Receipt. They use the same POST
operation bindings, without duplicating coverage. Current typed create DTOs
also represent VLAN, inner VLAN, VMware VDC and source OS identifiers.

Global Router refs use immutable account scope and preserve same-service
router/network/subnet parents. Returned account and ancestor IDs are checked.
Caller-selected source project, OS resource and zone IDs are checked before
adopting a returned attachment ID. Other service IDs remain metadata; source
refs are retained only when explicitly supplied by the caller. No name search,
newly appeared resource adoption or fallback across service scopes occurs.

## Execution and data

Delete calls expose explicit nullable `cascade` and `disconnect` boolean
options. A true cascade can also delete original VPC networks/subnets;
disconnect preserves original resources. Nil leaves the published provider
default untouched. The method selects no flags on the caller's behalf.
Updates use the documented PUT endpoints, preserving empty arrays and strings.

`PageOptions` supports published page, limit (1–200), sort keys/direction and
filters; zone and zone-group reads add their documented nesting booleans.
Each method returns one bounded `Page[T]`. There is no published next token,
total or completion guarantee in these arrays; short pages do not establish a
complete snapshot. `CallTimeout` and `MaxJSONBytes` bound each operation.

`WithHeaders` creates an immutable view for documented language and UUID-v4
request ID headers. Credentials and authorization headers remain core-owned.
Generated DTOs cover nested and ordinary response fields, preserve optional
values, exact integer quotas/VLAN/VPN IDs and UTC-normalized RFC3339 timestamps.
Names, descriptions, VDC names and options use protected values. Formatting
and JSON redact them; only caller-directed Reveal and private request encoding
reveal values.

Mutations run once and retain receipts on errors. Accepted malformed replies
retain trustworthy IDs before later DTO decoding. Unknown transmission outcomes
require exact-reference observation and never trigger replay. Provider error
bodies are excluded from diagnostics. The read-only
[example](../../../examples/global-router-page/main.go) prints record counts only.

Source inspection and tests at public Client/Config.Transport boundary provide
offline evidence. They establish neither live verification nor whole-SDK
coverage; the global denominator remains unknown. Per-operation contracts in
`catalog/contracts/global_router.v1.*.json` pin paths, parameters, statuses,
source revisions, typed mappings and evidence dependencies.
