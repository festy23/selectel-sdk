# DBaaS v1: создание и наблюдение кластера

Сценарий [#5](https://github.com/festy23/selectel-sdk/issues/5) — первая часть
[программы полного покрытия](https://github.com/festy23/selectel-sdk/issues/4).
Контракт закреплён в публичной [схеме Managed Databases](https://docs.selectel.ru/api/managed-databases.json).
DTO генерируются из поставляемого snapshot; локальная генерация не обращается
к сети. Текущий snapshot повторно сверен с опубликованной схемой при реализации.

## Публичные методы

Используется `client.Project(projectID).Location(region, pool).Databases().V1.Datastores`
с обработкой ошибок выбора проекта и location. Нужны явные credentials и
project IAM; endpoint выбирается по точному пулу или совместимому override.

| Метод | Операция | Результат |
|---|---|---|
| `Create(ctx, CreateRequest, CallOptions)` | `databases.v1.datastores.create`, POST `/v1/datastores`, 201 | `Result[Datastore]`, Receipt и ref нового кластера |
| `Get(ctx, ResourceRef, CallOptions)` | `databases.v1.datastores.get`, GET `/v1/datastores/{datastore_id}`, 200 | `Result[Datastore]` без Receipt |
| `Handle(ref, receipt)` | Локальная привязка наблюдения | `operations.Handle`, без I/O |
| `client.Resource(ref)` | Наблюдение существующего datastore | Handle без Receipt |
| `client.Resume(receipt)` | Продолжение наблюдения принятого/неизвестного создания | Handle с сохранённым Receipt |

Существующие `ListPage`, `Iterate` и `Collect` сохраняют свой контракт.
Handle/Resource/Resume/Status/Wait не создают новых provider operations в каталоге.

## Создание и параметры

`CreateRequest` покрывает все поля `DatastoreCreate`: name, node_count, type_id,
subnet_id, project_id, flavor_id, flavor, disk, floating_ips, config,
backup_retention_days, redis_password, pooler, restore, security_groups,
log_platform. Обязательные и необязательные скалярные поля — указатели,
целые — `types.Integer`, без округления через float64. Вложенные структуры
сохраняют поля и обязательность исходной схемы.

`project_id` берётся из immutable view. Явный отличный project_id возвращает
scope_mismatch до I/O. Переданный caller объект не изменяется. Неуказанные
поля не получают вымышленных defaults. Ноль floating IP передаётся явно.
Restore передаёт datastore_id источника и исходную строку target_time;
результат содержит ref **нового** кластера. UUID existence, совместимость type,
flavor и subnet, доступность движка и движковые ограничения проверяет провайдер:
создание не выполняет скрытых discovery-запросов.

До отправки проверяются обязательные поля, name pattern/length, положительные
размеры и retention, disk type, floating IP bounds относительно node_count,
pooler mode/size, источник restore, непустой уникальный список security groups,
UTF-8 строк и длина пароля. Config должен быть JSON object; его вложенные
null/false/zero/empty значения сохраняются. Внешний null config не является
объектом и отклоняется. Проверка существования и поддержанности отдельных
engine settings остаётся за API и будущими settings/discovery методами.

Config и redis_password представлены `ProtectedJSON`/`ProtectedValue`:
обычные JSON и форматирование редактируют их. Исходные байты раскрываются
только при построении outgoing body. Response config и connection также
защищены. Caller читает их через явный `Reveal`.

## Identity, ошибки и Receipt

ID кодируется одним path segment, включая slash и dot segments. Ref содержит
исходный account/project/location, generation и endpoint; parent для datastore
отсутствует. Чужой scope, generation или kind отклоняется. Handle проверяет
совместимость endpoint без I/O; Get проверяет pinned endpoint перед отправкой
resource request. Ответ с чужим project_id или ID возвращает scope_mismatch.
Поле ref провайдера игнорируется.

Create требует ID в успешном ответе. Get использует точный request ID, если
необязательный response ID отсутствует. Malformed/null envelope возвращает
protocol_error. Optional response fields сохраняют отсутствие, integer precision,
пустые коллекции и исходные naive datetime строки.

- 201 фиксирует accepted до декодирования. При ошибке данных Receipt сохраняется;
  уже извлечённый достоверный ID остаётся в ResourceRefs.
- 400/401/403 создания — rejected, соответственно provider_error,
  unauthenticated, permission_denied. Provider error body не раскрывается.
- Потерянный ответ и недоказанный исход — unknown; POST не повторяется.
- Ошибка preflight — not_sent. Safe Get использует bounded retries ядра;
  404 возвращает not_found.

Receipt должен сохраняться caller даже при ошибке. Resume проверяет account,
scope, endpoint, generation и contract revision; не выполняет POST или поиск
по имени. Unknown без exact ref не допускает восстановления наблюдения.
Unknown с ref допускает только наблюдение: наличие ACTIVE не доказывает
авторство создания и не превращает unknown в accepted.

## Цель datastore.active

| Provider state | Classification / результат Wait |
|---|---|
| ACTIVE | succeeded / complete |
| PENDING_CREATE, PENDING_UPDATE, PENDING_DELETE, RESIZING, REBUILDING | pending / продолжать опрос |
| ERROR, DEGRADED, DISK_FULL, DELETED | failed / operation_failed |
| Неизвестное или отсутствующее состояние | unknown / state_unknown |
| 404 | not_found; не успешное создание |

ACTIVE означает только состояние провайдера: оно не подтверждает SQL connectivity,
здоровье приложения или применение конкретной конфигурации. Delete goal пока
недоступен. Общий WaitTimeout/context deadline включает все polls; каждое чтение
ограничено CallTimeout. Отмена локального ожидания не отменяет облачное создание.
Receipt accepted сохраняется при timeout, cancellation и ошибке чтения.

## Проверка

`databases_management_test.go` проверяет публичный клиент с подменой только
HTTP transport и синтетическими credentials: payloads, protected output,
preflight, receipts, отсутствие replay, exact observation, Resume, states,
cancellation/deadline и offline catalog. `databases_example_test.go` содержит
компилируемый пример, который обычный `go test` не запускает в облаке.
Проверенность — offline; live availability и полное покрытие DBaaS не заявлены.
