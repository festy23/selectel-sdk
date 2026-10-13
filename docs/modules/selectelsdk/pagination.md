# Страницы и полнота

## Контракт

Ресурсы: capability.pagination, service List binding, auth/runtime,
[настройки](settings.md). Все List методы используют
[execution](execution.md#контракт-вызова); POST-read разрешён, если это задано
семантикой capability. Перечисление не изменяет облачные ресурсы.

| Поле | Контракт |
|---|---|
| `PageOptions.PageSize` | Максимум элементов запроса; 0 означает Config default |
| `PageOptions.MaxItems` | Предел выдачи одной collection; 0 означает default |
| `PageOptions.MaxPages` | Предел resource list requests одной collection; 0 означает default |
| `PageOptions.Cursor` | Optional cursor SDK, привязанный к operation/scope/endpoint/filters |
| `Page.Items` | Typed элементы в порядке wire response; без скрытой сортировки/дедупликации |
| `Page.NextCursor` | Отсутствует только при подтверждённом конце collection |
| `Page.Done` | Провайдер подтвердил конец и все элементы этой страницы выданы |
| `Page.Metadata` | Scope, endpoint, generation, observed_at, request IDs |
| `Page.Provider` | Documented pagination metadata: optional totalCount, protected prev_cursor, count |
| `Collection.Items` | Конкатенация уже выданных страниц в исходном порядке |
| `Collection.Complete` | true только после подтверждённого конца |
| `Collection.StopReason` | `end`, `max_items`, `max_pages`, `canceled`, `deadline`, `error` |
| `Collection.NextCursor` | Продолжение после выданных элементов, если доступно |
| `Collection.Pages` | Число list requests; auth/retry attempts не входят |
| `Collection.ProviderPages` | Metadata провайдера для прочитанных страниц в их порядке; отсутствие не заменяется вычисленным total |

`ListPage` выполняет один логический list request с retries ядра и возвращает
Page. `Iterate(request, options)` создаёт iterator без I/O. `Next(ctx)`
возвращает `(Page, error)` и читает не более одной страницы; после успешного
конца следующий Next возвращает `io.EOF` без I/O. `Collect(ctx, request, options)`
накапливает страницы до конца, лимита или ошибки. Memory iterator ограничена
одной decoded page; memory Collect ограничена MaxItems и пределом response.

Cursor содержит schema version 1, operation ID, contract revision, scope,
endpoint ID/generation, SHA-256 нормализованного semantic request и wire cursor.
В semantic request включаются filters и time bounds, исключаются auth и
total limits; PageSize включается только если wire contract связывает с ним cursor.
Для частично выданной страницы cursor сохраняет входной wire cursor и число
уже выданных элементов этой страницы. Credentials и payloads в cursor отсутствуют.
Sensitive provider cursor представлен ProtectedValue и требует явного export
для сохранения; безопасный JSON не раскрывает его.

Resume использует исходный request с тем же fingerprint. Продолжение mutable
collection не обещает snapshot consistency: consistency и stable ordering
берутся из API. Для `best_effort` ресурсы могут измениться между страницами.
SDK не объявляет полный снимок всех проектов/регионов по одной collection.

## Алгоритм ListPage / Next / Collect

Ошибки: ошибки execution, `invalid_argument`, `scope_mismatch`,
`capability_unavailable`, `protocol_error`, `incomplete`.

1. Проверить request, effective options и cursor. Неверные диапазоны/version
   возвращают `invalid_argument`; другой scope/endpoint/fingerprint —
   `scope_mismatch`; несовместимая contract revision — `capability_unavailable`.
   Для terminal iterator вернуть прежнюю ошибку или io.EOF без I/O.
   Для Collect начать общий budget CollectionTimeout, ограниченный context;
   все page calls получают его оставшуюся часть. Next использует свой context
   и CallTimeout; iterator не выполняет I/O между явными вызовами Next.
2. Для iterator/Collect определить remaining items и pages. Если лимит уже
   достигнут и подтверждённый конец не получен, закончить `incomplete`
   с соответствующим StopReason и сохранённым cursor. Иначе перейти к шагу 3.
3. Сформировать request List binding: semantic fields из исходного request,
   page size `min(effective PageSize, wire maximum, remaining items)` для API
   с регулируемым размером, wire cursor из cursor либо начальное значение
   контракта. Для фиксированного размера использовать wire размер контракта.
   Вызвать [execution](execution.md#контракт-вызова). При ошибке вернуть её,
   ранее выданные данные и Complete=false; iterator становится terminal.
4. Из response получить items и продолжение по capability.pagination.
   Конец определяется точно wire правилом; пустая страница с новым cursor
   не означает конец. Provider total count сохраняется только как metadata
   и не доказывает стабильность collection.
5. Если это replay частичной страницы, пропустить сохранённое число items.
   Если skip больше длины страницы, закончить `protocol_error` с reason
   `cursor_stale`. Иначе выдать оставшийся prefix до remaining items.
6. Если часть decoded page ещё не выдана, сохранить её входной cursor и новый
   skip. Если всё выдано, сохранить provider next cursor. Для contract-specific
   sentinel, включая unchanged forward token у GetLogEvents, применить
   задокументированное правило конца. Неизменный cursor без такого правила
   возвращает `protocol_error` с reason `cursor_no_progress`.
7. Если конец подтверждён и вся страница выдана, выставить Done=true,
   Collection.Complete=true и StopReason=end. Иначе Done=false. Достижение
   MaxItems/MaxPages возвращает полученную страницу и `incomplete`, сохраняет
   Complete=false и continuation; ошибку не заменять успешной полной выборкой.
8. ListPage возвращает страницу без требования обходить continuation.
   Next возвращает страницу и обновляет локальный cursor/counters.
   Collect добавляет Page.Items и повторяет шаг 2 до terminal result.

Request IDs и times всех страниц сохраняются как metadata, а сами данные
не попадают в Observer. SDK не выполняет параллельный обход страниц,
не угадывает missing cursor и не преобразует permission_denied в пустой список.
