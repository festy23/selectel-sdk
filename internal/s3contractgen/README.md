# S3 protocol extraction

Generator reads only committed snapshots. `catalog/sources/s3-protocol.json`
records SHA-256 hashes of every asset, source revision and upstream tag commit.
It is SDK-owned protocol extraction, **not a Selectel OpenAPI document**.

Primary Selectel pages, retrieved 2026-10-15:

- [S3 compatibility and signatures](https://docs.selectel.ru/api/object-storage-s3/): `assets/selectel-s3.html`.
- [Published endpoints](https://docs.selectel.ru/api/urls/): `assets/selectel-urls.html`.
- [Addressing styles](https://docs.selectel.ru/s3/buckets/addressing-types/): `assets/selectel-addressing.html`.
- [Bucket creation/naming](https://docs.selectel.ru/s3/buckets/create-bucket/): `assets/selectel-create-bucket.html`.
- [Limits](https://docs.selectel.ru/s3/about/limitations/): `assets/selectel-limits.html`.
- [Object Lock guide](https://docs.selectel.ru/s3/buckets/object-lock/): advanced variants remain evidence work for #46/#74; no retention API inferred here.

Native typed operation request/response fields and wire semantics are from
[AWS SDK Go service/s3 v1.114.3](https://github.com/aws/aws-sdk-go-v2/tree/service/s3/v1.114.3/service/s3),
commit `c8f4a4457aa933bd6c8ce60bc2ca2a0599b77478`, published 2026-10-09.
`assets/api_op_*.go.txt` are unmodified public source snapshots of the 13 selected
operations. The AWS S3 service module is a research source, not a runtime
service dependency. Existing AWS SigV4 runtime signs requests; Smithy path
escaping uses the runtime's pinned `github.com/aws/smithy-go v1.28.5`, with the
unmodified `assets/smithy-path-replace.go.txt` source snapshot.

[AWS S3 transfer utilities](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/sdk-utilities-s3.html)
document known-length unseekable input using
`SwapComputePayloadSHA256ForUnsignedPayloadMiddleware`. This adapter applies
HTTPS `UNSIGNED-PAYLOAD` only to the compiled S3 PutObject upload operation;
there is no general caller-selected signing bypass.

Generated contracts record covered input fields, full native model inventory,
and exact excluded request variants. AWS evidence alone does not establish
Selectel availability of advanced fields. All 13 bindings remain partial;
whole-family inventory is incomplete and coverage ratio remains null. Explicit
ListBuckets native pagination has upstream wire evidence only. Multipart (#45),
bucket settings (#46), and unresolved advanced compatibility evidence (#74/P1/#75)
remain separately owned. No live availability or mutation claim is made.

Shared code provenance: response streaming from reviewed Swift `8a07c83`;
receipt-only known outcome recovery from reviewed volume `379458e`.
