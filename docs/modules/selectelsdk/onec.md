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

Региональные `Regions` / `GetServiceConfig` возвращают глобальные metadata без
переключения scope: даже defaultRegion и returned endpoints не инициируют
новые вызовы. `ListClusterFlavors` / `ListClusterPlatforms` используют bounded
fixed-page collection и SDK cursor+skip для локального продолжения.
`includeDeprecated` сохраняет различие absence/false/true.
`GetClusterFlavorConstraints` возвращает source min/max как точные int64 строки.

Datastore — независимый ресурс project/location; SDK не создаёт cluster parent
из совпавшего ID или вложенной metadata. `GetDatastore`, `CreateDatastore`,
`DeleteDatastore` сохраняют source DTO и точные refs. Datastore receipt получает
только datastore goals: ACTIVE / DELETED с указанным в source смыслом,
ERRORED — failure; 404 не заменяет DELETED. Смешение cluster/datastore kind
в receipt отклоняется до I/O, даже если account/project/endpoint совпали.

Пример чтения с уже явно выбранными credentials и региональным endpoint:

```go
scope := types.Scope{AccountID: accountID, ProjectID: projectID, Region: "spb", Pool: "ru-9"}
endpoint := types.EndpointBinding{
    ID: "caller-selected-onec", URL: regionalAPIBase, // validated caller choice, includes /v2
    Scope: scope, APIGeneration: "v2", Audience: "selectel",
}
client, err := selectelsdk.New(selectelsdk.Config{
    AccountID: accountID, Credentials: credentialSource,
    EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{
        {Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint,
    },
})
if err != nil { panic(err) }
project, err := client.Project(projectID)
if err != nil { panic(err) }
regional, err := project.Location(scope.Region, scope.Pool)
if err != nil { panic(err) }
ref := types.ResourceRef{
    Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "datastore", ID: datastoreID,
    EndpointID: endpoint.ID, EndpointURL: endpoint.URL,
}
result, err := regional.OneC().GetDatastore(ctx, ref, types.CallOptions{})
if err != nil { panic(err) }
_ = result // caller consumes the typed response
```

`regionalAPIBase` выбирает вызывающая сторона после bootstrap discovery;
фрагмент не выполняет автоматического выбора или cloud mutation.

Datastore inventory: PATCH / resize сохраняют raw spec и source wrapper,
включая optional nodeCount; flavors/types используют bounded finite collections.
Datastore list и maintenance list остаются partial по тому же отсутствующему
terminal cursor contract. Maintenance event refs сохраняют caller datastore
parent; spec.datastoreId и projectId проверяются. Description защищён,
source timestamps нормализованы в UTC. Maintenance tasks не становятся
causal provider operation refs, а event ref не обещает отдельный Get/Wait API.

Во всех ref-based cluster/datastore вызовах EndpointID и EndpointURL должны
совпасть с явно настроенной binding до credential lookup и HTTP. Несовпадение
даёт endpoint_unavailable; mutation сохраняет revision-bound NotSent Receipt
с нулём attempts. Самостоятельный выбор другого адреса по ResourceRef исключён.

Health metric values и thresholds возвращаются как source-bound
`onec.ProtectedNumber`: JSON number проверяется и сохраняется побайтно,
format/JSON/Text безопасны; `Reveal()` — явная граница доступа вызывающего.
Это распространяется на health metadata cluster/datastore и не превращает
статус ACTIVE или threshold в доказательство прикладной готовности.

Proxy API: network-sharing project ID — target metadata, а не owner scope;
SDK не выполняет sharing и не меняет credentials/project. CreateClusterProxy
требует явный cluster ref и совпадение spec.clusterId. Условие networkId при
publicAccess=true следует тексту pinned source; при отсутствии subnetId SDK
не ищет первый subnet, а сохраняет documented server default без подстановки.

Get/Delete proxy сохраняют cluster parent только если его явно передали.
Ни spec.clusterId, ни embedded Cluster из ответа не создают parent самостоятельно.
Create сохраняет parent anchor и новый child; Resume выбирает один exact child,
допуская только его записанных ancestors. Parent-only unknown receipt имеет
recovery_unavailable и не вызывает list/name lookup/adoption. Accepted new ID
сохраняется при независимом decode failure. Proxy ACTIVE/DELETED goals ограничены
originating capability и не утверждают сетевую/прикладную готовность.
Proxy list ограничен и сохраняет metadata/cursor, но остаётся partial из-за
неизвестного terminal rule; только явный caller cluster filter становится parent.

Domain API сохраняет пять source operations: list остаётся partial из-за
неизвестного terminal cursor rule; register/patch/delete/transfer возвращают
типизированные envelopes. Custom TLS certificate/key защищены в DTO и раскрываются
только в приватном wire encoder; строки source format byte/PEM передаются без
самовольного преобразования. Register/transfer требуют предварительного DNS
изменения вызывающей стороной; SDK DNS не меняет и доступность TLS не проверяет.

Domain Status/Wait использует только native list и exact child ID с явным parent.
Чтение нескольких страниц имеет единый CallTimeout и native page/item bounds.
Наличие точного ID позволяет использовать наблюдение из частичной страницы;
отсутствие ID не доказывает удаления. Только source DELETED завершает deleted goal;
parent cluster ACTIVE не является подтверждением child readiness. Receipt goals
ограничены operation capability. Missing/UNSPECIFIED/new states остаются unknown.
`internal/core/reads.go` добавляет только read-only accessor `Runtime.CallTimeout`
для общей границы status scan; существующая HTTP/auth/retry orchestration не меняется.

Transfer требует отдельный explicit target cluster ref того же scope/endpoint и
совпадающий targetClusterId. Receipt сохраняет исходный child и target anchor;
новый child с тем же ID и target parent добавляется только по ответу провайдера.
Возвращённый cluster может быть одним из двух явных anchors, но не чужим owner.
Resume принимает только точный граф original child + target anchor + acknowledged
new child; потерянный ответ/old-only response не создаёт target ref и не даёт
recovery. Independent decode errors сохраняют уже подтверждённый новый child.
Ни повтор mutation, ни name/DNS lookup, ни adoption для восстановления не выполняются.

Volume API: supported regions — finite bounded source array; VerifyVolumeSpec
передаёт source body в POST verify, проверяет параметры до mount и имеет effect
read, без mutation receipt. Это SDK-owned семантическое уточнение catalog generator
по pinned description, а не изменение Swagger. Verify не выполняет mount.
Mount/Unmount сохраняют точный child и caller parent, accepted receipts и
не повторяют неизвестный исход. Status сканирует bounded native volumes list
с единым CallTimeout и exact ID; ACTIVE/DELETED source states ограничены
originating capability. Missing из partial list не доказывает unmount.
Volumes list остаётся partial по unknown cursor terminal rule, uint64 size
сохраняется строкой без округления. S3 accessKeyId/secretAccessKey, provider reason
и mountPath защищены; source credential values раскрываются только в wire encoder.
Состояние ACTIVE не доказывает права S3/приложения или доступность файлов.

Tech journal API содержит finite predefined configs/plans, current config,
CreateJournalPlan и DisableJournalPlan. Raw XML/configData защищены, private wire
encoder передаёт только явно заданный XML. Создание сохраняет ID до независимого
decode failure; parent-only unknown не даёт recovery. Disable имеет пустой body
и typed empty response, но retained receipt. Exact plan Status читает finite plans
с общим CallTimeout и bounds; create goal running, disable goal stopped ограничены
capability. Scheduled не означает running; CANCELLED/STOPPED/FINISHED завершают
stopped, ERRORED — failure. Отсутствие exact ID не доказывает отмены; unknown source
states сохраняются. Никакой log delivery или XML-семантики вне source не обещано.

Cluster maintenance сохраняет source event IDs, explicit cluster parent,
projectId/clusterId checks, UTC и protected description. Event/task metadata
не становится causal operation ref и не обещает отсутствующего event Get/Wait.
Metrics возвращает typed groups/time series/aggregates/bounds: все JSON numbers
защищены и сохраняются лексически, instance metadata защищена, timestamps UTC.
Optional timeRange.start/end передаются как UTC RFC3339Nano с проверкой порядка.

Sessions list — finite bounded source collection с optional infobaseId filter,
exact session refs с caller cluster parent, UTC и protected username/IP/app/locale
и int64/uint64-string telemetry. `TerminateClusterSessions` требует ровно один
явный sessionId или filter; zero-value body не завершает все сессии. Только явно
переданный пустой filter `{}` означает все сессии по pinned source. Reason защищён
и раскрывается в wire. Receipt сохраняет caller session ref для ID selector,
но accepted/empty response не доказывает завершения. Отсутствует documented
terminated state/causal completion token: wait/recovery unavailable, автоматическое
повторение или превращение отсутствия в списке в completion не выполняется.

Checkpoint #65: все 49 принадлежащих задаче native operations имеют bindings;
42 implemented, 7 partial. Partial — ListClusters, ListDatastores,
ListDatastoreMaintenanceEvents, ListClusterProxies, ListClusterDomains,
ListClusterVolumes, ListClusterMaintenanceEvents. Для каждого недостаёт terminal
cursor contract (next sentinel/empty/zero/omission/end condition); pinned schema
CursorResponse и официальная API-страница не определяют его. Эти семь gaps остаются
owned #65, а не перекладываются на #66/#67. Остальные 47 operations snapshot
сохраняют ownership #66 (12) и #67 (35), пока unimplemented в этой ветке.
Полнота публичного API неизвестна, denominator/coverage ratio остаётся null.
