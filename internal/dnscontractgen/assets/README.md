# Pinned current public DNS evidence

Primary source retrieved 2026-10-15:
https://docs.selectel.ru/api/dns-actual.json
The unmodified local snapshot is dns-actual.json; its SHA256 is the source
revision. The current OpenAPI includes 13 operations, 14 record types,
nullable comments, protection, owner transfer and X-Managed-By headers.

Global base URL/project authorization:
https://docs.selectel.ru/api/urls/
https://docs.selectel.ru/api/dns-actual/

Published domains-go source at commit6b87a8d9f7b4089a30a4a1dc361cf73117f1af61:
https://github.com/selectel/domains-go/tree/6b87a8d9f7b4089a30a4a1dc361cf73117f1af61
base/client/zoneManager/rrsetManager files are pkg/v2 files. sdk-test_utils.go.txt
is pkg/v2/testing/test_utils.go, including nullable terminal offsets. The SDK
lacks current owner transfer/new types/protection response mapping; current
OpenAPI remains primary. Native managed_by body serialization is separately
named rather than silently replacing the current header contract.

provider-domains_v2.go.txt is selectel/domains_v2.go from Terraform provider
commit cf33c549d0c910c137f4a51411f6d709eb2be7d4:
https://github.com/selectel/terraform-provider-selectel/blob/cf33c549d0c910c137f4a51411f6d709eb2be7d4/selectel/domains_v2.go
It confirms next_offset==0 as the traversal end condition. Count is not assumed
to be a total. Snapshots contain no live resources or credentials.
