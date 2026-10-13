# Verification scope

Inspect `catalog/manifest.json`, `bindings.json`, and
`verification.json`; use their current revisions rather than a copied count.
Each callable binding names implementation files, contract artifact and evidence
files. Verify that the relevant tests invoke the selected method and cover the
behavior claimed. A shared passing test file is not proof of every optional
parameter, failure mode or provider location.

From the repository root, `python3 scripts/verify_catalog.py` generates the
catalog, runs `go test -race ./...` and `go vet ./...`, and records `offline`
proof only after both checks pass. It makes no Selectel requests; HTTP transport
is replaced by test fixtures. Generation itself reads pinned public schemas.
Changing a contract, implementation or evidence revision invalidates stale
verification on regeneration.

For MKS v2, `mks_management_test.go` invokes every operation in the pinned schema
and separately checks identities/parents, redaction, known IDs on accepted decode
failures, waits, unknown states, pagination and lost mutation replies. These tests
do not prove that a real account has the roles/quota, that endpoints are reachable,
or that resources can be created in a particular pool.

Live verification is a separate operation with explicit scope and user-authorized
effects. Start with authentication and the requested bounded reads when access is
available. Record success/error and scope for each actual method called. Creating,
resizing or deleting resources requires authorization for those effects; running
the local suite or requesting a skill does not grant it. Reuse authorization
already present in the session without asking again.

Report `offline`, `wire` or `live` only for the scope and evidence that establish
that level. Current catalog generation does not ingest arbitrary live evidence:
report an actual live check separately until a supported evidence contract exists.
Never edit verification.json manually to upgrade offline records to live.
