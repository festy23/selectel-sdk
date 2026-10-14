# Logs: confirmed CloudWatch subset

Selectel explicitly documents five write operations: CreateLogGroup,
CreateLogStream, DeleteLogGroup, DeleteLogStream and PutLogEvents. Their typed
methods are CreateGroup, CreateStream, DeleteGroup, DeleteStream and PutEvents.
The four existing read bindings remain partial. The family inventory remains
incomplete and coverage_ratio is null; the full AWS API is not Selectel scope.

The [pinned Selectel evidence](../../../catalog/evidence/logs-write-sources.json)
contains add/delete, retention and access/client pages. A normalized virtual
catalog path selects X-Amz-Target; all actual requests POST `/` with
application/x-amz-json-1.1 and SigV4 service `logs` at the selected documented
pool endpoint. Credentials are explicit project-bound S3 material. Exact
account/project/region/pool, API generation and endpoint are immutable. An
explicit endpoint override must match that scope. Redirects are rejected.

CreateGroup takes the exact name; CreateStream takes an explicitly scoped group
ref and name. DeleteGroup requires a group ref; DeleteStream and PutEvents
require a stream ref with its explicit group parent. Names are API locators,
never ownership evidence. DeleteGroup removes all child streams/events;
DeleteStream removes its events. Active producers may recreate names. No
absence/readiness or authorship waiter is claimed.

PutEvents submits one ordered batch of generated InputLogEvent values. Exact
integer milliseconds are nonnegative int64, with at most 10,000 events,
1,048,576 total UTF-8 message bytes plus 26 bytes/event, and a 24h span. Messages
are ProtectedValue, revealed only at HTTP encoding. No implicit sorting,
splitting, provisioning, retention calculation or sequence-token repair occurs.
Age/retention rejection is left to the provider, not guessed from a local clock.
The response preserves rejection indexes, protected nextSequenceToken and
protected rejectedEntityInfo.errorType if returned by a compatible provider.
The latter is documented by the current AWS response and does not establish
Selectel support for entity input.
HTTP200/Receipt Accepted is an acknowledgement, not proof every event was
accepted. A rejected prefix/suffix is not discarded or silently retried.

Every mutation runs once and retains its Receipt on errors, including accepted
malformed responses and unknown transport outcomes. Existing exact refs remain
available for caller-directed observation. Recovery=resource in a receipt names
that locator class; no automated Resume, name lookup, adoption or replay is
provided. CallTimeout and MaxJSONBytes bound requests/responses. Provider error
messages and token hints are discarded; only known safe error codes survive.

Only fields explicitly demonstrated by Selectel establish request support.
AWS KMS/tags/class/entity/sequenceToken request variants are unconfirmed and
not exposed. The upstream response/error model supplies compatible wire shapes,
not evidence that every AWS feature exists. Retention is documented as a panel
workflow; PutRetentionPolicy/DeleteRetentionPolicy are not invented.

The [example](../../../examples/logs-write/main.go) requires an explicit service
view/ref and sends one caller-owned event without printing content. Offline
verification uses public Client methods with synthetic credentials and HTTP
transport substitution. It is not live cloud evidence.
