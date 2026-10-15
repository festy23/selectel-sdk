# AI data API

[#72](https://github.com/festy23/selectel-sdk/issues/72) имеет восемь callable
**partial** bindings и ноль implemented claims. Это узкий набор подтверждённых
entry routes и совместимых wire shapes, а не полная поддержка трёх API.
Полный inventory неизвестен; `coverage_ratio=null`. Control plane, модель,
принадлежность непрозрачного key и preview enrollment не выводятся из data API.

| View / method | Wire | Подтверждённый вход / выбранный response subset |
|---|---|---|
| Account().AIRouter().Chat / StreamChat | POST /chat/completions | model, protected text messages, explicit optional reasoning.effort=low / max_tokens; typed text choices или bounded SSE chunks |
| Account().AIRouter().Images | POST /images/generations | model, protected prompt, explicit n / url или b64_json; protected URL/base64/revised prompt, exact optional usage |
| Project(id).TokenFactory().Chat | POST /chat/completions | model, protected text messages, explicit optional max_tokens; typed text choices |
| Project(id).Inference().Chat | POST /chat/completions | model, protected text messages, explicit optional max_tokens; typed text choices |
| Project(id).Inference().Complete | POST /completions | model, protected prompt, optional exact temperature/max_tokens; protected text choices |
| Project(id).Inference().Embed | POST /embeddings | model, protected string input array, explicit encoding_format=float; protected exact numeric vectors / indexed outputs |
| Project(id).Inference().Rerank | POST /rerank | model, protected query/documents string array; indexed exact protected scores / text-document union |
| Project(id).Inference().Transcribe | multipart POST /audio/transcriptions | model, protected audio/filename, optional language, explicit json/verbose_json; protected typed text/segments/words/token IDs, typed usage |

StreamChat и два transcription/image representations являются режимами этих
операций, не дополнительными provider capabilities. Модель выбирает caller;
её availability не проверялась. Source-unknown API/option не выдаётся за
implemented. В каталоге доступны ссылки на individual request/response/error
contracts и ограничения каждого partial binding.

## Источники и ограничения

`catalog/sources/{ai-router,dedicated-inference,token-factory}.json` содержат
полученные 2026-10-16 factual extracts, URI и SHA-256 полного исходного Markdown.
Это SDK-owned нормализация, не provider OpenAPI. First-party источники:

- [Router overview](https://docs.selectel.ru/en/ai-router/about/about-ai-router/),
  [quickstart](https://docs.selectel.ru/en/ai-router/quickstart/) и
  [masking](https://docs.selectel.ru/en/ai-router/manage/data-masking/).
- [Inference interaction modes](https://docs.selectel.ru/en/dedicated-inference/manage/interaction-modes/)
  и [overview](https://docs.selectel.ru/en/dedicated-inference/about/about-dedicated-inference/).
- [Token Factory quickstart](https://docs.selectel.ru/en/token-factory/quickstart/)
  и [overview](https://docs.selectel.ru/en/token-factory/about/about-token-factory/).

Overview прямо указывает OpenAI-compatible API; Inference mode table ссылается
также на Jina reranker. Совместимые response shapes привязаны к официальному
openai-python `e71d781ac5e7b3f365ac130bf338baaf13ae720d` с лицензией и к
[Jina schema](https://docs.jina.ai/openapi.json) version 2026.10.13.1624
(SHA-256 исходника и замкнутый response excerpt закреплены локально). Это
compatible-wire evidence, не доказательство всех OpenAI/Jina параметров или
конкретной Selectel deployment. Selectel не закрепляет версию vLLM; её не
угадываем. Серверная «async job API» из concurrent client примеров не выводится.

Оставшийся scope принадлежит #72, reconciliation #74 и приемке #76:

- Полный data/control-plane inventory неизвестен. Deployment/key/limit CRUD
  из панельных инструкций не выводится. Требуется официальный public contract.
- Router images подтверждён как route/prompt в masking guide, но полная
  Selectel-specific operation schema/модельная доступность не опубликованы.
  Нормализованный compatible subset не подтверждает весь upstream images API.
- Для каждого продукта широкая upstream surface, multimodal/tool/prediction,
  дополнительные options и model-specific response variants не подтверждены
  целиком. Выбранные text/float/rerank/audio/image representations ограничены
  явно; каталог содержит excluded variants. Control-plane enrollment не заменяем.
- Streaming Router подтверждён напрямую masking guide; его выбранная chunk /
  terminal shape привязана к совместимому источнику. Отдельного first-party
  streaming contract для Token Factory/Dedicated не найдено. Наличие OpenAI
  compatibility и client async примеров не доказывает эти Selectel режимы.
- Token Factory quickstart/overview говорят private preview, тогда как общий
  [index](https://docs.selectel.ru/en/) показывает public preview. SDK не
  предоставляет доступ: caller получает его извне и явно предоставляет key.
  Расхождение остаётся source gap до согласованного first-party контракта.

## Scope, auth и endpoints

Bearer — отдельный protected credential material по [access](access.md):
protocol_specific разрешён только для этих трёх сервисов. Static/IAM/S3/Basic
не подменяют API key. Optional Material.ExpiresAt проверяется локально и
сохраняется в caller_asserted metadata. Истёкший key не отправляется.

Router использует caller-asserted account binding и фиксированный base
`https://api.selectel.ru/aig/v1`; project принадлежность из opaque key не
доказывается. Token Factory требует explicit project, base
`https://preview.tf.selcloud.ru/v1`. Inference требует explicit project и точный
EndpointOverride generation v1; base уже включает `/v1`. Без него возвращается
endpoint_unavailable с NotSent receipt до lookup. Модель, endpoint и key
выбирает caller; host из имени/модели не выводится. Регион не угадывается;
project не добавляется в недокументированный wire header/body.

Override выбирается только по точному service/generation/scope/audience.
Redirect не следует и не переносит Authorization. Нет fallback модели,
провайдера, key, API generation или account/project. Возврат 403 не означает
withdrawn API или отсутствие модели.

## Числа, защита и полнота

Prompt, audio, filename, messages, output text/refusal/model/ID, image URLs и
base64 защищены. Vectors, scores и token IDs имеют защищённые typed wrappers с
точными Number/Integer и caller-owned Reveal. Форматирование/JSON не раскрывают
литералы или byte decimal/hex представление. Image URL не скачивается.
Usage counters сохраняются точно без float64; optional отсутствие не означает
ноль. Duration/tokens usage различается по type; неизвестный union не считается
успешно декодированным. Usage не доказывает итоговую стоимость.

Optional поля не получают business defaults; explicit zero max_tokens
сохраняется. Compatible completion contract допускает zero; верхний context
limit модели не угадывается. MaxJSONBytes ограничивает structured response.
При меньшем количестве embedding/image outputs, чем explicit input/count,
возвращается incomplete с typed prefix и accepted receipt. Повтор не выполняется.
Rerank ordinal проверяется на уникальность/границы входного массива, но не
превращается в resource identity. Protected multipart audio копируется только
через MaxAudioBytes-bound writer; SDK не открывает файлы. Пустой/oversized audio,
неверный filename/format отвергаются NotSent до lookup/HTTP. TransferTimeout и
context ограничивают передачу и ожидание transcription.

## Receipt и ошибки

Все model POST консервативно считаются mutation: принятие может означать
генерацию/потребление. Для Inference это не утверждение оплаты токенов
(документирована оплата ресурсов). Нет automatic replay, включая 401/429/500,
transport loss, redirect или cancellation. 200 сохраняет Accepted даже при
malformed/limited body; NotSent хранит contract revision; неизвестный исход
не повторяется. Completion/model IDs не являются cloud resources. Recovery
unavailable, не создаются ref/task/wait или поиск по имени. Receipt не доказывает
сохранение prompt/output, readiness или прекращение server-side работы.

Общие HTTP ошибки сохраняют status и соответствующий receipt. Router 422
nested error.code guardrail_rejected сохраняется как compiled ProviderCode и
Rejected; JSON приходит до SSE и не вызывает consume. Optional diagnostic body
ограничен: неизвестные коды, raw message и body отбрасываются. Failure этого
необязательного чтения не стирает известный HTTP rejection. x-gateway-request-id
сохраняется только как ограниченный safe request ID. Optional error-body Read также прерывается context/budget через synchronized
once-only Close; 422 сохраняет Rejected, 500 — UnknownOutcome/context cause,
один receipt присутствует и в Result, и в SDKError, без повторного I/O. Pattern
узко переиспользован из reviewed OCI fix 8c3deead (без OCI service).

Compiled internal_error сохраняется также внутри UnknownOutcome без изменения
unknown receipt или запрета replay. Context cancellation сохраняется; никакой raw transport/callback error message не выводится.

## Поток и внутренний seam

StreamChat отправляет stream=true/Accept text/event-stream. Typed chunks
соответствуют pinned ChatCompletionChunk; `[DONE]` подтверждён официальным
`_streaming.py`. Caller задаёт положительные MaxEvents/MaxEventBytes/MaxBytes.
При malformed/EOF без terminal, лимите, отмене или ошибке consume сохраняются
уже переданные events/usage, Complete=false и Accepted. Complete означает конец
выбранного compatible stream, а не готовность cloud resource. Consume получает
защищённые typed поля; ошибка consume возвращает incomplete без её сообщения.
Usage summary не разделяет mutable значения с callback-owned chunk.

DoResponse узко переиспользован из reviewed
[Swift PR96](https://github.com/festy23/selectel-sdk/pull/96), head
8a07c839deabb098f81ac3ea016607736f23b24c: synchronous response decoder, общий
budget/auth/receipt/no-replay, body ownership и Close при context cancellation.
DoJSON сохраняет прежний JSON limit. Swift service не включён. Ранний contract
revision NotSent — reviewed overlap [PR97](https://github.com/festy23/selectel-sdk/pull/97),
379458e4f658b1216137fea4acce5dae6027d320. Future integration request.go должен
сохранить эти изменения, Bearer и Router-only diagnostic decoder совместно с
независимым numeric ErrorProtocol других веток; auth fallback отсутствует.

[Компилируемый пример](../../../examples/ai-chat/example.go) не выполняет сетевые
вызовы автоматически; writer — явная граница раскрытия model content. Все
приведённые проверки offline через public Client и synthetic RoundTripper,
никаких live запросов к моделям, enrollment или cloud changes не выполнялось.
