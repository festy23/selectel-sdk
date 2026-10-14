# Resell v2

`Client.Account().Resell()` reads account status with `Account` and inspects
DNS delegation with `DNSCheck(ctx, hostname, options)`. Both use account IAM,
the existing `projects` catalog service ID and
`https://api.selectel.ru/vpc/resell`, preserving the Projects API.
The generated response DTOs retain optional fields and explicit false values;
account enabled/locked/name/onboarding and each lock's fields are required.
Neither read invents a resource reference or a wait/recovery guarantee.
DNS hostname is escaped as one path segment; documented 400 maps to not_found.

The source snapshot was re-fetched on 2026-10-14 from the official
[Resell schema](https://docs.selectel.ru/api/cloud-projects-and-resources.json);
SHA-256 `0545ced4f3c4af315eba1027ebd166240f91bc7e8e7835041cc7626cb191d22e`
matched the pinned artifact. Verification remains offline, with no live calls.

Remaining create allocations, floating-IP and license/keypair operations are unimplemented.
Deprecated keypair and license operations remain documented and unimplemented:
the current source still publishes them. The stated 2025 removal date alone
is not evidence of withdrawal, and a permission error cannot prove withdrawal.
Their eventual adapters must retain explicit generation and disposition.
Resell subnet/IP allocation refs must be distinct from Neutron resources.

## Traffic and subnet allocation reads

`Traffic` retains typed paid/prepared/used values with arbitrary-precision
integers and normalizes documented RFC3339 timestamps to UTC.
`TrafficReport` accepts strict start/end calendar dates and returns protected CSV
bytes, bounded by `Config.MaxJSONBytes` and `CallTimeout`. It buffers at most the
configured limit; this read is not a streaming transfer. `PublishedSchema`
returns protected arbitrary JSON and does not update the embedded SDK catalog.

`Subnets.Collect` applies bounded collection semantics to the fixed provider
list and preserves explicit `Detailed=false`. `Subnets.Get` and `Delete` use
exact account-scoped `resell_subnet` refs. Integer Resell IDs preserve precision;
`subnet_id` identifies the associated Neutron subnet and is never adopted as
an allocation ID. Unknown deletion returns a Receipt and is not retried;
no deletion wait or root recovery handle is promised.

The official SDK at revision
[`79b0d0ff1d46ebfe7c50056f8c829e0d32349605`](https://github.com/selectel/go-selvpcclient/tree/79b0d0ff1d46ebfe7c50056f8c829e0d32349605)
uses plural arrays for allocation create responses while the current Swagger
schema describes singular objects for subnet and floating-IP create.
SDK subnet DTOs also publish `vlan_id` and `vtep_ip_address`, absent in Swagger.
These differences remain an explicit coverage gap; the Swagger subnet read
binding is partial relative to the combined SDK/API inventory.
