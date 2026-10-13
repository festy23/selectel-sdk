# Настройки

Настройки неизменяемы после New. Неверное значение возвращает `invalid_argument`
без сетевых вызовов. Лимиты задают общую политику SDK; меньший лимит wire API
применяется дополнительно. Default выбран как часть нормативного контракта.

| Название | Тип | Обязательность | Значение по умолчанию | Описание |
|---|---|---|---|---|
| `AccountID` | string | Да | Нет | Непустой номер аккаунта |
| `Credentials` | CredentialSource | Да | Нет | Явный источник доступа |
| `Transport` | http.RoundTripper | Нет | Transport SDK | TLS verification; proxy из обычной конфигурации HTTP transport |
| `EndpointOverrides` | map[EndpointKey]EndpointBinding | Нет | Пустая map | Явные адреса и область допустимого использования credentials |
| `CallTimeout` | duration | Нет | 30 s | Общий бюджет одного сервисного вызова вместе с auth, повторами и задержками; >0 |
| `TransferTimeout` | duration | Нет | 30 min | Общий бюджет явной передачи бинарных данных; >0 |
| `CollectionTimeout` | duration | Нет | 10 min | Общий бюджет Collect или Inventory.Scan вместе со всеми страницами/branches; >0 |
| `ReadMaxAttempts` | uint | Нет | 3 | Общее число сетевых попыток безопасного чтения, 1..5 |
| `RetryBaseDelay` | duration | Нет | 250 ms | >0 |
| `RetryMaxDelay` | duration | Нет | 2 s | >= RetryBaseDelay; предел exponential backoff |
| `RefreshSkew` | *duration | Нет | nil → 60 s | Явное значение >=0; обновление IAM до expires_at |
| `AuthCacheEntries` | uint | Нет | 128 | 1..1024 записей с LRU вытеснением |
| `PageSize` | uint | Нет | 100 | 1..1000 элементов; при необходимости уменьшается до предела API |
| `MaxItems` | uint | Нет | 1000 | >0; общий предел одной collection или одной inventory branch |
| `MaxPages` | uint | Нет | 100 | >0; общий предел одной collection или branch |
| `MaxJSONBytes` | uint64 | Нет | 16 MiB | >0; предел распакованного structured response, без transfer streams |
| `WaitTimeout` | duration | Нет | 10 min | Общий бюджет Wait; >0 |
| `PollInterval` | duration | Нет | 2 s | Интервал после завершения предыдущего status read; >0 |
| `MaxMetricSamples` | uint | Нет | 10000 | >0; предел возвращаемых samples |
| `Observer` | optional Observer | Нет | Отсутствует | Только [разрешённые метаданные](execution.md#наблюдаемость) |

Для вызова deadline равен минимуму `now + соответствующий timeout` и deadline
переданного context. Отсутствие deadline у context не отключает timeout SDK.
Timeout не продлевается при retries или refresh. Превышение лимита ответа
возвращает `response_limit`; данные, полученные до лимита в Collect, сохраняются.
Отмена context прерывает локальные I/O и ожидания по [operations.md](operations.md#отмена).

`PageOptions` может уменьшать или увеличивать MaxItems/MaxPages/PageSize для
одного вызова в указанных диапазонах; нулевой параметр означает default.
Изменение options не меняет shared Config или созданные ранее итераторы.

Нулевое значение optional числовой/duration настройки Go Config означает
отсутствие значения и применение default. RefreshSkew использует указатель:
nil означает default, указатель на 0 явно отключает упреждающее обновление.
