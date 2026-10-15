# User Certificate Manager

`Project.Certificates()` binds service `certificates`, generation `user_v1`,
project Keystone authorization and the published global
`https://cloud.api.selcloud.ru/certificate-manager/v1` endpoint. User certificates
are separate from Let's Encrypt issuance. Resource refs retain exact project,
API generation, kind `user_certificate` and endpoint; metadata Get does not read
private key or PKCS12. The `private_key.type` metadata field is an algorithm
description, not private-key bytes. Version integers retain precision, and
provider time strings retain their supplied representation.

The source baseline is the current public [User Certificates schema](https://docs.selectel.ru/api/certificates-manager.json)
and the [official SDK](https://github.com/selectel/secretsmanager-go/tree/5adf32225d3863ae40fd351ea33629f3fcca2447).
The SDK confirms endpoint/version and raw CA-chain/private-key/PKCS12 response
formats omitted from OpenAPI response schemas. Generated DTOs retain the complete
source metadata model. Private material requires explicit service methods and
protected values; mutations retain receipts, never replay Unknown outcomes.
No live calls form part of development or ordinary tests.

## Operations and guarantees

The 11 current User Certificate operations are Get, List, Upload, Delete,
UpdateVersion, UpdateName, AddConsumers, RemoveConsumers, ReadPublicChain,
ReadPrivateKey and ReadPKCS12. The complete issuer/validity/consumer metadata
models and all source request fields are generated. UpdateVersion creates the
next provider version via POST; no invented historical version-list/get API or
version number is sent. Consumers preserve the documented array and flat
id/region/type fields as caller-selected provider identifiers, without inferring
ownership or resource adoption. Delete Force is optional and caller-selected;
nil omits it, and explicit false/true is preserved.

Upload and UpdateVersion reveal protected PEM private-key input only to private
wire serialization at those explicit method boundaries. Metadata operations
never fetch private bytes. The three explicit content reads return unchanged
ProtectedValue bytes, bounded by MaxJSONBytes and TransferTimeout; PKCS12 is not
reinterpreted as JSON or base64. Ordinary JSON/fmt output remains redacted, and
callers explicitly Reveal only at their chosen boundary. Callers validate PEM
content according to application needs; SDK does not invent certificate fields
required by neither the selected OpenAPI nor HTTP contract.

Every mutation retains Receipt on error. Upload extracts a nonempty addressable
provider ID before decoding unrelated metadata, so malformed Accepted responses
retain trustworthy refs. Missing/malformed identity does not create a ref or
trigger a name lookup. Only documented rejection/Keystone refusal statuses are
Rejected; transport/5xx leave Unknown and are never replayed. All exact calls pin
the original endpoint and scope, including custom endpoint prefixes. The
provider error_text/status_text are discarded from SDK errors/diagnostics.

List exposes a bounded iterator over the documented single array response; no
remote limit/offset filters are invented. Local truncation reports Incomplete
with a replay+skip cursor bound to scope, endpoint ID and resolved URL, generation,
and contract revision. Malformed rows
preserve an already validated bounded prefix and original error. Replaying a
cursor reads a fresh provider list; it does not promise a provider snapshot.

Root Resource/Resume support exact certificate metadata observation. Presence is
`succeeded` classification with `present` provider state; it proves neither
application TLS health, key correctness, propagation nor creation by an unknown
call. Unknown recovery remains indeterminate. Known terminal receipts, including
no-ref receipts, recover conclusively without I/O. No wait goals are inferred:
version updates return no requested version ID or readiness state. Unknown create
without exact ID cannot be recovered through name search/adoption or replay.

## Example

```go
service := project.Certificates()
ref, err := service.Ref("exact-certificate-id")
if err != nil { return err }
metadata, err := service.Get(ctx, ref, types.CallOptions{})
// Reading private bytes is a separate explicit operation:
key, err := service.ReadPrivateKey(ctx, ref, types.CallOptions{})
// key.Data stays protected; Reveal is the caller's explicit output boundary.
```

These are offline transport-bound implementation claims, with synthetic test
credentials. No live cloud mutation or private-key retrieval is run by generation,
import or ordinary verification. All 11 User Certificate source operations are
mapped; source inventory/coverage for the broader certificates family remains
incomplete, and Let's Encrypt issuance stays separate under #52.

Observation does not infer resource absence from an identity/auth endpoint 404: the
exact resource endpoint must have been resolved before 404 can support that proof.
Read errors never publish a successful/present observation.
