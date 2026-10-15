# Облако 1С v2

Источник: [официальная API-страница](https://docs.selectel.ru/api/1c-cloud/)
и pinned `catalog/sources/1c-cloud.json`. Инвентарь 96 операций и распределение
ответственности записаны в `catalog/evidence/onec-current-inventory.json`:
49 infrastructure/discovery — #65, 12 infobase/config — #66, 35 backup,
licenses, metrics API keys и usage — #67. Это перечисленный snapshot;
полнота всего публичного API неизвестна, coverage ratio остаётся null.

`Client.Project(id).OneC().Endpoints(ctx, options)` читает официальный bootstrap
`https://api.selectel.ru/ones/paas/v2/service/endpoints` с project-scoped IAM.
Метод возвращает типизированные региональные записи и не выбирает регион,
пул или сетевой адрес за вызывающую сторону. Host `api.ru-3.1c.selcloud.ru`
в Swagger — пример регионального endpoint, а не универсальный default.
Региональные вызовы должны сохранять явно выбранный scope и base prefix `/v2`.

Локальные transport-тесты подтверждают wire mapping, а не доступность сервиса
или права реальных credentials. Никаких live операций выполнено не было.

`Client.Project(id).Location(region,pool).OneC().GetCluster(ctx,ref,options)`
требует точный override `(cloud_1c,v2,scope)` и cluster ref без parent.
ID и projectId ответа должны совпадать с caller scope/ref. Возвращаемое
состояние является наблюдением провайдера; ACTIVE не доказывает готовность
инфобаз, web publication или доступ приложений.

`CreateCluster` подтверждает только принятие: receipt сохраняется при decode
ошибке, trustworthy созданный ID извлекается до независимых полей. Неизвестный
исход не повторяется. Локальная ошибка возвращает revision-bound NotSent receipt.
Для этого узко переиспользована ранняя инициализация ContractRevision
из reviewed PR #97 commit `379458e4f658b1216137fea4acce5dae6027d320`
(delta от `82121d740a2788930ce7c4934d449c35b096966d`). Другие изменения
volumes не переносились; `internal/core/request.go` — общая точка интеграции.

`ListClusters(ctx, pagination.Options)` ограничивает страницы, элементы и общий
collection budget, возвращает ResourceRefs и исходные cursor metadata в Envelopes.
`cursor.next` и `cursor.total` — uint64 strings без потери точности. Returned
next используется как pointer; отсутствие next, `"0"` и пустая страница не
означают конец: pinned Swagger/API-page не определяют terminal sentinel.
Эта операция остаётся partial; лимиты дают incomplete, отсутствие прогресса —
protocol_error с сохранёнными наблюдёнными элементами, `Complete=false`.

`DeleteCluster` не выполняет cascade: провайдер требует предварительно удалить
связанные инфобазы. Receipt подтверждает принятие; `DELETING` ещё не конец.
`Client.Resource(ref)` / `Client.Resume(receipt)` наблюдают точный cluster через
GetCluster с исходным endpoint. `cluster.active` завершается на ACTIVE,
`cluster.deleted` — на DELETED; 404 не объявляется доказательством удаления.
ERRORED — failure, документированные переходные состояния — pending,
UNSPECIFIED/неизвестный/missing/null status — unknown с исходной строкой.
Receipt-bound цели ограничены originating capability: Create → active,
Delete → deleted. Unknown outcome остаётся indeterminate после наблюдения;
наличие ресурса не доказывает авторство, mutation никогда не повторяется.

Изменение cluster spec (`PatchCluster`), `ChangeClusterConfig`, `RestartCluster`,
`ResizeCluster`, `UpgradeCluster` сохраняют точную ссылку и accepted Receipt.
PATCH отправляет raw ClusterSpec; config/resize/upgrade используют документированные
оболочки; restart не добавляет body. Generated validation проверяет int64/uint64
строки до I/O без float conversion. `cluster.active` — только наблюдение ACTIVE,
не причинное доказательство окончания restart или применения целевого flavor,
version/config. Проверка прикладной готовности и сравнение желаемых параметров
остаются отдельными действиями вызывающей стороны.
