package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type StreamOptions struct {
	MaxEvents               uint
	MaxEventBytes, MaxBytes uint64
}
type StreamSummary struct {
	Events   uint
	Complete bool
	Usage    *Usage
}

// StreamChat delivers protected typed chunks synchronously. Returning an error
// from consume stops this request; no raw body escapes the runtime's ownership.
func (c *RouterClient) StreamChat(ctx context.Context, request ChatRequest, limits StreamOptions, consume func(ChatChunk) error) (types.Result[StreamSummary], error) {
	data := &dataClient{runtime: c.runtime, scope: c.scope, service: "ai_router"}
	body, validation := chatBody(data, request, true)
	if consume == nil || limits.MaxEvents == 0 || limits.MaxEventBytes == 0 || limits.MaxBytes == 0 || limits.MaxEventBytes > limits.MaxBytes || limits.MaxBytes >= uint64(^uint64(0)>>1) || limits.MaxEventBytes >= uint64(^uint(0)>>1)-1 {
		validation = types.NewError(types.InvalidArgument)
	}
	op := core.Operation{ID: "ai_router.v1.chat_completions.create", Service: "ai_router", APIGeneration: "v1", Method: http.MethodPost, Effect: "mutation", Auth: types.ProtocolSpecific, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422, 429}, ErrorProtocol: "ai_router", FixedEndpoint: "https://api.selectel.ru/aig/v1", EndpointID: "selectel-ai-router-v1", Headers: http.Header{"Accept": {"text/event-stream"}, "Content-Type": {"application/json"}}}
	return core.DoResponse(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/chat/completions", Body: body, Transfer: true, Validate: func() error { return validation }}, func(call context.Context, reader io.Reader, _ types.EndpointBinding, _ int, header http.Header) (core.Decoded[StreamSummary], error) {
		var summary StreamSummary
		fail := func(code types.Code) (core.Decoded[StreamSummary], error) {
			return core.Decoded[StreamSummary]{Data: summary}, types.NewError(code)
		}
		media, _, err := mime.ParseMediaType(header.Get("Content-Type"))
		if err != nil || media != "text/event-stream" {
			return fail(types.ProtocolError)
		}
		bounded := &io.LimitedReader{R: reader, N: int64(limits.MaxBytes) + 1}
		scan := bufio.NewScanner(bounded)
		var consumed uint64
		scan.Split(func(data []byte, atEOF bool) (int, []byte, error) {
			advance, token, err := bufio.ScanLines(data, atEOF)
			consumed += uint64(advance)
			if consumed > limits.MaxBytes {
				return 0, nil, types.NewError(types.ResponseLimit)
			}
			return advance, token, err
		})
		scan.Buffer(make([]byte, 0, min(int(limits.MaxEventBytes)+1, 4096)), int(limits.MaxEventBytes)+1)
		var fields []string
		var bytes uint64
		dispatch := func() (bool, error) {
			if len(fields) == 0 {
				return false, nil
			}
			payload := strings.Join(fields, "\n")
			fields = nil
			bytes = 0
			if payload == "[DONE]" {
				summary.Complete = true
				return true, nil
			}
			if summary.Events >= limits.MaxEvents {
				return false, types.NewError(types.ResponseLimit)
			}
			var chunk ChatChunk
			if json.Unmarshal([]byte(payload), &chunk) != nil || chunk.Validate() != nil || *chunk.Object != "chat.completion.chunk" {
				return false, types.NewError(types.ProtocolError)
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Role != nil && *choice.Delta.Role != "assistant" {
					return false, types.NewError(types.ProtocolError)
				}
				if choice.FinishReason != nil {
					switch *choice.FinishReason {
					case "stop", "length", "tool_calls", "content_filter", "function_call":
					default:
						return false, types.NewError(types.ProtocolError)
					}
				}
			}
			summary.Events++
			if chunk.Usage != nil {
				usage := *chunk.Usage
				prompt, completion, total := *usage.PromptTokens, *usage.CompletionTokens, *usage.TotalTokens
				usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens = &prompt, &completion, &total
				summary.Usage = &usage
			}
			if consume(chunk) != nil {
				return false, types.NewError(types.Incomplete)
			}
			return false, nil
		}
		for scan.Scan() {
			if call.Err() != nil {
				return core.Decoded[StreamSummary]{Data: summary}, types.ContextError(call.Err())
			}
			line := scan.Text()
			if line == "" {
				done, e := dispatch()
				if e != nil {
					return core.Decoded[StreamSummary]{Data: summary}, e
				}
				if done {
					return core.Decoded[StreamSummary]{Data: summary}, nil
				}
				continue
			}
			if strings.HasPrefix(line, ":") {
				continue
			}
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			if field == "data" {
				bytes += uint64(len(value)) + 1
				if bytes > limits.MaxEventBytes {
					return fail(types.ResponseLimit)
				}
				fields = append(fields, value)
			}
		}
		if bounded.N <= 0 || scan.Err() == bufio.ErrTooLong || types.ErrorCode(scan.Err()) == types.ResponseLimit {
			return fail(types.ResponseLimit)
		}
		if scan.Err() != nil {
			return fail(types.TransportError)
		}
		// EOF does not prove generation completed without the compiled terminal marker.
		return fail(types.ProtocolError)
	})
}
