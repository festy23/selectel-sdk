# Cinder volumes v3

`RegionalClient.Volumes()` предоставляет девять операций подтверждённого
Selectel blockstorage-go v0.2.0: Create/Get/Update/Extend/Delete, подробный
ListPage/Iterate/Collect, Types.Get/Default/ListPage/Iterate/Collect и
Types.QoSLimits. Types.Default — вариант одной Get remote operation.

[Официальный SDK](https://github.com/selectel/blockstorage-go/tree/f229309640ab15be64dcb12703d26d3a706f5abc)
закреплён локальными snapshots и SHA-256 в catalog source. `pkg/v1` обозначает
поколение Go SDK; wire API — Cinder v3, Keystone service type `volumev3`.
Account, project, region/pool и credentials заданы явно scoped client.
Endpoint берётся из project-scoped IAM catalog или exact override; path
добавляется к endpoint, уже содержащему `/v3/{project_id}`. Redirects запрещены.
Response Ref сохраняет exact auth view/endpoint; это не доказательство абсолютного
owner для отсутствующих в pinned DTO owner fields.

## Запросы и результат

Create покрывает все pinned SDK fields: size (GiB), name, description,
availability_zone, metadata, volume_type, snapshot_id, source_volid, imageRef,
backup_id. Источники взаимно исключаются. Size обязателен, кроме snapshot,
source volume или backup; supplied size положителен. Nil size опускается.
Все числа размера и QoS — `types.Integer`, без float64/int truncation.
Metadata keys/values ограничены 255 символами. Пустые optional create strings
опускаются как в опубликованном SDK. Type — provider selector, не доказательство
доступности по inventory: API может принять имя, отсутствующее в Types.Collect.

Update nil не меняет поле; пустые name/description передаются, metadata
**заменяет** карту, включая явную пустую `{}`. Pointer на nil map (JSON null)
не поддержан source contract и отклоняется до I/O. IOPS изменяется подтверждённым
provider metadata-вариантом (ключ `total_iops_sec`, десятичная строка); ограничения диапазона
для выбранного типа проверяет provider. Response service metadata не удаляется
и не интерпретируется SDK автоматически.

`OpenStack-API-Version: volume 3.0` по умолчанию; backup Create выбирает 3.47,
Extend с explicit Attached выбирает 3.42. Произвольной microversion нет.
[Матрица Selectel](https://docs.selectel.ru/cloud-servers/tools/openstack-cli/about-openstack-cli/)
подтверждает максимум 3.64; SDK не повышает запрос до него автоматически.
Extend передаёт new_size, не выполняет скрытый Get/resize/replay; вызывающий
сравнивает текущий и желаемый размер. Shrink не поддержан provider.

Response Volume сохраняет весь pinned View, включая attachments: `id` — volume ID,
`attachment_id` — attachment ID, `server_id` — server ID. Read-only attachment
metadata не превращается в Nova attachment mutation или adopted refs.
Create/Update malformed accepted response сохраняет Accepted и доверенный ID.
Delete/Extend возвращают Accepted на 202 с пустым body. Accepted не означает
готовность, detach или изменение файловой системы гостя.

400/405/422 — InvalidArgument; 401/403/404/409/429 используют общий status mapping;
406 — CapabilityUnavailable. Для Cinder 413 структурированный fault с code=413 и
непустым retryAfter различает QuotaExceeded и InvalidArgument; raw messages
не классифицируются и не попадают в diagnostics. Это не Retry-After HTTP header.
Все mutations сохраняют Receipt, имеют одну попытку; потерянный ответ — Unknown.

## Коллекции и ожидания

Lists следуют только next marker ссылкам того же exact endpoint/path, запрещают
чужие URL и смену фильтров. Cursor привязан к operation revision, scope и endpoint.
Общие bounds/contexts сохраняют prefix и Complete=false при ошибке/лимите.
QoS map сортируется как в SDK по VolumeTypeID и entry name; service scalar fields
игнорируются, object без volume_type_id — ProtocolError. Локальные bounds применяются
к полному provider body, ограниченному MaxJSONBytes; continuation переигрывает
**read**, не mutation. Entries не дедуплицируются по volume_type_id.

`Handle(ref,nil).Wait(ctx,"volume.available")` требует available; in-use не available.
`volume.absent` требует scoped404 **и** полный успешный scoped list без этого ID:
404 может скрывать ресурс от токена, отсутствие не доказывает физическое удаление
вне текущего view. Incomplete list не доказывает отсутствие.
`WaitExtended(ctx,ref,size,attached)` требует точного размера и available, либо
in-use при explicit attached=true. Старый размер в available/in-use после принятия
Extend остаётся pending: waiter наблюдает до точного target либо deadline. Размер
больше target и in-use без разрешения также не дают ложный успех; они ожидают
в рамках budget. Только документированные error states дают OperationFailed.
Неизвестное состояние сохраняется и не успех.
Методы используют общие WaitTimeout/PollInterval и context cancellation.
Handle/WaitExtended наблюдают caller-selected ref и не переигрывают неизвестную mutation.
Root Resource/Resume делегирует exact-ref Cinder Handle; Resume сохраняет Receipt.
Known receipt Recover conclusive без I/O; unknown остаётся indeterminate после exact GET.
Known accepted/rejected/not_sent receipt без ref/endpoint также поддерживает
conclusive Recover без I/O после проверки schema/account/revision/mutation effect
и scope. NotSent Receipt содержит catalog revision даже при локальной ошибке
валидации. Receipt-only Status/Wait не обещает готовность ресурса и unavailable.
No-ID unknown не позволяет recovery. Type/QoS read refs не имеют lifecycle handle.

## Граница покрытия

Это полный подтверждённый surface опубликованного SDK для volumes/types/QoS,
а не полный upstream Cinder API. Full inventory Cinder/microversions/extensions
остаётся неизвестным: inventory_complete=false, coverage_ratio=null. Snapshot,
backup и attachment lifecycle относятся к отдельным задачам. Не подтверждённые
admin actions, type/QoS mutations, transfers, migration, generic action/raw executor
не заявляются доступными. Отсутствие метода SDK не доказывает отсутствие server API.
[Selectel](https://docs.selectel.ru/cloud-servers/volumes/edit-volume/) отдельно
указывает, что прямой retype сетевого диска не поддержан; нужен новый диск.

Публичные тесты используют настоящий Client/scoped API, synthetic CredentialSource
и только Config.Transport как подмену. Генерация, race/vet и evidence — offline;
live availability и реальные cloud mutations не проверялись.

Пример после явного создания клиента и выбора location:

```go
size, _ := types.ParseInteger("20")
created, err := regional.Volumes().Create(ctx, volumes.CreateSpec{
    Size: &size, Name: "data", VolumeType: "universal.ru-1a",
}, types.CallOptions{})
// Сохраните created.Receipt даже при err; неизвестный исход не повторяйте.
if err == nil {
    handle, handleErr := regional.Volumes().Handle(*created.Data.Ref,nil)
    if handleErr == nil {
        waited, waitErr := handle.Wait(ctx, "volume.available")
        _, _ = waited, waitErr
    }
}
```
