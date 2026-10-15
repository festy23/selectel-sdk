# Registry protocol projection

The generator owns DTOs, endpoint selection, operation definitions, capability
bindings/contracts and `catalog/sources/registry-oci.json`. The latter is an
**SDK-owned projection**, not an official Selectel OpenAPI. Run `go generate .`.

Pinned primary protocol snapshots in `assets`:

- `distribution-spec.md` and `content-negotiation.md`: OCI Distribution 1.1.1,
  commit `a139cc423184af6078077b9b7ee336eddbd03f8f`, Apache 2.0 (`LICENSE-OCI.txt`).
- `token-auth.md` and `distribution-api.md`: Distribution commit
  `f7d69d28c2e302f31d1f6ce8e193da58407225a7`, Apache 2.0
  (`LICENSE-Distribution.txt`). Native cancellation is explicitly named; its
  presence in this protocol does not establish current Selectel availability.
- `auth-access-records.json`: SDK-owned facts from that Distribution commit's
  `registry/handlers/app.go` appendAccessRecords: GET/HEAD pull, POST/PUT/PATCH
  pull+push, DELETE delete. Challenge-advertised scope cannot broaden these.
- `selectel-data-origin.json`: SDK-owned factual projection of primary Selectel
  login/push/pull guides, with exact retrieval hashes. It is not a copied guide
  or a server-discovered API schema.

The projection records SHA256 for every local asset. Source inventory remains
incomplete and all 15 selected bindings remain partial. Protocol-confirmed
omissions and unavailable Selectel topology evidence are distinguished in each
contract and the service contract; ownership stays with #50/#74/#75.

Narrow runtime reuse has reviewed provenance: `DoResponse` from `8a07c83`,
receipt-only recovery/early revision from `379458e`, and bounded pagination error
prefix preservation from `a49e66b`. No unrelated service feature is copied.
New private OCI authentication and opaque session-query paths need both review
axes, including budget/cancellation, compiled scopes, redirects and no replay.


Referrers descriptors/platform models are pinned to OCI Image Spec v1.1.1 commit
147f9c13cedb47a0c4d9a11a222961073d585877; the licensed primary schema/docs snapshots
and revision record live in `assets/image-spec-*`. The generator asserts descriptor
and platform property sets so edits cannot silently omit newly pinned fields.
RFC5988 is retained as `assets/rfc5988.txt` for bounded continuation parsing. These
are source facts, not proof of current Selectel optional endpoint availability.

RFC9110 is pinned in `assets/rfc9110.txt`, retrieved from
https://www.rfc-editor.org/rfc/rfc9110.txt for entity tags (8.8.3), explicit
If-Match/If-None-Match (13.1.1/13.1.2), precedence (13.2) and rejection412.
The RFC copyright/license notice is retained. This protocol source does not
establish optional conditional HTTP support or atomicity on Selectel.

OCI-Chunk-Min-Length advisory POST response metadata follows pinned
`distribution-spec.md` chunked upload lines337–368. It does not enforce a final
chunk minimum or establish live Selectel advertisement.

Tag pagination reuses bounded RFC5988 quote/angle splitting for multiple Links
and relation/extension parameters, while preserving exact same-endpoint/path and
n/last semantics. Optional n with last follows distribution-spec tag pagination.
This is a documented parser subset, not full RFC5988 implementation.
