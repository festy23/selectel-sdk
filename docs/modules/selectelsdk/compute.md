# Nova: выбор параметров и создание VM

Scoped `Project(id).Location(region, pool).Compute()` предоставляет `Flavors`,
`Keypairs` и `Servers`. Все операции используют IAM project token и compute
public endpoint из подтверждённого IAM catalog для явно выбранной location либо
exact scoped override. Проект, endpoint и поколение не меняются после ошибки.

Источники закреплены в локальных snapshots: Gophercloud v2.13.0 и
[Nova Zed revision 3d1a705](https://github.com/openstack/nova/tree/3d1a705fc85a0f0a46e7cbc322e98fff892e21b8).
[Матрица Selectel](https://docs.selectel.ru/cloud-servers/tools/openstack-cli/about-openstack-cli/)
подтверждает Nova Zed с максимумом microversion 2.93. Generation использует только
локальные assets и SDK-owned mappings; cloud и сеть не нужны.

## Flavors

`Flavors.ListPage/Iterate/Collect(FlavorFilters, pagination.Options)` вызывает
`GET /flavors/detail` с microversion 2.93. Фильтры: minDisk/minRam, changes-since,
is_public и sort_key/sort_dir. `SortKeys/SortDirs` сохраняют повторяющиеся query
параметры; их нельзя смешивать с одиночными `SortKey/SortDir`.
`Get` и `ExtraSpecs` используют точный flavor ref из списка и проверяют scope,
endpoint, generation и kind. Возвращаемый ID должен совпадать с ожидаемым.

Все поля закреплённого flavor DTO представлены типами, включая disabled,
ephemeral, description, extra_specs и links. Integer/Number сохраняют точность;
legacy swap допускает integer, integer string и empty string (ноль).
Nullable description сохраняет отсутствие/null/value.
Flavor admin mutations и непроверенные расширения не объявлены callable.

Flavors pagination использует только marker из безопасного next link с тем же
origin/path. SDK строит следующий запрос сам, сохраняя исходные фильтры;
произвольный URL не открывается. MaxItems/MaxPages/PageSize, MaxJSONBytes и общий
CollectionTimeout ограничивают обход. Остановка по лимиту возвращает incomplete
и replay cursor с точной scope/endpoint/contract/filter привязкой.

## SSH keys

`Keypairs.Create/Get/Delete` и `ListPage/Iterate/Collect` — named-user API:
Create и filters требуют явный UserID; точные refs содержат parent `keypair_user`.
`Keypairs.Current` предоставляет те же действия относительно выбранного
аутентифицированного субъекта; UserID в Current.Create должен отсутствовать.
Список Current без owner-поля возвращает отдельный parent `keypair_current_user`.
Named и Current refs не принимаются противоположным API до I/O.
Когда провайдер возвращает user_id, ref становится точным named-owner ref;
последующие Get/Delete такого ref идут через `Keypairs`, передавая owner query.
Несовпадение явного owner с returned user_id — scope_mismatch.

Current ref без owner не доказывает исходный principal после замены credentials.
Он остаётся относительным к текущему субъекту, а не absolute-owner proof или
основанием для adoption неизвестного изменения. Endpoint/project совпадение
не подтверждает авторство неизвестного запроса.

Keypairs используют microversion 2.35 по умолчанию. Create и named list позволяют
выбрать версию 2.1..2.93 явно; type требует 2.2, user_id — 2.10. С 2.35 list
передаёт bounded limit/marker; marker — последнее имя, любая непустая страница требует
следующего чтения до empty page: провайдер может уменьшить requested limit. Для pre-2.35 применяется bounded local replay cursor.
С 2.92 создание требует явный public_key: SDK не делает скрытый downgrade ради
генерации private key. Отсутствие public_key в старой версии запрашивает генерацию;
полученный private_key — ProtectedValue. Public key/fingerprint остаются обычными
публичными данными. Timestamp без timezone сохраняется исходной строкой, включая дробную часть;
SDK не приписывает отсутствующий offset. `NovaTime.Instant()` возвращает UTC
instant только для timestamp с явно указанной timezone; такие значения
сериализуются RFC3339Nano в UTC. deleted_at/updated_at сохраняют explicit null.

Create принимает 200/201, Delete — 204. Mutations сохраняют receipts и выполняются
один раз. Неизвестный исход сохраняет точный request name/owner/endpoint, когда
endpoint уже известен; malformed accepted response сохраняет trustworthy identity
до декодирования private key. Ошибки owner/name не разрешают adoption. Root
Handle/Resume и автоматический wait для keypair не предоставлены.

## Server create

`CreateRequest.Microversion` явно выбирает 2.1..2.93; default 2.1 сохранён для
существующих callers. Отправляются согласованные OpenStack-API-Version и
X-OpenStack-Nova-API-Version headers. SDK не повышает версию по наличию нового
поля и не использует 2.94+.

`Options` сохраняет native Gophercloud CreateOpts. Все подтверждённые Zed поля
создания представлены native options или типизированными дополнениями:

- KeyName, nullable Description и TrustedImageCertificates, Host и SchedulerHints;
- Personality с ProtectedValue contents;
- NetworkAttachments с nullable port; явно заданный список заменяет Options.Networks;
- BlockDevices (v2 aliases, nullable boot index) и LegacyBlockDevices; их нельзя
  смешивать между собой или с Options.BlockDevice;
- ReturnReservationID для server/reservation response selection.

Protected AdminPassword/UserData и file contents раскрываются только в
source-bound wire fields; user-data/files кодируются base64 ровно один раз.
Plaintext native AdminPass, nonempty UserData и Personality отклоняются.
BDM connection_info и произвольный source-defined no_device — protected values,
которые не сериализуются в диагностический output. Scheduler hints используют
native typed hints и source-defined additional properties; это только объект
hints данного create endpoint, не HTTP escape hatch.

До HTTP проверяются minimum versions, gaps tags (BDM только 2.32 или 2.42+;
network tags 2.32..2.36 или 2.42+), personality removal с 2.57, обязательные
networks и UUID format с 2.37. Tags требуют 2.52, trusted certificates — 2.63,
volume_type — 2.67, host/hypervisor_hostname — 2.74, hostname — 2.90.
Hostname ограничен label длиной 2..63 и не принимает FQDN из 2.94.
Legacy implicit networks доступны до 2.37; auto/none — начиная с 2.37.
Провайдер проверяет оставшиеся schema, policy, quota и deployment constraints;
наличие typed поля не обещает текущую доступность расширения или права.

`CreateWithResult` сохраняет документированный union ответа 202:
Server или ReservationID/ReservationRef. `Create` сохраняет совместимый Server
result и отклоняет запрос reservation-ответа до HTTP. Для multiple create с
return_reservation_id=false API возвращает только первую VM. SDK не выдумывает
refs остальных машин. Положительный receipt подтверждает принятие create,
а не готовность VM.

ReservationRef имеет kind server_reservation и сохраняет исходные
scope/generation/endpoint. Это acknowledgement, не remotely readable operation;
receipt.Recovery — unavailable. Нет batch/reservation wait, сканирования по имени,
adoption или replay. CreateAndWait предназначен для одной VM и отклоняет multiple
create/reservation до HTTP. При transport loss/5xx outcome остаётся unknown;
malformed accepted response сохраняет accepted receipt и известные точные refs.

## Проверенность и границы

Public Client/HTTP transport tests покрывают selection reads, SSH lifecycle,
owner/current semantics, source-only create variants, protected values, exact
numbers, pagination/cursors, microversion constraints и receipts без replay.
Live cloud operations не выполнялись. Генерация, race tests, vet и offline evidence
не доказывают live availability.

Binding server create закрывает подтверждённый source contract до 2.93;
server list/get/delete сохраняют прежние partial ограничения. Images CRUD
относится к отдельному Glance contract. Общая полнота Nova inventory неизвестна,
coverage_ratio остаётся null; поддержка всей Nova API не заявлена.
