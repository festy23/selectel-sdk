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

The 15 remote operations are version check; tags list; referrers list; manifest Get/Head/Put/
Delete (explicit DeleteTag variant); blob Download/Head/Delete; upload Start,
Status, Append, Finalize and explicit Distribution `CancelNative`. All 15 are
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

Protocol-confirmed omissions retained under **#50**: optional chunk-min-length/other metadata; extended Link
syntax and additional content formats/algorithms. These are not described as
provider-unsupported. In particular Put sends exactly one manifest mutation;
subject/referrers fallback indexing uses explicit local preparation and caller-directed Get/Put stages, described below.
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


## Explicit optional single-POST upload

`Uploads.Monolithic(ctx, repository, digest, reader, size, options)` implements
OCI1.1.1 end-4b on the existing POST upload-start remote binding. This is an
explicit optional protocol variant, with no claim of current Selectel server
availability. It does not replace Start or automatically follow a202 with PUT.

The caller supplies the exact repository, digest and int64 byte length; the
compiled Basic/Bearer preflight finishes before reading the stream. All transfer,
error-body processing and cancellation share the existing transfer budget. Bytes
are streamed once and hashed locally; empty input has explicit Content-Length0,
not unknown-length chunked framing. A201 response must provide same-origin exact
repository/requested-digest Location; the Blob union arm has DigestVerified only
when every declared byte was read and its local digest matches. Malformed accepted
headers/size/digest preserve the Accepted receipt and requested address, without
claiming a valid completed blob.

A202 response returns only the protected Session union arm at offset0; it does not
prove blob completion or authorize automatic retransmission. The caller decides
whether/how to supply data to Finalize. The initial repository and requested blob
remain in the receipt; a validated202 session is retained as a third exact ref.
Its protected Location/query must be saved separately to perform session calls.

Unknown outcomes retain the caller-selected blob address for exact HEAD observation
through Resume, with no creation/ownership proof and no replay. Resume validates
repository/blob[/session] parent relationships and endpoint agreement; a known202
receipt recovers conclusively without turning its session into blob completion.
The additional requested-blob auth proof is accepted only for the compiled start
operation with exactly one digest query value matching that ref; no general
caller-controlled auth bypass or cross-origin trust is introduced.

Confirmed remaining protocol work remains owned by #50: optional hints/metadata, extended Link forms and further
formats/algorithms. Selectel optional variant availability and actual realm/offload
topology still require #74 evidence; #75 retains partial acceptance qualifications.


## Explicit optional mount/from

`Uploads.Mount(ctx, destination, digest, MountOptions{From: &source}, options)`
implements the optional OCI end-11 variant on the existing POST start binding.
Both repository refs must have the same explicit project and data-plane endpoint;
source ownership is not inferred. `From: nil` deliberately omits the optional
source parameter and lets the provider decide whether a digest can be mounted;
the SDK performs no lookup/adoption or background search.

The request has no blob body. Before mutation, auth preflight requests exactly
destination `pull,push` and, when different and explicitly supplied, source `pull`.
The pinned token protocol represents multiple access entries using repeated
`scope` query values. Source/destination/query/ref correspondence is compiled and
validated; advertised challenge scope never grants or requests additional access.
Foreign source endpoints/scopes and malformed parent chains fail before credential
lookup or HTTP, and input source refs are deep snapshots.

A201 result contains the Blob union arm with protected same-origin exact repository
Location. The source permits Docker-Content-Digest to differ from the requested
digest: CanonicalDigest preserves this unverified metadata and the accepted URL
must address either the exact requested or exact reported canonical digest.
DigestVerified is always false because mounting transfers no bytes for local
verification. Requested and source addresses stay in the receipt; a distinct
validated canonical destination address is additionally retained, not substituted
as proof that an unknown mutation created content.

A202 result contains only a protected Session at offset0. It never triggers an
upload or hidden fallback. Receipts can contain destination repository, requested
blob, optional source repository, and an accepted session/canonical blob. Resume
validates each exact parent/scope/endpoint shape; unknown outcome observes only
the requested destination blob and remains indeterminate without replay. Known
receipts recover without I/O. Protected session query is saved separately by the
caller for explicit continuation; receipt refs do not reconstruct it.

Current Selectel mount availability is still unknown; this explicit protocol
variant does not claim otherwise. Optional hints,
extended Links and additional formats remain #50 work; topology/availability #74
and final partial acceptance #75 remain open. All 15 selected bindings stay partial.


## Explicit referrers API reads

`Referrers.List(subjectManifestDigestRef, ReferrerOptions, pagination.Options)`
addresses OCI1.1.1 end-12a/b in one exact repository/project/endpoint. The compiled
GET requests pull access only. A404 is returned to the caller: no hidden GET of a
fallback tag, empty-list assumption or management discovery occurs. Optional
Selectel availability remains unknown and this 15th binding remains partial.

Models are generated against pinned OCI Distribution1.1.1 and OCI Image Spec1.1.1
commit 147f9c13cedb47a0c4d9a11a222961073d585877. Each Referrer retains a caller-view
exact manifest address, parsed digest, exact Integer size, mediaType/artifactType,
platform architecture/OS/version/features/variant, protected annotations/URLs and
protected encoded inline data. Required fields, nonnegative int64 size, annotation
string values, base64 syntax and known non-null JSON shapes are checked. This is
metadata observation: no referenced URL is followed, no manifest subject/content
is fetched, and no descriptor/inline-data digest or ownership verification is
claimed. Data Reveal returns its source base64 string, not decoded content.

The complete source image index (including unknown metadata/extensions) and
OCI-Filters-Applied text are retained protected in `Page.Provider` and
`Collection.ProviderPages`, including successful empty pages. Unknown index data
is not promoted to typed coverage. Response Content-Type and body schemaVersion 2
are required as documented; the optional body mediaType must agree if present.

ArtifactType is copied before fingerprinting and I/O. The exact optional value is
sent on the initial request. If the server does not advertise the filter as
applied, the iterator filters descriptors locally; if it claims artifactType but
returns a nonmatching descriptor, the collection is incomplete with protocol
error and only the validated prefix. No server page-size/n/last query is invented.

Source Link determines continuation, even on empty/short pages. Bounded RFC5988
parsing accepts multiple header values/links, quoted commas, relation lists and
case-insensitive registered next; duplicate next or alternate anchor context are
explicitly unsupported. Only same-origin, exact subject path is followed. Opaque
query bytes remain unchanged through the compiled private OCI query slot, and
cursor state stays protected. Reserved auth/mutation/view query names and changed
artifactType are rejected; an omitted artifactType on a provider Link is permitted
while the immutable local filter still applies. No other public raw-query or auth
bypass is exposed.

Cursor semantics include the full subject/ref parent/endpoint and filter snapshot,
plus common revision/view bounds. MaxItems truncation yields replay+skip rather
than false completeness; malformed rows or unsafe Links preserve only a bounded
validated prefix and the original error without extra I/O. No Link means terminal
according to the source contract. MaxPages/collection budget/body limit and normal
safe-read retries/cancellation apply. Further protocol work remains hints/metadata, extended Tag Link
forms and additional content formats/algorithms (#50); source topology/availability
#74 and P1 acceptance #75 remain open. No live requests were made.


## Explicit subject/tag preparation

OCI image/index Put now validates a supplied subject descriptor and reports
RequestedSubject, AcknowledgedSubject and NeedsTagFallback. Unsupported subject
algorithms are capability_unavailable before I/O; malformed known subject fields
are invalid_argument. A matching OCI-Subject response acknowledges that association
only, not content ownership or referrers availability. Its absence on201 sets the
fallback flag. Malformed/mismatched headers retain Accepted receipt and error.
Unknown mutation outcome is still unknown; no fallback or replay is started.
Docker payload extensions are not interpreted as OCI subject declarations.

`Referrers.FallbackTag(subjectDigestRef)` is a pure exact-view constructor for the
source `<algorithm>-<encoded reference, limited to64 characters>` tag schema.
The complete original subject remains explicit. Longer encodings can share a
prefix; the derived mutable tag is an address, not uniqueness/ownership proof or
an adopted subject. No tag lookup is performed by construction.

`PrepareTagIndex(subject, current, contentDigestRef, contentOptions, change,
TagIndexOptions)` is pure bounded preparation, not a remote operation:

- The caller explicitly Gets the fallback tag with OCIIndex Accept. Wrong media
  type/schema/snapshot/view is rejected. Only the caller decides that a confirmed
  resource404 permits `current=nil`; auth failure or unknown outcome is not this
  decision. No helper catches404, searches names, fetches content or changes scope.
- Add requires exact same-repository content digest and protected OCI image/index
  bytes matching that digest and subject. The new provider descriptor contains
  only mediaType/digest/size/artifactType/annotations. ArtifactType uses the source
  manifest field, or config mediaType for an image manifest; an index without it
  omits the field. All source annotations are copied. SDK refs/scope/endpoint are
  never serialized into the new descriptor.
- Remove needs only the exact caller-selected content digest ref; it does not
  require refetching a deleted body. Duplicate Add and absent Remove are no-ops.
  Existing descriptors and index metadata/extensions are preserved; matching
  Remove entries are removed. No resource existence or creation proof is inferred.
- Current must be the exact derived tag document with matching payload digest,
  size and OCIIndex metadata. MaxDescriptors defaults to collection MaxItems;
  input/output bytes use StructuredLimit. Invalid source payloads fail locally;
  bounds return response_limit without producing a cloud write.

PreparedTagIndex contains protected Write options, exact Subject/Tag refs,
Changed/count and optional BaseDigest. Duplicate no-op preserves original payload
bytes. Preparation creates no Receipt, performs no credential lookup/HTTP, and
returns no authorization or replay decision. The caller separately invokes
`Manifests.Put(prepared.Tag, prepared.Write, ...)` and retains that call's Receipt.
For cleanup, the caller selects Remove after handling the original delete outcome.
Do not advance the workflow by assuming an unknown mutation was applied.

BaseDigest is informational snapshot evidence, not an ETag or HTTP precondition.
The source documents races between clients updating the tag; this helper does not
claim atomic read-modify-write, locking or conditional-update protection. Explicit conditional HTTP is available as described below; current Selectel
support/topology stays #74 evidence and final acceptance #75. No network fallback,
rollback, index write, automatic retry or mutation replay is hidden in Put or the
preparation API. All15 remote bindings remain partial; local constructors and
preparation do not inflate remote-operation counts.


## Explicit conditional manifest HTTP

OCI1.1.1 permits conditional pushes for registries supporting ETag conditions.
The pinned [RFC9110](https://www.rfc-editor.org/rfc/rfc9110.html#section-13.1)
defines these semantics. `Manifests.Get` and `Head` expose an optional protected
`ETag`: an opaque quoted validator, including a possible `W/` prefix. It is not
computed from the content digest. Missing ETag remains nil; malformed or repeated
ETag is a structured protocol error. No trailer-derived validator is inferred.

`ManifestWriteOptions.IfMatch` and `IfNoneMatch` each accept an explicit
`EntityTagCondition`: either Any=true with no Tags (wire `*`), or a nonempty list
of protected quoted entity tags with Any=false. The SDK joins the list with comma
and space without unescaping the opaque bytes. Total condition value bytes are
bounded by StructuredLimit; malformed/injected tags are rejected before credentials
or HTTP. Weak tags are valid syntax; If-Match uses strong comparison so a weak tag
cannot satisfy a strong match, whereas If-None-Match uses weak comparison. If both
are selected, RFC9110 section13.2 precedence applies (If-Match first).

The caller may explicitly copy a Get ETag into IfMatch for the prepared tag write,
or select IfNoneMatch Any after their own known-absence decision. Preparation never
chooses either condition, and BaseDigest is never converted into ETag. HTTP412
means conclusive Rejected with the receipt retained. A201 remains Accepted; it does
not prove that a particular Selectel deployment enforced the condition or provided
an atomic read-modify-write. Availability/concurrency evidence remains owned #74
and acceptance #75. There is no automatic retry, conflict resolution or fallback;
an unknown outcome still requires exact-reference observation without replay.
