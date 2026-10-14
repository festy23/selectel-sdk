# Glance image metadata and membership v2

`RegionalClient.Images()` exposes image Create/Get/Update/Delete and
ListPage/Iterate/Collect, plus Members.Create/Get/Update/Delete and bounded
Members.ListPage/Iterate/Collect. Ten canonical remote operations are bound.
Update is **partial**: owner transfer and configurable location/store mutations
are excluded. A generic SDK PatchOp is not proof of Selectel admin permission or
server configuration. Upload/import/tasks/download belong to issue #39.

The [Selectel images-go source](https://github.com/selectel/images-go/tree/ace640b1fd6a1bc13cf3c8d7be7a77c9777a3362)
is pinned locally with SHA-256. Its pkg/v1 is the Go SDK contract, not wire API.
[Selectel's version matrix](https://docs.selectel.ru/en/cloud-servers/tools/openstack-cli/about-openstack-cli/)
confirms Glance Zed 2.16. Metadata request semantics are also checked against
[pinned Glance25.0.0](https://github.com/openstack/glance/blob/d45b7c26af2963bb811c5596a42be10cb75c0685/glance/api/v2/images.py).
No newer Glance behavior is implicitly selected. No full upstream inventory or
live availability is claimed: inventory_complete=false, coverage_ratio=null.

## Scope and endpoint

Explicit account/project/location and CredentialSource select project IAM auth
and the regional `image` catalog service. Endpoint may be root or end in `/v2`;
the compiled version-prefix rule prevents `/v2/v2`, including a proxy prefix.
Refs preserve the original exact endpoint URL/ID and the **caller auth view**.
Image owner remains response metadata; reading a public/shared image does not
adopt its owner's project. Mutations defer owner/admin permissions to the API.

Member Ref.ID is the member project ID and Parent is the exact image reference.
This is a composite provider address, not an unrelated UUID. To accept an image
in a destination project, first obtain the image/member references through that
project's scoped client. Owner-view references cannot be reused in another view.
[Selectel's membership documentation](https://docs.selectel.ru/en/terraform/openstack-provider-reference/images-glance/resources/openstack_images_image_access_accept_v2/)
confirms the destination project semantics; privileged membership status changes
are also described in the [source-project documentation](https://docs.selectel.ru/en/terraform/openstack-provider-reference/images-glance/resources/openstack_images_image_access_v2/).
No local owner!=caller check invents or removes admin permissions.

## Metadata and PATCH

Create covers the SDK fields: name, optional caller UUID, visibility, protected,
os_hidden, container_format, disk_format, min_disk, min_ram, tags and flattened
custom properties. Optional pointers preserve explicit zero/false/empty arrays.
Name/container/disk formats use Nullable to preserve documented null. Sizes and
requirements use exact Integer, including provider null/absence without a float.
Custom write properties are ProtectedValue strings, explicitly revealed only at
the caller-directed request boundary. They cannot collide with base/reserved
fields, owner, or the os_glance namespace.

Update uses add/replace/remove JSON Patch and the source media type
application/openstack-images-v2.1-json-patch. Each Patch names one escaped JSON
pointer property; escaped slash/tilde in custom names work. Value is ProtectedJSON
and is validated against the selected field type. False, zero, empty strings,
empty tag arrays and nullable base strings remain distinct. Remove omits value.
Owner and system/read-only/reserved properties cannot be changed this way.
Nested tag element paths are not supported by pinned Glance; replace the tags
array instead. Location paths and admin owner transfer are explicit exclusions,
not declarations that the server lacks them.

Response preserves all pinned SDK base fields and optional/null integers.
Custom extensions retain exact JSON inside ProtectedJSON, including non-string
provider variants and names such as `ref` or `Name` that must not collide with
SDK fields. direct_url and locations are protected. They are not followed as
endpoints or used as credentials. Timestamps retain original timezone-less text;
Instant is available only for offset-bearing values, normalized to UTC.
Malformed timestamps return ProtocolError rather than inventing an offset.

## Collections and receipts

Image filters include pinned SDK fields, repeated tags, exact size bounds,
created_at/updated_at comparisons, base-attribute comparisons and custom-property
filters. Tags are server-trimmed while an explicitly supplied empty tag remains
`tag=` (nil/empty slice omits the filter); this extends the native SDK omission
helper using pinned server semantics. The documented in:/comparison syntax is preserved. Legacy sort_key and
sort_dir serialization is explicit and cannot be mixed with combined sort.
Custom property filters cannot override marker/limit/sort or other protocol
controls. Protected filter values contribute their actual values to a cursor
fingerprint without being serialized into the cursor or diagnostics.

Next links must preserve the exact operation/filter/host; only the documented
`/v2/images` API root can be reanchored to the selected proxy prefix. Foreign URLs
are rejected. All pagination uses bounded prefixes and revision/scope/endpoint
cursors. Members is a provider nonpaginated response; bounded continuation replays
only that read and applies a local skip. MaxJSONBytes bounds the provider body.

All mutations retain Receipt on errors and never replay after an unknown outcome.
Accepted metadata registration is not an uploaded/active image. A malformed
accepted response keeps trustworthy identity keys and Accepted; read errors have
zero object Data. HTTP400/405/406/415/422 map to InvalidArgument; 401/403/404/409/429
use shared status handling. HTTP413 maps QuotaExceeded by the pinned Selectel SDK
status rule (distinct from429 RateLimited), without provider-text inference. It
does not identify a particular quota counter. Messages/raw bodies are not retained or classified.

A caller-selected Create UUID is validated before I/O. An internal compiled pure
TargetRefs builder binds it after endpoint resolution and validates exact
scope/service/generation/endpoint. This seam is necessary because baseline has
no BindDocumented/pre-execution endpoint API. It performs no credential lookup,
HTTP, name search or provider discovery. The Receipt key is a **requested address**,
not creation proof; unknown Recover remains indeterminate after exact GET.
A mismatched response ID is rejected without adopting that resource.
Membership Create likewise retains its explicit composite request target.

Root Resource/Resume delegate to exact image/member handles. Known terminal
receipt-only Recover is conclusive without I/O, including no-ID accepted or
not_sent errors; unknown without an exact ref stays RecoveryUnavailable.
The shared implementation is reused from reviewed commit379458e, including
revision-before-validation; no Cinder service code is included.

Goals are image.active/image.absent and member.accepted/member.absent.
Unknown state is never success; bounds and cancellation use common wait settings.
Absent means404 for the selected auth view and exact address; it does not prove
physical deletion outside that view, membership revocation cause, or guest health.
Unknown mutations are observed without converting presence/absence into proof.

Public tests use a real Client/scoped API, synthetic CredentialSource and only
Config.Transport as the mock boundary. Generation, race/vet and evidence are
offline; no live cloud mutations were performed.

```go
name := types.Value("custom-image")
created, err := regional.Images().Create(ctx, images.CreateSpec{
    Name: name,
}, types.CallOptions{})
// Retain created.Receipt even on err; never replay an unknown mutation.
if err == nil {
    _, _ = regional.Images().Get(ctx, *created.Data.Ref, types.CallOptions{})
}
```
