# Registry token lifecycle

Registry token management uses the existing explicit project IAM registry views,
with separate v1/v2 accessors. The 12 source operations are four v1 operations
(create/get/refresh/revoke) and eight v2 operations
(create/list/get/update/refresh/regenerate/revoke/delete). Refresh and regenerate
have distinct wire actions; regenerate revokes and replaces a token, while refresh
updates expiration. There is no automatic fallback or replay.

V1 token path values are credentials, accepted as ProtectedValue inputs and never
placed into plain ResourceRef IDs, receipts, diagnostics or cursors. The caller
explicitly selects the project/location view for these credential inputs; no
absolute resource-owner proof is claimed. V2 uses a separate non-secret ID and
exact project/generation/endpoint refs; token and Docker-config outputs remain
protected. Unknown secret-address mutations cannot use generic root exact-ref
recovery, since receipts deliberately do not persist the credential path.

Sources: [CRAAS v2.2](https://docs.selectel.ru/api/craas.json), pinned SHA-256
`a6e68b67cd861617e5d66304891d4523f685593b36fbcdb4538ffc76de2bb314`, and
[craas-go](https://github.com/selectel/craas-go/tree/8f0d0cba0b59360f344b3b2d32157b1549c2645a).
Both pins match current primary sources on 2026-10-15. Token schemas are generated
from the pinned source. Wire tests replace only HTTP transport with synthetic
credentials; offline evidence does not establish live availability.

## Typed interface

`V1.Tokens().Create` accepts optional TTL (12h/1y) and DockerConfig=false/nil;
provider defaults are not inserted. `CreateDockerConfig` explicitly requests the
JSON configuration variant and returns a whole ProtectedJSON value. Its internal
schema is not specified by CRAAS, so the SDK does not interpret keys or invent a
resource ID. `Get`, `Refresh`, `Revoke` accept a ProtectedValue path input. Path
escaping includes slash/query/fragment/dot-only values. V1 Get returns metadata;
if Refresh omits token, the SDK does not insert the input credential as output.
Unix expireAt and remaining expireIn seconds retain arbitrary Integer precision.

`V2.Tokens().Create` accepts source-generated TokenCreateRequest, including a
query-only DockerConfig pointer; false is preserved and not serialized in the
body. `CreateDockerConfig` requests the protected configuration variant explicitly
(nil/true is compatible; false conflicts). The normal Create method rejects true
before I/O because its result is TokenInfo. Normal TokenInfo responses expose
non-secret ID refs; token is optional ProtectedValue. Optional request fields
preserve omission, explicit empty name/registryIds and false isSet/modeRW/
allRegistries. Required scope/expiration fields are validated before I/O.

`Get`, `Update`, `Refresh`, `Regenerate`, `Revoke`, `Delete` use exact v2 token
ResourceRef. Refresh and Regenerate have separate generated request types and
separate endpoints; revocation POST and deletion DELETE also remain distinct.
All received token IDs must match an exact request. Create extracts a trustworthy
ID before decoding unrelated fields, retaining it in an Accepted receipt even
when the rest of the response is malformed. No name search or ownership adoption
occurs. The access scope's registryIds are explicit source string selections;
they are not resource ownership proof or automatic ref conversion.

`Iterate/ListPage/Collect` support all source token-list filters: search,
sort_field (name/createdAt/lastUsedAt/expiresAt), sort_type (asc/desc), scope_mode
(ro/rw), and initial offset. Filters and offset are snapshotted before fingerprint;
the endpoint URL is part of the semantic request. Wire limit/offset obey shared
bounds. Empty array is the end rule; totalCount remains provider metadata because
the source does not distinguish filteredCount or promise snapshot consistency.
A malformed row preserves the validated prefix and original error; Next becomes
terminal without more I/O. MaxItems/MaxPages/response limits never imply complete.

## Observation and errors

Root Resource/Resume and `V2.Tokens().Handle` observe exact token metadata. Status
retains active/expired/deleted as ProviderState, with succeeded classification for
existence only; future statuses are unknown. Resource handles offer exists/absent;
delete receipts offer absent only. Revocation/refresh/regeneration do not promise
an invented wait goal or secret validity proof. Unknown v2 recovery remains
indeterminate after exact Get, even if active. It never proves successful secret
rotation or authorizes replay. V1 unknown receipts have no persisted secret target
and generic recovery is unavailable. Known terminal receipts recover conclusively
without I/O, including receipt-only cases, with schema/account/revision/effect
validation. Shared behavior is reused narrowly from reviewed 379458e; validated
pagination prefix preservation is reused from reviewed a49e66b.

Shared errors apply: 400 is provider_error, 401 refresh/authentication, 403
permission_denied, 404 not_found, 409 conflict and 429 rate_limited. Source messages
are discarded rather than parsed for codes. Only documented mutation rejection
statuses are conclusive (v2 DELETE does not document404). Transport/5xx loss after
possible send yields Unknown Receipt; no mutation is replayed. Malformed Accepted
responses retain receipts and trustworthy addressed identities. Sensitive values
remain redacted in JSON/text/formatting; Reveal/CopyTo are caller-directed export
boundaries. No atomic rotation or live availability claim is made.

Example (synthetic values; obtaining project-scoped management credentials is
explicit client setup):

```go
no, yes := false, true
created, err := project.Registry().V2.Tokens().Create(ctx, registry.TokenCreateRequest{
    Scope: &registry.TokenScope{AllRegistries: &yes, ModeRW: &no},
    Expiration: &registry.TokenExpiration{IsSet: &no},
}, types.CallOptions{})
// Always retain created.Receipt on error; do not repeat an unknown mutation.
if err == nil && created.Data.Ref != nil {
    observed, err := project.Registry().V2.Tokens().Get(ctx, *created.Data.Ref, types.CallOptions{})
    _, _ = observed, err
}
// Export created.Data.Token only at an explicit caller-directed Reveal/CopyTo boundary.
```
