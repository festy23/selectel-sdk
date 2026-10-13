# selectelsdk

Go-библиотека управления Selectel. Нормативный контракт находится в
[спецификации](docs/modules/selectelsdk/README.md).

Модуль предоставляет явные credential sources, неизменяемые account/project/
location views, IAM-авторизацию с проверкой области и обновлением токенов,
единые ошибки, защищённые значения и встроенный каталог публичных API.

## Установить SDK и скилл одной командой

В своём терминале на macOS, Linux или WSL выполните:

```sh
curl -fsSL https://raw.githubusercontent.com/festy23/selectel-sdk/main/install.sh | bash
```

Нужны `curl`, Git и Python 3.9+. Установщик сохраняет SDK в домашнем каталоге,
загружает зависимости и устанавливает скилл `selectelsdk` в `~/.agents/skills`.
Если совместимого Go нет, он загружает официальный архив с
[go.dev](https://go.dev/dl/) и проверяет SHA-256. `sudo` и изменение системного
Go не требуются; путь выбранного Go записывается в метаданных установки.

Затем мастер открывает панель Selectel и запрашивает **номер аккаунта, имя
сервисного пользователя и пароль**. Пароль вводится скрыто; он передаётся
writer через отдельный file descriptor, сохраняется вне Git и не попадает в
аргументы процессов или диагностический вывод. JSON-файл имеет права `600`,
его каталог — `700`. Каждый коллега вводит собственные credentials.

По умолчанию профиль называется `default`. Существующий профиль с выбранным
именем используется без изменения, остальные профили сохраняются. В конце
установщик проверяет IAM-авторизацию через SDK; она не доказывает наличие прав
на все сервисы. Если проверка не пройдёт, установка и сохранённый профиль
останутся доступны для исправления доступа и повторной проверки.

Профиль можно назвать или выбрать явно, например:

```sh
curl -fsSL https://raw.githubusercontent.com/festy23/selectel-sdk/main/install.sh | bash -s -- --profile photorus
```

После установки попросите агента: «Используй `$selectelsdk` и мой профиль
`default`, проверь доступные проекты». Для ресурсов проекта задайте точный
target в запросе. Если скилл не появился, начните новый запрос или перезапустите
Codex; [каталог пользовательских скиллов](https://learn.chatgpt.com/docs/build-skills#where-codex-loads-local-skills)
описан в официальной документации.

Пути установки по умолчанию:

- SDK: `~/.local/share/selectelsdk/sdk/<commit>`; версии сохраняются отдельно.
- Скилл: `~/.agents/skills/selectelsdk`; ссылки на SDK работают из любого проекта.
- Credentials: `~/.config/selectelsdk/profiles.json`.

Для установки без ввода credentials добавьте `--skip-profile`; для своих
каталогов — `--data-dir`, `--skills-dir`, `--profiles-path`. `--go` задаёт
путь к существующему Go. Полный список: `bash install.sh --help` из checkout.
Повтор команды устанавливает актуальную версию SDK и принадлежащую установщику
копию скилла. Изменённый пользователем SDK или сторонний скилл сохраняются;
конфликт требует выбора другого каталога. Dependencies отдельного Go-приложения
обновляются его собственным `go get`, а не этим установщиком.

## Поддерживаемые операции

Каталог содержит 191 операцию: восемь закреплённых официальных API-схем
Selectel и документированные части Nova, CloudWatch Logs и Prometheus.
Из них 53 имеют статус `implemented`, 11 — `partial`, 127 — `unimplemented`.
Для 64 callable bindings записана локальная проверка `offline`; проверки
на живом облаке в этот результат не входят.
Реализованы три метода Billing, все девять операций текущей схемы Secrets Manager,
чтение проектов, Kubernetes v2, баз данных v1, Registry v2 и аудит-событий.
Проекты Resell v2 поддерживают Create/Get/Update/Delete. Registry v1 имеет
отдельные Create/Get/Delete и ListPage/Iterate/Collect с собственными DTO.
Все 29 операций закреплённой схемы MKS v2 имеют callable bindings: кластеры,
группы узлов, узлы, задачи, интеграции реестров и справочники Kubernetes.
Четыре операции Nova — Create, Get, Delete и ListPage — имеют статус `partial`:
они покрывают начальный сценарий одной VM; ограничения записаны в каталоге.
Доступны CreateAndWait, Client.Resource/Resume и Handle.Status/Wait/Recover.
Остальные операции отмечены `unimplemented`. Описание API не означает наличие
callable метода. Inventory.Scan объединяет пять явно выбранных семейств:
projects, servers, clusters, datastores, registries. Полный SDK требует остальных
сервисных методов. Доступны начальные Logs и Metrics: CloudWatch Groups,
Streams, Events, Filter; Prometheus Scrape, Query и QueryRange. Эти bindings
имеют статус partial; неподтверждённые варианты upstream API перечислены
в каталоге. Все начальные сквозные сценарии теперь имеют callable bindings.
`inventory_complete=false` и `coverage_ratio=null` сохраняются до полного
перечисления всех семейств Selectel.

## Начало работы с Selectel

Для регулярной работы используйте сервисного пользователя Selectel. SDK принимает
его имя и пароль, получает IAM-токены нужной области и обновляет их при следующих
вызовах перед истечением срока. Вручную выпускать API-ключ для этого способа не нужно.
Порядок авторизации описан в [документации Selectel](https://docs.selectel.ru/api/authorization/).

### Какие данные подготовить

| Данные | Где получить и для чего нужны |
| --- | --- |
| Номер аккаунта | В правом верхнем углу панели Selectel. Это `AccountID`, а не ID проекта |
| Имя сервисного пользователя | IAM → Сервисные пользователи. Используется как IAM username |
| Пароль сервисного пользователя | Сохранить при создании пользователя; забытый пароль нужно заменить |
| Разрешения пользователя | Роли и области доступа для нужных операций: аккаунт или выбранные проекты |
| ID проекта | В меню проектов панели Selectel. Нужен для ресурсов проекта; SDK использует ID, а не название |
| Пул и при необходимости регион | Из локации ресурса, например pool=`ru-1`, region=`spb`. Нужны для региональных сервисов |

Пароль и ключи передавайте через локальные переменные окружения, отдельный файл
профиля или хранилище секретов. Они не должны попадать в README, исходники,
Git, сообщения агенту или диагностический вывод. Номер аккаунта, ID проекта и
локация задают область работы, но сами по себе не дают доступа.

### Создать сервисного пользователя и назначить доступ

В панели [my.selectel.ru](https://my.selectel.ru) откройте **IAM → Сервисные
пользователи → Добавить сервисного пользователя**, задайте имя и сгенерируйте
пароль. Назначьте разрешения и сохраните пароль в выбранном хранилище.
Создавать пользователя может владелец аккаунта или пользователь с `iam.admin`.
Подробные шаги: [Добавить сервисного пользователя](https://docs.selectel.ru/access-control/manage/add-user/#add-service-user).

Выберите доступ под задачу по [справочнику ролей](https://docs.selectel.ru/access-control/role-reference/):

- Для первого примера ниже нужен `reader` в области **Аккаунт**: он позволяет
  читать настройки проектов и данные биллинга.
- Для чтения ресурсов одного проекта можно назначить `reader` только в области
  **Проекты** с выбранным проектом. Тогда используйте пример MKS ниже;
  account-scoped чтение проектов и биллинга этим разрешением не открывается.
- Для создания, изменения и удаления ресурсов назначьте подходящие сервисные
  роли или `member` в выбранном проекте. Управление самими проектами требует
  разрешения на уровне аккаунта. SDK не расширяет роли пользователя.

Наличие IAM-токена подтверждает авторизацию, но не гарантирует право на каждый
сервисный вызов. Например, доступ к аудит-логам задаётся отдельно ролью
`audit_logs.admin` на уровне аккаунта; операции с секретами проверяйте по
разрешениям Secrets Manager.

### Установить SDK и передать IAM credentials

Требуется **Go 1.26 или новее**. В существующем Go-проекте:

```sh
go get github.com/festy23/selectel-sdk
```

Для отдельного первого примера создайте рабочий каталог:

```sh
mkdir selectel-sdk-example
cd selectel-sdk-example
go mod init example.com/selectel-sdk-example
go get github.com/festy23/selectel-sdk
```

В этой же сессии терминала задайте данные своего сервисного пользователя.
Следующий блок предназначен для Bash; замените значения в угловых скобках.
Пароль вводится отдельным скрытым вводом, поэтому его нет в тексте команды:

```bash
export SELECTEL_ACCOUNT_ID='<номер-аккаунта>'
export SELECTEL_USERNAME='<имя-сервисного-пользователя>'
unset SELECTEL_IAM_TOKEN SELECTEL_TOKEN_EXPIRES_AT SELECTEL_PROJECT_ID
printf 'Пароль сервисного пользователя: '
IFS= read -r -s SELECTEL_PASSWORD
printf '\n'
export SELECTEL_PASSWORD
```

Для этого примера `SELECTEL_PROJECT_ID` оставлен пустым: credentials привязаны
к аккаунту, а target проекта выбирается отдельно через `client.Project(id)`.
SDK читает env только после явного `credentials.FromEnvironment("SELECTEL_")`.
Он не загружает `.env`, OpenStack RC-файл или AWS-профиль автоматически.

### Первый запрос: чтение списка проектов

Сохраните следующий код в `main.go` рабочего каталога. Он делает реальные
read-only запросы и выводит только число проектов и полноту результата:

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"

    sdk "github.com/festy23/selectel-sdk"
    "github.com/festy23/selectel-sdk/credentials"
    "github.com/festy23/selectel-sdk/pagination"
    "github.com/festy23/selectel-sdk/projects"
    "github.com/festy23/selectel-sdk/types"
)

func main() {
    if err := run(); err != nil {
        fmt.Fprintf(os.Stderr, "SDK error: %s\n", types.ErrorCode(err))
        os.Exit(1)
    }
}

func run() error {
    source, err := credentials.FromEnvironment("SELECTEL_")
    if err != nil {
        return err
    }
    client, err := sdk.New(sdk.Config{
        AccountID: os.Getenv("SELECTEL_ACCOUNT_ID"),
        Credentials: source,
    })
    if err != nil {
        return err
    }
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    result, err := client.Account().Projects().Collect(ctx,
        projects.ListOptions{},
        pagination.Options{MaxItems: 100, MaxPages: 10})
    fmt.Printf("projects=%d complete=%t\n", len(result.Items), result.Complete)
    return err
}
```

```sh
go run .
```

Первый сервисный вызов получает IAM-токен автоматически. `sdk.New` и выбор
scope не обращаются к облаку. Явная предварительная проверка возможна через
`client.Authenticate(ctx, types.AuthRequest)` с выбранными Scope, Mode, Service
и Audience; она не проверяет разрешение на конкретную операцию.

### Выбрать проект и локацию

Для проекта задайте ID и пул. Эти две переменные — входные параметры примера,
credential loader их не читает:

```sh
export SELECTEL_TARGET_PROJECT_ID='<id-проекта>'
export SELECTEL_TARGET_POOL='<пул-ресурса>'
```

В функции `run` первого примера замените вызов `Projects().Collect` и вывод
результата следующим блоком, а импорт `projects` замените на
`"github.com/festy23/selectel-sdk/mks"`:

```go
project, err := client.Project(os.Getenv("SELECTEL_TARGET_PROJECT_ID"))
if err != nil {
    return err
}
location, err := project.Location("", os.Getenv("SELECTEL_TARGET_POOL"))
if err != nil {
    return err
}
result, err := location.MKS().V2.Clusters.Collect(ctx,
    mks.ListOptions{}, pagination.Options{MaxItems: 100, MaxPages: 10})
fmt.Printf("clusters=%d complete=%t\n", len(result.Items), result.Complete)
return err
```

Пустой region допускается для известного SDK пула: например, `ru-1` соответствует
`spb`. Pool — это `ru-1`, а не сегмент вроде `ru-1a`. Для поддерживаемых сервисов
SDK выбирает endpoint из IAM service catalog или встроенного реестра. Для своего
endpoint используется явный `Config.EndpointOverrides` с соответствующей областью
доступа. Список адресов провайдера: [URL API](https://docs.selectel.ru/api/urls/).

### Другие способы доступа: токены, логи и метрики

| Задача / способ | Переменные для `FromEnvironment("SELECTEL_")` | Особенности |
| --- | --- | --- |
| IAM по паролю | `ACCOUNT_ID`, `USERNAME`, `PASSWORD` | Основной способ; SDK получает account/project token и обновляет его по необходимости |
| Готовый IAM-токен | `ACCOUNT_ID`, `IAM_TOKEN`, `TOKEN_EXPIRES_AT`; для project token также `PROJECT_ID` | Срок в RFC3339, например `2026-10-14T12:00:00Z`. Передавайте фактический срок; новую пару token/expiry выдаёт внешний источник |
| Статический API-ключ для Billing | `ACCOUNT_ID`, `STATIC_TOKEN` | Выберите `types.CallOptions{AuthMode: types.StaticToken}`; ключ не заменяет project IAM для VM, MKS или Secrets Manager |
| Логи CloudWatch | `ACCOUNT_ID`, `PROJECT_ID`, `S3_ACCESS_KEY`, `S3_SECRET_KEY` | S3-ключ нужного проекта; вызов через `location.Logs()` |
| Метрики Prometheus | `ACCOUNT_ID`, `PROJECT_ID`, `BASIC_USER`, `BASIC_PASSWORD` | `BASIC_USER` — UID сервисного пользователя, а не его IAM-имя; вызов через `location.Metrics()` |

В таблице перечислены суффиксы; полные имена — например,
`SELECTEL_IAM_TOKEN` и `SELECTEL_S3_SECRET_KEY`. IAM password и IAM token
нельзя задавать одновременно. Внешний IAM-токен действует только в своей
области: project token одного проекта нельзя использовать для другого или для
account Billing. SDK проверяет token через Keystone introspection, но не может
выпустить новый без password source. По документации Selectel IAM-токен живёт
24 часа; используйте `expires_at` выданного токена, а не придуманную дату.

`SELECTEL_PROJECT_ID` ограничивает credential source конкретным проектом.
Он не выбирает target автоматически и блокирует account-scoped методы SDK.
Для Billing или управления проектами используйте отдельный source без project
binding; для project-only доступа сохраните точный binding. S3 и Basic sources
обязательно должны иметь project binding. При нескольких областях доступа можно
использовать разные prefix или именованные профили и отдельные SDK clients.

Готовый тестовый IAM-токен пользователя панели можно получить через
[Профиль → Доступ → IAM-токен](https://docs.selectel.ru/account/manage-keys/#manage-iam-tokens).
Статический API-ключ создаётся там же на вкладке API-ключи. В SDK сейчас он
предусмотрен как явная альтернатива для трёх методов Billing; поддержка такого
ключа провайдером для других API не добавляет их в SDK автоматически.

Для логов выдайте сервисному пользователю S3-ключ нужного проекта по инструкции
[Управлять S3-ключами](https://docs.selectel.ru/account/manage-keys/#manage-s3-keys).
Selectel подтверждает пару Access key / Secret key в
[инструкции SDK клиента логов](https://docs.selectel.ru/logs/tools/sdk-client/).
Для метрик используйте UID и пароль сервисного пользователя с доступом к
проекту (`metrics.admin`, `member` или `reader`), как описано в
[инструкции Prometheus](https://docs.selectel.ru/metrics/tools/prometheus/).
UID можно посмотреть в IAM → Сервисные пользователи, поле UID.

В CI задайте те же переменные через secrets вашего CI; храните IAM, S3 и Basic
материалы в отдельных записях при разных областях доступа. Альтернативы env:
`credentials.FromProfile(path, name)` с явным JSON-профилем или
`credentials.FromKeyring(store, entry)` с адаптером хранилища, переданным caller.
Файл профиля храните вне репозитория с доступом только для владельца (`chmod 600`);
SDK сам не выбирает системный keyring и не проверяет права файла.

### Если первый запрос не работает

| Код SDK | Что проверить |
| --- | --- |
| `invalid_argument` | Непустой `AccountID`, ID проекта, корректные pool/region и параметры вызова |
| `credentials_unavailable` | Имена env и prefix, наличие обеих частей пары, формат expiry; не задан ли одновременно password и IAM token |
| `scope_mismatch` | Совпадение аккаунта и проекта credentials с target; не используется ли project source для account view |
| `unauthenticated` | Верность пароля, активность сервисного пользователя, истечение или отзыв токена |
| `permission_denied` | Роль пользователя и область разрешения для конкретного сервиса и операции |
| `endpoint_unavailable` | Поддержка нужного пула и API generation, IAM catalog или явный endpoint override |
| `transport_error` / `deadline_exceeded` | DNS, исходящий HTTPS, прокси и сетевые ограничения, общий deadline |
| `incomplete` / `response_limit` | Получены частичные данные: проверьте `Complete`, лимиты и continuation |

Не выводите пароль, token или сырые provider responses при поиске причины.
Полезные поля ошибки SDK — Code, HTTPStatus, OperationID и RequestIDs.
Оба варианта первого запроса проверяются локально на компиляцию; реальная
успешная авторизация и доступ к ресурсам требуют проверки в вашем аккаунте Selectel.

## Контракты и использование SDK

Go-модуль находится в корне репозитория. Для агента доступен
[скилл selectelsdk](.agents/skills/selectelsdk/SKILL.md), который описывает
каталог возможностей, доступ, scope, сервисные вызовы и обработку результатов.
Исходящие cloud mutations выполняются в рамках разрешённых пользователем действий.

```go
import (
    "encoding/json"

    sdk "github.com/festy23/selectel-sdk"
    "github.com/festy23/selectel-sdk/catalog"
    "github.com/festy23/selectel-sdk/credentials"
)

// Discovery выполняется без аккаунта, credentials и сетевых запросов.
contracts, err := catalog.Default().List(catalog.Filter{Service: "mks"})
catalogJSON, err := json.Marshal(catalog.Default())

// Источник указан явно. Ядро не выбирает активный профиль или переменные env.
source, err := credentials.FromEnvironment("SELECTEL_")
client, err := sdk.New(sdk.Config{
    AccountID: "YOUR_ACCOUNT_ID",
    Credentials: source,
})
project, err := client.Project("YOUR_PROJECT_ID")
location, err := project.Location("YOUR_REGION", "YOUR_POOL")
```

Каждую ошибку необходимо обработать перед использованием результата.
`New`, выбор scope и чтение каталога не обращаются к облаку. `Authenticate`
получает context и явный `types.AuthRequest`; IAM password source получает
token, external IAM token source проверяет его через Keystone introspection.
Для static/S3/Basic material результат помечается `caller_asserted`.

Для environment source используются только поля с заданным prefix:
ACCOUNT_ID, USERNAME/PASSWORD либо IAM_TOKEN/TOKEN_EXPIRES_AT, PROJECT_ID,
STATIC_TOKEN, S3_ACCESS_KEY/S3_SECRET_KEY, BASIC_USER/BASIC_PASSWORD.
PROJECT_ID является ограничением binding credentials, а не активным target.
IAM password и external IAM token нельзя задавать одновременно.

`FromProfile(path, name)` читает указанную запись JSON, где корневой object
отображает имена профилей в records с полями snake_case. `FromKeyring(store,
entry)` использует предоставленный `credentials.Store`. Чтение выполняется
при Lookup с context. Изменение материала создаёт новую непрозрачную revision.

`ProtectedValue` скрывает bytes в Go formatting, Text и JSON. Для явного доступа
используйте Reveal/CopyTo. SDK не гарантирует обнуление всех копий памяти.

Secrets Manager доступен через `project.Secrets()`: ListMetadata/GetMetadata
читают метаданные, ReadValue/ReadVersion явно получают ProtectedValue.
Create/CreateVersion/Update/Delete/ActivateVersion возвращают Receipt.

Nova доступна через `location.Compute().Servers`. CreateRequest использует
native `servers.CreateOpts` Gophercloud; сеть/порт задаётся явно, одна VM.
Чувствительные inputs задаются отдельными AdminPassword/UserData; UserData
содержит исходные bytes, которые SDK кодирует base64 ровно один раз.
Создание и удаление не повторяются автоматически. Wait отменяет локальное
ожидание; accepted Receipt сохраняется. Recover по ресурсу возвращает
наблюдение, которое не доказывает принятие исходного unknown вызова.

Пагинация ограничена PageSize/MaxItems/MaxPages и общим collection deadline.
При лимите caller получает частичные Items, Complete=false, continuation и
ошибку incomplete. Cursor привязан к scope, endpoint, contract и параметрам.
Чувствительный cursor скрыт в JSON/formatting; только Cursor.Export явно
раскрывает token для сохранения, после чего его можно прочитать json.Unmarshal.

`client.Inventory().Scan(ctx, inventory.Plan)` проверяет весь plan до I/O и
возвращает ordered результаты всех branches, их ошибки и точный Coverage.
403 одной ветки не отменяет остальные; cancel/deadline прекращает новые
requests. Уже полученные данные сохраняются. Protected payloads исключены.

Location допускает pool без region, если он есть в документированном registry:
`project.Location("", "ru-1")` создаёт scope region=spb, pool=ru-1. Ключи
регионов: spb/msk/nsk/tas/alm/nrb. Сегмент пула задаётся отдельно в опциях
соответствующего ресурса и не подставляется вместо pool.

DTO inventory и audit генерируются из pinned schema с сохранением optional
полей и точных integers. Schemaless JSON защищён типом ProtectedJSON;
его Reveal возвращает исходные JSON bytes. Provider не может подменить SDK ref
своим одноимённым полем. Пагинация сохраняет provider totalCount/prev_cursor/count
в Provider/ProviderPages.

`client.Account().Projects()` управляет проектами с account IAM. Nullable
поля изменения различают отсутствие, `types.Null[string]()` и
`types.Value(value)`; явные false и пустые строки сохраняются. Get возвращает
quotas и theme. Изменение использует документированный HTTP 201; отсутствие
проекта в Get/Delete обозначается провайдером HTTP 400 и возвращает not_found.

`project.Registry().V1.Registries` управляет Registry v1. Его sizeLimit/used
сохраняются отдельно от Registry v2. List v1 обходит bare-array ответы до []
или 204; короткая непустая страница не считается концом. V1ListOptions.Offset
принимает точный Integer для явного начального offset. Create/Delete не
повторяются при потере ответа; принятые responses с ошибкой декодирования
сохраняют Receipt и достоверно извлечённые ID.

`location.MKS().V2` предоставляет Clusters, Nodegroups, Nodes, Tasks,
Registries и Configuration. Clusters поддерживает CreateAndWait; Resource/Resume
работают с известными MKS refs, включая parent refs вложенных ресурсов.
Create групп узлов возвращает 204 без новых ID: SDK сохраняет accepted Receipt
и известный parent cluster, не выбирает группы по разнице списков и не ждёт
их готовности через состояние parent. Unknown изменение не повторяется.

Kubeconfig и nodegroup user_data возвращаются защищёнными значениями.
NodegroupCreateSpec.UserData содержит уже закодированный base64 текст;
SDK передаёт его без повторного кодирования. AdditionalSoftware задаётся
явным ProtectedJSON object согласно required полю схемы. Request DTO сохраняют
nil/false/zero/empty arrays/maps и не вставляют server defaults.

Wait подтверждает cluster.active/nodegroup.active, task.done или exact 404
для удаления. Новые states возвращают state_unknown. Свободный текст ошибок
задач защищён; with_error_details отправляется только если задан caller.
API v1 и работоспособность на живом облаке этими проверками не подтверждаются.

`location.Logs()` использует явный S3 credential source, который привязан к
project/pool. Подпись SigV4 выполняет AWS SDK Go v2; default credential chain
и middleware retries не включаются. Events требует Start/End и обходит поток
вперёд. Messages, sequence tokens и cursors возвращаются защищёнными значениями.

`location.Metrics()` использует явный Basic source с ID пользователя и паролем.
Namespace выбирается явно из compute/dedicated/inference. Scrape возвращает
samples в порядке text response; Query и QueryRange читают подтверждённый
Prometheus integration endpoint проекта/namespace. Поддержаны vector/matrix/
scalar/string и native histogram response. Значения и дробные timestamps
сохраняются без float64. При MaxMetricSamples выдаётся prefix с complete=false
и response_limit; MaxJSONBytes ограничивает и JSON, и scrape text.

QueryRange.Step передаётся как точное число секунд. В Go result сохраняется
time.Duration; JSON result использует step_seconds, включая дробную часть.
Upstream admin/write API, неподтверждённые namespaces и другие версии text
protocol не считаются покрытыми этими methods.

## Проверки и каталог

Требуется Go 1.26 или новее. В корне репозитория:

```sh
go generate ./catalog
go generate .
go test -race ./...
go vet ./...
python3 scripts/verify_catalog.py
```

Генерация читает только локальные public schema snapshots в catalog/sources
и не обращается к сети. Manifest создаётся детерминированно; source revisions
представлены SHA-256 исходных файлов. Catalog.List/Describe возвращают копии.
Сервисные методы засчитываются в покрытие только по правилам спецификации.

Первые billing reads доступны через `client.Account().Billing()`. Методы
получают context и `types.CallOptions`. Для статистики обязательны provider keys,
период и группировки. Integer/Money сохраняют точность исходного числа;
неизвестные денежные единицы и timezone не заменяются догадками.

`verify_catalog.py` запускает проверки и сохраняет только hashes/статусы
проверки в catalog/verification.json. Изменение implementation, contract или
evidence файлов сбрасывает устаревшую verification в none при генерации.

Dependabot проверяет Go-зависимости, включая косвенные, и GitHub Actions по
понедельникам в 09:00 по Москве и создаёт PR с обновлениями. Слияние требует
проверки совместимости и успешного CI. Для PR с изменением `go.mod` или `go.sum`
нужно выполнить `go generate .` и `python3 scripts/verify_catalog.py` в ветке PR,
затем закоммитить обновлённые `catalog/manifest.json` и
`catalog/verification.json`: версии зависимостей входят в контрольные суммы
реализации. PR с обновлениями Actions обычно не меняют каталог.
