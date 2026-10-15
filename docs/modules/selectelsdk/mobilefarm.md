# Мобильная ферма

Pinned [Mobile Farm JSON](https://docs.selectel.ru/api/mobile-farm.json), текущая
версия документа 1.1.0, содержит 30 method/path операций v3 плюс auth ticket v2.
Первоначальный inventory count 30 сохранился; полнота всего публичного API
не доказана, inventory_complete=false и coverage_ratio=null.
Endpoint `https://api.selectel.ru/mobfarm` подтверждён [списком URL](https://docs.selectel.ru/api/urls/)
и [официальным примером Selectel](https://selectel.ru/blog/tutorials/compose-ui-test-for-android/).
Host отсутствует в Swagger, но официальный endpoint не считается отсутствующим.
Raw snapshot сохраняется без SDK правок; источник/sha и статусы операций находятся
в `catalog/evidence/mobilefarm-current-inventory.json`.

`Project(id).MobileFarm()` и `Account().MobileFarm()` — явные scoped views.
Project device methods требуют project IAM; account/user-admin операции не
становятся project methods через скрытую смену token scope. Пул/регион не выводятся.

AvailableDevices поддерживает repeated `fields` по source collectionFormat=multi.
ListDevices и AvailableDevices — finite root arrays, bounded shared FixedIterator;
его новый field-empty режим передаёт корневой массив, а существующий envelope path
сохраняет поведение. Локальный cutoff возвращает partial data и incomplete/SDK cursor.
GetDevice требует exact device ResourceRef и endpoint match до credential lookup;
returned serial проверяется, owner metadata не создаёт user parent/adoption.

Source discrepancy: SmallDevice.status задан object `Status` с ACTIVE/IN_USE/BUSY/
MAINTENANCE/ERROR numeric members; полный Device.status — integer. DTO сохраняют
обе формы, SDK не исправляет source догадкой и не объявляет readiness по краткой
форме. Device timestamps без source format/timezone сохраняются исходными строками.
Remote ADB URL, owner/phone/forwarding details и battery telemetry защищены. Числа
сохраняются лексически без float64; Reveal — явная caller-directed граница.

Текущий scope охватывает все30операций pinned inventory:25implemented/5partial,
0unimplemented. Частичны firewall DELETE200, assign/release/stopremote200 и
settings202: source response schemas отсутствуют. Inventory целиком неизвестен,
coverage_ratio=null; это не complete API claim. Connections/live не выполнялись;
remoteConnectUrl возвращается protected, SDK не открывает ADB/WebSocket.


ADB current-user list/store/delete доступны в account и project views; explicit
user-admin list/store/delete требуют account view и точный user ref. User child
key refs сохраняют только caller-provided user parent; parent kind/service/gen/
scope/endpoint проверяются до credential lookup и HTTP. Root key нельзя случайно
удалить через user-admin path, а nested key — через implicit current-user endpoint.
Protected publicKey раскрывается только wire encoder, minLength/title length
генерируются из source. Accepted fingerprint сохраняется до независимого decode
failure; unknown не повторяется и не восстанавливается по title/public key.
NoContent204 должен быть пустым. Recovery/wait не обещают отсутствующего key Get.

Catalog.Auth.Alternatives — additive optional enum list из source security;
для mixed account/project операций минимальный required scope — account,
primary mode iam_account и alternative iam_project. Реальный scoped view
выбирает соответствующий IAM режим, автоматического fallback нет. Каталог
остаётся immutable, включая новый alternatives slice.

Узко переиспользованы reviewed receipt ContractRevision-before-local-validation
PR97 commit379458e4f658b1216137fea4acce5dae6027d320 (delta82121d740a2788930ce7c4934d449c35b096966d)
и private protected wire encoder OneC из reviewed PR106 (6028e51 в reviewed
7cd0b1469adaba5367ee0d92d43b044100ac2aa4). Другие feature изменения не переносились.

Proxy token create возвращает single-delivery protected token; diagnostics/JSON не раскрывают его. Explicit user-admin методы требуют account scope и caller-provided user parent. Fingerprint сохраняется до независимой ошибки decoding; DELETE проверяет ответ против точного caller ref. Name length проверяется после trim, wire сохраняет caller spelling. Recovery/wait не обещаны.

IssueTicket использует v2 endpoint binding и project IAM. Ticket/description protected, ticket не становится resource ref; unknown не повторяется, recovery unavailable. SDK не открывает WebSocket/ADB соединение, success false сохраняется как provider data, HTTP acceptance не означает connection/readiness.

Firewall create требует все три поля по source prose; update допускает частичный body и использует native PUT, хотя описание request DTO упоминает PATCH. Scope project-only; root rule refs без inferred parents. Ответ с чужим ID не усыновляется, accepted receipt сохраняет caller ref.

Firewall reorder передаёт явные caller refs в заданном порядке; ответ не доказывает полноту списка или порядок. Идентичности сканируются до независимых полей. DELETE200 не имеет source schema: UnmodeledResponse сохраняет bounded protected body и подтверждённый status, всегда Incomplete с Accepted receipt/exact caller ref. Отсутствие schema не трактуется как bodyless или completion. Remaining response contract принадлежит #69.

AssignDevice, ReleaseDevice и StopRemoteConnect имеют200 без schema. Блог Selectel для assign вызывает response.json(), поэтому отсутствие schema не доказывает отсутствие тела; orphan Success DTO не привязывается догадкой. #69 владеет тремя missing response contracts. Bounded protected acceptance-only ответы всегда Incomplete, сохраняют caller device ref без user/session association. StartRemoteConnect возвращает source optional protected URL; не подключается к нему и не обещает readiness. Все4пути проверяют exact endpoint до credentials/HTTP; unknown не повторяется.


Device Order/Replace/Remove/Settings/Tariff используют generated source DTOs.
Order сканирует все trustworthy serials до независимых полей; malformed row не
теряет последующие IDs. Replacement может вернуть другой serial и сохраняет
original+returned refs без inferred ownership. Remove200 означает monthly
autopay update с typed SmallDevice;204 bodyless отличается по status. Settings204
bodyless,202 unknown schema — protected bounded partial даже при пустом теле.
Никакой status не доказывает readiness/connection и не разрешает replay.

Узко переиспользован reviewed DoJSONStatus adapter PR91 head300a4f0aa467c3651f199fa3cf0877597d4f9777
(только internal/core/request.go delta от7f8dd2a), сохраняющий текущий reviewed
earlyreceipt ContractRevision fix. Existing DoJSON delegates to status adapter;
budget/auth/cancellation/no-replay/receipt сохраняют общий путь. Будущая интеграция
P1 ErrorProtocol/diagnostic handling затронет тот же request.go; её unmerged diff
не копировался. Review scope отдельно включает root-array FixedIterator seam и
Catalog.Auth.Alternatives deep-copy metadata без runtime fallback.

Remaining source contracts принадлежат #69: firewall DELETE200 response,
user assignment200 (Swagger no schema vs official blog response.json()), user
release200, stopremote200 и settings202. Не привязан orphan Success DTO, нет
bodyless/completion предположений или выдуманных session associations. Только
источник с конкретным schema/status mapping может перевести эти5partial вimplemented.
