# Pinned public sources

- private-dns.json: https://docs.selectel.ru/api/private-dns.json
- ip-addresses.json: https://docs.selectel.ru/api/ip-addresses.json
- client.go.txt, service.go.txt, zone.go.txt, record.go.txt, errors.go.txt: published selectel/private-dns-go commit 606a8a34e1607e29dc0e1e354aa58c4559cfb8fd, pkg/v1; upstream HEAD checked unchanged on 2026-10-15.
- URL table: https://docs.selectel.ru/api/urls/ (direct ip-addresses reference link to https://api.selectel.ru/ipam/v1; explicit Private DNS pool addresses).

Current Private DNS schema extends the SDK: name update, generated flags, project list filters, PATCH record values. Native SDK record list type/domain filters are explicitly retained without asserting current server availability. PTR count-only response shape is unknown, not unsupported. Snapshots contain public source material only, no credentials or cloud observations.
