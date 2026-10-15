# Файловое хранилище

`Project(id).Location(region, pool).FileStorage()` использует явный project IAM,
точный pool endpoint и заголовок `X-OpenStack-Manila-API-Version: 2.73`.
Поколение refs — `v2_73`; оно не означает поддержку более новых microversions.
Источник [версий Selectel](https://docs.selectel.ru/cloud-servers/tools/openstack-cli/about-openstack-cli/)
называет Zed и максимум 2.75. Pinned upstream Manila 15.0.0 (Zed), commit
`f8f89548f322107e56dcc1d016a2f88dd3ecb9f3`, объявляет максимум 2.73.
Это расхождение принадлежит #70: 2.74/2.75 и полный deployment inventory остаются
неподтверждёнными. Upstream API или скопированная Terraform reference не равны
доказательству поддержки всех операций в Selectel.

[Создание хранилища](https://docs.selectel.ru/en/file-storage/create-storage/)
подтверждает CLI share create/list, network create/list и type list.
[Правила доступа](https://docs.selectel.ru/en/file-storage/manage/configure-access-by-ips/)
подтверждают access create/list/delete.
[Увеличение размера](https://docs.selectel.ru/en/file-storage/manage/resize-storage/)
подтверждает project IAM, endpoint `https://<pool>.cloud.api.selcloud.ru/share/v2`
и `os-extend`. SDK фиксирует 9 отдельных операций этих сценариев; три разных
POST action имеют отдельные source assets и canonical mappings, хотя wire path
совпадает. Это не алиасы. В текущем срезе implemented ListTypes, ListNetworks, CreateNetwork и ListShares, остальные
5 unimplemented; весь inventory неизвестен, coverage_ratio=null.

Default endpoint recipe применяется только к известным российским cloud pools;
для других локаций нужен точный caller override, host не выводится по аналогии.
Projectless resource route поддерживается pinned протоколом начиная с 2.60;
он согласуется с curl Selectel. Scope не исчезает: IAM остаётся project-scoped.

ListTypes возвращает finite `share_types` array с bounded FixedIterator.
Локальное ограничение оставляет partial collection и cursor. Legacy duplicate
`volume_types` не удваивает coverage. Optional boolean `is_public` сохраняет false;
`extra_specs` передаётся как string dictionary. В заголовке API reference query
назван `extra-specs`, но parameter table и pinned controller используют
`extra_specs`; реализация следует controller. Административный `is_public=all`
не входит в подтверждённый сценарий.

Generated ShareType сохраняет все 7 source fields. Description защищён;
pinned view допускает null, поэтому required-present-null и missing различаются
через Nullable. `extra_specs` и `required_extra_specs` — protected JSON objects,
а не произвольные диагностические строки. Exact type refs привязаны к caller
project/pool/endpoint без inferred ownership/parent.

Узко переиспользован reviewed formatter/MarshalText fix `types/nullable.go` из
PR80 commit `4534c4e79c8cc281490f075b0f2e094770d16e8a`. Feature-код PR80 не переносился.
Public tests проверяют JSON/fmt, включая decimal/hex представления внутренних
bytes, required null/missing, source false и ограничение collection.

Share lifecycle, quota reads и operation-specific waits ещё не реализованы этим
срезом. Нет claims о deployment, live проверках, готовности хранилища, реальном
монтировании или изменениях облака. #70 остаётся незавершённой; #74 должен сверить
inventory/version drift, #76 не может принять сервис при оставшихся пробелах.


ListNetworks следует pinned controller, который применяет limit/offset, хотя
API reference не описывает их для этого endpoint. Offset увеличивается на число
полученных строк; только пустая страница доказывает конец. Короткая страница может
означать server clamp, поэтому не становится Complete. Response href никогда
не выбирает host или credential destination. Предыдущие страницы сохраняются,
если последующая содержит foreign project или другую ошибку. На каждой строке
project_id проверяется до создания ref; вложенные Neutron IDs остаются metadata.
Модель учитывает переход 2.51 к share_network_subnets и versioned поля 2.63/2.70;
старые flat subnet fields не объявлены частью compiled2.73 ответа.


CreateNetwork принимает explicit Neutron network/subnet refs одного project и
location, service network/generation v2; available subnet parent должен точно
совпадать с caller network. Возвращённый share-network parent не выводится из
Neutron metadata. Поскольку pinned create явно создаёт default share-network
subnet, receipt сохраняет independently trustworthy returned child IDs с parent
из native nested response context. Все IDs сканируются до независимых полей;
malformed row не теряет последующие IDs. Unknown сохраняет только известные
caller anchors без inferred newly created resource и никогда не повторяется.
Источник API reference объявляет 202, pinned controller использует WSGI default
200; обе source-bound формы принимаются без readiness claim. Quota413 является
rejection; SDK не обещает preflight quota reservation или увеличение квот.

Private protected wire renderer узко переиспользован из reviewed OneC PR106
(7cd0b1469adaba5367ee0d92d43b044100ac2aa4), с package rename и explicit known Nullable
primitive cases, чтобы request null/secret не превратились в REDACTED на wire.
Неизвестные Nullable types отклоняются. Reviewed PR97 early receipt revision fix
379458e4f658b1216137fea4acce5dae6027d320 (delta от82121d740a2788930ce7c4934d449c35b096966d)
применён только к internal/core/request.go; volume feature-код не переносился.


ListShares сохраняет versioned2.73 response fields и raw unknown status без
readiness classification. Project каждого ряда проверяется до root ref; публичные
или административные selectors не разрешают adoption foreign project resources.
Metadata, links href, description и host защищены; required nullable fields
отличаются от missing. Deprecated flat export locations не переносятся в2.73 DTO.
API reference указывает202 для list, pinned controller/WSGI default200; обе формы
привязаны к одному source-bound typed response. Count хранится точным Integer
и служит только информацией. Он не доказывает конец обхода или coverage denominator.

Protected metadata/extra-spec selectors раскрываются только для wire query и
private cursor hashing. Сам cursor не содержит selector; другой secret selector
даёт другой fingerprint и отклоняет reuse до credential/resource I/O. Shared
source object validators не допускают scalar вместо schema object.

ListAccessRules использует GET /share-access-rules с обязательным caller share_id
и finite access_list (pinned controller не пагинирует этот результат). Explicit
caller share становится parent rule ref; доступный response share_id обязан
совпадать, foreign rule не принимается. access_to и nullable access_key защищены;
state остаётся raw, без обещания применения правила или сетевой доступности.
Legacy access_list action исключён именно из compiled2.73: pinned source явно
возвращает404 начиная2.45. Это не утверждение withdrawal других generations.

Все modern2.73 bindings помечены partial: upstream wire mappings подтверждены,
но exact Selectel modern route/body/microversion availability не доказана CLI
сценариями. Пробел принадлежит #70 с reconciliation #74 и acceptance #76.

AllowAccess сканирует returned id/share_id до остальных полей и сохраняет rule
с exact caller share parent при accepted malformed response. Unknown outcome
содержит только caller anchor; повторного allow и inferred rule нет. Request
access_to и metadata раскрываются только в private wire renderer. DenyAccess
требует exact rule ref с caller parent и не угадывает share из rule ID. Bodyless202
подтверждает acceptance, не применение ACL или завершение удаления. Recovery и
wait goals отсутствуют: exact observation доступен явно через ListAccessRules,
без автоматического replay. Upstream допускает driver-specific access types;
Selectel guide подтверждает IP сценарий, остальные driver contracts не доказаны.
