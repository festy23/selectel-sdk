# Публичный Go-интерфейс

## Пакеты

| Пакет | Ответственность |
|---|---|
| Корневой `selectelsdk` | Создание клиента и неизменяемых scoped views |
| `types` | Scope, ResourceRef, Receipt, Error, Money, ProtectedValue |
| `catalog` | Перечисление и описание возможностей без сетевых вызовов |
| `credentials` | Явно вызываемые loaders и CredentialSource |
| Пакет каждого сервиса | Типизированные операции конкретных поколений API |
| Внутренний runtime | Auth, endpoints, transport, retries и pagination |

Публичный интерфейс не предоставляет универсальный `Execute(map)` вместо
сервисных методов. Внутренние адаптеры и transport не экспортируются как
обход общих гарантий. `Config.Transport` является явной точкой внедрения HTTP
transport; применение собственного transport не делает адрес официальным API.

## Общие типы

Необязательные значения представлены указателем или отдельным признаком
присутствия. Нулевой размер, пустой ID и отсутствие значения не смешиваются.
Nullable request поле использует `*types.Nullable[T]`: nil опускает поле,
`types.Null[T]()` передаёт JSON null, `types.Value(value)` передаёт значение.
Null допустим только для поля, чей официальный контракт разрешает null.
Временные метки SDK представлены `time.Time`, длительности — `time.Duration`.
JSON времён SDK использует RFC3339Nano в UTC. Времена провайдера без timezone
сохраняются исходной строкой; числовые timestamps совместимых протоколов
сохраняют исходную точность. SDK не приписывает им отсутствующий offset.
JSON длительностей SDK использует целые миллисекунды, кроме полей с явно
закреплённой единицей в сервисном контракте. Wire значения используют единицы
и точность соответствующего API; `QueryRange.Step` передаётся и возвращается
в JSON как точное десятичное число секунд в `step_seconds`.

| Тип / поле | Тип значения | Контракт |
|---|---|---|
| `Scope.AccountID` | string | Обязательный номер аккаунта, без нормализации регистра |
| `Scope.ProjectID` | optional string | ID проекта; отсутствие означает account view |
| `Scope.Region` | optional string | Документированное имя региона |
| `Scope.Pool` | optional string | Документированное имя пула, отдельно от региона |
| `AuthRequest.Scope` | Scope | Явная область авторизации |
| `AuthRequest.Mode` | enum | iam_account, iam_project, static_token, s3_sigv4, basic или protocol_specific |
| `AuthRequest.Service` | string | Service ID каталога |
| `AuthRequest.Audience` | string | Audience binding выбранного service/auth contract |
| `ResourceRef.Scope` | Scope | Исходная область ресурса |
| `ResourceRef.Service` | string | Service ID каталога |
| `ResourceRef.APIGeneration` | string | Поколение wire API, независимо от major Go SDK |
| `ResourceRef.Kind` | string | Тип ресурса из контракта операции |
| `ResourceRef.ID` | string | Идентификатор, возвращённый провайдером; обязателен |
| `ResourceRef.Parent` | optional ResourceRef | Родитель, если адресация требует его ID |
| `ResourceRef.EndpointID` | string | Нечувствительная идентичность выбранного endpoint |
| `ResourceRef.EndpointURL` | string | Проверенный URL без credentials и query |
| `Receipt` | структура | Подтверждение вызова по [execution.md](execution.md#receipt) |
| `SDKError` | структура, реализует error | Ошибка по [execution.md](execution.md#ошибки) |
| `ProtectedValue` | закрытая структура | Чувствительные bytes по [execution.md](execution.md#чувствительные-значения) |
| `Money.Raw` | string | Десятичное число провайдера без float64 |
| `Money.Currency` | optional string | Код, возвращённый провайдером; отсутствие явно сохранено |
| `Money.Unit` | string | Документированная единица либо `provider_unit_unknown` |
| `Money.Scale` | optional int32 | Десятичный масштаб, только при документированном значении |
| `Metadata` | структура | Scope, endpoint ID, generation, observed_at, request_ids |

`ResourceRef` не содержит credentials, snapshot ресурса или тело запроса.
Все возвращённые ссылки сохраняют scope и endpoint исходного вызова.
Имена ресурсов не используются вместо ID. Объекты с адресацией по имени
используют в `ID` документированный ключ и `Kind`, явно обозначающий эту форму.

## Методы ядра

| Метод | Вход | Выход | Сетевые действия |
|---|---|---|---|
| `New(Config)` | Credentials, account и настройки | Client либо SDKError | Отсутствуют |
| `Client.Account()` | Нет | AccountClient исходного аккаунта | Отсутствуют |
| `Client.Project(projectID)` | Непустой ID | ProjectClient либо SDKError | Отсутствуют |
| `ProjectClient.Location(region, pool)` | Region и pool | RegionalClient либо SDKError | Отсутствуют |
| `Client.Catalog()` | Нет | Неизменяемый встроенный каталог | Отсутствуют |
| `catalog.Default()` | Нет | Тот же встроенный каталог без Client и credentials | Отсутствуют |
| `Catalog.List(Filter)` | Service/generation/effect/availability/verification; каждый фильтр optional | ([]Capability, error) | Отсутствуют |
| `Catalog.Describe(operationID)` | Точный ID | Capability либо SDKError | Отсутствуют |
| `Client.Authenticate(ctx, request)` | Явный AuthRequest | Метаданные авторизации либо SDKError | [Auth](access.md#авторизация) |
| `Client.Resume(receipt)` | Receipt и совместимый клиент | Handle либо SDKError | Отсутствуют |
| `Client.Resource(ref)` | Exact ResourceRef | Handle без Receipt либо SDKError | Отсутствуют |
| `Handle.Status(ctx)` | Context | Observation либо SDKError | [Status](operations.md#status) |
| `Handle.Wait(ctx, goal)` | Context и WaitGoal | WaitResult либо SDKError | [Wait](operations.md#wait) |
| `Handle.Recover(ctx)` | Context | RecoveryResult либо SDKError | [Recover](operations.md#recover) |

Context является первым аргументом любого метода, выполняющего I/O.
`nil` context недопустим. Context применяется к credential source, auth,
HTTP, transfer, retries, pagination и ожиданию.

CallOptions содержит только optional AuthMode и optional provider idempotency
key. AuthMode выбирается из явно поддержанных схем capability; отсутствие
означает preferred scheme её контракта. В initial billing preferred — account
IAM, alternative — static token; у остальных initial операций alternative
не задана. Для IAM CredentialSource возвращает password или external token
material нужного scope. Idempotency key допустим только если replay contract
определяет wire mapping; иначе invalid_argument. Timeout изменяется через
Config или более короткий context deadline, а не скрытый default options.

Scoped views не изменяют Client или ранее созданные views. Каждый сервис
доступен только из подходящего view: account-сервисы из AccountClient,
project-сервисы из ProjectClient, региональные из RegionalClient.
Необходимые дополнительные scopes задаются явно аргументами метода.

### Создание и выбор scope

Ресурсы: Config, встроенный каталог и [настройки](settings.md).
Ошибки: `invalid_argument`, `scope_mismatch`, `capability_unavailable`.

1. Для `New` проверить обязательные поля Config и диапазоны settings.
   Отсутствующее поле или значение вне диапазона завершает вызов
   `invalid_argument`; I/O отсутствует.
2. Для scope-метода скопировать account и настройки исходного клиента.
   Пустой project ID или отсутствующая обязательная location завершают
   вызов `invalid_argument`. Pool без region допустим только при однозначной
   связи в встроенном документированном location registry; неоднозначность
   завершает вызов `scope_mismatch`.
3. Сохранить новое значение scope без изменения источника. Наличие region
   не подразумевает наличие этого сервиса в region; это проверяется при вызове.
4. Вернуть новый immutable view с общим потокобезопасным runtime.

### Каталог

Ресурс: [каталог](catalog.md). Ошибки: `invalid_argument`, `capability_unavailable`.

1. `List` проверяет enum каждого заданного фильтра. Неизвестный enum возвращает
   `invalid_argument`. Строковые service/generation сравниваются точно.
2. Выбрать записи, удовлетворяющие всем заданным фильтрам; отсутствующий фильтр
   не ограничивает выборку. Сортировать по `operation_id` лексикографически.
   Вернуть копии записей; пустая выборка успешна.
3. `Describe` проверяет непустой ID. Пустой ID возвращает `invalid_argument`;
   неизвестный ID — `capability_unavailable`; известный — копию записи.

## Сервисные методы

Методы группируются по сервису, ресурсу и явному поколению API. Стандартные
формы — `Get`, `ListPage`, `Iterate`, `Collect`, `Create`, `Update`, `Delete`;
предметные действия сохраняют имя и семантику сервиса. Несуществующее в API
действие не добавляется из-за наличия одноимённого действия у другого сервиса.

Каждый сетевой метод возвращает `(Result[T], error)`. `Result` содержит
`Data`, `Metadata` и optional `Receipt`. Для изменения Receipt присутствует
даже при ошибке; для не начатого вызова его outcome равен `not_sent`.
Read result не содержит Receipt. При ошибке чтения одного объекта Data имеет
нулевое значение result type. Collect/Inventory сохраняют уже полученные
данные с `Complete=false`. Метрики при превышении MaxMetricSamples сохраняют
ограниченный prefix и `Complete=false` вместе с `response_limit` по
[контракту телеметрии](scenarios.md#телеметрия).

Request/result DTO официального SDK переиспользуется, если он выражает
публичный контракт и не нарушает правила scope, context и чувствительных
значений. Общие типы принадлежат selectelsdk. Для отсутствующих операций DTO
задаётся документированной схемой API. Sensitive result поля заменяются
`ProtectedValue`, включая пароли, приватные ключи, kubeconfig и secret payload.

Mutation возвращает принятие запроса и известные ID без встроенного ожидания.
`CreateAndWait` выполняет Create ровно один раз и затем отдельный Wait;
его ошибка сохраняет Create result и Receipt. Он не создаёт зависимости
ресурса и не удаляет принятый ресурс при timeout ожидания.

## Совместимость

Public API и JSON каталога версионируются отдельно от wire API Selectel.
Изменение имени метода, DTO, outcome, обязательного поля, default или
смысла ошибки требует несовместимой версии SDK после 1.0.
Добавление capability и optional поля с прежней семантикой совместимо.
Unknown provider fields сохраняются в типизированном extension container
только если они нечувствительны; остальные не попадают в diagnostics.
Неизвестное provider state сохраняется строкой и не признаётся успешным.

Major версии SDK, используемых в публичных DTO, входят в контракт selectelsdk.
Обновление dependency не может молча менять API generation или семантику.
Версии MKS v1/v2, DNS v1/v2 и DB engine API предоставляются раздельно.
Выбор legacy поколения является явным действием вызывающей стороны.
