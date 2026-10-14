# Swift current API

`RegionalClient.Swift()` selects current Swift v1 and project IAM through
`X-Auth-Token`. The account path is the explicitly selected project. This is a
protocol adapter for the documented Selectel Swift endpoint, independent of S3
credentials and the legacy Swift API. [Sources](../../../catalog/evidence/swift-current-sources.json)
are pinned public documentation; generation is offline.

The current binding set has 15 normalized method/path or bulk-query operations: 12 implemented and three partial bindings (JSON-only listings and the documented segmented-delete acknowledgement).
Metadata and quota methods share POST; upload, segment upload, copy-from,
dynamic/static manifests and symlinks share PUT. Convenience methods do not add
coverage. Whole Swift inventory completeness is unknown and `coverage_ratio`
remains null. XML listing representation, generic upstream Swift extensions,
temporary URL signing, cross-project copy and the legacy API are not claimed.
Legacy discovery remains the separate legacy coverage issue; no endpoint is
classified withdrawn merely because it has not been called.

| Methods | Wire behavior |
| --- | --- |
| Account, Buckets, UpdateAccount | HEAD account; JSON GET listing; POST metadata/removal/quota |
| BucketInfo, Objects, CreateBucket, UpdateBucket, DeleteBucket | HEAD, JSON GET, PUT, POST, DELETE container |
| Download, SymlinkInfo | GET bytes/manifest/symlink; HEAD with explicit `symlink=get` |
| Upload, CopyFrom, PutDynamicManifest, PutStaticManifest, PutSymlink | PUT with the explicitly selected body/header variant |
| UpdateObject, Copy | POST object metadata/HTTP headers; source-addressed COPY with explicit destination |
| DeleteObject, DeleteSegments, BulkDelete | DELETE object; explicitly selected manifest/segment cascade; POST bulk query |

`Bucket(name)` and `Object(bucketRef, name)` construct exact caller-selected
locators locally. They do not discover objects or establish ownership. References
pin account/project/location/generation/endpoint and account/container ancestry.
Names are nonempty UTF-8, at most 1024 bytes, without CR/LF/NUL or dot-only names; container names have no slash. Object slashes are escaped on the wire.
Source and destination must use the same explicit scope and endpoint. Copy does
not delete the source; moving requires a separately authorized Copy then
DeleteObject, each with its own receipt. There is no implicit name adoption.

Header DTOs retain exact Integer/Number values and optional presence. Creation
`X-Timestamp` values remain decimals; HTTP and listing times retain raw strings
rather than acquiring an invented timezone. Metadata, temporary URL keys, content
headers, manifest/link targets and bulk diagnostic paths/bodies are protected.
Only forming the explicit request or writing to the caller's download writer
reveals their bytes. Ordinary JSON and formatting do not reveal those values.
Request metadata rejects case-folded duplicates, set/remove collisions and invalid HTTP field-value control bytes; quota/type fields use their typed fields, preserving zero and false.

Listings request JSON and retain per-page account/container headers. PageSize,
MaxPages, collection deadline and existing opaque cursor checks apply. A clamped
short page is not proof of completion; an empty page terminates the scan. Markers
must progress, and cursors bind the scope, endpoint, contract revision and filters.
Delimiter pseudo-directories have no manufactured object reference. Partial
collections preserve their elements and protected continuation cursor. XML is
explicitly outside these JSON bindings.

Upload requires a caller reader and explicit byte size, and reads at most that
size without probing or closing the caller input. Download requires a positive
MaxBytes and caller writer; it writes at most that bound and probes one further
response byte to distinguish EOF from truncation. No object body is buffered in
full. Transfer results retain byte progress and Complete=false on cancellation,
write/read failure or bounds; Complete means completed transfer, not readiness.
CallTimeout bounds structured operations; TransferTimeout bounds the entire
binary request including auth and decoding. Both respect the caller deadline.
The SDK closes response bodies exactly once, including on cancellation, and never
closes caller readers/writers. Caller Reader/Writer implementations must return
from blocked calls themselves; arbitrary blocking application I/O cannot be
forcibly interrupted without taking ownership. No retry starts after response
stream consumption. Upload mutations are never replayed after uncertain delivery.
Control JSON and aggregate response header bytes obey MaxJSONBytes.

DLO/SLO creation takes explicit segment container/prefix or exact segment refs.
SLO JSON preserves exact size_bytes and optional checksums; the SDK creates no
segments automatically. SDK control limits are 1000 explicit SLO segments and
10000 bulk object refs, with MaxJSONBytes additionally bounding both control
bodies. BulkDelete encodes only caller-selected object paths, validates returned
error paths against those targets, and retains per-object protected diagnostics.
HTTP 200 with an unsuccessful embedded status or errors is OperationFailed with
an Accepted receipt and partial typed summary. Not-found counts do not prove that
this request deleted those objects. DeleteSegments explicitly requests the
provider cascade; the documented 204 is an acknowledgement, not an enumeration
or absence proof for unknown segment IDs. No unconfirmed 200 bulk-style cascade
response is decoded as success.

Every mutation preserves a NotSent/Rejected/Accepted/Unknown receipt on errors,
including endpoint binding and request validation failures. Accepted malformed
headers or partial bulk failures keep the exact submitted refs. Unknown outcomes
require observation of exact known locators, not replay, list-and-adopt or lookup
by a newly appeared name. There is no readiness waiter or automatic Resume.

The internal DoResponse adapter uses the existing core execution engine for
scope, endpoint, credentials, deadline, attempts, redirects, receipts and sanitized
errors. DoJSON keeps its existing decoder signature and bounded JSON behavior.
Streaming request bodies currently support the compiled IAM mutation path only;
S3 signed streaming is not implied by this seam. Pinned source discrepancies
(account quota/object metadata table typos, misspelled request ID, kz-1 version
suffix and single-quoted bulk example) have explicit choices in the evidence.

For example, after constructing an explicitly authenticated project/location view,
a caller can direct a bounded download into its own writer:

```go
svc := regional.Swift()
bucket, err := svc.Bucket("selected-container")
if err != nil { return err }
object, err := svc.Object(bucket, "selected/object")
if err != nil { return err }
result, err := svc.Download(ctx, object, destinationWriter,
    swift.DownloadOptions{MaxBytes: 64 << 20}, types.CallOptions{})
// result.Data.Bytes is the emitted prefix even when err is non-nil.
// destinationWriter stays caller-owned; handle a partial local file explicitly.
if err != nil { return err }
if !result.Data.Complete { return types.NewError(types.Incomplete) }
```

For an upload, pass `Upload(ctx, object, sourceReader, exactSize, options, call)`.
Always inspect the returned receipt before recovering from an error. An Unknown
receipt forbids automatically invoking Upload again, even with the same name.
Public executable transport examples are also retained in
[swift_test.go](../../../swift_test.go); they use synthetic credentials and no cloud.
