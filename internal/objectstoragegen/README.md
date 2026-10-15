# Object Storage management models and bindings

Generator consumes only the committed current
[Object Storage schema](https://docs.selectel.ru/api/object-storage.json),
`catalog/sources/object-storage.json`, SHA-256
`892af4f017c5e21aa9cb2647ce817700e43e675fd27096e7b2307f2104694d4c`.
Retrieved 2026-10-15; matches the 2026-10-14 inventory without drift. Native
operation IDs and all 13 method/path pairs remain source-bound. Generated DTOs
preserve optional and nullable quota fields, exact numeric formats, explicit
false/empty values, source-described request enums, and raw timestamps.
Schemaless log filter objects stay protected and only typed request serialization
reveals them. Endpoint snapshot is
[Selectel URLs](https://docs.selectel.ru/api/urls/), stored as
`assets/selectel-urls.html`; kz-1's published Object Storage domain remains `.uz`.
No S3 endpoint or storage class/address is guessed.

Two operations are partial: domains/certificates schema returns only domain
strings, and logsList omits security without a global declaration while other
operations explicitly require project IAM. SDK logs list is an authenticated
project view; anonymous availability and certificate object fields are not
invented. Unreferenced source definitions do not establish additional callable
operations. Legacy storage remains a separate discovery obligation.

Reviewed shared receipt-only recovery/early ContractRevision change is reused
from `379458e4f658b1216137fea4acce5dae6027d320` (diff from `82121d7`). New pagination
error-prefix preservation is exercised through public Client transport tests;
only metadata-validated, bounded/skipped items are exposed, original source
error remains terminal, and replay cursor retains the consumed prefix.
No live API requests or mutations are part of generation or evidence.
