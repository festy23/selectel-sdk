# Каталог возможностей

## Источник и формат

Каталог встроен в релиз SDK и доступен без credentials и сетевых вызовов.
Он содержит service registry, OperationContract records и coverage summary.
Operation ID имеет форму `service.generation.resource.action`; компоненты
состоят из lowercase букв, цифр и underscore. Wire `v2.1` кодируется `v2_1`
в operation ID, но APIGeneration сохраняет wire значение.

Все public methods с I/O связаны с operation ID. Local New/scope/catalog
методы не являются provider API coverage. Composite Inventory/CreateAndWait
содержат child operation IDs и не увеличивают число покрытых provider operations.

Сериализация — JSON UTF-8 с schema_version=1. Обязательный формат записи:

```yaml
operation_id: string
service: string
api_generation: string
resource: string
action: string
effect: read | mutation | transfer
availability: documented | discovery_pending | withdrawn
source:
  uri: string
  revision: string
  operation_locator: string
contract_revision: string
contract_artifact_revisions: {artifact_ref: sha256}
go_method: string | null
input_type: string | null
output_type: string | null
input_schema_ref: string | null
output_schema_ref: string | null
scope_requirements: [account | project | region | pool | parent]
auth:
  scheme: iam_account | iam_project | static_token | s3_sigv4 | basic | protocol_specific
  audience: string
endpoint_recipe_ref: string | null
request_binding_ref: string | null
response_binding_ref: string | null
error_binding_ref: string | null
replay_policy_ref: string | null
pagination_contract_ref: string | null
wait_goals: [string]
recovery_contract_ref: string | null
sensitive_fields: [string]
parameter_support:
  covered_schema_ref: string | null
  excluded_variants: [string]
implementation:
  status: unimplemented | partial | implemented
  binding: string | null
verification:
  level: none | offline | wire | live
  contract_revision: string | null
  implementation_revision: string | null
  evidence_ref: string | null
limitations: [string]
```

`revision` источника — pinned commit/version либо content SHA-256 публичной
схемы. `contract_revision` — SHA-256 canonical JSON OperationContract с
отсортированными ключами, без derived contract_revision, implementation и verification metadata.
ContractArtifactRevisions содержит hashes referenced SDK-owned binding artifacts;
они включаются в canonical OperationContract перед вычислением contract_revision.
Refs указывают на поставляемые с релизом immutable contract/schema artifacts;
одна внешняя ссылка не заменяет mapping и параметры OperationContract.

Для discovery_pending допускается source с URI страницы продукта и locator
`no_confirmed_public_contract`; callable binding отсутствует. Это запись
service discovery, а не доказанная API операция. KMS учитывается таким образом.
Withdrawn сохраняет прежний ID и источник, но не является callable.

## Значение статусов

| Поле | Значение |
|---|---|
| documented | Официальный публичный operation contract найден |
| unimplemented | В SDK отсутствует полный callable binding |
| partial | Callable binding существует только для явно описанного subset параметров/вариантов |
| implemented | Public method и mapping покрывают все documented параметры/варианты source operation |
| offline | Подтверждён локальный контракт конкретной implementation revision |
| wire | Подтверждено соответствие auth/request/response публичному протоколу |
| live | Операция проверена на Selectel в явно указанной location/generation |

Verification evidence хранит только нечувствительные reference/hash и область
проверки; credentials, ресурсы аккаунта и payloads не публикуются в catalog.
Live в одном pool не означает live во всех pools. Новый source/contract или
implementation revision сбрасывает не относящуюся к нему verification в none.
Presence в каталоге не подтверждает текущие права, баланс, quotas или доступность
region. Permission/availability для caller определяется конкретным вызовом.

## Учёт полного покрытия

1. Получить список семейств из [официального каталога](https://docs.selectel.ru/api/)
   и подтверждённых protocol integrations из [services.md](services.md).
2. Для каждого семейства закрепить revision источника. Перечислить все
   documented paths/methods или protocol operations в границах Selectel,
   включая legacy отдельно, вложенные ресурсы, transfer и actions.
3. Сопоставить каждую source operation ровно одному OperationContract.
   Неописанные или отсутствующие bindings записать как unimplemented;
   bindings с исключёнными documented parameter variants — partial.
   Raw HTTP escape hatch не изменяет этот статус. Partial operation доступна
   только в covered schema; excluded variant возвращает capability_unavailable.
4. Вернуть summary с числом documented/partial/implemented/offline/wire/live операций
   по service/generation и explicit unimplemented IDs. Числитель не включает
   partial, composite helpers, discovery_pending и withdrawn; знаменатель содержит
   все documented active operations закреплённого source revision.
5. Если хотя бы один active source inventory не разобран целиком, summary
   получает `inventory_complete=false`, `coverage_ratio=null` и перечень
   incomplete families. Неизвестный знаменатель не обозначается 100%.
6. Полное покрытие закреплённого API revision объявляется только при
   inventory_complete=true и отсутствии unimplemented/partial active operations.
   Verification distribution выводится отдельно; полное implemented coverage
   не обозначается полной live проверенностью.

Native SDK method list и Terraform resource list не являются знаменателем
API coverage. При изменении API source новая версия каталога сохраняет старые
идентификаторы, добавляет новые и явно отмечает withdrawn операции.
