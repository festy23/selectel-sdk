# Квоты и лимиты

`client.Project(id).Location(region, pool).Quotas().V1` предоставляет три операции
[Quota Management](https://docs.selectel.ru/api/quota-management/):
`Get`, `Limits` и `Update`. Публичная схема закреплена в
[catalog/sources/quota-management.json](../../../catalog/sources/quota-management.json).
Генерация работает без сети. Адрес выбирается только из опубликованной
[таблицы API](https://docs.selectel.ru/api/urls/) либо exact scoped endpoint override.
Для пула без опубликованного адреса вызов завершается до HTTP с
`endpoint_unavailable`; SDK не вычисляет hostname по имени пула.

## Область и авторизация

Проект в URL всегда берётся из неизменяемого scoped view. Регион и пул выбираются
явно. По умолчанию используется IAM project token. Источник также разрешает
`CallOptions{AuthMode: types.IAMAccount}`: авторизация запрашивает account token,
но target project, location, metadata и refs остаются исходными. SDK не меняет
авторизацию после `403` и не выбирает другой проект.

`Filters.Resources` передаётся повторяющимся query-параметром `resource`.
Отсутствие фильтра оставляет выбор API; пустое имя ресурса недопустимо.
`UpdateRequest.Quotas` — map имени ресурса в список `QuotaSet{Value, Zone}`.
Пустые map/списки/имена и отсутствующее value отклоняются до credentials lookup.
Нулевое value передаётся явно. Источник не задаёт диапазонов чисел; SDK не
придумывает ограничения или семантику отрицательных sentinel values.
Все `used`, `value` и provider error codes сохраняются через `types.Integer`.

## Частичные ответы и исход изменений

`Get` и `Update` принимают HTTP 200 и 207; `Limits` принимает только 200.
207 всегда возвращает `incomplete` вместе с доступными данными.
Наличие `error` или непустого `errors` также означает неполный результат.
Сохраняются точные successful resource/zone values и все resource error codes,
resource и zone. Error-only ответ сохраняет ошибки и пустую map successful quotas.
Поддерживаются nested `common.Error.errors` из публичной схемы и flat `errors`
из [закреплённого официального SDK](https://github.com/selectel/go-selvpcclient/blob/79b0d0ff1d46ebfe7c50056f8c829e0d32349605/selvpcclient/quotamanager/quotas/schemas.go);
SDK-вариант также сохраняет optional returned used counters у PATCH/limits;
request остаётся zone/value. Эти варианты описаны отдельным generator-owned overlay и не создаёт новых
remote operations. Все message-поля этих DTO — `ProtectedValue`.

PATCH сохраняет receipt при ошибках. Для 200/207 его outcome — `accepted`, что
подтверждает приём запроса, но не применение всех изменений. Точный target ref
`project_quotas` сохраняет account/project/location/endpoint/generation.
После транспортной потери или неподтверждённого ошибочного ответа outcome —
`unknown`; PATCH не повторяется. Для документированного 400 и обычных 401/403
outcome — `rejected`. Повторное точное `Get` наблюдает текущее состояние, но не
доказывает авторство неизвестного изменения. `Wait`, root `Resource`/`Resume`
handle и автоматическое восстановление для quota ref не предоставлены.

403 возвращает `permission_denied`, 429 — `rate_limited` при чтении. Для PATCH с
неизвестным исходом наружная ошибка — `unknown_outcome`, исходный код сохраняется
в `CauseCode`. Публичный источник не определяет numeric code для отдельного
`quota_exceeded`: SDK не классифицирует protected provider text или regex.
В частичном результате вызывающий код получает точные числовые provider codes.
HTTP error payloads не логируются и не сохраняются как открытый текст.

Ответ — одна map, без remote pagination; размер ограничен `MaxJSONBytes`, весь
вызов — `CallTimeout`. Неполный ответ не превращается в успешную коллекцию.

## Отдельный Resell binding

Account-scoped `Account().Projects().Get(exactProjectRef, GetOptions{}, ...)`
сохраняет существующий Resell v2 endpoint и binding
`projects.v2.projects.get`. Его `DetailedProject.Quotas` содержит region, zone,
value и used. `ExcludeQuotas: false` передаётся явно; quota values сохраняют
точность. Этот binding не подменяется regional Quota Management.
Текущий официальный SDK оставляет `resell/v2/quotas` только как deprecated DTO;
публично подтверждённого отдельного Resell quota mutation endpoint нет.
Наличие DTO не считается дополнительной remote capability.

## Проверенность

Публичные offline transport tests покрывают все три операции, обе разрешённые
IAM scope, source-only PATCH JSON, mixed 207, protected errors, точные числа,
validation, неизвестный исход без replay, 403/429 и независимый Resell read.
Live проверки и реальные облачные изменения не выполнялись. Полнота общего
source inventory остаётся неизвестной, `coverage_ratio` остаётся `null`.
