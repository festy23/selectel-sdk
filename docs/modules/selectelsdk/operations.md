# Операции и ожидание

## Handle и Resume

Handle связывает Receipt, исходный scope/endpoint, credential runtime и
конкретный wait/recovery contract. Он не содержит mutable default profile.
Новый view или profile не меняет существующий Handle.

Client.Resource(ref) создаёт Handle наблюдения существующего ресурса без Receipt.
Он применяет шаги 2–4 Resume к ref и его capability generation; state read и
goal берутся из resource contract. Malformed ref возвращает invalid_argument.
Такой Handle позволяет читать/ожидать состояние и не заявляет факт принятия
какого-либо изменения. Его WaitResult.Receipt отсутствует; Recover возвращает
recovery_unavailable без I/O.

Ресурсы: Receipt, catalog, Client.Config. Ошибки: `invalid_argument`,
`scope_mismatch`, `capability_unavailable`, `endpoint_unavailable`.

1. Resume проверяет schema_version=1, обязательные receipt поля, operation ID
   и contract_revision. Malformed receipt возвращает `invalid_argument`;
   неизвестный или несовместимый contract — `capability_unavailable`.
2. Проверить совпадение AccountID с Client и структурную согласованность refs,
   parents и endpoint scope. Несовпадение возвращает `scope_mismatch`.
   Новая credential revision того же source допустима; явно переданный новый
   source того же account допустим после обычной auth проверки при первом I/O.
3. Проверить endpoint URL по access rule. Endpoint должен оставаться
   документированным либо иметь явно настроенный совместимый override.
   Отсутствие такой binding возвращает `endpoint_unavailable`.
   Для endpoint из IAM catalog Resume проверяет наличие совместимого recipe
   и структуру сохранённого endpoint без I/O. Его совпадение с текущим catalog
   проверяется при первом Status до отправки resource request; несовпадение
   возвращает `endpoint_unavailable` и не меняет адрес сохранённого ref.
4. Создать Handle без I/O. SDK не отправляет сохранённый request повторно.
   Accepted/rejected/not_sent Receipt без resource/task ref допускает receipt-only
   Handle после schema/account/revision/mutation effect/scope validation; endpoint
   необязателен. Recover conclusive без I/O, Status и Wait unavailable без цели
   наблюдения. Unknown без exact ref остаётся recovery_unavailable.

## Status

Вход: context и Handle. Результат Observation содержит resource/provider task
ref, ProviderState, classification `pending/succeeded/failed/unknown/absent`,
observed_at и Metadata; classification определяется контрактом этой операции.
Ресурсы: status read binding capability, refs, execution runtime.
Ошибки: ошибки execution, `capability_unavailable`, `scope_mismatch`.

1. Выбрать exact provider_operation_ref, если status contract использует task,
   иначе exact resource ref с требуемым parent. При отсутствии требуемой ссылки
   вернуть `capability_unavailable`; поиск по имени отсутствует.
2. Вызвать status binding через [execution](execution.md#контракт-вызова)
   с ID/parent/scope/endpoint из Handle. Auth проверяется для исходного scope.
   Ошибка чтения возвращается без изменения outcome Receipt.
3. Decode state поля и применить точную state map capability. Неизвестную строку
   сохранить как ProviderState и classification=unknown. Вернуть Observation.
   404 передаётся как not_found, если контракт Status не определяет absent.

## Wait

Вход: context, Handle, WaitGoal ID из capability. Выход WaitResult содержит
исходный Receipt, goal, последнюю Observation при наличии и completed_at.
Ресурсы: status binding, state map, CallTimeout, WaitTimeout, PollInterval.
Ошибки: ошибки Status, `invalid_argument`, `capability_unavailable`,
`unknown_outcome`, `operation_failed`, `state_unknown`, `deadline_exceeded`,
`canceled`.

1. Начать общий wait budget по settings и context. Проверить goal и необходимые refs. Неизвестный goal возвращает
   `capability_unavailable`; malformed goal — `invalid_argument`.
   Для Handle без Receipt перейти к шагу 2. Для unknown outcome сначала
   выполнить Recover в оставшемся wait budget. Его read/protocol ошибка
   возвращается с исходным Receipt. По результату заново классифицировать
   outcome: accepted продолжает шаг 2; rejected/not_sent возвращает
   capability_unavailable с reason `mutation_not_accepted`; unknown возвращает
   unknown_outcome/recovery_unavailable с полученными evidence.
   Для исходного rejected/not_sent также вернуть capability_unavailable
   с reason `mutation_not_accepted`.
2. Использовать исходный wait deadline; не менять его между polls.
   Первый status read выполнить немедленно. Для каждого чтения budget равен
   минимуму оставшегося wait budget и CallTimeout.
3. Если status read подтвердил success goal, вернуть WaitResult success.
   Goal удаления считается выполненным по 404 только для exact ref и
   endpoint, когда это явно задано wait contract. 403, network error и
   недоступный endpoint не являются подтверждением удаления.
4. Документированное failure state возвращает operation_failed; unknown state
   возвращает state_unknown. Прочая неуспешная status read после внутренних
   bounded retries возвращает исходную ошибку; Receipt accepted сохраняется.
5. Для pending state ждать PollInterval после завершения status read и
   перейти к шагу 2. Context cancel/deadline немедленно прекращает ожидание
   и возвращает соответствующую ошибку с Receipt и последней Observation.

Wait не запускает изменение ресурса и не объявляет приложение здоровым.
CreateAndWait выполняет Create один раз, проверяет accepted/ref и применяет
этот алгоритм. Ошибка ожидания не изменяет факт принятия Create.

## Recover

Вход: context и Handle. RecoveryResult содержит Receipt, optional Observation,
`ProofStatus=conclusive/indeterminate` и optional ProviderProofID.
Ресурсы: recovery contract capability, refs/correlation, execution runtime.
Ошибки: ошибки execution, `recovery_unavailable`, `unknown_outcome`,
`protocol_error`.

1. Для accepted/rejected/not_sent вернуть текущий Receipt и conclusive
   без I/O; статус завершения ресурса этим не определяется.
2. Для unknown без документированного recovery binding либо без требуемого
   exact key/ref вернуть recovery_unavailable, сохранив unknown.
   Если recovery=resource, перейти к шагу 5; если recovery=provider_operation
   или correlation, перейти к шагу 3.
3. Выполнить только документированный read lookup по provider operation ID
   или фактически отправленной correlation. Request формируется из исходных
   scope/endpoint/key/ref, timeout/retries применяются как для safe read.
   Ошибка возвращается с исходным Receipt.
4. Если ответ однозначно связывает call с provider acceptance/rejection,
   обновить outcome, refs и ProviderProofID по ответу; вернуть conclusive.
   Несколько кандидатов, нарушенная schema или несовпадение binding возвращают
   protocol_error; ничего не выбирать по newest/первому имени.
5. Для resource-only recovery вызвать Status exact ref и вернуть Observation,
   indeterminate и unknown_outcome. Для документированного absent/404 также
   вернуть Observation.classification=absent и unknown_outcome; остальные
   read errors вернуть без изменения Receipt. Outcome остаётся unknown:
   Recover без входного goal не выбирает желаемое состояние за caller.
   Отсутствующий correlation результат сам по себе не доказывает rejected,
   если API не гарантирует полноту/финальность этого lookup.

Если lookup шага 3 не даёт окончательного proof и resource observation
недоступна, вернуть indeterminate/unknown_outcome без Observation. Если exact
resource observation доступна, перейти к шагу 5. Наблюдение ресурса не
доказывает авторство исходного изменения.

SDK не повторяет create, не сравнивает «ресурсы до/после» и не выбирает
последний task для разрешения unknown outcome.

## Отмена

Отмена context прекращает локальный call/stream/polling. Принятый облачный
запрос продолжает выполняться. Caller получает Receipt accepted либо unknown
согласно execution, а не обещание отмены облачной операции.

Remote abort/cancel предоставляется только отдельным typed методом, если
он документирован для данного сервиса. Это самостоятельная mutation с Receipt,
собственным outcome и точным resource/task ID; context cancel не вызывает её.
Прерванный multipart transfer сохраняет известный upload ID. AbortMultipart
и удаление частично созданного ресурса требуют явного вызова caller.
