# Legacy DNS v1

#117 owns the 19 current source operations; #71 retains family reconciliation and
#74 the inventory/canonical naming debt. This branch binds all 19 enumerated operations: 15 implemented and four partial.
This is not a complete legacy inventory or deployment availability claim. Current wire source:
[DNS legacy JSON](https://docs.selectel.ru/api/dns-legacy.json), SHA-256
`d8cba98138b3d1bc00de0cc4d84479ee7289f836b9f0359e15ea386455c60b39`.
The source calls the service outdated and recommends migration; this is not
withdrawal evidence. No live DNS/migration/auth request is performed.

The pinned official [domains-go client](https://github.com/selectel/domains-go/blob/6b87a8d9f7b4089a30a4a1dc361cf73117f1af61/pkg/v1/client.go)
and README establish `https://api.selectel.ru/domains/v1` and Selectel token
`X-Token`, distinct from current DNS v2 project Keystone. Only the explicitly
named `Account().LegacyDNS()` view is exposed; there is no implicit migration or
v2 fallback. Exact endpoint overrides use dns/v1/account scope.

## Frozen source-to-ID correspondence

Existing PR120 operation IDs are retained. The generic root `secrets` slug,
`list` action on singleton state/export and collision suffix are naming debt,
not proof of Secrets Manager equivalence or extra operations. No rename or alias
is added; a future #74 migration needs a separate agreed catalog migration.

| Source locator | Existing operation ID |
|---|---|
| `GET /{domain}/export` | `dns.v1.export.list` |
| `POST /{domain}/records` | `dns.v1.records.create` |
| `DELETE /{domain}/records/{record_id}` | `dns.v1.records.delete` |
| `GET /{domain}/records/{record_id}` | `dns.v1.records.get` |
| `GET /{domain}/records` | `dns.v1.records.list` |
| `PUT /{domain}/records` | `dns.v1.records.replace` |
| `PUT /{domain}/records/{record_id}` | `dns.v1.records.replace_1158c7f1` |
| `PATCH /{domain}/records/batch_update` | `dns.v1.records_batch_update.update` |
| `POST /` | `dns.v1.secrets.create` |
| `DELETE /{domain}` | `dns.v1.secrets.delete` |
| `GET /{domain}` | `dns.v1.secrets.get` |
| `GET /` | `dns.v1.secrets.list` |
| `PATCH /{domain}` | `dns.v1.secrets.update` |
| `GET /{domain}/state` | `dns.v1.state.list` |
| `PATCH /{domain}/state` | `dns.v1.state.update` |
| `DELETE /tags/{tag_id}` | `dns.v1.tags.delete` |
| `GET /tags/{tag_id}` | `dns.v1.tags.get` |
| `PUT /tags/{tag_id}` | `dns.v1.tags.replace` |
| `PUT /version/v2` | `dns.v1.version.replace` |

## Current subset and safeguards

All 19 enumerated operations have typed public methods. The four partial contracts
remain owned by #117/#71/#74/#76: domain and record list arrays have no documented
terminal/cursor rule; domain and tag DELETE200 declare schema `{}` but the rendered
sample is null. Lists preserve bounded validated prefixes, always remain incomplete
and never invent a cursor, including for empty arrays. DELETE preserves bounded
protected optional JSON and accepted Receipt, without guessing completion fields.

Generated DTOs cover all eleven record response variants and source create/update
unions. SRV, SSHFP and CAA require explicit content alongside structured fields;
the caller supplies both independently. Exact integers, optional false/empty
values and protected names/contents survive wire encoding. The closed union
validates discriminator, source ranges, formats and required fields. Unknown raw
objects remain protected. Server-injected refs are ignored.

Account().LegacyDNS() uses static X-Token and the explicit v1 endpoint. Domain
user_id is metadata, never caller identity. Explicit name operations manufacture
no numeric owner refs. Exact caller refs and protected name selectors are copied
before I/O; scope, generation, endpoint and available parent identities are
validated. Identity scanning retains independently trustworthy refs before later
field errors. Unknown outcomes never trigger name adoption or automatic replay.
ReplaceAll defaults to preview: proposed IDs are not adopted unless applied is
explicitly true. Batch scans all returned identities, excludes foreign IDs and
retains independently trustworthy effects. Domain creation selects direct versus
bound-zone response using the official SDK's nonempty bind_zone branch. Migration
uses account v1 static auth and returns no invented destination v2 refs or recovery.

Export writes only to the caller writer, bounds bytes/time and records progress
on failure. The SDK closes the response exactly once on cancellation/deadline;
it never closes the caller writer or replays a transfer after writing. Completion
means bounded EOF transfer, not DNS readiness or durable destination storage.
Every mutation retains receipt finalization even on local validation errors.

Narrow reviewed reuse: early ContractRevision binding from
379458e4f658b1216137fea4acce5dae6027d320 (PR97), and DoResponse/StructuredLimit
adapter from 8a07c839deabb098f81ac3ea016607736f23b24c (PR96). No feature packages
are copied. The adapter preserves DoJSON orchestration and introduces the DNS
export stream boundary. internal/core/request.go is a future integration overlap
with independent numeric ErrorProtocol/optional-error-body changes.

Public Client tests replace only HTTP transport, using synthetic credentials.
Generation uses pinned local sources; offline tests establish no live/wire or
Selectel deployment proof. Whole inventory remains unknown, coverage ratio null.
