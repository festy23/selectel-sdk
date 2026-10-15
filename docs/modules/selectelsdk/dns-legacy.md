# Legacy DNS v1

#117 owns the 19 current source operations; #71 retains family reconciliation and
#74 the inventory/canonical naming debt. This branch implements a growing subset,
not full legacy support. Current wire source:
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

Tag GET/PUT and domain GET have source-bound generated DTOs, exact integers,
protected names/record contents, ignored server-injected refs and scoped exact
v1 refs. Domain `user_id` is metadata, not the caller account identity. Domain
lookup is explicit by numeric ID or protected name; name lookup never recovers
an unknown mutation. PUT validates Unicode name length, domain array bound and
explicit empty/absent fields; protected values reveal only in the wire encoder.

Tag DELETE remains partial: success200 declares an empty JSON schema, while the
rendered page samples null. The result preserves a bounded protected optional
JSON acknowledgement and accepted Receipt, without guessing fields or completed
deletion/readiness. Missing body shape remains owned by #117/#71/#74/#76.

Every mutation uses core receipt finalization even on local validation errors;
accepted malformed responses retain supplied trustworthy exact refs, unknown
outcomes are never automatically replayed. Narrow early ContractRevision reuse
comes from independently reviewed commit
`379458e4f658b1216137fea4acce5dae6027d320` (PR97); no volumes feature is copied.

Tests use the public Client with synthetic credentials and only HTTP transport
replacement. Generation is local/pinned; offline evidence is not wire/live proof.
Inventory remains incomplete, coverage ratio null. Remaining 15 operations are
not claimed implemented by this checkpoint.
