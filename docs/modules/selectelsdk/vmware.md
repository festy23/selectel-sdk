# VMware Cloud Director: подтверждённый subset

Источник Selectel: [VMware API](https://docs.selectel.ru/api/vmware/).
В примерах указана версия 36.3, а не гарантированная текущая версия конкретного
развертывания. Список non-deprecated версий получает вызывающая сторона явно.
Проверенный upstream reference:
[36.3 SupportedVersionsType](https://developer.broadcom.com/xapis/vmware-cloud-director-api/36.3/doc/types/SupportedVersionsType.html).
Upstream reference включает недоступные tenant операции; его полный размер
не является знаменателем Selectel coverage. InventoryComplete=false, ratio=null.

`Client.Project(projectID).Location(region, pool).VMware().Versions(ctx, options)`
использует точный override `(vmware,v36_3,scope)`, с base origin без `/api`.
Region обязателен, pool может быть пустым и не выводится из облачных pools.
Официальные origin: Санкт-Петербург `https://vcd.selectel.ru`, Москва
`https://vcd-msk.selectel.ru`; SDK не меняет caller selection и не выполняет DNS
поиск или пробный запрос для определения location.

GET `/api/versions` имеет документированную anonymous auth семантику. Только
этот exact compiled operation получает узкое исключение в shared Runtime;
CredentialSource не вызывается и credentials не отправляются. Нового публичного
AuthMode/универсального no-auth обхода нет, ProtocolSpecific и публичный
Authenticate сохраняют прежний gate для остальных неподдержанных протоколов.
Остальная orchestration (scope, endpoint, budget, cancellation, bounded body,
read retries, redirect rejection, observer redaction) остаётся в core.DoJSON;
имя функции не ограничивает service decoder JSON-форматом.

Ответ — generated typed SupportedVersions / VersionInfo / AlphaVersion /
MediaTypeMapping / SchemaRoot. Optional deprecated сохраняется указателем:
missing не означает false/актуальную версию. Никакой автоматический выбор версии,
login или запрос по LoginUrl/ProviderLoginUrl/SchemaRoot/SchemaLocation не выполняется.
XML namespaces, child names/attributes и singleton duplicates проверяются до DTO;
DTD/external entities не разрешены, metadata schemaLocation не загружается.

Нормализованный `catalog/sources/vmware-cloud-director.json` — SDK-owned
transcription первичного reference и восьми Selectel examples (семь distinct method/path operations: GET /api/query
имеет VDC/VM parameter variants, которые не умножают coverage), не vendor OpenAPI
или XSD download. Vendor versions.xsd link возвращает documentation shell,
прямое urllib чтение reference вернуло 403; indexed reference доступен.
Полные модели/status contracts остальных семи примеров, task/VM observation и
Selectel-supported vApp/VM/network/storage lifecycle inventory ещё требуют
source reconciliation и остаются owned #68. Ни этот начальный метод, ни наличие
vendor SDK не закрывают #68. Никаких live Cloud Director вызовов не выполнялось.

Remaining scope закреплён native blocking children #107 (version-bound auth и
supported inventory), #108 (vApp/VM/task lifecycle), #109 (network/storage).
#108/#109 blocked by #107; #68 blocked by всеми тремя и не закрывается этим subset.
