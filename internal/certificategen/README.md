# Certificate Manager generation

The generator reads the unmodified public User Certificates OpenAPI snapshot
`catalog/sources/certificates-manager.json`, retrieved 2026-10-16, and emits typed
request/response DTOs plus 11 bindings/contracts. It does not call the network.
Optional request arrays use pointers so explicit empty arrays survive separately
from omission. Private PEM key input uses ProtectedValue and a private source-only
wire DTO; it never uses a response model containing SDK Ref fields as a request.

Pinned source: secretsmanager-go commit 5adf32225d3863ae40fd351ea33629f3fcca2447.
Licensed verbatim SDK evidence in assets (`LICENCE`, Apache 2.0): secretsmanager.go
(endpoint), service/certs/certs.go (11 HTTP operations/raw CA/key/PKCS12 result
formats), model.go (wire structures). Current OpenAPI has additional issuer and
flat consumer fields not present in those SDK DTOs: all are retained in generated
types. OpenAPI omits 3 raw response schemas; SDK provides the precise wire bytes
without guessing JSON/base64. Contract records hashes for the source snapshot and
every sorted asset; source inventory for the overall certificates service stays
incomplete because Let's Encrypt is a separate API/task.

Shared runtime reuse is narrow and reviewed: DoResponse 8a07c83,
receipt-only recovery/early revision 379458e, pagination error-prefix a49e66b.
No unrelated service feature or unreviewed shared fix is copied.
