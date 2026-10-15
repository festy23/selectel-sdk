# OCI registry data plane

This service is separate from CRAAS management: service registry_data, generation
oci_v1 (OCI Distribution 1.1.1 over the protocol's /v2 HTTP paths), explicit project
view and saved registry-token Basic username/password. Management IAM credentials
and UUID refs are not automatically converted into data-plane credentials or
registry names. Selectel documents cr.selcloud.ru/<registry-name>/<image>:<tag>.
Repository constructors are explicit caller-selected addresses, not ownership or
creation proof. Exact refs retain endpoint, view and registry/repository parents.

A compiled private protocol handshake negotiates authentication before any
mutation body is read/transmitted, under the same call/transfer budget. Basic and
Bearer values stay protected. Bearer realm must use HTTPS and the exact selected
registry origin, with no userinfo or fragment; redirects are never followed.
Service is bounded source-advertised text; repository/action scope is compiled
from the validated request, not broadened by the challenge. Foreign realm and
redirect targets never receive credentials. This same-origin policy is an
explicit compatibility restriction: current Selectel docs do not publish token
realm topology. No external realm is guessed to be trusted.

Manifest payloads remain protected JSON. Digests are parsed and verified, not
adopted from unrelated metadata. Binary transfer uses caller-directed readers/
writers, explicit bounds, a common transfer budget and no full buffering. Upload
Locations and session refs must remain bound to the exact endpoint/repository;
there is no automatic cross-origin follow. Every mutation retains a Receipt;
unknown outcome never authorizes replay or name adoption. Optional protocol
variants and Selectel availability remain explicit, without claiming a complete
source inventory or live evidence.

Pinned sources: [OCI Distribution 1.1.1](https://github.com/opencontainers/distribution-spec/tree/a139cc423184af6078077b9b7ee336eddbd03f8f),
[Distribution token auth](https://github.com/distribution/distribution/blob/f7d69d28c2e302f31d1f6ce8e193da58407225a7/docs/content/spec/auth/token.md),
and Selectel [login](https://docs.selectel.ru/en/craas/images/use-docker/),
[push](https://docs.selectel.ru/en/craas/images/push-image/) and
[pull](https://docs.selectel.ru/en/craas/images/pull-image/). Protocol snapshots
retain their Apache 2.0 licenses. A source-bound SDK-owned protocol projection is
used for catalog generation; it is not presented as an official OpenAPI schema.

## Selected protocol surface and limitations

The 14 remote operations are version check; tags list; manifest Get/Head/Put/
Delete (explicit DeleteTag variant); blob Download/Head/Delete; upload Start,
Status, Append, Finalize and explicit Distribution `CancelNative`. All 14 are
partial in the capability catalog. Selectel documents Docker push/pull, but does
not publish an exhaustive OCI endpoint/realm/offload inventory. No live server
availability or full OCI coverage is inferred.

`Repository(registryName, repositoryPath)` constructs the explicit namespace;
`Manifests.Tag`, `Manifests.Digest` and `Blobs.Digest` bind addresses to that
repository. The four media types are OCI image manifest/index and Docker v2
image manifest/list. Protected whole manifest JSON retains fields without
silently reserializing bytes. Content checks support lowercase sha256/sha384/
sha512. Unknown algorithms/formats are unavailable, not guessed.

`Blobs.Download` writes synchronously to a caller writer. `MaxBytes` bounds the
whole blob, including any resume prefix. `Offset>0` requires caller `Prefix`
reader with exactly that many existing bytes and a writer positioned to append
the suffix. A206 response must start at the requested offset; a 200 fallback is
an error. Completion requires exact range/length and whole prefix+suffix digest.
Partial results retain transferred counts/digest progress on errors; they never
claim complete/verified content. The prefix reader and writer are caller-owned,
must make progress and should honor cancellation; SDK loops check cancellation
and close the response body when the shared transfer budget ends.

Upload sessions retain a plain exact ref with registry/repository/session ID and
**protected Location**, including opaque critical query. Subsequent requests use
that query byte-for-byte; it is never copied into a plain ref, cursor or Receipt.
Existing `digest` query, cross-origin/offloaded or other-repository Locations are
rejected. Session ID is not reconstructed from a guessed URL or provider search.
Start returns Offset 0. Status/Append interpret Range 0-last inclusive as
Offset=last+1; Range 0-0 therefore means 1, never inferred empty progress. Append
uses caller-selected offset and bounded reader length, then checks exact returned
offset. Caller sessions are explicit inputs, not immutable provider proofs.
Finalize accepts an optional final chunk and whole-blob digest. `DigestVerified`
is true only for offset 0 when that call locally hashes the complete blob. A
finalization after prior chunks preserves the requested digest without claiming
whole local verification. A disconnect returns Unknown with Receipt and no
replay; an accepted response with malformed metadata remains Accepted.

Root `Resource` and `Resume` validate exact scopes/parents/endpoints. Content
handles observe HEAD presence only; HEAD metadata is not payload verification,
creation proof or ownership. Known terminal receipts recover conclusively without
I/O, including upload receipts with repository+session or session+blob refs.
Unknown content mutations observe only their exact caller-selected address and
remain indeterminate. A Receipt cannot reconstruct opaque session Location;
unknown Start/Append/Cancel recovery/status is unavailable through the generic
handle. Finalize retains the caller-requested exact blob digest address before
I/O; an unknown Finalize can observe HEAD of that address, remaining
indeterminate without claiming creation or uploaded-byte proof. The
caller-held protected session supports explicit `Uploads.Status` offset
observation; this does not prove which bytes an unknown call applied. No wait
goals, image push orchestration, rollback or automatic replay are inferred.

Tags lists snapshot the complete repository ref before fingerprinting. Pagination
uses bounded n/last and prefers a validated next Link even on a short page.
Without Link a short page ends; a full page probes after the last returned tag.
The supported Link subset has one rel="next", same origin/path, n unchanged and
last equal to the last returned tag. Extensions, extra query and multiple Links
are excluded. Invalid namespace/row/Link preserves an already validated bounded
prefix and the original terminal error, without another request.

Protocol-confirmed omissions retained under **#50**: single-POST monolithic
upload and mount/from variants; referrers/artifactType pagination and subject
fallback workflow; optional chunk-min-length/other metadata; extended Link
syntax and additional content formats/algorithms. These are not described as
provider-unsupported. In particular Put sends exactly one manifest mutation;
subject/referrers fallback indexing is a separate unsupported workflow.
Primary Selectel evidence still needed under **#74**: actual token realm/service
and custom query topology, cross-origin offload trust, optional deletion/
referrers/mount/native cancellation availability. **#75** owns P1 acceptance of
all these partials. None may silently become implemented after merge.

## Offline example

```go
service := project.RegistryData() // configured explicit saved-token Basic source
repository, err := service.Repository("my-registry", "team/image")
if err != nil { return err }
tag, err := service.Manifests.Tag(repository, "latest")
if err != nil { return err }
manifest, err := service.Manifests.Get(ctx, tag, registrydata.ManifestReadOptions{}, types.CallOptions{})
// manifest.Data.Payload remains protected; reveal only at a caller-directed boundary.
```

For upload, call Start, then explicit Append/Finalize using the returned protected
session; retain every mutation Receipt on both success and error. The library
performs no cloud call merely by constructing repository/content refs.

Optional OCI error diagnostics remain inside the whole call/transfer budget.
Cancellation closes a blocking diagnostic body once; a source-documented
rejection is recorded before diagnostic processing and remains Rejected even
when cancellation prevents reading its optional provider code. Cancellation
does not authorize a retry or change the already known mutation refusal.
