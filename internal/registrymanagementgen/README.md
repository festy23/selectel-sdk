# Registry management source provenance

Generation reads only the existing pinned `catalog/sources/craas.json` (CRAAS
v2.2, SHA-256 a6e68b67cd861617e5d66304891d4523f685593b36fbcdb4538ffc76de2bb314).
It was byte-identical to https://docs.selectel.ru/api/craas.json on 2026-10-15.
Canonical catalog IDs are preserved, including existing collision suffixes.
The existing five registry lifecycle bindings are untouched; twelve token
operations belong to #49 and OCI distribution belongs to #50.

The SDK snapshots in assets are verbatim public source from selectel/craas-go
revision 8f0d0cba0b59360f344b3b2d32157b1549c2645a:

- pkg/v1/repository/requests.go: native explicit204 repository list and array
  Layer response differ from current OpenAPI; named choices avoid fallback.
- pkg/v1/gc/requests.go: native GC wire evidence.

Two list operations remain partial because short=true names-only prose conflicts
with the object response schema; string-array shape is unknown. #48/P1 owns
implementation, #74 evidence, #75 acceptance. Public mock transport tests are
wire/offline evidence only. No live cloud operation was performed.

Reviewed shared reuse: pagination error-prefix seam from a49e66b (PR111), and
receipt-only binding/early contract revision from 379458e (PR97). Only those
shared source deltas are reused, with public registry regressions; unrelated
unmerged service features are not copied.
