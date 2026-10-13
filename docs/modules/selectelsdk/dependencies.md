# Зависимости и переиспользование

## Официальные библиотеки

Таблица задаёт baseline reuse. Версия модуля и поколение HTTP API независимы.
Все зависимости фиксируются в go.mod/go.sum; обновление требует сохранения
публичного DTO-контракта и общих гарантий SDK. Библиотеки подключаются только
для реализованных операций, а не ради увеличения числа импортов.

| SDK и baseline | Предметная область | Граница reuse |
|---|---|---|
| [go-selvpcclient/v5 v5.0.0](https://github.com/selectel/go-selvpcclient/tree/v5.0.0) | Resell v2, проекты, quota, catalog | Не Nova VM lifecycle; использует Gophercloud v1.14.1 |
| [iam-go v0.9.0](https://github.com/selectel/iam-go/tree/v0.9.0) | IAM и federation | Не предоставляет bucket/object API; default адаптер HTTP до подтверждения license declaration |
| [mks-go v1.1.0](https://github.com/selectel/mks-go/tree/v1.1.0) | MKS legacy v1 | Текущий MKS v2 — отдельный HTTP адаптер |
| [dbaas-go v0.23.0](https://github.com/selectel/dbaas-go/tree/v0.23.0) | DB v1, ClickHouse/OpenSearch v2 | Engine-specific bindings; retention не backup download |
| [craas-go v0.4.2](https://github.com/selectel/craas-go/tree/v0.4.2) | Registry v1 и tokens v2 | Registry management v2 и OCI data transfer отдельно |
| [domains-go v1.1.0](https://github.com/selectel/domains-go/tree/v1.1.0) | DNS v1/v2 | Records v1 и RRsets v2 различаются; не registrar API |
| [private-dns-go v1.1.0](https://github.com/selectel/private-dns-go/tree/v1.1.0) | Private DNS v1 | PutRecords сохраняет collection semantics |
| [secretsmanager-go v0.2.1](https://github.com/selectel/secretsmanager-go/tree/v0.2.1) | Secrets/certificates | Недостающие version methods через HTTP; default HTTP до подтверждения license declaration |
| [cloudbackup-go v1.0.0](https://github.com/selectel/cloudbackup-go/tree/v1.0.0) | Plans/checkpoints, data-protect/v2 | Restore и отсутствующие actions через HTTP |
| [dedicated-go/v2 v2.1.3](https://github.com/selectel/dedicated-go/tree/v2.1.3) | Dedicated servers v2 | Отсутствующий order/create не выводится из ServerGet |
| [globalrouter-go v1.2.1](https://github.com/selectel/globalrouter-go/tree/v1.2.1) | Global router, naas/v1 | Network attach не Neutron create |
| [public-net-api-go v1.0.1](https://github.com/selectel/public-net-api-go/tree/v1.0.1) | Public-network/v1 | Default HTTP до подтверждения license declaration |
| [blockstorage-go v0.2.0](https://github.com/selectel/blockstorage-go/tree/v0.2.0) | Cinder v3 | Отсутствующие snapshot create/delete через другой adapter |
| [vpc-go v1.1.0](https://github.com/selectel/vpc-go/tree/v1.1.0) | Neutron-like v2 | Отсутствующие floating IP create/delete отдельно; default Gophercloud/HTTP до подтверждения license declaration |
| [images-go v0.1.0](https://github.com/selectel/images-go/tree/v0.1.0) | Glance v2 | Package pkg/v1 не HTTP /v1 |

IAM, Secrets Manager, public-net-api и vpc библиотеки не импортируются только
на основании лицензии в GitHub metadata: требуется declaration конкретного
используемого source revision. Для остальных перечисленных SDK исходный
LICENSE задаёт Apache-2.0; условия сохраняются при reuse.
[python-selvpcclient](https://github.com/selectel/python-selvpcclient) является
архивным Python SDK/CLI reseller API и не runtime-зависимостью Go-библиотеки.

## Совместимые протоколы

Compute использует context-aware
[Gophercloud v2.13.0](https://github.com/gophercloud/gophercloud/tree/v2.13.0/openstack/compute/v2/servers)
для Nova v2.1 либо собственный HTTP adapter с тем же контрактом. Для других
OpenStack services используются точные generation bindings из services.
ProviderClient Gophercloud v1 из go-selvpcclient не передаётся в Gophercloud v2.
Bridge переносит только проверенные scope/catalog metadata и token через
контролируемый transport; auto-reauth и скрытые retries отключены.

S3 и CloudWatch используют
[AWS SDK Go v2 v1.47.3](https://github.com/aws/aws-sdk-go-v2/tree/v1.47.3) с явно заданными endpoint,
region/pool и CredentialSource. Default AWS credential chain и default AWS
endpoints не активируются. Retries AWS middleware подчиняются execution SDK.
Prometheus использует документированный read HTTP protocol без привязки
к Grafana server. OCI transfer использует distribution protocol; repository
credentials и tokens возвращаются как ProtectedValue.

Если native SDK не позволяет соблюсти context, единый budget, replay policy
или безопасный diagnostic contract, операция использует HTTP/protocol adapter.
SDK не оборачивает неконтролируемый I/O в goroutine, выдавая отмену ожидания
за отмену продолжающегося запроса.

## Terraform provider

Источник интеграционных контрактов —
[terraform-provider-selectel v9.0.0](https://github.com/selectel/terraform-provider-selectel/tree/v9.0.0).
Его package, Terraform framework, state handlers и import identifiers не
подключаются как API selectelsdk.

| Переиспользуемое решение | Контракт selectelsdk |
|---|---|
| Scoped client factories, region/service type lookup | Общий resolver access.md; scoped identity неизменяем |
| Native request DTO и field mapping | Native types с выбранной generation; sensitive fields защищены |
| MKS/DB/Registry состояния и polling | Exact operation goals; context, budget и ошибки задаёт operations.md |
| Parent refs и pagination DNS/вложенных ресурсов | Exact IDs, явный parent, cursor и incomplete result |
| IAM role deltas | Typed operations сохраняют документированный порядок и partial outcomes |
| Quota/version/configuration validation | Только ограничения официального API; ForceNew не доказывает API immutability |

Не переносятся глобальный singleton Config между accounts, Terraform
`d.SetId("")` как API result, выбор первого нового nodegroup по diff списков,
поиск по неэкранированному regex, flatten/expand Terraform state и debug
payload logging. Их семантика не отвечает контракту библиотеки.

При копировании содержательного кода provider сохраняются требования
[MPL-2.0 source](https://github.com/selectel/terraform-provider-selectel/blob/v9.0.0/LICENSE),
provenance и notices соответствующих файлов. Самостоятельный adapter по
публичному API не импортирует Terraform state или acceptance runner.
Внешняя live verification не запускается автоматически при установке/импорте SDK.

## Yandex Cloud

Документированный интерфейс yc используется только как ориентир группировки
по сервисам и явных операций get/wait. Неподтверждённые внутренние устройство
и исходники yc не являются dependency или основанием архитектуры selectelsdk.
Yandex Cloud profiles, cloud/folder IDs и модели операций не переносятся на Selectel.
