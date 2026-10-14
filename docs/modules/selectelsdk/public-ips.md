# Public IP API boundaries

`Account().PublicIPAllocations()` управляет Resell allocation; project view
предоставляет `Project(...).PublicIPAllocations()` для Get/Create/Delete.
`Project(...).Location(...).PublicIPs()` предоставляет `FloatingIPs` (Neutron)
и `DirectPorts` (Public Network). Эти три API имеют разные resource kinds:
`floating_ip_allocation`, `floating_ip`, `direct_public_port`.

## Sources and explicit scope

Подтверждены 15 remote operations: Resell Get/List/Create/Delete, Neutron
Get/List/Create/Update/Delete, direct port Get/List/Create/Update/Delete и Quotas.
Sources: pinned Resell API snapshot, go-selvpcclient v5.0.0 revision79b0d0f,
vpc-go v1.1.0 revision460f09b, Gophercloud v2.13.0, Neutron-lib Zed revision89dc650,
[текущий Public Network OpenAPI](https://docs.selectel.ru/api/cloud-public-network.json)
и public-net-api-go v1.0.1 revision75bee6c. Локальные source assets и hashes
сохраняются в [public-ip-models](../../../catalog/sources/public-ip-models.json)
и [public-network](../../../catalog/sources/public-network.json).

Resell использует fixed `https://api.selectel.ru/vpc/resell` или exact override.
Get/Create/Delete поддерживают явную account или project auth view, без скрытого
fallback. List является account-only. В Create target ProjectID задаётся явно;
project view не может менять target project. Account token не становится
project token из-за project_id в request/response.

Neutron использует project IAM catalog `network`, direct API — `public-net-api`.
Endpoint берётся для явных region/pool; публичные URL не угадываются. Direct
request path начинается `/v1`, Neutron — `/v2.0`. Неизвестная deployment
extension не объявляется доступной из-за наличия typed поля.

## Resell allocation

AllocationCreateRequest.Specs передаёт все source поля: Region, Quantity, IPs.
Quantity — положительный int32, encoded через точный Integer. Primary API
требует Quantity; Primary Create не угадывает SDK legacy default без Quantity. Явный
CreateNative/AllocationNativeCreateRequest передаёт единственный region-only
AllocationNativeSpec по pinned SDK. Он сохраняет все returned IDs, не выводит
provider-default quantity из SDK и не делает fallback к primary body. Количество
проверяется без float64. Create принимает обе подтверждённые source формы
response: `floatingip` object и `floatingips` array. Обе формы одновременно,
пустой/malformed body и duplicate IDs не дают успешного результата. Недостаток
подтверждённых IDs возвращает Incomplete с Accepted receipt и известными IDs;
SDK не повторяет недостающую часть batch.

Allocation ref сохраняет auth view и project Parent из проверенного owner.
Get/Delete требуют эту exact ref, parent и endpoint. List имеет один
документированный provider array; MaxItems/MaxPages и cursor используют локальный
replay/skip. Поддерживается явный Detailed, включая false. Provider timestamps
Resell сохраняются строками без изобретённого offset.

## Neutron floating IP and association

FloatingIPCreateRequest.Network — точная external network ref; optional Port —
точная Neutron port ref, включая source parent. Spec покрывает description,
floating_ip_address, fixed_ip_address, subnet_id, project_id/tenant_id. Неявного
выбора external network, pool, router или порта нет. Owner aliases должны
согласовываться. Shared network и foreign owner metadata не означают ни выдачу,
ни отсутствие прав: их проверяет провайдер. Ref.Scope сохраняет auth view.

FloatingIPUpdateRequest.Port различает omission, `types.Value(portRef)` и
`types.Null[types.ResourceRef]()` для disassociation. FixedIPAddress также
nullable. Optional Revision передаётся точным If-Match `revision_number=<n>`.
Foreign scope, endpoint, generation, kind или подмена direct port на Neutron port
отклоняется до HTTP. Read refs сохраняют floating_network_id Parent; implicit
allocation→Neutron conversion отсутствует. Для известного принятого allocation
можно явно запросить Neutron list с точным ID; это наблюдение конкретного ID,
а не поиск нового кандидата по имени после unknown mutation.

Get/List поддерживают Fields; list также typed filters, tag selectors,
sort pairs, page_reverse и bounded marker continuation. Fields не расширяется
скрытыми identity/parent. Projected list без них имеет nil Ref; projected exact
Get сохраняет request ref. Следующий link ограничен тем же origin/path,
переносится только marker; исходная выборка и endpoint сохраняются. Cursor
связан со scope/endpoint/generation/filters/contract revision. Короткая страница
не доказывает конец. CreatedAt/UpdatedAt сохраняются raw strings.

## Direct public ports

Текущий OpenAPI использует project token, не legacy project_id в body.
Create/Update Spec включают nullable Description (до5000 characters),
AdminStateUp и SecurityGroupIDs. Nil slice означает omission, [] сохраняется;
Create допускает пустой список, PATCH explicit list требует 1–20 элементов.
Owner ответа является metadata, Ref.Scope — auth view. Create проверяет
известный target owner; чтение/изменение не выводит owner==token из IAM mode.
Права на конкретный ресурс проверяет провайдер.
Read timestamps — RFC3339 с нормализацией UTC. List filters NetworkID,
Description и Subnet сохраняют explicit empty values; недокументированные
remote limit/marker не отправляются. Коллекции ограничены локальным replay/skip.
Quotas берёт target project только из auth view, сохраняет точные optional
Integer counters и не подменяет отсутствие нулём.

Published SDK1.0.1 содержит legacy body ProjectID и DELETE force=true; current
OpenAPI не описывает force и документирует409 для attached port. Это разные
подтверждённые source request contracts. Current Create/Delete не используют
legacy параметры. Явные CreateNative/DirectPortNativeCreateSpec передают body
ProjectID, который обязан совпадать с auth view, и security_group_ids всегда:
nil сериализуется null, [] сохраняется. Description nil опускается, как в SDK.
UpdateNative/DirectPortNativeUpdateRequest также сохраняет всегда присутствующий
security_group_ids (nil→null), без подмены current PATCH constraints.
DeleteNative/DirectPortNativeDeleteRequest передаёт force=true только при явном
Force=true; false опускается, как в SDK. ListPageNative/IterateNative/CollectNative и DirectPortNativeListOptions
передают непустые ProjectID/NetworkID/Description query строки, опуская пустые,
как SDK. Явный ProjectID filter не переключает auth view; если returned owner
присутствует, он обязан совпадать с этим фильтром. Current list дополнительно
поддерживает Subnet и explicit empty filters из current API.
Canonical operation IDs остаются общими
для двух request variants: remote endpoints/methods не дублируются в inventory.
Native mutations сохраняют receipts и single-attempt semantics; native list
использует те же bounded collection/cursor правила.
Fallback между native/current и replay после unknown запрещены. Published SDK
подтверждает wire contract, но не проверяет текущую live server availability.
Whole inventory остаётся incomplete/coverage_ratio=null.

## Receipts, errors and verification

Любая mutation сохраняет Receipt. Неоднозначная передача/5xx даёт UnknownOutcome
без replay; exact Get лишь наблюдает ресурс и не доказывает авторство mutation.
Request parent/association refs в Receipt — context, не identity созданного
child. Malformed accepted response оставляет Accepted и доверенные IDs.
Wait, dataplane readiness и root Resource/Resume здесь unavailable.

400 следует shared ProviderError; 401/403/404/409/429 отображаются общим runtime.
Direct error integer `code` сохраняется точно на read/rejected error через
узкий compiled protocol decoder; `msg` и raw error body не выводятся. Unknown
mutation использует общие UnknownOutcome/CauseCode semantics. Resell400 с
описанием Not found не превращается автоматически в точный NotFound.
Foreign plugin/raw fields, tag/QoS/port-forwarding mutations не реализуются.

Проверки: настоящий public Client/scoped API, synthetic CredentialSource и
подмена только Config.Transport. Wire cases offline; live операции не запускались.
