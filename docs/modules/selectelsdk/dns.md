# Public DNS v2

`ProjectClient.DNS()` exposes current public DNS v2 through `Zones` and `RRSets`.
Its 13 bindings cover the complete operation set of the pinned current OpenAPI:
zone Create/Get/Delete/List, comment update, disabled state, deletion protection,
owner transfer, and RRSet Create/Get/Update/Delete/List. Each list has ListPage,
Iterate and Collect forms. There is no generic zone replacement method.

The primary snapshot is [the current DNS OpenAPI](https://docs.selectel.ru/api/dns-actual.json),
version v2.0.0, with its SHA256 recorded in the generated contracts. Additional
wire evidence is pinned to domains-go commit
`6b87a8d9f7b4089a30a4a1dc361cf73117f1af61` and Terraform provider commit
`cf33c549d0c910c137f4a51411f6d709eb2be7d4`. Generation uses local snapshots only.
Full service inventory remains incomplete because legacy v1 and its different
account/token semantics are outside this current-API delivery. No v1 fallback,
legacy X-Token authorization, or whole-service coverage is claimed.

## Scope and identity

The current API uses IAM project authorization with X-Auth-Token. It is global
within a project: region/pool are not selected. The documented base URL is
`https://api.selectel.ru/domains/v2` for all locations, or an explicit override
with the exact project scope. Every call using a resource/parent ref pins its
original endpoint ID, URL, generation and scope; changing an override cannot
silently send an old ID to another endpoint.

Zone refs are top-level provider IDs; RRSet refs always carry the exact zone
parent. Scope retains the caller authorization view. Response project_id is
owner metadata, not an adoption or permission rule. TransferOwner sends the
explicit new project UUID but retains the original Receipt/ref scope. It does
not adopt the new project's authorization or search there after an error.

Identity and mandatory parent relations are validated before the rest of a
successful mutation response. An acknowledged malformed TTL or other payload
retains its trustworthy target ref and accepted Receipt. Malformed IDs and
foreign zone relations never establish a new target. Names are never IDs.

## Wire values and update semantics

Zone and RRSet names remain caller-supplied FQDNs ending with a dot. The SDK
does not lowercase, append a dot or otherwise normalize them. RRSet create
accepts all 14 current record types, including DNAME, HTTPS and SVCB. TTL is an
exact Integer in seconds; the generated request bounds are 60–604800. A request
contains 1–100 records. Update requires TTL and the complete replacement records
array; it does not merge individual records. A record's optional disabled pointer
preserves explicit false without inserting the provider default.

RRSet comments use `*types.Nullable[string]`: nil omits, Null sends null, and
Value("") sends an empty string. Zone UpdateComment is a separate required string
field and preserves empty strings. Request comment length is bounded at 255
Unicode code points. SetDisabled and SetProtected send explicit booleans.
Timestamp fields remain optional raw provider strings; no absent timezone is
invented. Response TTL/counters preserve arbitrary integer precision.

Record content is a ProtectedValue for both request and response. Reveal is an
explicit caller disclosure boundary. The private wire encoder reveals only for
the caller-requested operation; normal JSON, text and formatting remain redacted.
RRSet search is protected because the API searches record content as well as
names; its private query digest binds cursors without exposing search terms.

Current Update/Delete accept an optional X-Managed-By UUID header via
RRSetOptions. Null/absent HTTP header means omission, never a literal "null"
header. CreateNative and UpdateNative are explicit published-SDK management-field
variants that serialize optional nonempty managed_by in the body instead.
They do not assert that the current server accepts this older wire field, do
not switch to v1 and never fall back between body and header after a response.
The standard methods follow the current schema.

Create and Get accept 200 with bare DTOs. Mutation actions, RRSet Update and
Delete accept 204 without a body. Source-documented 4xx responses are rejected
mutations; 400/422 map to invalid_argument, 403 permission_denied, 404 scoped
not_found, and 409 conflict. Fault text/description/location are discarded.
Unexpected/server/transport mutation failures remain unknown and are never
replayed. Every mutation includes a Receipt, including validation/auth errors
before sending and acknowledged malformed responses.

## Collections and observation

Lists send explicit limit/offset, repeated sort_by and rrset_types values, and
source filters without comma splitting. Provider next_offset drives traversal;
0 terminates in the current/provider contract, and null is also supported by
pinned published SDK fixtures. Positive offsets must strictly advance. Neither
a short page nor count proves the end. Count is retained as ReportedCount;
the schema does not label it a total and the SDK does not invent that meaning.
Cursors bind operation, revision, scope, endpoint URL/ID, parent, initial offset
and a private query digest. Item/page/time budgets preserve incomplete prefixes.

Root Resource and Resume support zone/RRSet refs. Child create receipts retain
known parent plus returned child; Resume chooses the unique target kind.
Known accepted/rejected/not_sent receipts Recover conclusively without I/O,
including no-ID receipts. Unknown creates without a target return
recovery_unavailable; other unknown mutations permit exact saved-target
observation only, remain indeterminate, and never search names or replay.
Shared receipt-only validation/revision handling derives from reviewed commit
`379458e`; endpoint pinning and identity-first patterns derive from reviewed
`8351f2f`. No unrelated unmerged service code is included.

A successful exact GET reports ProviderState `present` and shared Classification
`succeeded`; this establishes resource existence only. Wait goals are only
zone.present/zone.absent and rrset.present/rrset.absent.
They describe exact GET existence or scoped 404. Comment, state, protection,
owner and RRSet updates have no receipt-bound convergence goal. No wait claims
DNS propagation, delegation, resolver cache expiry, service readiness or global
absence after a resource leaves the authorization view. Inspect the typed Get
result to observe current metadata and values.

```go
dnsAPI := project.DNS()
zone, err := dnsAPI.Zones.Create(ctx, dns.ZoneCreate{Name: "example.test."}, types.CallOptions{})
// Always retain zone.Receipt, including when err is non-nil.
if err == nil && zone.Data.Ref != nil {
    records, listErr := dnsAPI.RRSets.Collect(ctx, *zone.Data.Ref,
        dns.RRSetListOptions{}, pagination.Options{MaxItems: 100})
    _ = records // Complete=false and a cursor preserve bounded partial results.
    _ = listErr
}
```

Public Client tests substitute only Config.Transport and use synthetic
CredentialSource material. Their evidence is offline/wire only; no live DNS
zone, record, owner, protection or resolver configuration is changed.
