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
