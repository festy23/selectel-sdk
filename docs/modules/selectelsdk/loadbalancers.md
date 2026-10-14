# Octavia load balancers v2

`regional.LoadBalancers()` exposes project/location-scoped `Create`, `Get`,
`Update`, `Delete`, `ListPage`, `Iterate`, `Collect`, `StatusTree`, `Statistics`
and `Failover`. `Listeners`, `Pools`, `Members` and `Monitors` expose typed CRUD
and the three bounded list forms. Listeners additionally expose `Statistics`.
Members expose `BatchUpdate` with explicit optional `additive_only`; `Replace`
sets it to false. Replacement deletes omitted address/port pairs; additive mode
preserves them. Neither form promises atomicity.

The pinned Selectel compatibility matrix identifies Octavia 2023.1, API 2.26.
Local schemas/API reference are pinned to Octavia 12.0.0 commit
`4486d2416f600df15df1b5c98a676f898e2b1d42`; published Gophercloud v2.13.0
requests/results/URLs provide additional SDK wire evidence. Generation never
contacts the cloud. The release advertises versions through 2.26 at `/v2` and
`/v2.0`; these are published route aliases, not Nova-style negotiated
microversions. No implicit upgrade or invented version header is sent.

The catalog contains 30 operations: 25 implemented bindings and five partial
Create/Update bindings whose TLS/L7 inline request variants belong to #41.
Create LB can create the full typed core graph; its response preserves full
listeners, pools, members and health monitor data as well as ID-only variants.
L7 policy/rule operations and TLS request settings are outside #40. They are
confirmed omissions, not unsupported API. Whole upstream inventory, provider
administration, flavors, quota/availability-zone APIs and Selectel extensions
(such as log offloading) are not claimed complete. Current Terraform docs also
mention security-group settings beyond the pinned 2.26 core schema; their
Selectel extension/version semantics require separate source inventory proof.
`inventory_complete=false` and `coverage_ratio=null` remain mandatory.

## Scope, identity and authorization

Methods use IAM project credentials and the exact public `load-balancer`
endpoint in the selected pool from the authenticated catalog, or an explicit
matching override. Root, `/v2`, `/v2.0` and proxy-prefixed bases are supported;
version routing never changes the saved exact endpoint URL. Endpoint ambiguity
fails instead of selecting the first catalog entry.

`project_id` remains provider owner metadata. Returned refs retain the caller's
authorization view; ownership does not prove permissions or change scope.
Listeners link to their LB, pools link to their LB, and monitors/members link to
their pool. Pool creation accepts an exact LB or listener parent; response
relationships are checked. A pool ref inferred from a monitor response contains
the known pool identity only: that response does not establish its LB parent.
Member addressing always requires the pool ref. Known ancestor chains are
retained, including fully populated create responses. Missing projected IDs or
relationships produce no ref; the SDK never adopts a provider-supplied `ref`.
Every exact resource/parent call pins the saved endpoint ID, URL, scope and
generation against the newly resolved endpoint. Changed authenticated catalogs
or overrides fail before target I/O, including Resume/Resource Status,
diagnostics, child mutations and parent lists. Wrong scopes, kinds, parent
chains or endpoints are rejected. No implicit parent
lookup or name adoption occurs.

VIP network/subnet/port IDs, default pool IDs and other native related-resource
attributes retain source DTO semantics; authorization and cross-project
accessibility remain provider decisions. They are not inferred ownership proof.

## Requests, results and errors

Optional pointers preserve zero, false and empty arrays. Explicit null uses
`types.Nullable` only for supported fields, including session persistence, VIP
QoS removal and member monitoring overrides. Parent IDs are injected from exact
refs. Listener timeouts are integer milliseconds; health monitor delay/timeout
are integer seconds. Counters and request integers retain arbitrary precision.
Provider `created_at` and `updated_at` remain raw strings; no missing timezone
is invented. Required inputs and source integer bounds are checked locally;
provider protocol/plugin combinations and permissions remain server checks.

Create accepts 201 with the resource envelope. Updates preserve source success
variants: LB/listener/monitor/pool 200 or 202, member 200/201/202. Delete accepts
204. Failover and member batch updates accept 202 without resource creation
proof. Every mutation returns a Receipt, including local `not_sent`, documented
400/401/403/404/409 rejection, acknowledged malformed responses and unknown
outcomes. Identity and required relationships are validated before the rest of
the response DTO; an acknowledged malformed counter or other unrelated field
retains the trustworthy target ref. Malformed IDs and foreign parent relations
never establish a target. Mutations are sent once. Other unexpected statuses, including 429,
remain conservative unknown outcomes with a sanitized cause; reads retain
shared bounded retries and status classification. HTTP 400 is `provider_error`;
403 is `permission_denied`, 404 is scoped `not_found`, 409 is `conflict`, and
429 reads are `rate_limited`. The API also uses 403 for quota failures but does
not document an unambiguous machine code in this pinned error contract. SDK
errors discard fault strings and debug information rather than guessing quota
classification from provider text.

## Collections, waits and recovery

Lists preserve repeated source filters, tags/tags-any/not-tags/not-tags-any,
Fields, sort and reverse pagination. Selectors and SDK parent-filter aliases
are generated separately. Marker links must remain on the exact endpoint/path
and preserve query semantics; SDK pagination applies item/page/time budgets,
saves bounded prefixes and validates continuation scope/endpoint/fingerprint.

Provisioning and operating states remain separate fields. `*.provisioned`
waits for provisioning `ACTIVE`; it does not require traffic `ONLINE` or prove
that applications are healthy. `ERROR` fails provisioning, documented pending
states continue, and unknown states fail as `state_unknown`. `*.absent` requires
exact GET 404 in the saved authorization view. Existing `DELETED` remains a
pending absence observation. Receipt-bound waits allow only the original
operation's catalog goals. Failover and member batch updates have no causal
completion goal: observing a pre-existing ACTIVE parent cannot prove that the
action completed. Callers may independently observe exact member refs they
already know; the SDK never discovers new members by address/name for recovery.

Root `Resource`/`Resume` dispatch all five kinds. A child create Receipt retains
its known parent and returned child; Resume chooses the unique matching child.
Known accepted/rejected/not_sent receipts recover conclusively without I/O,
including a receipt with no resource ID. Unknown receipts require the exact
operation target; parent-only creates are `recovery_unavailable`. Exact target
observation stays indeterminate and returns `unknown_outcome`, never replays.
The shared receipt-only/revision validation comes from reviewed commit
`379458e`; compiled endpoint prefix handling derives from reviewed `afaa6e9`
and adds only the pinned Octavia `/v2.0` alias.

```go
lb := regional.LoadBalancers()
result, err := lb.Create(ctx, loadbalancers.LoadBalancerCreate{
    VIPSubnetID: &subnetID,
}, types.CallOptions{})
// Persist result.Receipt even if err != nil. Only a returned exact ref supports
// a separate provisioning wait; an unknown create is never replayed.
if err == nil && result.Data.Ref != nil {
    handle, bindErr := client.Resume(*result.Receipt)
    if bindErr == nil {
        _, err = handle.Wait(ctx, "loadbalancer.provisioned")
    }
}
```

Public Client/HTTP tests are offline wire evidence with synthetic credentials.
No live deployment, failover, deletion or successful cloud availability is
claimed by these tests.
