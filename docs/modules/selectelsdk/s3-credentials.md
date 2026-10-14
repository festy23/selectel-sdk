# IAM v1 S3 credentials

`client.Account().S3Credentials().V1` is an independent control-plane facade in
package `iamcredentials`, separate from runtime credential loading and S3 object
transfers. It supports all 13 credential operations in the pinned
[IAM schema](../../../catalog/sources/users-and-roles.json), sourced from
[Selectel IAM API](https://docs.selectel.ru/api/users-and-roles.json).
The schema bytes and canonical operation IDs are shared with reviewed IAM commit
`e1beb01d797e06388204f520428b5849852dc52f`; there is one IAM family/inventory,
not a second imported copy. Targeted refs are compatible with that IAM facade.
Endpoint `https://api.selectel.ru` follows
[official iam-go f104d951](https://github.com/selectel/iam-go/blob/f104d9516f832b3ce7ee33840f3665aeaf8e8706/iam.go).
The official SDK's service-user subset is extended by the pinned public API.

| Interface | Operations |
| --- | --- |
| `ServiceUsers` | Targeted `Create`, `Iterate`/`ListPage`/`Collect`, `Delete`, `SearchIterate`/`SearchPage`/`Search` |
| `Users` | Targeted `Create`, `Iterate`/`ListPage`/`Collect`, `Get`, `Delete`, `SearchIterate`/`SearchPage`/`Search` |
| `Current` | `Create`, `Iterate`/`ListPage`/`Collect`, `Get`, `Delete` using the explicit current-subject route |

`Target(userID)` is a local constructor for an explicitly selected user ID.
It performs no credentials lookup or resource observation and does not adopt a
resource by name. Service-user targets have kind `service_user`; panel-user
targets have kind `user`. The named-user interface rejects the reserved `me`
alias, which belongs to `Current`. The API has no service-user single-key GET;
no such method is invented.

Every control-plane request uses **account IAM auth**. `CreateRequest.ProjectID`
is the explicitly selected credential project; `SearchFilters.ProjectID` is a
required target filter, and UserID is an optional filter. These fields do not
change auth scope to project IAM. Create requires a nonempty name and explicitly
selected project, following the official SDK contract. Source-only request DTOs
serialize just name/project_id. Returned project IDs must match a supplied
create/search project. Ref scope, generation, kind, parent and endpoint checks
reject foreign targets before resource I/O.

AccessKey is the non-secret credential identifier, preserved in exact refs.
`CreatedCredential.SecretKey` is `types.ProtectedValue`, returned only by Create.
Formatting, JSON and text serialization redact secret material; `Reveal` is the
explicit caller-directed disclosure boundary. Metadata Get/list/search never
promise to recover the secret. Provider error bodies are discarded; receipts
and observers contain metadata and identifiers, never secret keys.

Targeted credential refs retain their exact user parent and opaque access key.
Current-user credential refs have a distinct `s3_current_user_credential` kind
and `current_user` parent with the provider's `me` selector. They are explicitly
relative to the caller-selected authenticated principal, not proof of an
absolute panel-user identity; retain the same principal when observing them.
They cannot be passed into named-user or service-user methods. There is no
automatic Resource/Resume adoption or replay contract for these relative refs.

Create returns an accepted Receipt at HTTP 201. A trustworthy key reference is
retained if subsequent secret/body decoding fails. A foreign returned project
is rejected before adopting a new key reference. Delete returns accepted at
HTTP 204. Documented 4xx rejections and account-auth 401/403 retain rejection;
lost transport responses and ambiguous mutation failures retain unknown and
are never replayed. Cancellation before sending preserves not_sent.

There is **no atomic rotation API or automatic rotation helper**. Callers create
a new credential, explicitly store/install/test it in their application, and
separately revoke the exact old key. Keep each step's Result/Receipt, including
errors: accepted create plus unknown revoke is partial completion. The SDK
never deletes old/new keys automatically, infers successful installation, or
repeats an unknown create/revoke. See [the explicit step example](../../../examples/s3-credentials/main.go).

Lists are one provider response. Search returns provider groups of credentials;
the adapter flattens these into credential items before applying MaxItems, so
one large group cannot bypass collection limits. Search retains source user and
role metadata; refs are attached only when the source or explicit filter identifies
a user. Lists/search use MaxItems/MaxPages, MaxJSONBytes, CallTimeout and one
CollectionTimeout. Truncation is incomplete with a bound replay cursor and skip;
cursors bind scope, endpoint, generation, target/filter and contract revision.
They do not promise snapshot consistency. All DTOs are generated; verification
is offline only and global coverage remains unknown while other inventories
are incomplete.
