# Кэши

SDK использует только in-memory auth cache. Resource status, billing, logs,
metrics, secrets, страницы результатов и receipts не кэшируются между вызовами.
Встроенные contracts/location maps неизменяемы и не являются runtime-кэшем.

## Авторизация и service catalog

| Свойство | Контракт |
|---|---|
| Тип | In-memory, один экземпляр на runtime клиента |
| Назначение | Повторное использование IAM-токена и полученного с ним каталога |
| Ключ | `(CredentialSource.Identity, CredentialSource.Revision, AccountID, scope_kind, ProjectID)`; пустой ProjectID только для account scope |
| Значение | Protected token, expires_at, подтверждённая область, subject ID, catalog snapshot, acquired_at |
| Время жизни | До `now >= expires_at`; после `now >= expires_at - RefreshSkew` требуется refresh для нового вызова |
| Размер | До AuthCacheEntries записей, LRU по последнему использованию |
| Формирование | Только успешная [авторизация](access.md#авторизация); token и catalog заменяются атомарно |
| Инвалидация | 401 для этой token generation, смена credential revision, expiration, LRU eviction или закрытие runtime |

Инвариант: token и catalog одного account/project не применяются к другому
scope или credential revision. Отдельный account token не заменяет project token.
AccountID в Config не выводится из первого доступного проекта.

Одновременные refresh одного ключа объединяются одной cancellable flight.
Другие ключи обновляются независимо. Flight использует deadline/context её
инициатора; отмена ожидающего caller прекращает только его ожидание. Отмена
инициатора прекращает HTTP auth; участники flight получают его ошибку, если
их contexts ещё действуют. Новый вызов может инициировать новый refresh.
Failed refresh не сохраняется и не создаёт отрицательный кэш.

Запись заменяется только если её credential revision всё ещё совпадает с
revision текущего lookup. 401 удаляет только generation токена использованного
запроса; уже обновлённый токен другой concurrent flight не удаляется.
Просроченный или refresh-required token не используется после неудачного refresh.
SDK не сохраняет token, catalog или credentials на диск и не обещает физическое
обнуление всех копий памяти Go runtime.
