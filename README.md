# selectelsdk

Go-библиотека управления Selectel. Нормативный контракт находится в
[спецификации](docs/modules/selectelsdk/README.md).

Модуль предоставляет явные credential sources, неизменяемые account/project/
location views, IAM-авторизацию с проверкой области и обновлением токенов,
единые ошибки, защищённые значения и встроенный каталог публичных API.

Каталог содержит 191 операцию: восемь закреплённых официальных API-схем
Selectel и документированные части Nova, CloudWatch Logs и Prometheus.
Из них 53 имеют статус `implemented`, 11 — `partial`, 127 — `unimplemented`.
Для 64 callable bindings записана локальная проверка `offline`; проверки
на живом облаке в этот результат не входят.
Реализованы три метода Billing, все девять операций текущей схемы Secrets Manager,
чтение проектов, Kubernetes v2, баз данных v1, Registry v2 и аудит-событий.
Проекты Resell v2 поддерживают Create/Get/Update/Delete. Registry v1 имеет
отдельные Create/Get/Delete и ListPage/Iterate/Collect с собственными DTO.
Все 29 операций закреплённой схемы MKS v2 имеют callable bindings: кластеры,
группы узлов, узлы, задачи, интеграции реестров и справочники Kubernetes.
Четыре операции Nova — Create, Get, Delete и ListPage — имеют статус `partial`:
они покрывают начальный сценарий одной VM; ограничения записаны в каталоге.
Доступны CreateAndWait, Client.Resource/Resume и Handle.Status/Wait/Recover.
Остальные операции отмечены `unimplemented`. Описание API не означает наличие
callable метода. Inventory.Scan объединяет пять явно выбранных семейств:
projects, servers, clusters, datastores, registries. Полный SDK требует остальных
сервисных методов. Доступны начальные Logs и Metrics: CloudWatch Groups,
Streams, Events, Filter; Prometheus Scrape, Query и QueryRange. Эти bindings
имеют статус partial; неподтверждённые варианты upstream API перечислены
в каталоге. Все начальные сквозные сценарии теперь имеют callable bindings.
`inventory_complete=false` и `coverage_ratio=null` сохраняются до полного
перечисления всех семейств Selectel.

## Использование

Добавление библиотеки в Go-проект:

```sh
go get github.com/festy23/selectel-sdk
```

Go-модуль находится в корне репозитория. Для агента доступен
[скилл selectelsdk](.agents/skills/selectelsdk/SKILL.md), который описывает
каталог возможностей, доступ, scope, сервисные вызовы и обработку результатов.
Исходящие cloud mutations выполняются в рамках разрешённых пользователем действий.

```go
import (
    "encoding/json"

    sdk "github.com/festy23/selectel-sdk"
    "github.com/festy23/selectel-sdk/catalog"
    "github.com/festy23/selectel-sdk/credentials"
)

// Discovery выполняется без аккаунта, credentials и сетевых запросов.
contracts, err := catalog.Default().List(catalog.Filter{Service: "mks"})
catalogJSON, err := json.Marshal(catalog.Default())

// Источник указан явно. Ядро не выбирает активный профиль или переменные env.
source, err := credentials.FromEnvironment("SELECTEL_")
client, err := sdk.New(sdk.Config{
    AccountID: "YOUR_ACCOUNT_ID",
    Credentials: source,
})
project, err := client.Project("YOUR_PROJECT_ID")
location, err := project.Location("YOUR_REGION", "YOUR_POOL")
```

Каждую ошибку необходимо обработать перед использованием результата.
`New`, выбор scope и чтение каталога не обращаются к облаку. `Authenticate`
получает context и явный `types.AuthRequest`; IAM password source получает
token, external IAM token source проверяет его через Keystone introspection.
Для static/S3/Basic material результат помечается `caller_asserted`.

Для environment source используются только поля с заданным prefix:
ACCOUNT_ID, USERNAME/PASSWORD либо IAM_TOKEN/TOKEN_EXPIRES_AT, PROJECT_ID,
STATIC_TOKEN, S3_ACCESS_KEY/S3_SECRET_KEY, BASIC_USER/BASIC_PASSWORD.
PROJECT_ID является ограничением binding credentials, а не активным target.
IAM password и external IAM token нельзя задавать одновременно.

`FromProfile(path, name)` читает указанную запись JSON, где корневой object
отображает имена профилей в records с полями snake_case. `FromKeyring(store,
entry)` использует предоставленный `credentials.Store`. Чтение выполняется
при Lookup с context. Изменение материала создаёт новую непрозрачную revision.

`ProtectedValue` скрывает bytes в Go formatting, Text и JSON. Для явного доступа
используйте Reveal/CopyTo. SDK не гарантирует обнуление всех копий памяти.

Secrets Manager доступен через `project.Secrets()`: ListMetadata/GetMetadata
читают метаданные, ReadValue/ReadVersion явно получают ProtectedValue.
Create/CreateVersion/Update/Delete/ActivateVersion возвращают Receipt.

Nova доступна через `location.Compute().Servers`. CreateRequest использует
native `servers.CreateOpts` Gophercloud; сеть/порт задаётся явно, одна VM.
Чувствительные inputs задаются отдельными AdminPassword/UserData; UserData
содержит исходные bytes, которые SDK кодирует base64 ровно один раз.
Создание и удаление не повторяются автоматически. Wait отменяет локальное
ожидание; accepted Receipt сохраняется. Recover по ресурсу возвращает
наблюдение, которое не доказывает принятие исходного unknown вызова.

Пагинация ограничена PageSize/MaxItems/MaxPages и общим collection deadline.
При лимите caller получает частичные Items, Complete=false, continuation и
ошибку incomplete. Cursor привязан к scope, endpoint, contract и параметрам.
Чувствительный cursor скрыт в JSON/formatting; только Cursor.Export явно
раскрывает token для сохранения, после чего его можно прочитать json.Unmarshal.

`client.Inventory().Scan(ctx, inventory.Plan)` проверяет весь plan до I/O и
возвращает ordered результаты всех branches, их ошибки и точный Coverage.
403 одной ветки не отменяет остальные; cancel/deadline прекращает новые
requests. Уже полученные данные сохраняются. Protected payloads исключены.

Location допускает pool без region, если он есть в документированном registry:
`project.Location("", "ru-1")` создаёт scope region=spb, pool=ru-1. Ключи
регионов: spb/msk/nsk/tas/alm/nrb. Сегмент пула задаётся отдельно в опциях
соответствующего ресурса и не подставляется вместо pool.

DTO inventory и audit генерируются из pinned schema с сохранением optional
полей и точных integers. Schemaless JSON защищён типом ProtectedJSON;
его Reveal возвращает исходные JSON bytes. Provider не может подменить SDK ref
своим одноимённым полем. Пагинация сохраняет provider totalCount/prev_cursor/count
в Provider/ProviderPages.

`client.Account().Projects()` управляет проектами с account IAM. Nullable
поля изменения различают отсутствие, `types.Null[string]()` и
`types.Value(value)`; явные false и пустые строки сохраняются. Get возвращает
quotas и theme. Изменение использует документированный HTTP 201; отсутствие
проекта в Get/Delete обозначается провайдером HTTP 400 и возвращает not_found.

`project.Registry().V1.Registries` управляет Registry v1. Его sizeLimit/used
сохраняются отдельно от Registry v2. List v1 обходит bare-array ответы до []
или 204; короткая непустая страница не считается концом. V1ListOptions.Offset
принимает точный Integer для явного начального offset. Create/Delete не
повторяются при потере ответа; принятые responses с ошибкой декодирования
сохраняют Receipt и достоверно извлечённые ID.

`location.MKS().V2` предоставляет Clusters, Nodegroups, Nodes, Tasks,
Registries и Configuration. Clusters поддерживает CreateAndWait; Resource/Resume
работают с известными MKS refs, включая parent refs вложенных ресурсов.
Create групп узлов возвращает 204 без новых ID: SDK сохраняет accepted Receipt
и известный parent cluster, не выбирает группы по разнице списков и не ждёт
их готовности через состояние parent. Unknown изменение не повторяется.

Kubeconfig и nodegroup user_data возвращаются защищёнными значениями.
NodegroupCreateSpec.UserData содержит уже закодированный base64 текст;
SDK передаёт его без повторного кодирования. AdditionalSoftware задаётся
явным ProtectedJSON object согласно required полю схемы. Request DTO сохраняют
nil/false/zero/empty arrays/maps и не вставляют server defaults.

Wait подтверждает cluster.active/nodegroup.active, task.done или exact 404
для удаления. Новые states возвращают state_unknown. Свободный текст ошибок
задач защищён; with_error_details отправляется только если задан caller.
API v1 и работоспособность на живом облаке этими проверками не подтверждаются.

`location.Logs()` использует явный S3 credential source, который привязан к
project/pool. Подпись SigV4 выполняет AWS SDK Go v2; default credential chain
и middleware retries не включаются. Events требует Start/End и обходит поток
вперёд. Messages, sequence tokens и cursors возвращаются защищёнными значениями.

`location.Metrics()` использует явный Basic source с ID пользователя и паролем.
Namespace выбирается явно из compute/dedicated/inference. Scrape возвращает
samples в порядке text response; Query и QueryRange читают подтверждённый
Prometheus integration endpoint проекта/namespace. Поддержаны vector/matrix/
scalar/string и native histogram response. Значения и дробные timestamps
сохраняются без float64. При MaxMetricSamples выдаётся prefix с complete=false
и response_limit; MaxJSONBytes ограничивает и JSON, и scrape text.

QueryRange.Step передаётся как точное число секунд. В Go result сохраняется
time.Duration; JSON result использует step_seconds, включая дробную часть.
Upstream admin/write API, неподтверждённые namespaces и другие версии text
protocol не считаются покрытыми этими methods.

## Проверки и каталог

Требуется Go 1.26 или новее. В корне репозитория:

```sh
go generate ./catalog
go generate .
go test -race ./...
go vet ./...
python3 scripts/verify_catalog.py
```

Генерация читает только локальные public schema snapshots в catalog/sources
и не обращается к сети. Manifest создаётся детерминированно; source revisions
представлены SHA-256 исходных файлов. Catalog.List/Describe возвращают копии.
Сервисные методы засчитываются в покрытие только по правилам спецификации.

Первые billing reads доступны через `client.Account().Billing()`. Методы
получают context и `types.CallOptions`. Для статистики обязательны provider keys,
период и группировки. Integer/Money сохраняют точность исходного числа;
неизвестные денежные единицы и timezone не заменяются догадками.

`verify_catalog.py` запускает проверки и сохраняет только hashes/статусы
проверки в catalog/verification.json. Изменение implementation, contract или
evidence файлов сбрасывает устаревшую verification в none при генерации.
