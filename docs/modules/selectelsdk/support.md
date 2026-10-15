# Support tickets and attachments

`client.Account().Support()` exposes `Tickets` (current v2), `Legacy` (v1
contracts and shared legacy-path actions), and `Attachments` (`attachments_v1`).
Ticket numbers, comment IDs and file UUIDs identify different kinds. There is no
automatic generation fallback. Deprecated v1 CRUD/topics remain explicitly
callable; a 403 alone does not mean withdrawal.

## Scope, endpoints and authentication

All views bind the explicit account, without project/location. Attachment schema
publishes `https://api.selectel.ru/files`. Ticket schema/page has no host/server/
base path; the official URL table and additional primary searches did not establish
one. Exact ticket endpoint overrides are required separately for v1 and v2.
This search limitation does not prove that no official endpoint exists.

Static `X-Token` is the default. Ticket operation schemas prefer account IAM and
permit static, while the official authentication table says tickets/attachments
are static-only. Ticket calls support an explicitly requested `IAMAccount`
`X-Auth-Token` wire variant, without automatic fallback or a live deployment claim.
Attachments support static only. Observation uses the default static mode.

## Coverage and source gaps

Twenty ticket operations and seven attachment operations are reconciled against
pinned sources: **23 implemented and four partial** in this enumerated subset.
Whole-service inventory is incomplete and coverage_ratio is null. Issue #64 owns:

- Current ticket list has no total/next/end rule. Bounded explicit pages and
  partial replay are supported; no count request is silently added.
- Legacy ticket list proves its last page with `page_count` when provided.
  Missing optional `page_count` cannot establish continuation or completion.
- Attachment list has no pagination metadata/end rule. `state`/`visibility`
  have unconstrained schemas; explicit opaque string filters are supported,
  arbitrary structured serialization is not invented.
- Attachment download defines only 422, with no successful status/body/headers/
  redirect contract. `Download` validates the exact ref and refuses before
  credentials/HTTP/writer I/O. It claims zero successful transfer coverage.

Common pagination bounds and scope/operation/endpoint/revision/filter-bound
cursors apply. Partial-page replay keeps issued row order; envelopes omit their
duplicate rows while retaining typed metadata. Missing arrays are not empty
success. Taxonomy, comments and topics use their documented nonpaginated page;
caller truncation remains incomplete. Unconfirmed end conditions return
`protocol_error` with issued rows retained, never Complete=true.

## Typed payloads and protection

DTOs, inline schemas, inherited shapes and request validation are generated from
local snapshots. Required/nullable fields, enums, UUIDs, ranges, string lengths
and array limits are checked before credentials/HTTP. Exact integers do not pass
through floats. Optional false/zero and explicit nullable null stay distinct.
The legacy `is_client_read` boolean is pinned from its explicit source example,
which lacks a type. The source's ticket vote prose says two-point scale but its
numeric bounds are 1..10; the SDK uses those explicit bounds.

Ticket/comment text, localized names, filenames, links, antivirus logs and
unconstrained values/unknown extensions are protected. Ordinary JSON and fmt,
including direct nullable wrappers, do not reveal literal or decimal/hex byte
storage. Explicit typed mutation encoding alone reveals caller-provided values;
opaque `body_values` is limited to its source-declared object field. Provider
messages and validation content never become diagnostics. Provider-injected SDK
refs are ignored.

All mutations retain receipts on every path and never replay unknown outcomes.
Acknowledged ticket/comment/file IDs are scanned before independent fields are
decoded. Comments keep only an explicitly supplied caller ticket parent;
`ticket_number` metadata alone never manufactures ownership. Comment creation
retains the ticket anchor and acknowledged child ref; Resume selects the child
only when the exact parent anchor is also present. Missing child identity cannot
be recovered by name lookup or by adopting another comment.

## Transfers and observation

Upload sends multipart field `data` using the declared finite payload size,
optional caller filename/MIME/actor UUID, and the shared transfer budget. It does
not probe beyond the size or close the caller reader. The synchronous response
decoder owns response closure exactly once. Bytes reports input consumed on
success, transport error, cancellation and rejection; it does not report persisted
server bytes. Acknowledged metadata is protected and links are never followed.

Root Resource/Resume support exact current/legacy ticket, comment and file refs.
Receipt goals are filtered by the originating catalog operation. Ticket close/
reopen use distinct goals. Legacy waits require explicit `is_closed` true/false;
missing/null cannot complete. Current v2 preserves raw status keys and leaves
future states unknown. `ticket_observed`, `comment_observed` and
`file_metadata_observed` describe exact metadata observation; they do not prove
ACL changes, votes, support replies, antivirus cleanliness or download readiness.
Unknown mutation outcomes remain indeterminate and never authorize replay.

## Evidence and example

The [ticket snapshot](../../../catalog/sources/tickets.json),
[attachment snapshot](../../../catalog/sources/tickets-attachments.json) and
[source/reuse evidence](../../../catalog/evidence/support-current-sources.json)
record revisions, discrepancies, reviewed common-code provenance and integration
overlap. The narrow Nullable formatter comes from reviewed PR #80 commit
4534c4e; DoResponse/StructuredLimit comes from reviewed Swift PR #96 head8a07c83.
Early receipt contract revision initialization narrowly reuses reviewed PR #97
commit379458e4 (delta82121d7), adapted to DoResponse and covered by all ten
local mutation receipt paths. Future integration must preserve the parallel
ErrorProtocol/status adapter work.
No unreviewed service feature diff was copied.

```go
service := client.Account().Support()
fileRef, err := service.Attachments.File(fileUUID)
if err != nil { return err }
metadata, err := service.Attachments.GetMetadata(ctx, fileRef, types.CallOptions{})
// URLs, filenames and antivirus output remain protected.
```

Tests use the public Client with synthetic credentials and only HTTP transport
substitution. No real tickets, comments, support contact, files, emails or live
cloud changes were created. Offline evidence is not live compatibility evidence.
