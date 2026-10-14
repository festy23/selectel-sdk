# Resell v2

`Client.Account().Resell()` reads account status with `Account` and inspects
DNS delegation with `DNSCheck(ctx, hostname, options)`. Both use account IAM,
the existing `projects` catalog service ID and
`https://api.selectel.ru/vpc/resell`, preserving the Projects API.
The generated response DTOs retain optional fields and explicit false values;
account enabled/locked/name/onboarding and each lock's fields are required.
Neither read invents a resource reference or a wait/recovery guarantee.
DNS hostname is escaped as one path segment; documented 400 maps to not_found.

The source snapshot was re-fetched on 2026-10-14 from the official
[Resell schema](https://docs.selectel.ru/api/cloud-projects-and-resources.json);
SHA-256 `0545ced4f3c4af315eba1027ebd166240f91bc7e8e7835041cc7626cb191d22e`
matched the pinned artifact. Verification remains offline, with no live calls.

Remaining allocations, traffic/report and schema operations are unimplemented.
Deprecated keypair and license operations remain documented and unimplemented:
the current source still publishes them. The stated 2025 removal date alone
is not evidence of withdrawal, and a permission error cannot prove withdrawal.
Their eventual adapters must retain explicit generation and disposition.
Resell subnet/IP allocation refs must be distinct from Neutron resources.
