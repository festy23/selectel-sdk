# Federations v1

`client.Account().Federations().V1` exposes separate SAML and OIDC clients,
SAML certificates, group mappings, and public preview. All 28 operations in the
pinned [federations schema](../../../catalog/sources/federations.json) have typed
bindings. The source is [Selectel Federations API](https://docs.selectel.ru/api/federations.json).
The endpoint `https://api.selectel.ru` follows
[official iam-go f104d951](https://github.com/selectel/iam-go/blob/f104d9516f832b3ce7ee33840f3665aeaf8e8706/iam.go),
whose upstream HEAD was checked during implementation.

| Interface | Methods |
| --- | --- |
| `V1.SAML`, `V1.OIDC` | `Iterate`, `ListPage`, `Collect`, `Create`, `Get`, `Update`, `Delete`, `Exists` |
| `V1.SAML.Certificates` | `Iterate`, `ListPage`, `Collect`, `Create`, `Get`, `Update`, `Delete` |
| `SAML.GroupMappings`, `OIDC.GroupMappings` | `Iterate`, `ListPage`, `Collect`, `Replace`, `Target`, `Put`, `Exists`, `Delete` |
| `V1.Preview` | Read federation preview by the explicitly supplied ID or alias |

Account IAM authentication is mandatory for all operations except **SAML
federation HEAD existence and preview GET**, whose source declares `security:[]`.
OIDC existence inherits the global account IAM requirement. The two public
reads perform no credential lookup and send no auth headers. Their internal
binding is restricted to the exact operation IDs, generation, effect and method;
caller `AuthMode` cannot bypass authorization on another operation. Catalog auth
for these public reads is `protocol_specific`, with explicit request contracts.
Preview has no ResourceRef: its public fields cannot establish account ownership.
Existence returns a successful empty result when present, `not_found` on 404 and
the original SDK error for other failures; it is not a deletion wait contract.

Generated request DTOs preserve all declared properties. Optional pointers omit
absent fields and preserve false/empty values. OIDC update additionally uses
`types.Nullable` for description, alias, client_secret, session_max_age_hours,
auto_users_creation and enable_group_mappings. Explicit null remains distinct
from omission and value. Required OIDC fields, documented string/pattern/session
bounds, and group mapping limits are checked before resource I/O. No provider
configuration defaults are injected.

OIDC client secrets and certificate PEM data are `types.ProtectedValue`.
Standard formatting, JSON and text serialization remain redacted, including
nullable wrappers and nested DTOs. Only the transport request encoder reveals
input material. Caller-directed `Reveal` remains available for returned
certificate data. The shared nullable protection fix was taken from commit
`4534c4e79c8cc281490f075b0f2e094770d16e8a`, without DBaaS changes.

Federation and certificate calls use exact scoped refs. A certificate ref has
its SAML federation parent. Account IDs, federation IDs, resource kinds,
generation and endpoint relationships are checked whenever supplied or returned.
A response from a foreign account or parent is never adopted into a receipt.
Accepted creates retain a trustworthy provider ID if unrelated response fields
fail decoding. All mutations preserve not_sent/rejected/accepted/unknown
receipts; unknown outcomes are never replayed. There is no automatic rollback,
name lookup, adoption, wait or Resume contract for federations.

`GroupMappings.Replace` replaces the entire supplied collection, including an
explicit empty array. It is not an additive operation. SAML returns the provider
mapping response at HTTP 200; OIDC acknowledges at HTTP 204 and returns no
observed mapping data. `Put` adds one exact mapping, and `Delete` removes one.
A mapping ref identifies external_group_id, with internal_group_id and federation
as its parent chain. `Target` constructs this exact local ref without I/O.
Reusing listed mappings as replace input sends only the two provider identifier
fields; SDK refs, scope and endpoint metadata are excluded by the wire encoder.
External IDs are escaped once as path segments; group mappings have a limit of
100 items and the source's identifier bounds.

An empty certificate ID yields `protocol_error` and an incomplete collection
before constructing its parent-bound ref.

Lists have one provider response and inherit MaxItems/MaxPages, MaxJSONBytes,
CallTimeout and CollectionTimeout. Local truncation returns an incomplete prefix
and scope/endpoint/generation/parent/contract-bound replay continuation. No list
promises snapshot consistency. Verification is offline only; global source
inventory remains incomplete and its coverage ratio remains null.

See [the bounded SAML read example](../../../examples/federations/main.go).
