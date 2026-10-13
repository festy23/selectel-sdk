# Repository agent guide

## Sources of truth

- SDK contracts: [docs/modules/selectelsdk](docs/modules/selectelsdk/README.md).
- Public API coverage: `catalog/manifest.json`, generated from pinned sources,
  SDK-owned contracts and `catalog/bindings.json`.
- Cloud operations: use [selectelsdk](.agents/skills/selectelsdk/SKILL.md).
- Domain terms: [CONTEXT.md](CONTEXT.md).

## Development

The Go module is at the repository root. Read the selected service contract
before changing its method or observable behavior; update the contract with the
change. Preserve unrelated working-tree changes.

Edit schemas/models through their generators. `go generate .` regenerates DTOs
and the capability catalog using local public snapshots only. A new binding
needs typed methods, complete request/response/error mappings, and relevant
evidence files; a raw HTTP call does not establish API coverage.

Run `python3 scripts/verify_catalog.py` for catalog generation, race tests, vet
and revision-bound offline evidence. Check generated output and Markdown links
before committing. Keep offline, wire and live claims distinct. Source inventory
incompleteness requires `coverage_ratio=null`.

## Cloud operations

Use explicit credentials, account, project, location and API generation. Existing
user authorization carries across steps; SDK development and local testing do
not themselves authorize creating, resizing or deleting live cloud resources.

Retain mutation receipts on errors. Unknown outcomes require exact-reference
observation, not replay, name lookup or adoption of newly appeared resources.
Apply operation-specific wait goals and preserve incomplete collections.

Keep credentials, secret values, kubeconfig, user-data and raw telemetry out of
Git and diagnostic output. Use protected values only at the explicit request or
caller-directed reveal boundary.
