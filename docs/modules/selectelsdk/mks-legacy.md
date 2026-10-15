# Managed Kubernetes legacy

`Project(id).Location(region,pool).MKSLegacy()` selects MKS generation `v1`.
It does not convert v2 refs or fall back between generations. Source inventory
contains28 operations from the current published [legacy schema](https://docs.selectel.ru/api/managed-kubernetes-legacy.json).
The current slice implements eleven operations: cluster list/get/create/update/delete, minor/patch upgrade, certificate rotation and three configuration reads; the remaining17 operations
are unimplemented. Whole legacy inventory including DNS/storage is incomplete,
coverage_ratio=null; published sources are not live deployment verification.

Project Keystone auth and HTTPSv1 pool endpoints come from the official
[mks-go README](https://github.com/selectel/mks-go/blob/f92a1665d37c52c3a28c70dae39981363d81c623/README.md).
This pinned source resolves the schema's HTTPS-global/HTTP-operation discrepancy
without sending credentials to HTTP. ru-9v1 is its own documented endpoint,
not the v2 ru-8 alias. Unlisted pools require exact Config.EndpointOverride.
SDK source region means pool ID, compared with caller scope.Pool; no field is
used to select a host or infer resource ownership.

Generated DTOs preserve optional fields, exact numeric values and protected
opaque data/certificate configuration. Raw unknown status/CNI values survive
without invented readiness classification. Returned root refs require matching
caller project/pool; Get also requires exact ID/generation/endpoint. Response
provider `ref` is ignored. Neutron associations remain metadata, not parent refs.
List is the finite provider array documented by the pinned SDK; local bounds
return incomplete collections/cursors, never false completeness.

Owned source gap #71: kubeconfig schema declares204/bodyless while official SDK
ExtractRaw expects payload. No successful YAML/status contract is invented.
DNS legacy is marked outdated/migration recommended, not proven withdrawn.
S3 notice states old settings/users/URLs stop working after2026-09-15; it does not
prove withdrawal of every Swift-old/Selectel-storage-old operation. These families
need separate source reconciliation; #74 tracks inventory drift and #76 acceptance.
File Storage legacy resize remains #70/PR115 and is not duplicated here.

No cloud resources, old credential auth endpoints or kubeconfigs were contacted.
All tests use the public Client with synthetic credentials and only transport
replacement. No v1 wait goals are claimed.

ListKubeVersions, ListAdmissionControllers and ListFeatureGates expose bounded
finite source arrays. Native capitalized KubeVersionMinor/Names fields are
preserved; Names is an array, and no default configuration is invented.

CreateCluster validates the generated native request tree, caller pool and
base64 user_data, forwarding the protected value unchanged only at the private
wire boundary. An accepted identity (id/project_id/region) is scanned before
other fields so malformed unrelated fields do not lose the created root ref.
DeleteCluster204 is acceptance, not an absence assertion. Unknown mutations
never replay or infer a new resource; exact known refs and receipts survive.
The narrow reviewed early ContractRevision fix from PR97 commit
379458e4f658b1216137fea4acce5dae6027d320 (delta82121d740a2788930ce7c4934d449c35b096966d)
is reused in internal/core/request.go; no volume feature code is included.
Private wire rendering follows the existing baseline mks serializer.

UpdateCluster, UpgradeMinorVersion and UpgradePatchVersion check exact returned
cluster identity before adopting a ref, retaining caller receipt/ref on accepted
foreign/malformed responses. RotateCertificates204 is only acceptance and does
not invent a task. Optional false update fields are preserved. Generated opaque
source-object fields reject scalar/null shapes rather than accepting arbitrary
ProtectedJSON as a schema object.
