# AI data API

Этот адаптер относится к [#72](https://github.com/festy23/selectel-sdk/issues/72).
Полный inventory неизвестен; `coverage_ratio=null`. Документированные data routes
не подтверждают control plane: создание deployments, ключей, лимитов и доступ к
preview выполняются в панели, а публичный CRUD из этого не выводится.

## Источники

Локальные `catalog/sources/ai-router.json`, `dedicated-inference.json` и
`token-factory.json` содержат полученные 2026-10-16 документы с URI и SHA-256.
Это SDK-owned нормализация, а не provider OpenAPI. First-party подтверждение:

- [AI Router overview](https://docs.selectel.ru/en/ai-router/about/about-ai-router/),
  [quickstart](https://docs.selectel.ru/en/ai-router/quickstart/) и
  [masking](https://docs.selectel.ru/en/ai-router/manage/data-masking/).
- [Dedicated Inference interaction modes](https://docs.selectel.ru/en/dedicated-inference/manage/interaction-modes/)
  и [overview](https://docs.selectel.ru/en/dedicated-inference/about/about-dedicated-inference/).
- [Token Factory quickstart](https://docs.selectel.ru/en/token-factory/quickstart/)
  и [overview](https://docs.selectel.ru/en/token-factory/about/about-token-factory/).

First-party overview прямо указывает OpenAI-compatible API. Совместимые response
shapes привязаны к официальному openai-python
`e71d781ac5e7b3f365ac130bf338baaf13ae720d`; это не доказательство поддержки всех
параметров OpenAI или конкретного Selectel deployment. Dedicated Inference
поддерживает пять model-dependent типов; его «async» пример означает конкурентный
клиент с тем же HTTP connection, а не job API.

## Текущий срез

`Account().AIRouter().Chat(ctx, ai.ChatRequest)` отправляет POST
`https://api.selectel.ru/aig/v1/chat/completions`, JSON model/messages/stream=false.
Поддержан text content ролей user/system/assistant/developer. Multipart,
multimodal, tools, prediction и reasoning не входят в этот срез. Streaming
Router описан отдельно ниже.
Модель всегда выбирает caller. Нет автоматического выбора модели/провайдера.
Защищённые content раскрываются только при построении wire body. Ответ содержит
source-bound typed choices, защищённые content/refusal/ID/model и точные Integer
usage counters без float64. `MaxJSONBytes` ограничивает ответ; malformed/limited
200 сохраняет accepted receipt без выдуманной готовности или resource ref.

Bearer — отдельный credential material по [access](access.md). Account binding
Router утверждает source; opaque key не доказывает принадлежность. Endpoint
override применяется только по точному service/generation/scope и audience,
без fallback. Перенаправление не следует и не переносит Authorization.

Все model POST консервативно считаются mutation: принятие запроса может означать
генерацию/потребление. Для Dedicated Inference это не утверждение оплаты токенов
(документирована оплата ресурсов). Автоматического replay нет, в том числе после
transport error, 401, 429 или 500. Receipt не означает серверное сохранение
prompt/output. Completion ID не преобразуется в наблюдаемый cloud resource.
Без документированного recovery API recovery unavailable, неизвестный исход
не повторяется и не восстанавливается поиском по имени.

`Project(id).TokenFactory().Chat` использует подтверждённый preview endpoint
`https://preview.tf.selcloud.ru/v1`. `Project(id).Inference().Chat` требует точного
endpoint override generation v1 (base уже заканчивается `/v1`), иначе возвращает
`endpoint_unavailable` с NotSent receipt до lookup. Модель и ключ выбираются
caller; project не добавляется в недокументированный wire header/body. Binding
ключа project-specific, без вывода принадлежности из непрозрачного key.
Все три Chat используют один text-only response mapping.

Router 422 guardrail_rejected сохраняется как compiled ProviderCode и rejected
receipt. Optional diagnostic JSON ограничен MaxJSONBytes: неизвестные коды и raw
message отбрасываются; failure необязательного чтения не стирает известный HTTP
rejection. x-gateway-request-id включён в ограниченные safe request IDs.

## Открытый source scope

Владельцы оставшихся контрактов: #72, reconciliation #74, приемка #76.
В inventory пока перечислены восемь first-party data routes: Router chat/images,
Dedicated completions/chat/embeddings/rerank/transcriptions, Token Factory chat.
Это не полный denominator. Streaming Router подтвержден masking guide: JSON422
`guardrail_rejected` приходит до SSE. Chunk/terminal/usage связаны с pinned совместимым источником ниже; полная
Selectel-specific event surface неизвестна. Images endpoint указан там же,
но ни schema всех параметров, ни Selectel-specific response не опубликованы.
Для Dedicated endpoint задаётся caller явно; hostname из имени сервиса не выводим.
Token Factory private preview указан в quickstart/overview, тогда как общий index
показывает public preview: доступ SDK не предоставляет, расхождение сохраняется.

## Router stream

`Account().AIRouter().StreamChat(ctx, request, limits, consume)` отправляет тот же
chat route с stream=true и Accept text/event-stream. Это режим одной операции,
а не дополнительная provider capability. Typed chunks соответствуют pinned
OpenAI `ChatCompletionChunk`; terminal marker `[DONE]` подтверждён pinned
официальным `_streaming.py`. Это compatible wire evidence, не live проверка
deployment и не доказательство всех provider-specific events/options.

Caller задаёт положительные MaxEvents/MaxEventBytes/MaxBytes. Лимиты,
TransferTimeout и context ограничивают чтение. При malformed/EOF без `[DONE]`,
лимите, отмене или ошибке consume сохраняются уже переданные events/usage,
Complete=false и accepted receipt; raw body/error текста не сохраняются.
Consume получает защищённые typed поля и раскрывает content явно. Возврат
ошибки прекращает чтение с `incomplete`, без сообщения этой ошибки в diagnostics.
JSON422 guardrail до SSE не вызывает consume. Usage опционален, отсутствие
не означает нулевое потребление. `[DONE]` означает конец совместимого потока,
не readiness ресурса, сохранение данных или гарантию server cancellation.

Внутренний seam DoResponse узко переиспользован из reviewed
[Swift PR96](https://github.com/festy23/selectel-sdk/pull/96), head
8a07c839deabb098f81ac3ea016607736f23b24c: synchronous response decoder, общий
budget/auth/receipt/no-replay, body ownership и Close при context cancellation.
DoJSON остаётся совместимым adapter с прежним JSON limit. Swift service не
включён. Ранний contract revision для NotSent receipts — reviewed overlap
[PR97](https://github.com/festy23/selectel-sdk/pull/97),
379458e4f658b1216137fea4acce5dae6027d320. Future integration request.go должен
сохранить эти изменения, Bearer authorization и Router-only diagnostic decoder
совместно с независимым numeric ErrorProtocol других веток; fallback auth нет.
