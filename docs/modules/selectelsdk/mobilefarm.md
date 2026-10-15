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

Текущий slice содержит три чтения; остальные 27 operations остаются unimplemented
до source-bound TDD slices, обязательных проверок и двух review. Transfers и device
connections не выполнялись; remoteConnectUrl возвращается как protected data,
SDK не открывает ADB/WebSocket соединение автоматически. Offline не означает live.
