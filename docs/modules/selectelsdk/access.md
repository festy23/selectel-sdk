# Доступ и endpoints

## Источник credentials

`CredentialSource.Lookup(ctx, AuthRequest)` возвращает один snapshot доступа
или SDKError. AuthRequest содержит Scope, Mode, Service и Audience по
[interface.md](interface.md#общие-типы). Material содержит непрозрачные `Identity` и
`Revision`, binding, scheme и защищённые значения credentials.
Identity и Revision являются нечувствительными идентификаторами; при ротации
секрета Revision меняется. Source, переданный в New, обязателен.

Поддерживаются материалы IAM service-user password, externally issued IAM
token с expires_at и binding, static token, S3 access/secret key, BasicAuth и отдельный Bearer API key.
Данные одного auth scheme не подставляются вместо другого.

Loaders вызываются явно до New: `FromEnvironment(prefix)`,
`FromProfile(path, name)` и `FromKeyring(store, entry)`. Каждый loader
возвращает CredentialSource либо `credentials_unavailable`. Автоматического
поиска файлов, активного профиля и env override в ядре нет.

Environment loader читает только имена с переданным непустым prefix:
`ACCOUNT_ID`, `USERNAME`, `PASSWORD`, `PROJECT_ID`, `IAM_TOKEN`,
`TOKEN_EXPIRES_AT`, `STATIC_TOKEN`, `BEARER_TOKEN`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`,
`BASIC_USER`, `BASIC_PASSWORD`. Суффикс добавляется к prefix без изменения.
Profile loader читает только указанную запись JSON с теми же полями в
snake_case; Keyring loader читает эту же структуру из указанной записи store.
AccountID материала должен совпадать с Config.AccountID. Частично заданная
пара credentials завершает lookup `credentials_unavailable`.
Loader не пишет в исходное хранилище и не выводит его содержимое.
Profile JSON имеет корневой object `profile name → record`. Source читает
указанную запись при Lookup с его context; constructors loaders не обращаются
к облаку и не выбирают active profile.
Для IAM loader требует ровно один вид материала: username/password либо
iam_token/token_expires_at; одновременное наличие обоих возвращает
credentials_unavailable. Для остальных modes loader выбирает только
соответствующую пару/token из указанной записи. Token expiry парсится как RFC3339;
неверный формат возвращает credentials_unavailable. Source snapshot для
IAM password может быть account-bound с отсутствующим project binding:
права на requested project проверяет Keystone. Для external project token
точный project binding обязателен.

| Scheme | Поля material | Установка в запрос |
|---|---|---|
| `iam_password` | account, username, password | Выдача account/project IAM через Keystone |
| `iam_token` | protected token, expires_at, scope binding | `X-Auth-Token` |
| `static_token` | protected token, account binding | `X-Token` |
| `s3_sigv4` | access key, secret key, optional session token, binding | SigV4 signing по контракту S3/Logs |
| `basic` | user ID, password, binding | Basic authorization сервиса Метрики |
| `bearer_token` | protected token, exact account/project binding | `Authorization: Bearer` только AI Router, Dedicated Inference, Token Factory |

External token, static/S3/Basic binding является утверждением источника.
SDK помечает его `caller_asserted`, если конкретный API не предоставляет
документированный способ подтверждения. SDK не заявляет проверку project
membership по непрозрачному ключу. Scope из ответа Keystone получает
`provider_verified` после проверки ниже.

Bearer использует `protocol_specific` только для `ai_router`,
`dedicated_inference`, `token_factory`. Другие protocol-specific adapters
остаются недоступными до lookup. Static/IAM token не подменяют API key.
Optional `Material.ExpiresAt` для Bearer проверяется локально; истёкший key
возвращает `unauthenticated` без HTTP. Binding остаётся `caller_asserted`: auth
не подтверждает доступ к модели или preview. Loader выбирает только явно
заданный `bearer_token` / `BEARER_TOKEN`; права не выводятся из других полей.

## Авторизация

Методы: `Authenticate(ctx, AuthRequest)` и внутренний auth этап сервисного вызова.
Ресурсы: CredentialSource, auth cache, Keystone, settings, contract.auth.
Вход: context и AuthRequest. Внутренний вызов формирует его из scope и
preferred/явно выбранного auth contract capability. Standalone caller
передаёт эти поля явно; default scheme не угадывается.
Выход: scope, verification, scheme, subject_id при наличии, expires_at при
наличии и acquired_at. Credentials и token в результат не включаются.

Ошибки: `invalid_argument`, `credentials_unavailable`, `scope_mismatch`,
`unauthenticated`, `permission_denied`, `transport_error`, `provider_error`,
`protocol_error`, `response_limit`, `deadline_exceeded`, `canceled`.
Недоступный документированный protocol-specific auth возвращает
capability_unavailable до I/O.
HTTP ошибки отображаются по [execution.md](execution.md#ошибки).

1. Проверить context, scope, Mode, Service и Audience согласно
   [interface.md](interface.md#общие-типы) и auth contract сервиса.
   Некорректный вход возвращает `invalid_argument`; уже завершённый context —
   `canceled` или `deadline_exceeded` без I/O.
2. В пределах call budget вызвать CredentialSource.Lookup с AuthRequest.
   Ошибка source возвращает `credentials_unavailable` без раскрытия underlying
   message; отмена возвращает соответствующую context error.
3. Проверить account и binding snapshot. Другой account/project/audience
   возвращает `scope_mismatch`; отсутствующий материал выбранного scheme —
   `credentials_unavailable`. SDK не пробует другое право или аккаунт.
   iam_account требует account scope, iam_project — project scope; material
   для этих modes имеет scheme iam_password либо iam_token. Другой material
   scheme возвращает credentials_unavailable. Для iam_password отсутствующий
   project binding допускается; противоречащий заданный binding запрещён.
4. Для static/S3/Basic вернуть binding metadata и material внутреннему
   адаптеру. Сетевой auth вызов отсутствует. Для external IAM token проверить
   `now < expires_at`; иначе вернуть `unauthenticated`. При действующем token
   перейти к шагу 5. External source отвечает за выдачу новой revision;
   SDK не продлевает срок строкового token.
5. Для IAM password/token прочитать кэш по ключу из [caches.md](caches.md).
   Если `now < expires_at - RefreshSkew`, использовать запись и вернуть её
   metadata. Иначе присоединиться к flight ключа или создать её.
   После успешной чужой flight использовать полученную запись. Ошибка flight
   завершает текущий вызов; context caller прекращает его ожидание.
6. Инициатор flight для IAM password отправляет POST
   `https://cloud.api.selcloud.ru/identity/v3/auth/tokens` один раз в оставшемся
   call budget по таблице ниже. Для external IAM token выполняет GET того же
   URL без body/query с X-Auth-Token и X-Subject-Token равными material.token;
   это introspection собственного token и получение service catalog.
   Автоматический retry этого auth этапа отсутствует. Отправка и ответ
   не наблюдаются через payload logging.

   | Поле запроса | Источник |
   |---|---|
   | `Content-Type` | `application/json` |
   | `auth.identity.methods` | `["password"]` |
   | `auth.identity.password.user.name` | Snapshot.username |
   | `auth.identity.password.user.domain.name` | Scope.AccountID |
   | `auth.identity.password.user.password` | Раскрытый внутри transport ProtectedValue |
   | `auth.scope.domain.name` | AccountID только при отсутствии ProjectID |
   | `auth.scope.project.id` | ProjectID только для project scope |

7. При HTTP 201 password auth прочитать непустой X-Subject-Token; при HTTP 200
   external token introspection использовать исходный material.token.
   В обоих случаях прочитать token.expires_at,
   token.domain для account scope либо token.project и его domain для project
   scope, optional token.user.id и token.catalog. `expires_at <= now`,
   отсутствие обязательного поля или неверный JSON возвращают `protocol_error`.
   Project ID и domain account name должны совпасть со scope; несовпадение
   возвращает `scope_mismatch`. Для account token сравнить domain.name с
   AccountID. Каталог без нужного сервиса допустим до endpoint resolution.
8. Для external token effective expires_at равен минимуму provider expiry
   и material expiry; для password — provider expiry. После ответа заново
   сравнить effective expires_at с текущим now. Если effective expires_at <= now,
   вернуть unauthenticated без записи кэша и без material для resource request.
   Иначе атомарно сохранить
   проверенный token и catalog по [caches.md](caches.md),
   установить `provider_verified`, вернуть metadata. При иной HTTP/network
   ошибке применить таблицу ошибок execution; failed flight завершается без
   записи кэша. Повторной выдачи token в том же auth вызове нет.

Способ выдачи token определяется
[Selectel auth](https://docs.selectel.ru/api/authorization/);
адресация project по ID определяется
[Keystone v3](https://docs.openstack.org/api-ref/identity/v3/#password-authentication-with-scoped-authorization).
External-token introspection использует
[Keystone token validation](https://docs.openstack.org/api-ref/identity/v3/#validate-and-show-information-for-token),
тот же источник catalog, что
[официальный SDK](https://github.com/selectel/go-selvpcclient/blob/v5.0.0/selvpcclient/clients/services/catalog.go).
Если introspection запрещена, вернуть permission_denied; SDK не расширяет роль.
Refresh не означает изменение ролей, пользователя или выбранного scope.

## Разрешение endpoint

Вход: Capability, scope, catalog snapshot и EndpointOverrides.
Выход: EndpointBinding с ID, HTTPS URL, generation, scope и auth audience.
Ресурсы: token.catalog, встроенный location registry из
[Selectel URL](https://docs.selectel.ru/api/urls/), Config.
Ошибки: `invalid_argument`, `scope_mismatch`, `endpoint_unavailable`,
`capability_unavailable`, `protocol_error`.

1. Проверить требуемые scope поля capability. Отсутствующий project/region/pool
   возвращает `invalid_argument`. `discovery_pending` или операция без
   implementation binding возвращает `capability_unavailable` до отправки.
2. Если задан override точного EndpointKey `(service, generation, account,
   project, region, pool)`, использовать его только при совпадении auth audience
   и scope binding. Несовпадение возвращает `scope_mismatch`.
3. Без override применить ровно recipe capability: выбрать catalog entries
   по service_type, generation, region/pool и `interface=public`, либо точную
   запись документированной URL map. Ноль совпадений возвращает
   `endpoint_unavailable`; несколько разных URL — `endpoint_unavailable`
   с reason `ambiguous`. Первый доступный регион не выбирается.
4. Проверить URL: scheme HTTPS, host непустой, userinfo/fragment отсутствуют,
   query отсутствует у base URL. Неверный URL возвращает `protocol_error`.
   Override также обязан использовать HTTPS; тестовый transport может
   обслуживать этот URL без реального сетевого соединения.
5. Проверить path variables recipe: значения происходят из выбранного scope
   и refs, кодируются как path segment ровно один раз. URL содержит wire
   generation, а не major Go-модуля. Неподдерживаемая location возвращает
   `endpoint_unavailable`.
6. Вернуть immutable EndpointBinding. Provider aliases фиксируются в URL map:
   имя pool не выводится из hostname. Endpoint, сохранённый в ResourceRef/
   Receipt, не заменяется endpoint другого региона при последующих вызовах.

TLS verification включена. Redirects не выполняются автоматически; даже
redirect внутри домена возвращает `protocol_error` с reason `redirect`.
Авторизация не переносится на Location URL. ProjectID в body/query/path
берётся из view; конфликтующее значение native opts возвращает `scope_mismatch`.

Location registry использует ключи регионов SDK из location tabs URL map:
`spb` — Санкт-Петербург, `msk` — Москва, `nsk` — Новосибирск,
`tas` — Ташкент, `alm` — Алматы, `nrb` — Найроби.
Связь pool → region берётся из документированной
[таблицы локаций](https://docs.selectel.ru/infrastructure/locations/).
Для известного pool конфликт с явно заданным region возвращает
`scope_mismatch`. Pool и его сегмент различаются; SDK не отбрасывает suffix
сегмента для выбора другого endpoint. Наличие pool в location registry
не подтверждает доступность каждого сервиса в этом pool.

## Повтор после 401

1. После 401 инвалидировать использованную generation auth cache.
2. Для safe read с доступной IAM password source выполнить один refresh,
   если общий attempt budget ещё не исчерпан; повтор read расходует ещё одну
   попытку. Второй 401 возвращает `unauthenticated`.
3. Для mutation refresh может подготовить следующий явный вызов, но текущая
   mutation не повторяется, кроме документированной безопасной replay policy
   этой capability. Источник external/static/S3/Basic перечитывается для
   следующего вызова; автоматическое переключение scheme отсутствует.
