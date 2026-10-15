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
совпадает. Это не алиасы. В текущем срезе implemented только ListTypes, остальные
8 unimplemented; весь inventory неизвестен, coverage_ratio=null.

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

Lifecycle, quota semantics и operation-specific waits ещё не реализованы этим
срезом. Нет claims о deployment, live проверках, готовности хранилища, реальном
монтировании или изменениях облака. #70 остаётся незавершённой; #74 должен сверить
inventory/version drift, #76 не может принять сервис при оставшихся пробелах.
