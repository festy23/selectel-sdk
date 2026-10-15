# Let's Encrypt certificate lifecycle

`Project.LetsEncrypt()` binds the separate `certificates/le_v1` generation to an
explicit account and project. It never reuses User Certificate Manager references
or silently changes DNS generations. The current primary sources are the pinned
[OpenAPI snapshot](../../../catalog/sources/lets-encrypt-certificates.json) from
[Selectel](https://docs.selectel.ru/api/lets-encrypt-certificates/) and the
[official URL table](https://docs.selectel.ru/api/urls/), whose certificate section
is retained in [source evidence](../../../internal/letsencryptgen/assets/urls-excerpt.txt).
The published endpoint is `https://api.selectel.ru/certs/le` for all listed
locations; project Keystone `X-Auth-Token` authorization is explicit. Exact
`Config.EndpointOverrides` remain supported and do not grant permissions.

Three current remote operations are implemented:

| Method | Wire operation | Result |
| --- | --- | --- |
| `List(pagination.Options)` | `GET /` | bounded typed certificate iterator |
| `Issue(ctx, IssueRequest, IssueOptions, CallOptions)` | `POST /issue` | certificate plus mutation Receipt |
| `Delete(ctx, ResourceRef, CallOptions)` | `DELETE /{id}` | mutation Receipt |

`IssueRequest` contains the required source fields `name` and `domains`.
`IssueOptions.DNSV2` preserves nil omission and explicit false/true on the wire.
The source says new certificates must use current DNS Hosting; preserving the
legacy parameter does not claim that a new legacy certificate can be issued.
No fallback, DNS record creation, DNS generation selection or implicit renewal
operation is performed. There is no documented standalone renew/update/Get
endpoint in this snapshot. Additional availability requires primary evidence.

For example, after configuring explicit credentials/account and selecting a
project, call `project.LetsEncrypt().List(pagination.Options{MaxItems: 100,
MaxPages: 1})`. An explicit mutation is
`Issue(ctx, letsencrypt.IssueRequest{Name: "example", Domains: []string{"example.com"}},
letsencrypt.IssueOptions{DNSV2: &currentDNS}, types.CallOptions{})`, where
`currentDNS` is caller-selected true. Inspect and retain the Result Receipt even
when an error is returned; do not replay an unknown issue.

The response maps every `response.LeCert` field. Timestamps retain their provider
strings, including timezone-less fractions; no timezone is invented. Version uses
exact `types.Integer`. `error_description` is a ProtectedValue; metadata output
cannot expose raw provider diagnostics. `knox_cert_id` remains provider metadata,
not automatic proof of a User Certificate Manager reference or cross-service
ownership. A supplied `project_id` must match the selected project.

List performs one bounded provider request and does not invent limit/offset
parameters. Local truncation returns incomplete plus a replay/skip cursor bound
to account/project, generation, contract revision, endpoint ID and resolved URL.
Replay reads a fresh list, not a snapshot. Invalid or duplicate IDs and malformed
rows preserve only the validated prefix and the original error. Structured body,
collection and item/page limits apply. No background polling is started.

Refs use kind `lets_encrypt_certificate`, exact caller/auth scope, ID and endpoint.
Mutations retain NotSent/Rejected/Accepted/Unknown Receipts. Only the explicit
issue response ID and matching project are extracted before decoding other
metadata; malformed identity never creates an adopted reference. A valid ID
survives malformed unrelated fields in an Accepted Receipt. Unknown issue without
an ID cannot be recovered by name. Delete never retries after unknown outcome.
Raw provider error text is discarded and status errors use the shared mapping;
there is no provider-specific code inference from text.

Root `Client.Resource`/`Resume` delegate to this generation. Resume validates
account/project, operation/effect, revision, endpoint and exact reference agreement.
Known terminal receipts without references support conclusive receipt-only Recover
without I/O, but cannot observe state or wait. Unknown receipts require an exact
reference and remain indeterminate even after a successful observation.

Because the provider has no exact GET, Status observes only the already supplied
ID in a bounded complete List; it never searches by name or adopts another ID.
Incomplete/malformed lists do not establish absence. A 404 from authentication or
the collection endpoint is not exact-resource absence. Missing ID in a complete
list returns classification absent; observed unknown status strings remain unknown.
The endpoint is checked before list I/O, so an old reference cannot drift to a
new override sharing its ID.

For an issue Receipt, `Wait("active")` succeeds only on active;
creating/validating/issuing/renewing are pending and error/invalid/deleted fail.
For a delete Receipt, `Wait("deleted")` accepts deleted or absence in a complete
list. A resource-only Handle offers both observation goals. Wait uses the shared
budget/cancellation contract. Neither goal proves DNS propagation, consumer
application, certificate deployment, or successful creation by an unknown call.

The family inventory remains incomplete (User Certificate API #51 is separate,
and uncertain future variants are not advertised). Catalog coverage ratio remains
null. Tests use the real public Client with synthetic credentials and replaced
HTTP transport only; offline evidence is not live service validation. Narrow shared
reuse: receipt-only recovery from reviewed `379458e`, and validated pagination
error-prefix handling from reviewed `a49e66b`; no unreviewed service feature reuse.
