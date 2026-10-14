# Resell v2

`Client.Account().Resell()` manages account metadata, allocation and the
explicitly selected deprecated Resell interfaces. `Client.Project(id).Resell()`
uses project IAM for the allocation operations published with that alternative.
Existing `Account().Projects()` methods and catalog service ID `projects`
remain compatible. The endpoint is `https://api.selectel.ru/vpc/resell`;
exact overrides use service `projects`, generation `v2` and the chosen scope.

| Public API | Contract and identity |
|---|---|
| `Account` | Account IAM; enabled/locked/name/onboarding and nested locks required |
| `DNSCheck` | Account IAM; escaped hostname; documented 400 is not_found |
| `Subnets.Collect/Get/Create/Delete` | Account or project IAM; `resell_subnet` ID is the Resell integer allocation ID |
| `FloatingIPs.Collect/Get/Create/Delete` | List account IAM; remaining operations account or project IAM; `resell_floatingip` ID |
| `Licenses.Collect/Get/Create/Delete` | Account or project IAM; explicit deprecated API; `resell_license` integer ID |
| `Keypairs.Collect/Create/Delete` | Account IAM; explicit deprecated API; `resell_keypair` name with `resell_user` owner parent |
| `Traffic` | Account IAM; typed domain map and exact byte quantities |
| `TrafficReport` | Account IAM; strict calendar dates; bounded protected CSV bytes |
| `PublishedSchema` | Account IAM; bounded protected JSON; does not update SDK coverage |

Resell allocation, Neutron resource association and direct public ports remain
different concepts. `Subnet.SubnetID` and `NetworkID` describe associated
Neutron objects; neither is adopted as the allocation ID. Project creation
requests explicitly provide the target project; a project view rejects a target
outside its selected project before I/O. Provider project IDs, when present,
must agree with the chosen project. No endpoint or scope fallback is attempted.

Collections return one documented provider response under shared collection
limits. Local truncation returns an incomplete result and a cursor tied to
scope, endpoint, generation, contract and filters; it never reports completion.
`Detailed` preserves omission and explicit false. The floating-IP source defines
a string filter, so its separate `FloatingIPListOptions.Detailed` retains that
wire type rather than narrowing arbitrary values to boolean.

Optional mutation bodies may be omitted. Typed nonnil bodies retain all fields,
including `cidrs`, `ips`, `quantity`, regions and keypair owner/public key.
The official SDK omits quantities for its allocation create variants; the SDK
preserves that omission without inventing a quantity/default. Explicit int32
parameters are range-checked; license quantity must be at least one when given.
Request arrays and all returned allocations are retained, never first-only.

Mutations return Receipt on accepted, rejected, not-sent and unknown outcomes.
There is no mutation replay. Create extracts all trustworthy returned identities
before decoding other fields so a malformed accepted response still preserves
known refs. Foreign project/identity and duplicate allocation IDs are rejected.
Exact Get/Delete use scoped endpoint-bound refs; unknown create never adopts an
object by name. No allocation wait, readiness guarantee or root Resume handle is
published; exact Get observes known refs without proving authorship.

## Sources and discrepancies

The [official Resell schema](https://docs.selectel.ru/api/cloud-projects-and-resources.json)
was re-fetched on 2026-10-14. SHA-256
`0545ced4f3c4af315eba1027ebd166240f91bc7e8e7835041cc7626cb191d22e`
matched the pinned artifact. All 25 method/path operations in that snapshot
have catalog bindings, including the five pre-existing Projects operations.

The official [go-selvpcclient revision](https://github.com/selectel/go-selvpcclient/tree/79b0d0ff1d46ebfe7c50056f8c829e0d32349605)
was also checked against upstream HEAD on 2026-10-14. Source snippets and
source-derived schema extensions are pinned in
`catalog/contracts/resell-sdk-supplement.json`; relevant operation contracts
hash that artifact, so changes invalidate stale verification.

Subnet/floating-IP create has singular object envelopes in Swagger and plural
arrays in the SDK. Both independently published shapes decode into typed arrays;
a response containing both envelopes is ambiguous and rejected. SDK-only
`vlan_id`/`vtep_ip_address`, and keypair `region`/`regions` variants are retained
through the model generator. Native SDK quantity omission remains a separate
published request variant rather than a manufactured provider default.

Traffic domain keys are typed map entries, preserving both Swagger `prepared`
and SDK `prepaid`. `TrafficTimestamp.Raw` retains provider text. `UTC` is present
only for timestamps carrying an explicit offset; timezone-free timestamps are
validated but never assigned a timezone. CSV reports preserve bytes and remain
protected from ordinary formatting/serialization; explicit Reveal/CopyTo is
the caller-directed output boundary. Report/schema reads buffer at most
`Config.MaxJSONBytes` under `CallTimeout`; they are not streaming transfers.

## Deprecated disposition and verification

Keypair and license operations remain documented in the current schema and
callable only through their explicit Resell v2 adapter. The source says keypairs
would be removed in 2025 and directs users to Nova. That date, deprecated flag
and a caller's permission error cannot establish withdrawal. No automatic Nova
migration or alternate endpoint retry is performed; provider rejection remains
visible. License APIs similarly retain their deprecated source designation.

This is implemented coverage of the enumerated Resell baseline, with offline
contract tests. No live cloud changes were performed, and availability for a
particular account/project is not established. Other families still have
incomplete inventories, so global coverage_ratio remains null.
