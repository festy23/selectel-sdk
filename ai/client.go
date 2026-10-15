// Package ai provides source-bound AI data requests. It does not manage model
// deployments, issue API keys, or grant access to preview products.
package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type RouterClient struct {
	runtime *core.Runtime
	scope   types.Scope
}

func BindRouter(runtime *core.Runtime, scope types.Scope) *RouterClient {
	return &RouterClient{runtime: runtime, scope: scope}
}

type Message struct {
	Role    string               `json:"role"`
	Content types.ProtectedValue `json:"content"`
}
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

func (c *RouterClient) Chat(ctx context.Context, request ChatRequest) (types.Result[ChatCompletion], error) {
	op := core.Operation{ID: "ai_router.v1.chat_completions.create", Service: "ai_router", APIGeneration: "v1", Method: http.MethodPost, Effect: "mutation", Auth: types.ProtocolSpecific, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422, 429}, FixedEndpoint: "https://api.selectel.ru/aig/v1", EndpointID: "selectel-ai-router-v1", Headers: http.Header{"Accept": {"application/json"}, "Content-Type": {"application/json"}}}
	var validation error
	type wireMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	wire := struct {
		Model    string        `json:"model"`
		Messages []wireMessage `json:"messages"`
		Stream   bool          `json:"stream"`
	}{Model: request.Model, Messages: make([]wireMessage, 0, len(request.Messages))}
	if strings.TrimSpace(request.Model) == "" || len(request.Messages) == 0 || c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
		validation = types.NewError(types.InvalidArgument)
	}
	for _, m := range request.Messages {
		if m.Role != "user" && m.Role != "system" && m.Role != "assistant" && m.Role != "developer" {
			validation = types.NewError(types.InvalidArgument)
		}
		wire.Messages = append(wire.Messages, wireMessage{Role: m.Role, Content: string(m.Content.Reveal())})
	}
	body, marshalErr := json.Marshal(wire)
	if marshalErr != nil {
		validation = types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/chat/completions", Body: body, Validate: func() error { return validation }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[ChatCompletion], error) {
		var out ChatCompletion
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil || out.Object == nil || *out.Object != "chat.completion" || len(out.Choices) == 0 {
			return core.Decoded[ChatCompletion]{}, types.NewError(types.ProtocolError)
		}
		for _, choice := range out.Choices {
			if choice.Message.Role == nil || *choice.Message.Role != "assistant" || (choice.Message.Content == nil && choice.Message.Refusal == nil) {
				return core.Decoded[ChatCompletion]{}, types.NewError(types.ProtocolError)
			}
			switch *choice.FinishReason {
			case "stop", "length", "tool_calls", "content_filter", "function_call":
			default:
				return core.Decoded[ChatCompletion]{}, types.NewError(types.ProtocolError)
			}
		}
		return core.Decoded[ChatCompletion]{Data: out}, nil
	})
}
