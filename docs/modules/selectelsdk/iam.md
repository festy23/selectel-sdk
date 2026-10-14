# IAM v1: users, service users and groups

`client.Account().IAM()` is the account-scoped management API. It is separate
from runtime IAM authentication. The provider generation is explicitly `v1`.
The complete pinned [IAM schema](../../../catalog/sources/users-and-roles.json)
is sourced from [Selectel IAM API](https://docs.selectel.ru/api/users-and-roles.json).
Endpoint `https://api.selectel.ru` is confirmed by
[official iam-go f104d951](https://github.com/selectel/iam-go/blob/f104d9516f832b3ce7ee33840f3665aeaf8e8706/iam.go).
Current upstream HEAD was checked against that revision during implementation.

The callable subset contains 24 operations:

| Interface | Methods |
| --- | --- |
| `Users` | `Iterate`, `ListPage`, `Collect`, `Create`, `Get`, `Update`, `Delete`, `ResendInvite`, `AddGroups`, `RemoveGroups` |
| `ServiceUsers` | `Iterate`, `ListPage`, `Collect`, `Create`, `Get`, `Update`, `Delete`, `AddGroups`, `RemoveGroups` |
| `Groups` | `Iterate`, `ListPage`, `Collect`, `Create`, `Get`, `Update`, `Delete`, `AddUsers`, `RemoveUsers` |
| `CurrentUser` | Read the authenticated subject |
| `CurrentGroups` | `Iterate`, `ListPage`, `Collect` for the authenticated subject's groups |

Create and update DTOs cover every declared property, including descriptions,
roles, federation identifiers, group IDs and subscription fields. Optional
pointers preserve absence, false and empty strings; non-nil empty slices encode
an explicit empty array. No subscription defaults are injected. User updates
modify descriptions; service user updates additionally support enabled, name
and password. Role and credential management operations remain unimplemented
and visible in the catalog for follow-up issues #25 and #27.

Existing resources are addressed by exact `ResourceRef`, never by name.
Group `AddUsers`/`RemoveUsers` require Keystone IDs. `Users` methods address panel
user IDs; `ServiceUsers` methods address service user IDs. User-side group
methods add/remove the explicitly supplied IDs, without replacing memberships.
The documented resend-invite operation uses **POST**, despite the older official
SDK using PATCH. Calls can send invitation emails; they are explicit mutations.
Delete service user also deletes its keypairs according to the provider contract.

Service user passwords use `types.ProtectedValue`. Formatting and normal JSON
serialization redact them. The request encoder reveals the password only to
the configured transport. Protected passwords are never attached to results,
receipts or observers. Provider error bodies are discarded by the shared runtime.

Mutations return `types.Result[T]` with a Receipt even for validation, rejection
or unknown outcome. Successful create requires a nonempty provider ID. A valid
ID already decoded is retained in an accepted Receipt if other fields fail to
decode. Exact reads and updates reject a conflicting returned ID. Documented
4xx rejections and HTTP 403 preserve rejection; lost transport responses and
ambiguous mutation failures produce unknown and are never replayed. There is
no IAM wait or automatic resume/adoption contract; use exact `Get` observation.

Lists are documented as one provider response without server pagination.
The shared iterator enforces MaxItems/MaxPages, MaxJSONBytes, CallTimeout and
CollectionTimeout. Local truncation returns an incomplete collection and a
scope/endpoint/generation/contract-bound continuation, which replays the page
and skips its prior prefix; completeness does not imply snapshot consistency.
All DTOs are generated from the pinned schema. Verification is offline only.

See [the account IAM example](../../../examples/iam-users/main.go).
