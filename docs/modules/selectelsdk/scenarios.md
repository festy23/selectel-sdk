# Начальные сквозные операции

## Общие ресурсы и правила

Каждый сервисный read/mutation использует
[execution](execution.md#контракт-вызова), [access](access.md) и settings.
При List добавляется [pagination](pagination.md); при Wait —
[operations](operations.md). Эти ссылки задают timeout, auth, все общие
error branches и формирование Metadata/Receipt. Ни одна операция этой страницы
не разрешает mutation replay: потерянный ответ возвращает unknown_outcome.

Typed request/result состоит из всех полей referenced official schema
конкретной операции, кроме SDK-owned auth/scope и ProtectedValue replacements.
Optional provider поля сохраняют отсутствие; имена JSON и единицы сохраняются.
Ни один field не переносится из schema другого поколения. Явные ограничения
initial параметров этой страницы отражаются в covered schema и excluded
variants каталога. Такая source operation имеет status partial до поддержки
всех documented вариантов; валидный, но исключённый вариант возвращает
capability_unavailable до resource request.

## Инвентарь

Метод: `Inventory.Scan(ctx, InventoryPlan)`.
Описание: перечисление ресурсов явно заданных scopes и семейств.
Ресурсы: plan, каталог, следующие read bindings, CollectionTimeout.

| Operation ID / метод | Wire read и result mapping | Scope / pagination |
|---|---|---|
| `projects.v2.projects.list` / Projects.ListPage | GET resell `/v2/projects`, response.projects | Account IAM; single response, optional `all` из opts; default не отправляется |
| `compute.v2_1.servers.list` / Compute.Servers.ListPage | GET Nova `/servers/detail`, response.servers | Project IAM, catalog compute endpoint; marker/limit/servers_links.next |
| `mks.v2.clusters.list` / MKS.V2.Clusters.ListPage | GET `/v2/clusters`, response.clusters | Project IAM, URL map MKS v2; single response |
| `databases.v1.datastores.list` / Databases.V1.Datastores.ListPage | GET DB `/v1/datastores`, response.datastores | Project IAM, URL map DB v1; query project_id из scope, остальные documented filters из opts; single response |
| `registry.v2.registries.list` / Registry.V2.Registries.ListPage | GET registry `/v2/registries`, response.registries, response.totalCount | Project IAM; registry API base включает `/api`; limit/offset; filters search/sort_field/sort_type из opts |

Resell base URL не получает второй `/v2`, если generation уже входит в base.
Registry absolute path содержит `/api/v2/registries`.
Для registry следующий offset равен текущему offset + числу wire items;
конец при offset >= totalCount либо пустой странице. TotalCount может меняться.
Single response имеет один wire page: локальное ограничение MaxItems сохраняет
replay cursor по pagination, а не выдаёт prefix за полный результат.

Контракты источников: [Projects](https://docs.selectel.ru/api/cloud-projects-and-resources/),
[Nova list](https://docs.openstack.org/api-ref/compute/#list-servers-detailed),
[MKS v2](https://docs.selectel.ru/api/managed-kubernetes/),
[DB](https://docs.selectel.ru/api/managed-databases/),
[Registry v2](https://docs.selectel.ru/api/craas/).

| Вход / выход | Формирование |
|---|---|
| `InventoryPlan.Branches` | Обязательный непустой ordered массив: scope, operation_id, typed filters, PageOptions |
| `InventoryResult.Branches` | Один результат на каждую requested branch, в порядке plan |
| `Branch.Items` | Уже полученные typed data; секреты/кubeconfig/пароли исключены |
| `Branch.Scope`, `OperationID` | Из plan |
| `Branch.Complete`, `StopReason`, `Cursor` | Из collection result |
| `Branch.Error` | Ошибка конкретной branch, optional при успехе |
| `InventoryResult.Complete` | true только если все branches complete |
| `InventoryResult.Coverage` | Точный список requested branches; не все ресурсы аккаунта |
| `InventoryResult.ObservedFrom/To` | Время начала scan и его завершения, без обещания atomic snapshot |

1. Проверить plan, уникальность branch keys `(scope, operation_id, filters hash)`,
   read effect и доступность bindings. Malformed plan возвращает
   invalid_argument до I/O; неизвестная capability — capability_unavailable.
   Начать общий budget CollectionTimeout, ограниченный context.
2. Выполнять branches последовательно в порядке plan. Создать scoped view
   branch, вызвать Collect по table binding в оставшемся общем budget.
3. Записать все полученные Items/metadata/continuation/error branch. При
   обычной ошибке branch продолжить следующую branch. При cancel/deadline
   завершить scan и отметить ещё не начатые branches incomplete с той же ошибкой.
4. Вернуть полный result. Если все branches complete — nil error; иначе
   incomplete с сохранёнными branch errors. SDK не перечисляет скрыто другие
   accounts/projects/pools и не объявляет empty items при 403 успешным списком.

## Жизненный цикл VM

Ресурс: Nova server в конкретном project/location, существующие image,
flavor и network/port. Сервис: `Compute.Servers`, wire generation Nova v2.1,
базовая microversion 2.1. Typed Create opts сохраняет native Nova fields,
дополнительные extensions допустимы только через отдельный documented binding.
Все операции используют один exact project-scoped catalog endpoint.

| Метод / ID | Вход и request mapping | Успех / result mapping |
|---|---|---|
| `Create` / `compute.v2_1.servers.create` | Nova Create opts: name, flavorRef, imageRef либо boot volume mapping, explicit networks/ports и optional documented fields → body.server; auth/scope из view | HTTP 202; response.server → typed Server с protected adminPass; ID → ResourceRef; accepted Receipt |
| `Get` / `compute.v2_1.servers.get` | Exact ResourceRef.ID → GET `/servers/{id}` | HTTP 200; response.server → Server; status сохранён строкой |
| `Delete` / `compute.v2_1.servers.delete` | Exact ref → DELETE `/servers/{id}`, без body | HTTP 204; accepted Receipt с исходным ref; provider operation ID отсутствует |
| `CreateAndWait` | Create opts и goal `server.active` | Create result/Receipt и WaitResult, включая ошибку ожидания |

Источник: [Nova server operations](https://docs.openstack.org/api-ref/compute/#servers-servers).
Create requires ровно одну VM; multi-create min_count/max_count вне initial
binding. Отсутствие сети не заменяется автоматическим выбором сети. SDK не
создаёт dependency resources. Nova-опции удаления boot volumes применяются
только если явно заданы caller в Create opts; Delete не добавляет cleanup.

### Алгоритм

1. Для Create проверить обязательные fields и взаимную исключительность
   image/boot-volume вариантов по Nova schema, exact dependency refs и scope.
   Ошибка возвращает invalid_argument/scope_mismatch и not_sent Receipt.
2. Вызвать Create через execution один раз. HTTP 400/401/403, подтверждающие
   отказ Nova request validation/auth, дают rejected. 5xx/transport loss после
   возможной отправки дают unknown. Другие коды классифицируются contract error
   binding; при отсутствии доказательства неприменения outcome unknown.
3. HTTP 202 подтверждает принятие. Из response ID сформировать exact ref,
   сохранить scope/endpoint. Без достоверного ID вернуть protocol_error с
   accepted Receipt; CreateAndWait не пытается искать созданную VM по имени.
4. Get выполняет read exact ID; 404 → not_found, 403 → permission_denied.
   Delete выполняет mutation exact ID; 404 → not_found с rejected Receipt;
   это не запускает повтор или поиск другого server ID.
5. Для `server.active` применить Wait: ACTIVE → success; ERROR, DELETED,
   SOFT_DELETED → operation_failed; документированные BUILD, HARD_REBOOT,
   MIGRATING, PASSWORD, REBOOT, REBUILD, RESCUE, RESIZE, REVERT_RESIZE, SHUTOFF,
   SUSPENDED, VERIFY_RESIZE → pending; UNKNOWN и новая строка → state_unknown.
   404 для этой цели возвращает not_found; SDK не запускает server start.
6. Для `server.deleted` success подтверждается 404 exact Get.
   ERROR → operation_failed; любой другой документированный существующий
   server status → pending; UNKNOWN/новая строка → state_unknown.
   Timeout/cancel сохраняет accepted Delete Receipt и известный ref.

VM Create не имеет доказанной lookup correlation в initial binding:
unknown без server ID возвращает recovery_unavailable. Get существующего
server по update/delete ref может подтвердить текущую цель, но не авторство
исходного изменения; outcome остаётся unknown согласно Recover.

## Биллинг

Методы `Billing.Balances`, `Billing.Summary`, `Billing.Consumption` — safe reads
с account IAM. Static token может быть выбран caller явно для API, который его
документирует; автоматически scheme не переключается. Reports/Transactions
имеют отдельные static-token contracts при расширении.

| Operation ID / wire | Request | Result |
|---|---|---|
| `billing.v3.balances.get` / GET `https://api.selectel.ru/v3/balances` | Без query/body | data.settings.currency, data.billings/debt/balances → typed result; amounts → Money.Raw без изменения значения |
| `billing.v1.statistics.summary` / GET `/v1/cloud_billing/statistic/summary_stats` | provider_keys, start, end, group_type, period_group_type, items_count_period_group_type | Полная official response schema; value → Money с неизвестной единицей, если schema её не задаёт |
| `billing.v1.statistics.consumption` / GET `/v1/cloud_billing/statistic/consumption` | provider_keys, start, end, locale, group_type, period_group_type | Полная official response schema; metric.quantity/unit и денежное value раздельны |

1. Проверить непустой provider_keys из official enum, Start < End и required
   group enums/locale. Missing values возвращают invalid_argument; период,
   project grouping и locale не выбираются скрыто.
2. Передать time inputs как RFC3339 UTC без сдвига границ; HTTP arrays
   сериализовать repeated query parameters по schema. Выполнить safe read
   через execution. Источники: [balance](https://docs.selectel.ru/api/balance/)
   и [statistics](https://docs.selectel.ru/api/balance-statistics/).
3. Сохранить currency/unit только из документированных fields/units. Если
   currency/scale отсутствует, сохранить отсутствие и provider_unit_unknown.
   Timestamp ответа без offset сохранить как ProviderTimeRaw с неопределённой
   timezone; SDK не приписывает ему UTC.
4. Вернуть typed result/Metadata. Прогноз, баланс и consumption не смешиваются.
   SDK не делит raw integers на предполагаемый коэффициент и не распределяет
   account totals по ресурсам при отсутствии attribution source.

## Телеметрия

Описанные ниже операции чтения являются safe reads. Пять операций записи Logs
определены отдельно в [контракте Logs](logs.md). Time bounds обязательны для логов,
аудита и range queries: после преобразования в wire precision Start < End.
Данные передаются caller; содержимое не выводится в diagnostics.

| Метод / binding | Auth и request | Result и limits |
|---|---|---|
| Logs.Groups / DescribeLogGroups | S3 SigV4; typed filters и nextToken | Typed groups, nextToken; pagination |
| Logs.Streams / DescribeLogStreams | S3 SigV4; exact logGroup и typed filters/nextToken | Typed streams; pagination |
| Logs.Events / GetLogEvents | S3 SigV4; exact group/stream, Start/End UnixMilli, limit/cursor; startFromHead=true с первой страницы | Events: timestamp, ingestionTime и ProtectedValue message; nextForwardToken; конец при token равном входному token |
| Logs.Filter / FilterLogEvents | S3 SigV4; group, typed stream/filter fields, Start/End UnixMilli | Events с ProtectedValue message, continuation; pagination |
| Metrics.Scrape / GET `/projects/{project}/namespaces/{namespace}/prometheus/metrics` | Basic user ID/password; project/namespace path, региональный metrics URL | Prometheus samples и labels; MaxJSONBytes применяется к text bytes, MaxMetricSamples к samples |
| Metrics.Query / GET `/api/v1/query` относительно integration base | Basic; query из PromQL, optional time из EvaluationTime; без start/end/step | resultType и typed vector/matrix/scalar/string result, warnings; MaxMetricSamples и response limit |
| Metrics.QueryRange / GET `/api/v1/query_range` относительно integration base | Basic; query из PromQL, обязательные start/end/step | resultType=matrix, typed series/samples/warnings; MaxMetricSamples и response limit |
| Audit.Query / POST `/audit-logs/v1/logs` | Account IAM; project_ids — явный фильтр body; limit/cursor/dir — query по official schema | response.data → events; pagination.next_cursor → continuation; пустой/отсутствующий next_cursor → конец; payload protected |

Источники: [Selectel Logs](https://docs.selectel.ru/logs/tools/aws-cli/),
[CloudWatch read contracts](https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_GetLogEvents.html),
[Metrics scrape](https://docs.selectel.ru/metrics/tools/prometheus/),
[Metrics integration](https://docs.selectel.ru/metrics/tools/grafana/),
[Prometheus query API](https://prometheus.io/docs/prometheus/latest/querying/api/),
[Audit](https://docs.selectel.ru/api/audit-logs/).

1. Проверить scope и required fields соответствующего метода. Для Metrics
   namespace берётся из документированного справочника. Для logs/audit/range
   проверить Start < End; для QueryRange дополнительно step >0. Для instant
   Query проверить непустой PromQL и optional EvaluationTime; start/end/step
   отсутствуют. Logs методы требуют group и stream только согласно их schema.
   Ошибка возвращает invalid_argument.
2. Сформировать все wire fields из typed request по указанной schema;
   project/location берутся из view. Выполнить execution и при наличии страниц
   pagination. Empty page с continuation не объявляется концом.
3. Decode events/samples. Samples сохраняют provider timestamps и значения,
   включая строковые NaN/Inf protocol values; logs/audit payload защищается.
   При samples > MaxMetricSamples вернуть bounded prefix, Complete=false
   и response_limit; body limit возвращает response_limit без частично
   декодированного structured body.
4. Вернуть result со scope, completeness и bounds для interval-методов.
   Для instant Query сохранить заданный EvaluationTime, если он присутствует;
   при отсутствии time параметр не отправляется, действует provider current
   time, а фактические sample timestamps сохраняются из response.
   Отсутствие событий
   успешно только при подтверждённом конце; 403 и endpoint failure остаются
   ошибками. SDK не включает provider log delivery и не устанавливает агенты
   сбора метрик как побочный эффект чтения.

Query/QueryRange получают callable binding только для подтверждённых
Prometheus endpoints Selectel. Интеграция с Grafana не подтверждает поддержку
всех admin/write APIs Prometheus. Current scrape является самостоятельным
initial operation и не зависит от готовности range-query binding.

| Метод | Operation ID |
|---|---|
| Logs.Groups | logs.cloudwatch.log_groups.list |
| Logs.Streams | logs.cloudwatch.log_streams.list |
| Logs.Events | logs.cloudwatch.events.get |
| Logs.Filter | logs.cloudwatch.events.filter |
| Metrics.Scrape | metrics.prometheus.samples.scrape |
| Metrics.Query | metrics.prometheus.samples.query |
| Metrics.QueryRange | metrics.prometheus.samples.query_range |
| Audit.Query | audit.v1.logs.query |

Initial Logs.Events обходит interval вперёд. Явный startFromHead=false
возвращает capability_unavailable; backward traversal является отдельным
parameter variant полного API coverage. startFromHead=true не изменяется
при передаче nextForwardToken. Audit time fields формируются как
event_saved_time_from/to из Start/End в RFC3339; event_types, project_ids,
source_types берутся из явно заданных filters. Account scope не ограничивается
project автоматически при отсутствии project_ids.

## Секреты

Методы: `Secrets.ListMetadata`, `Secrets.GetMetadata`, `Secrets.ReadValue`,
`Secrets.ReadVersion`.
Ресурсы: Project IAM, secrets URL map, secret key, optional version ID.
Источник: [Secrets Manager](https://docs.selectel.ru/api/secrets-manager/).

| Метод | Wire mapping | Result |
|---|---|---|
| ListMetadata | GET `/v1/?list=true`; response.keys | Names/type/metadata; без secret value |
| GetMetadata | GET `/v1/{secret_name}/versions` | name, description, created_at и все versions metadata; current version определяется is_current, без выбора value |
| ReadValue | GET `/v1/{secret_name}` | Metadata текущей версии и decoded base64 value как ProtectedValue |
| ReadVersion | GET `/v1/{secret_name}/versions/{version_id}` | Metadata указанной версии и decoded base64 value как ProtectedValue |

1. Проверить непустой secret key и version ID из official schema. Key
   кодируется как один path segment; конфликт scope возвращает scope_mismatch.
2. Выполнить safe read через execution. 404 возвращает not_found; 403 —
   permission_denied. SDK не ищет secret в другом project.
3. Разобрать referenced official schema. GetMetadata возвращает version_id,
   created_at/is_current каждой версии без value; ReadValue/ReadVersion base64-decodes
   value и возвращает ProtectedValue.
   Malformed base64 или response schema возвращает protocol_error.
4. Вернуть typed Metadata/result. List использует bounded pagination для
   single response. Чтение значений не включается в Inventory и не вызывается
   автоматически из metadata reads.

Operation IDs: ListMetadata — secrets.v1.secrets.list;
GetMetadata — secrets.v1.versions.list; ReadValue — secrets.v1.secrets.get;
ReadVersion — secrets.v1.versions.get. GetMetadata возвращает все metadata
версий в provider order; отсутствие is_current не приводит к выбору newest.
