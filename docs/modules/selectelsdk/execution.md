# Выполнение вызовов

## Контракт вызова

Каждая capability связывает типизированный request с полной схемой wire API:
path/query/header/body поля, successful statuses, response schema и provider
errors. SDK-owned scope и auth поля формируются ядром. Остальные поля
переносятся из request по binding операции без неявных business defaults.
Native builder допускается только с тем же mapping и отключёнными повторами.
Secrets раскрываются только при формировании wire body/signature.

Ресурсы: Capability, request DTO, scope, CredentialSource, auth cache,
EndpointBinding, HTTP transport, settings и Observer.
Вход: context, operation ID, типизированный request, optional CallOptions.
Выход: Result с typed data, Metadata и Receipt для изменения.
Сетевые и process-local действия не образуют общую транзакцию.
Receipt отражает подтверждённый исход конкретного запроса.

### Алгоритм работы

До validation установить обязательный finalizer шага 8. Любой успешный,
ошибочный или ранний возврат шагов 1–7 проходит через него; шаг 8 не выполняет
нового I/O и использует только значения, уже полученные в текущей ветви.

1. Создать call budget из context и CallTimeout/TransferTimeout. Для mutation
   создать Receipt `not_sent` по таблице ниже. Проверить request по schema,
   scope, refs и capability binding. Ошибка возвращает `invalid_argument`,
   `scope_mismatch` или `capability_unavailable`; mutation receipt сохраняется.
2. Выполнить [auth](access.md#авторизация) и
   [endpoint resolution](access.md#разрешение-endpoint). Ошибка завершает вызов
   с исходом `not_sent`; ни один ресурсный запрос не отправлен.
3. Сформировать wire request: method/path из binding, scope и escaped refs
   в отведённые path/query/body поля, фильтры/options из request, auth scheme
   и headers из material. Установить только документированные protocol
   headers, включая microversion. Collision SDK-owned поля возвращает
   `scope_mismatch`. Сформировать безопасные observer metadata.
4. Выполнить request в оставшемся budget. Вложенные transport/SDK retry и
   автоматический reauth отключены. Попытка учитывается до передачи transport.
   Если transport подтверждает отсутствие отправки байтов resource request,
   mutation остаётся `not_sent`; если этого подтверждения нет, считать
   возможной передачу запроса. Сам факт timeout не доказывает отсутствие записи.
5. При successful status capability установить mutation `accepted` прежде
   декодирования result. Decode valid body по схеме, выделить exact resource/
   operation IDs и ProtectedValue поля; заполнить result и вернуть успех.
   Ошибка decode, oversized response или отсутствующий обязательный ID
   возвращает `protocol_error`/`response_limit` вместе с `accepted` Receipt
   и уже достоверно извлечёнными refs. Потеря тела не превращает 2xx в rejected.
6. При non-success status применить [таблицу ошибок](#ошибки).
   Для mutation только документированное подтверждение неприменения запроса
   устанавливает `rejected`. 5xx, разрыв соединения, timeout, cancel после
   возможной отправки и неопределённая семантика статуса устанавливают `unknown`.
   Отмена до отправки сохраняет `not_sent`. SDK не создаёт вымышленных IDs.
7. Для safe read применить [правило повторов](#повторные-попытки).
   Если replay разрешён и budget достаточен, перейти к шагу 4; после auth
   refresh пересобрать только auth headers. Для mutation replay допускается
   исключительно по документированной policy; при отсутствии policy завершить
   вызов с сохранённым Receipt. Неуспешный request не запускает rollback,
   компенсацию или удаление других ресурсов.
8. Зафиксировать completed_at/attempts/request IDs, передать Observer только
   разрешённые поля и вернуть Result/error. Body ошибки не добавляется в
   error message. Если outcome mutation unknown, внешняя ошибка имеет code
   `unknown_outcome`; underlying classification сохраняется в CauseCode.

## Повторные попытки

Safe read задаётся семантикой capability, а не HTTP verb. POST запроса
аудит-логов является safe read; произвольный GET с side effect им не является.

| Событие safe read | Поведение |
|---|---|
| 429, 408, 500, 502, 503, 504 | Retry в пределах ReadMaxAttempts и исходного budget |
| Временная network ошибка | Retry, если context действует |
| 401 | Один [auth refresh](access.md#повтор-после-401), входит в общий attempts limit |
| 400, 403, 404, 409, 422 | Возврат соответствующей ошибки без retry |
| Decode/schema/TLS/certificate/redirect ошибка | Возврат ошибки без retry |
| Context cancel или budget exhaustion | Немедленное завершение |

Перед попыткой номер `n+1`, после неуспешной попытки `n`, задержка выбирается
равномерно в `[0, min(RetryMaxDelay, RetryBaseDelay * 2^(n-1))]`.
При валидном Retry-After используется не меньшая серверной задержка; она не
обрезается до RetryMaxDelay. Если delay >= оставшегося budget, запрос не
повторяется и возвращается исходная ошибка с reason `retry_budget_exhausted`.
Malformed Retry-After игнорируется. Sleep отменяется context.

Mutation имеет одну попытку по умолчанию, включая Delete и PUT. Capability
может разрешить replay только с источником, доказывающим идемпотентность:
provider idempotency key/условие, точный период гарантии, область ключа и
результат повторного запроса. Один и тот же request/key сохраняется при replay;
локальный CallID не становится provider key. Без такого контракта HTTP method,
совпадение имени или наличие Terraform retry не разрешают replay.

## Receipt

JSON receipt имеет schema version 1. Сериализация содержит только перечисленные
поля. Пользовательские descriptions/tags, request bodies, credentials,
secret/log/object payloads и signed URLs в Receipt отсутствуют.

| Поле | Правило формирования |
|---|---|
| `schema_version` | 1 |
| `call_id` | Локальный случайный UUID до resource request; не provider operation ID |
| `operation_id` | ID capability |
| `contract_revision` | Hash operation contract из каталога |
| `scope` | Копия исходного Scope |
| `credential_identity` | Нечувствительный Identity после успешного Lookup; до него отсутствует; secret/revision не сохраняется |
| `endpoint` | EndpointBinding без credentials; отсутствует до resolution |
| `outcome` | `not_sent`, `rejected`, `accepted`, `unknown` по алгоритму |
| `submitted_at` | Начало передачи transport; отсутствует, если transport не вызван |
| `completed_at` | Время возврата текущего вызова |
| `attempts` | Число resource transport attempts; auth attempts не входят |
| `request_ids` | Provider request IDs из документированных headers/response fields, в порядке попыток |
| `resource_refs` | Только exact IDs из request адресуемого существующего ресурса или подтверждённого response; одинаковые refs объединяются с сохранением первого появления |
| `provider_operation_ref` | Только operation/task ID, доказанно связанный с этим запросом; иначе отсутствует |
| `recovery` | `resource`, `provider_operation`, `correlation` или `unavailable` по capability и фактическим refs |
| `correlation` | Только документированный нечувствительный provider key, если реально отправлен; иначе отсутствует |

Для update/delete ref из request не подтверждает применение изменения.
При success без async ID отсутствие provider_operation_ref является нормальным.
Для unknown без refs и correlation recovery равен `unavailable`.
SDK возвращает receipt в памяти; crash до его сохранения caller не покрывается
гарантией восстановления. SDK не заявляет exactly-once delivery.

## Ошибки

SDKError поддерживает `errors.Is/As`. Его поля: Code, CauseCode при наличии,
OperationID, Scope, EndpointID, optional HTTPStatus/ProviderCode, RequestIDs,
optional Receipt, RetryableRead, безопасный Message и внутренний Cause.
`Error`, String, JSON и formatting не раскрывают Cause или raw provider message.
Доступ к исходной transport error через Unwrap предназначен для программного
сопоставления; credentials и raw response не включаются в созданный SDK cause.

| Code | Message | Условие |
|---|---|---|
| `invalid_argument` | Invalid argument | Ошибка локальной schema/range/context validation |
| `scope_mismatch` | Scope mismatch | Конфликт account/project/ref/endpoint binding |
| `credentials_unavailable` | Credentials unavailable | Отсутствующий либо недоступный material/source |
| `unauthenticated` | Authentication failed | 401 или expired external token |
| `permission_denied` | Permission denied | 403; не превращается в not_found |
| `not_found` | Resource not found | 404 либо другой статус с документированной семантикой отсутствия в error binding операции; кроме специального success правила delete wait |
| `conflict` | Resource conflict | 409 |
| `rate_limited` | Rate limited | 429 |
| `capability_unavailable` | Capability unavailable | Неизвестная/не реализованная операция или wait/recovery goal |
| `endpoint_unavailable` | Endpoint unavailable | Нет однозначного доступного endpoint |
| `provider_error` | Provider request failed | Остальной non-success HTTP status; 400/422 включают safe field violations без исходных values |
| `transport_error` | Transport failed | Network/TLS ошибка до подтверждённого результата |
| `protocol_error` | Invalid provider response | Нарушение response schema, redirect или cursor protocol |
| `response_limit` | Response limit exceeded | Превышен предел body/samples |
| `incomplete` | Result incomplete | Collection ограничена или одна inventory branch не завершена |
| `deadline_exceeded` | Deadline exceeded | Истёк context/SDK budget |
| `canceled` | Call canceled | Context отменён |
| `unknown_outcome` | Mutation outcome unknown | Изменение могло примениться, подтверждения нет |
| `recovery_unavailable` | Recovery unavailable | Нет доказуемого механизма восстановления |
| `operation_failed` | Operation failed | Документированное terminal failure |
| `state_unknown` | Provider state unknown | Wait встретил неописанное состояние |

ProviderCode и request IDs извлекаются только из allowlisted protocol fields;
Message не копируется из error body. Error cause одного inventory branch
сохраняется отдельно от общего `incomplete`.
Документированное status/code mapping конкретного error binding применяется
перед общей таблицей; оно не переносится на другие операции того же сервиса.

## Чувствительные значения

ProtectedValue хранит bytes без публичного поля. `String`, Go formatting,
GoString и MarshalJSON возвращают только `[REDACTED]`. Явный
`Reveal()` возвращает отдельную копию bytes; caller отвечает за её хранение
и использование. Для stream `Read`/`CopyTo` раскрывает данные только явно
переданному writer. Бинарные transfer не буферизуются целиком.

Schemaless provider JSON представлен `ProtectedJSON`: он сохраняет исходные
JSON bytes без преобразования чисел, использует redaction ProtectedValue и
раскрывается через явный Reveal. Чтение JSON string в ProtectedValue сохраняет
его декодированные bytes. Сериализованная redaction не восстанавливает
исходное чувствительное значение. Inventory исключает защищённые payloads
из своих Items; standalone service read сохраняет их для явного доступа.

Native result с secret fields не возвращается напрямую. Public wrapper
сохраняет нечувствительные typed fields, чувствительные заменяет ProtectedValue.
Внешние native input DTO могут содержать plain values; SDK не форматирует
и не журналирует их. Автоматическое выполнение инструкций из logs, labels,
descriptions и secret content отсутствует.

## Наблюдаемость

Observer получает только operation ID, scope, generation, endpoint ID,
attempt, latency, status/code, request IDs, outcome, page/item counts и
complete. Resource names, tags, filters, URL query, headers, tokens, bodies
и provider messages не передаются. SDK не пишет stdout/stderr и файлы логов.
Observer вызывается синхронно; callback не изменяет результат облачной операции.
Вызывающая сторона отвечает за неблокирующий callback; его panic перехватывается
и не меняет Receipt или result.
