# Сервисы и граница покрытия

## Реестр семейств

Service ID является стабильным именем пакета и каталога. Семейство содержит
все операции указанного документированного API, включая действия, вложенные
ресурсы и transfer; наличие одинакового ресурса в SDK не ограничивает эту
границу. Новые официальные API добавляются в реестр по правилам каталога.

| Service ID | Публичный контракт | Адаптер |
|---|---|---|
| `identity` | [Auth](https://docs.selectel.ru/api/authorization/) | Keystone v3 HTTP |
| `iam` | [Пользователи, роли, S3 credentials](https://docs.selectel.ru/api/users-and-roles/) | IAM SDK / HTTP |
| `federations` | [Федерации](https://docs.selectel.ru/api/federations/) | IAM SDK / HTTP |
| `projects` | [Проекты и ресурсы](https://docs.selectel.ru/api/cloud-projects-and-resources/) | go-selvpcclient/v5 / HTTP |
| `quotas` | [Квоты и лимиты](https://docs.selectel.ru/api/quota-management/) | go-selvpcclient/v5 / HTTP |
| `compute` | [Nova](https://docs.openstack.org/api-ref/compute/) в Selectel | Gophercloud v2 / HTTP |
| `network` | [Neutron](https://docs.openstack.org/api-ref/network/v2/) в Selectel | vpc-go / Gophercloud / HTTP |
| `volumes` | [Cinder](https://docs.openstack.org/api-ref/block-storage/v3/) в Selectel | blockstorage-go / Gophercloud / HTTP |
| `images` | [Glance](https://docs.openstack.org/api-ref/image/v2/) в Selectel | images-go / Gophercloud / HTTP |
| `loadbalancers` | [Octavia](https://docs.openstack.org/api-ref/load-balancer/v2/) в Selectel | Gophercloud / HTTP |
| `public_network` | [Прямые публичные IP](https://docs.selectel.ru/api/cloud-public-network/) | public-net-api-go / HTTP |
| `private_dns` | [Приватный DNS](https://docs.selectel.ru/api/private-dns/) | private-dns-go / HTTP |
| `mks` | [Текущий v2](https://docs.selectel.ru/api/managed-kubernetes/), [legacy v1](https://docs.selectel.ru/api/managed-kubernetes-legacy/) | HTTP v2; mks-go v1 |
| `databases` | [Managed databases](https://docs.selectel.ru/api/managed-databases/) | dbaas-go / HTTP; поколения разделены по engine |
| `registry` | [Реестры, репозитории, токены](https://docs.selectel.ru/api/craas/) | craas-go / HTTP |
| `registry_data` | [OCI distribution](https://github.com/opencontainers/distribution-spec) в Selectel | Protocol adapter; отдельный от registry management |
| `s3` | [S3 subset Selectel](https://docs.selectel.ru/api/object-storage-s3/) | AWS SDK Go v2 |
| `swift` | [Swift](https://docs.selectel.ru/api/object-storage-swift/), [old](https://docs.selectel.ru/api/object-storage-swift-old/) | Protocol adapter; old явно выбран |
| `object_storage` | [Management](https://docs.selectel.ru/api/object-storage/), [old](https://docs.selectel.ru/api/selectel-storage-old/) | HTTP; old явно выбран |
| `backups` | [Scheduled backup / restore](https://docs.selectel.ru/api/scheduled-backups/) | cloudbackup-go / HTTP |
| `dedicated` | [Dedicated servers](https://docs.selectel.ru/api/dedicated/) | dedicated-go/v2 / HTTP |
| `vmware` | [VMware access](https://docs.selectel.ru/api/vmware/) | Документированный Cloud Director adapter |
| `cloud_1c` | [Облако 1С](https://docs.selectel.ru/api/1c-cloud/) | HTTP |
| `dns` | [Текущий DNS](https://docs.selectel.ru/api/dns-actual/), [legacy](https://docs.selectel.ru/api/dns-legacy/) | domains-go / HTTP |
| `cdn` | [CDN](https://docs.selectel.ru/api/cdn/) | HTTP |
| `email` | [Почтовый сервис](https://docs.selectel.ru/api/email-service/) | HTTP / документированный почтовый протокол |
| `global_router` | [Глобальный роутер](https://docs.selectel.ru/api/global-router/) | globalrouter-go / HTTP |
| `ip_addresses` | [Учет IP](https://docs.selectel.ru/api/ip-addresses/) | HTTP |
| `billing` | [Баланс](https://docs.selectel.ru/api/balance/), [статистика](https://docs.selectel.ru/api/balance-statistics/), [отчёты](https://docs.selectel.ru/api/billing-reports/), [транзакции](https://docs.selectel.ru/api/billing-transactions/) | HTTP; схемы auth раздельны |
| `logs` | [CloudWatch-compatible Logs](https://docs.selectel.ru/logs/tools/aws-cli/) | AWS SDK Go v2 CloudWatchLogs, subset Selectel |
| `metrics` | [Scrape](https://docs.selectel.ru/metrics/tools/prometheus/), [query integration](https://docs.selectel.ru/metrics/tools/grafana/) | Prometheus HTTP adapter |
| `audit` | [Аудит-логи](https://docs.selectel.ru/api/audit-logs/) | HTTP read query |
| `secrets` | [Секреты](https://docs.selectel.ru/api/secrets-manager/) | secretsmanager-go / HTTP |
| `certificates` | [Пользовательские](https://docs.selectel.ru/api/certificates-manager/), [Let's Encrypt](https://docs.selectel.ru/api/lets-encrypt-certificates/) | secretsmanager-go / HTTP |
| `mobile_farm` | [Мобильная ферма](https://docs.selectel.ru/api/mobile-farm/) | HTTP |
| `tickets` | [Тикеты](https://docs.selectel.ru/api/tickets/), [вложения](https://docs.selectel.ru/api/tickets-attachments/) | HTTP / stream transfer |
| `kms` | Публичный контракт не подтверждён | discovery_pending; методы encrypt/decrypt отсутствуют |

OpenStack, AWS, OCI и VMware операции входят в покрытие только в пределах
подтверждённой реализации и документации Selectel. Полная upstream API surface
не объявляется поддержанной автоматически. Неподдержанная microversion или
extension возвращает capability_unavailable до отправки.

Secrets Manager не считается KMS: хранение версий секрета не предоставляет
само по себе управление ключами, encrypt/decrypt или envelope encryption.
Названия продуктов Yandex Cloud не используются как имена Selectel API.

## Расширение покрытия

Каждая операция имеет один нормативный OperationContract. Контракт связывает
официальную схему с Go method и wire binding; общие правила ядра применяются
ссылками и не дублируются в сервисе. Контракт содержит:

| Часть | Обязательные поля |
|---|---|
| Идентичность | Service, generation, resource kind, action, стабильный operation ID |
| Источник | Official URI, revision/content hash, точное path/protocol operation |
| Публичный API | Go method, request/result types, full schema refs, covered parameter schema/исключённые variants и sensitive fields |
| Доступ | Account/project/location требования, auth scheme и audience |
| Endpoint | Service catalog recipe или URL map, path variables и microversion |
| Wire request | Method, path, все path/query/header/body поля и их mapping из DTO/scope |
| Result | Successful statuses, все result поля, refs, timestamps, required/optional mapping |
| Ошибки | Documented provider statuses/codes, явный rejection proof, общая классификация |
| Execution | read/mutation/transfer, timeout class, replay policy и источник идемпотентности |
| List | Items/next/end/order mapping; fixed/variable page size, consistency и limits |
| Wait | Status method, exact ref, goals, pending/success/failure/unknown states или unavailable |
| Recovery | Exact correlation/status mechanism с доказательством либо unavailable |

Для documented поля без default SDK не вводит default. Optional поле
не отправляется, если caller его не задал. Provider default сохраняется как
provider default, а не как гарантия SDK. Обязательное поле без значения
возвращает invalid_argument до resource request.

Если official source не определяет безопасность replay, rejection proof,
snapshot consistency или remote cancellation, контракт назначает соответственно
`never`, `not_proven`, `best_effort`, `unavailable`. Отсутствующие обязательные
wire данные не угадываются; операция остаётся unimplemented.

Публичный метод реализует validation, request, response и error mapping
контракта через [execution](execution.md#контракт-вызова). List добавляет
[pagination](pagination.md#алгоритм-listpage--next--collect); wait добавляет
[operations](operations.md). Прямой вызов upstream вне этих правил не входит
в public API selectelsdk.

## Предметные ограничения

### Проекты Resell v2

Account-scoped `Projects.Create/Get/Update/Delete` используют base
`https://api.selectel.ru/vpc/resell`. Create передаёт body.project с name,
optional description и skip_quotas_init; длина name составляет 1..64 Unicode
codepoints, description — 0..255. Get принимает exact project ref и optional
exclude_quotas. Update передаёт только заданные name, description, custom_url
и theme.brand_color/color/logo; nullable поля сохраняют omitted/null/value.
Delete адресует exact ref. Создание не переключает account view на новый project.

Все методы применяют [execution](execution.md#контракт-вызова). Create принимает
HTTP 200 и требует ID провайдера; Update принимает HTTP 201, Delete — 204.
Get/Update сохраняют исходный ref, если optional response ID отсутствует;
несовпадающий ID возвращает scope_mismatch. Поля quotas в Get представлены
типизированной map, включая region, zone, value и used; числа сохраняют точность.
HTTP 400 у Get/Delete означает not_found по официальному контракту этих
операций; у Create/Update он означает provider_error. Наличие названия проекта
не разрешает поиск для восстановления исхода Create.

### Container Registry v1/v2

`Registry.V1.Registries` предоставляет Create/Get/Delete и ListPage/Iterate/Collect.
V1 DTO сохраняет id, name, status, createdAt, size, sizeLimit и used; size-поля
являются Integer, used — точным Number. Create передаёт body.name и принимает
201; Delete принимает 204. Get сохраняет exact request ref при отсутствии
optional response ID и отклоняет несовпадение. Create без достоверного ID
возвращает accepted Receipt вместе с protocol_error.

V1 list передаёт limit/offset и явно заданные search/sort_field/sort_type.
Начальный offset задаётся caller либо равен 0. HTTP 200 содержит bare array;
204 не содержит тела. Конец подтверждается 200 [] либо 204. Каждая непустая
страница увеличивает точный offset на число wire items; короткая непустая
страница не доказывает конец. Обход применяет [pagination](pagination.md).
V2 list использует отдельный DTO и envelope registries/totalCount.

Endpoint base включает /api; generation включается в path один раз.
Методы не преобразуют ref между v1 и v2 и не выбирают другую generation после
ошибки. Wait/recovery contract для этих CRUD bindings отсутствует; exact Get
является наблюдением ресурса и не доказывает авторство неизвестного изменения.

### Managed Kubernetes v2

MKS v2 предоставляет 29 операций закреплённой схемы: Clusters
List/Create/Get/Replace/Update/Delete/Kubeconfig/RotateCertificates/
UpgradePatchVersion/UpgradeMinorVersion; Nodegroups List/Create/Get/Replace/
Delete/Resize; Nodes Get/Delete/Reinstall; Tasks List/Get; RegistryIntegrations
Create/Get/Replace/DeleteAll/Delete; Configuration AdmissionControllers/
KubernetesVersions/FeatureGates. Wire methods, схемы и ответы каждой операции
задаются SDK-owned OperationContract в каталоге.

Все операции требуют project/location scope и IAM project token. Endpoint
выбирается по документированной карте v2 без перехода на v1. Pool в Create
должен совпадать с scope. Parent refs обязательны: nodegroup принадлежит cluster,
node — nodegroup, task — cluster; их scope, endpoint и generation совпадают.
Response project/pool/parent IDs проверяются до сохранения известных refs.

Create cluster принимает 201 с DetailedCluster; Update/Replace и upgrades —
200 с DetailedCluster. Создание групп узлов принимает 204 без ID групп и задач;
receipt сохраняет только известный parent cluster ref. SDK не выбирает новые
группы по разнице списков. Nodegroups Create принимает весь явно заданный массив,
Replace и Resize передают body.nodegroup. Интеграции реестров передают
body.registries; Replace сохраняет семантику замены collection.

Request DTO сохраняет все поля соответствующей source schema и не вставляет
provider defaults. Required additional_software задаётся ProtectedJSON object,
даже при deprecated статусе поля в схеме. UserData в NodegroupCreateSpec является
ProtectedValue с уже закодированным base64 текстом: он проверяется и передаётся
без повторного кодирования. Возвращённые user_data, kubeconfig и свободный текст
ошибки задачи защищены. Kubeconfig возвращается как bounded YAML bytes без
изменения файлов, kube context или подключения к Kubernetes API.

Nodegroups list имеет один provider response и локальный replay cursor при
ограничении выдачи. Tasks list использует limit/offset и response.count;
with_error_details отправляется только при заданном значении. Count сохраняется
как точный provider total; конец наступает при пустой странице либо достижении
count. Полнота не означает snapshot consistency.

Resource/Resume поддерживают cluster, nodegroup, node и task refs v2. Цели
cluster.active/nodegroup.active требуют ACTIVE; ERROR завершает ожидание
operation_failed, остальные документированные состояния являются pending.
Task.done требует DONE; ERROR/CANCELED завершают ожидание ошибкой, IN_QUEUE и
IN_PROGRESS являются pending. Новое состояние возвращает state_unknown.
Цели *.deleted подтверждаются только exact Get 404. Node не содержит status:
существующий Get означает pending удаления с пустым ProviderState.

Receipt-bound Wait разрешает только цели каталога для исходной операции.
Создание групп без ID не разрешает ожидание их завершения по готовности parent
cluster. Active подтверждает текущее состояние ресурса, а не конкретный task,
изменение числа узлов или здоровье приложений. Source mutations не возвращают
causally связанную task ref; SDK не выбирает последнюю задачу. Recover по exact
ресурсу остаётся indeterminate и не меняет неизвестный исход изменения.

### Ограничения поколений и предметных действий

MKS v2 не заменяется v1 при ошибке. Registry token API v2 не означает поддержку
registry management v2 через craas-go. DB v2 bindings фиксируются для конкретного
engine; actions PostgreSQL v1 не переносятся на ClickHouse/OpenSearch автоматически.

Public-network port, Neutron floating IP и public IP control plane являются
разными ресурсами с разными refs. Global-router network attach не создаёт
обычную Neutron network. Private DNS PutRecords сохраняет семантику замены
collection; SDK не маскирует его независимым CreateRecord.

Backup retention settings не объявляются скачиванием backup. Cloud backup
restore реализуется отдельным документированным HTTP action, если его нет
в SDK. Биллинг account, project consumption, provider prediction и resource
cost имеют раздельные методы и типы. Telemetry отражает только доступные
и реально собранные данные; SDK не синтезирует отсутствующие логи/метрики.

[Контракт Mobile Farm scoped adapter](mobilefarm.md).
