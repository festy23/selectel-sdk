# Neutron core v2

`Client.Project(project).Location(region, pool).Network()` предоставляет
`Networks`, `Subnets` и `Ports`: Create, Get, Update, Delete, ListPage, Iterate,
Collect. Подтверждены 15 remote operations из
[локального source subset](../../../catalog/sources/network-core.json):
[Selectel vpc-go v1.1.0, revision 460f09b](https://github.com/selectel/vpc-go/tree/460f09b941de260160c68d1c9cac3ae792096bf5/pkg/v2),
[Gophercloud v2.13.0 networking](https://github.com/gophercloud/gophercloud/tree/v2.13.0/openstack/networking/v2)
и [официальный API reference](https://docs.openstack.org/api-ref/network/v2/).
API generation `v2`, wire prefix `/v2.0`; endpoint — public `network` из IAM
catalog для явного project/location либо точный EndpointOverride. Автоматического
перехода на другую версию или регион нет.

## Авторизация, владелец и ссылки

Scope задаёт **область авторизации вызова**, а `project_id`/`tenant_id` в DTO —
владельца ресурса. Без фильтра SDK запрашивает доступную провайдером коллекцию,
включая shared resources; скрытого `project_id=<auth project>` нет. Ссылка на
shared network сохраняет исходный auth scope. Её можно явно передать родителем
создаваемого subnet/port, если провайдер разрешает такое действие. SDK не выводит
право записи из shared flag или несовпадения owner и auth project.

Create принимает source-only Spec; `SubnetCreateRequest.Network` и
`PortCreateRequest.Network` задают точную network ref, из неё передаётся
`network_id`. Request parent в Receipt — известный контекст, не доказательство
identity созданного child без подтверждённого response ID. Явные owner aliases должны совпадать между собой, но могут
отличаться от auth project: IAM view при этом не меняется, права проверяет
провайдер. При отсутствии owner ожидается проект токена. Response aliases
проверяются на согласованность. Reads и updates не приравнивают owner к scope.

Полные ответы требуют непустой ID; для subnet/port нужен network_id, который
становится Parent в том же auth view и endpoint. Последующие методы требуют
эту точную parent-bound ссылку. Ссылки другого scope/generation/kind/endpoint
отклоняются. Возвращённый `ref` провайдера игнорируется.

## Типы и выборка

Generated DTO включают все поля трёх resource-моделей pinned vpc-go; Request
Spec включают поля соответствующих CreateRequest/UpdateRequest и стандартные
owner aliases/shared/MAC update из Gophercloud. Подтверждённые
[документацией Selectel network arguments](https://docs.selectel.ru/terraform/openstack-provider-reference/networking-neutron/resources/openstack_networking_network_v2/)
MTU, router:external, vlan_transparent и QoS policy ID передаются явно;
[port QoS policy ID](https://docs.selectel.ru/terraform/openstack-provider-reference/networking-neutron/resources/openstack_networking_port_v2/)
также типизирован. Для QoS сохраняются omitted/null/value; null удаляет явную
policy. Наличие typed conditional поля не обещает deployment extension или
права: провайдер проверяет net-mtu-writable, external, vlan-transparent и qos.
MTU для приватных сетей Selectel документирован до 8500 Б. SDK не использует
этот частный лимит для произвольной shared/external network без доказанной
классификации ресурса. Дополнительные plugin поля не принимаются
через map или raw bypass. В частности исключены physical `provider:*`, VIF
`binding:*`, subnet pools/segments, bulk mutations, `propagate_uplink_status`,
`value_specs`; tag mutations относятся к другой capability. Запись port_security_enabled исключена по
[прямому ограничению Selectel](https://docs.selectel.ru/cloud-servers/cloud-networks/about-networks/#traffic-filtering-port-security):
в новых сетях фильтрацию нельзя выключить, а в старых нельзя включить. Это
не вывод из отсутствующего поля SDK. Поддержка provider-segments и binding
плагинов не следует из общей Terraform reference и не эмулируется.
Это не заявление полного покрытия Neutron: inventory_complete=false, coverage_ratio=null.

Nil опускает optional поле; false, пустая строка и пустой
slice передают исходное значение. Slice `nil` опускается, slice `[]` передаётся.
GatewayIP использует `types.Null[string]()` для JSON null; nullable DHCP option
value сохраняет null удаления. Integer не проходит через float64. CreatedAt и
UpdatedAt остаются исходными строками, без добавления отсутствующего timezone.

Typed `NetworkFilters`/`SubnetFilters`/`PortFilters` принимают Filters, Fields,
SortKeys/SortDirs и PageReverse. Повторные значения фильтра сохраняются
(OR одного поля, AND разных); пустое значение не теряется. Fields не расширяется
скрытыми ID/owner/parent. Если проекция не содержит identity/parent, list DTO
имеет nil Ref. Exact Get сохраняет request ref даже при проекции без ID;
возвращённый ID/parent, если присутствует, обязан совпадать. Неизвестный selector
отклоняется до HTTP. Доступность sorting/filter extensions остаётся свойством
провайдера, SDK не эмулирует их и не трактует ошибку как полный список.

Collection использует limit/marker и source links; короткая страница не означает
конец. PageReverse=true выбирает previous link. Принимается только ссылка на
тот же endpoint/path без credentials/fragment; переносится только marker,
остальная выборка восстанавливается из immutable snapshot. MaxItems/MaxPages,
PageSize и CollectionTimeout ограничивают обход. Cursor связан с selection,
scope, endpoint, generation и contract revision; согласованного snapshot нет.

## Изменения и ошибки

Update/Delete могут передавать точный неотрицательный Revision через If-Match
`revision_number=<n>`. Успех — Create 201, Update/Get/List 200, Delete 204.
Receipt сохраняется на любой ошибке mutation. Неоднозначный transport/5xx
даёт UnknownOutcome без replay. Malformed accepted body оставляет Accepted;
достоверный извлечённый ID сохраняется даже при ошибке другого поля.

400/401/403/404/409 отображаются общим runtime; 412 остаётся ProviderError.
Provider message/detail не попадают в диагностику. Неподтверждённые машинные
коды не классифицируются по тексту. Wait, root Resource/Resume и доказательство
авторства неизвестной mutation не предоставляются. Exact Get лишь наблюдает
ресурс; логический CRUD success не доказывает готовность dataplane.

Проверки выполняются через публичный Client с подменой только HTTP transport
и synthetic CredentialSource. Live совместимость и фактические права на shared
resource не проверялись.
