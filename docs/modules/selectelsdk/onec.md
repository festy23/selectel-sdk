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
