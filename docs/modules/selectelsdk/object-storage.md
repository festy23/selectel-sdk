# Object Storage management v2

`RegionalClient.ObjectStorage()` uses a separate `object_storage/v2` management
view, explicit account/project/location and IAM project token. Provider “account”
stats/quotas describe this storage project view, not IAM account scope. It never
uses S3 signatures, credentials or data-plane refs. Endpoints use only the
published Object Storage regional URL map, with exact explicit overrides.

The pinned v2.1.0 schema has 13 operations: account info/quota get/set, container
domains list/add/delete, domains/certificates get, options get/set, and log export
list/create/get/cancel. Generator-owned request/response models preserve omitted,
null, false, empty and exact integers; arbitrary filters/provider free text stay
protected. No undocumented certificate structure or log state enum is inferred.
The domains/certificates response references ContainerDomains (strings), despite
its heading; certificate details remain source-unknown. Logs list omits security
while every other operation explicitly uses project IAM: the SDK authenticated
view is qualified partial for this source inconsistency.

Container refs are explicit named management addresses, not S3 resource adoption.
Mutation receipts survive errors and never authorize replay. Log export unknown
create without an ID cannot be recovered by list/name matching. Exact task GET
may observe a known ID without proving creation; unsupported readiness/state
semantics are not invented. Bounded pagination preserves valid prefixes/errors.
Provider timestamps without declared timezone formats remain raw strings.

Source: [current schema](https://docs.selectel.ru/api/object-storage.json), SHA-256
`892af4f017c5e21aa9cb2647ce817700e43e675fd27096e7b2307f2104694d4c`,
[API](https://docs.selectel.ru/api/object-storage/) and
[published endpoints](https://docs.selectel.ru/api/urls/).
All verification is offline with synthetic credentials and HTTP transport only;
no live storage changes are authorized by development.

## Public API and examples

`Account.Info/GetQuota/SetQuota`, `Containers.Ref/Domains/AddDomain/DeleteDomains/
DomainsAndCertificates/Options/SetOptions`, and `LogExports.Ref/Create/Get/Cancel/
ListPage/Iterate/Collect` use the scoped view. Container names and task IDs are
encoded as single path segments; all exact refs pin endpoint and generation.
The two source ambiguities above remain explicit partial bindings; other eleven
current-schema operations are implemented. Inventory completeness is not inferred
for legacy/discovered surfaces and coverage_ratio remains null.

```go
project, err := client.Project(projectID)
if err != nil { return err }
regional, err := project.Location(region, pool)
if err != nil { return err }
management := regional.ObjectStorage()
updated, err := management.Account.SetQuota(ctx, objectstorage.SetQuotaOptions{
    QuotaBytes: types.Null[types.Integer](), // explicit removal, not omitted
}, types.CallOptions{})
// Save updated.Receipt even when err != nil; never replay an unknown outcome.
if err != nil { return err }
_ = updated
container, err := management.Containers.Ref(containerName)
if err != nil { return err }
current, err := management.Containers.Options(ctx, container, types.CallOptions{})
if err != nil { return err }
_ = current
```

Lists send explicit limit/uint64 offset/order_by; absent initial offset starts
SDK iteration at zero. A short nonempty page is not end; offset advances by exact
wire item count and an empty tasks array ends best-effort traversal. SDK wire
page cap is 1000, not a claimed provider default/maximum. Cursor binds initial
offset, order, project/location, contract revision and endpoint URL. Errors and
limits retain available prefix/cursor without claiming snapshot consistency.

Root `Client.Resource/Resume` dispatches only these management refs. Status is
presence observation (classification succeeded/provider state present), not log
export completion/cancellation/file readiness. No wait goals are advertised.
Known receipt outcomes are conclusive without I/O, including receipts without
refs; unknown receipts require a trustworthy exact ref and remain indeterminate
after observation. Account quota unknown with no ref is RecoveryUnavailable.
Container readback is parent/address observation, not proof of a domain/options
mutation. Request validation failures retain revision-bound NotSent receipts.
No S3 resource conversion, task list/name lookup, adoption or mutation replay.

Public Client tests use synthetic CredentialSource, real external-token
validation and mock HTTP RoundTripper only. Source snapshots and shared code
provenance are recorded in the [generator README](../../../internal/objectstoragegen/README.md).
