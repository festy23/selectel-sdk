# Очередь сверки каталога

#74 остаётся незавершённой. Локальный Audit измеряет только shipped pinned
subset; отсутствие записи не означает отсутствие provider API. Этот документ
фиксирует reviewed unmerged checkpoints, не переносит их bindings в независимую
ветку и не увеличивает counts текущего релиза. После merge сверяются source
revision, generations, canonical mappings и excluded variants заново.

| Владелец | Review checkpoint | Оставшийся source contract / scope |
|---|---|---|
| #60 | [PR94](https://github.com/festy23/selectel-sdk/pull/94) | 3 partial reads, whole Metrics inventory unknown |
| #62 | [PR99](https://github.com/festy23/selectel-sdk/pull/99) | GetCertificate PEM fields absent from primary response contract; 18 implemented / 1 partial |
| #63 | [PR101](https://github.com/festy23/selectel-sdk/pull/101) | 5 partial mail contracts, exact endpoint source unresolved; 15 implemented |
| #64 | [PR104](https://github.com/festy23/selectel-sdk/pull/104) | 3 list terminal contracts and download success contract unresolved; 23 implemented / 4 partial |
| #65 | [PR106](https://github.com/festy23/selectel-sdk/pull/106) | 7 terminal cursor contracts unresolved; 42 implemented / 7 partial of 49; #66/#67 retain dependent ownership |
| #68 | [PR110](https://github.com/festy23/selectel-sdk/pull/110) | Versions only; full inventory unknown; #107 auth/version, #108/#109 dependent lifecycle/network/storage |
| #69 | [PR113](https://github.com/festy23/selectel-sdk/pull/113) | 5 missing response schemas; 25 implemented / 5 partial; absent 200 schema does not establish empty result or completion |
| #70 | [PR115](https://github.com/festy23/selectel-sdk/pull/115) | All 10 partial: 9 modern counterparts plus 1 legacy wire; Selectel Zed max2.75 vs pinned upstream15/max2.73; lifecycle/quota/wait/inventory unknown |
| #71 | [PR120](https://github.com/festy23/selectel-sdk/pull/120) | 26 implemented / 2 partial MKS; 19 DNS unimplemented; storage unknown; native source slices #116–#118 |
| #72 | [PR123](https://github.com/festy23/selectel-sdk/pull/123) | All 8 partial wire subsets; whole API/model/control-plane/availability/streaming unknown; #72/#74/#76 retain ownership |
| #73 | [PR129](https://github.com/festy23/selectel-sdk/pull/129) | 5 discovery_pending families; #124–#128 block #76; no API implementation completion |

Точные field exclusions и primary URI/revision находятся в OperationContracts
соответствующих checkpoints. Primary source absence/contradiction не заменяется
догадкой или live пробой. Source resolution остаётся #74, typed implementation —
соответствующей family issue, приёмка — #75/#76 и итоговая #77.

P1 handoffs в комментариях #74 также остаются открыты: Glance owner/location/store
(#38); Octavia security_group_ids/log_offloading (#40/#41); PTR only-count response
shape (#43); S3 advanced availability and newer Object Lock evidence (#44–#46);
object-storage certificate shape/log auth (#47); CRAAS short response (#48);
OCI token/offload topology и optional availability (#50). Объединение только
после review/merge; source revision drift должен инвалидировать старое evidence.

Native dependency frontier на checkpoint: #58 → #57; #66 → #65; #67 → #66;
#68 → #107/#108/#109; #71 → #116/#117/#118. #76 заблокирована family issues и
#124–#128; #77 заблокирована #23/#75/#76/#74/#73. Наличие ready source slice не
позволяет обходить эти зависимости или считать P2 завершённой.

Зафиксированные HEAD review checkpoints на 2026-10-16:

- PR #94: [5b46865673f6cae9d967dd11d924bc50abd0da66](https://github.com/festy23/selectel-sdk/tree/5b46865673f6cae9d967dd11d924bc50abd0da66).
- PR #99: [f8750d642dfa2a5125d1291f75d0e2cff231d4db](https://github.com/festy23/selectel-sdk/tree/f8750d642dfa2a5125d1291f75d0e2cff231d4db).
- PR #101: [3f3961f2ad30b2cd14823ae3d9b60131cef74e39](https://github.com/festy23/selectel-sdk/tree/3f3961f2ad30b2cd14823ae3d9b60131cef74e39).
- PR #104: [709f85cbd253b4108f9e24b17eac2fbbf82dcfb1](https://github.com/festy23/selectel-sdk/tree/709f85cbd253b4108f9e24b17eac2fbbf82dcfb1).
- PR #106: [7cd0b1469adaba5367ee0d92d43b044100ac2aa4](https://github.com/festy23/selectel-sdk/tree/7cd0b1469adaba5367ee0d92d43b044100ac2aa4).
- PR #110: [55fddea629c36f2d6b9d7702907252293794e2b8](https://github.com/festy23/selectel-sdk/tree/55fddea629c36f2d6b9d7702907252293794e2b8).
- PR #113: [b5c9d65018dd61dd958d37dec7acffd64433c2d6](https://github.com/festy23/selectel-sdk/tree/b5c9d65018dd61dd958d37dec7acffd64433c2d6).
- PR #115: [dfc9f7cd01e48f0e6d0ad92040b1840a8f92d893](https://github.com/festy23/selectel-sdk/tree/dfc9f7cd01e48f0e6d0ad92040b1840a8f92d893).
- PR #120: [6cef17c11c535482a283ac9bafa933666b797287](https://github.com/festy23/selectel-sdk/tree/6cef17c11c535482a283ac9bafa933666b797287).
- PR #123: [168fb7d49b7cdbac96e7866e034f45f379502991](https://github.com/festy23/selectel-sdk/tree/168fb7d49b7cdbac96e7866e034f45f379502991).
- PR #129: [6a1cc57bb8045a3ee022e1365a4e7303b01f6bd6](https://github.com/festy23/selectel-sdk/tree/6a1cc57bb8045a3ee022e1365a4e7303b01f6bd6).
