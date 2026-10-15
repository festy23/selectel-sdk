# CRAAS token lifecycle sources

Generation reads only pinned `catalog/sources/craas.json`, CRAAS v2.2 SHA-256
`a6e68b67cd861617e5d66304891d4523f685593b36fbcdb4538ffc76de2bb314`.
It matched https://docs.selectel.ru/api/craas.json byte-for-byte on 2026-10-15.
The official public SDK HEAD also remained
`8f0d0cba0b59360f344b3b2d32157b1549c2645a`; snapshots are verbatim files:

- assets/v1-requests.go.txt: pkg/v1/token/requests.go
- assets/v2-request.go.txt: pkg/v2/token/request.go
- assets/v2-request-opts.go.txt: pkg/v2/token/request_opts.go

Canonical IDs remain four existing v1 token IDs and eight v2 token IDs. The
existing five registry lifecycle bindings remain compatible; no queued repository
management feature is copied. Every source DTO field and required declaration is
generated. Query-only DockerConfig is added to TokenCreateRequest from the
operation's declared docker-config parameter, never serialized as a body field.
CreateDockerConfig returns protected JSON as a whole because its internal schema
is not documented; it does not infer a token ID from a configuration.

Shared reused deltas: operations.BindReceipt and early contract revision from
reviewed 379458e, pagination validated-error-prefix behavior from reviewed a49e66b.
Root dispatch is limited to exact v2 token handles. Legacy v1 credential paths
never enter ResourceRef or Receipt. Public tests substitute HTTP RoundTripper and
synthetic CredentialSource only; no live token or cloud operation is executed.
