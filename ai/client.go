// Package ai provides source-bound AI data requests. It does not manage model
// deployments, issue API keys, or grant access to preview products.
package ai

import (
	"context"
	"encoding/json"
	"math/big"
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
type Reasoning struct {
	Effort string `json:"effort"`
}
type ChatRequest struct {
	Reasoning *Reasoning     `json:"reasoning,omitempty"`
	MaxTokens *types.Integer `json:"max_tokens,omitempty"`
	Model     string         `json:"model"`
	Messages  []Message      `json:"messages"`
}

type dataClient struct {
	runtime                       *core.Runtime
	scope                         types.Scope
	service, endpoint, endpointID string
	requireProject                bool
}
type TokenFactoryClient struct {
	runtime *core.Runtime
	scope   types.Scope
}

func BindTokenFactory(r *core.Runtime, s types.Scope) *TokenFactoryClient {
	return &TokenFactoryClient{r, s}
}
func (c *TokenFactoryClient) Chat(ctx context.Context, request ChatRequest) (types.Result[ChatCompletion], error) {
	return chat(&dataClient{runtime: c.runtime, scope: c.scope, service: "token_factory", endpoint: "https://preview.tf.selcloud.ru/v1", endpointID: "selectel-token-factory-v1", requireProject: true}, ctx, request)
}
func (c *RouterClient) Chat(ctx context.Context, request ChatRequest) (types.Result[ChatCompletion], error) {
	return chat(&dataClient{runtime: c.runtime, scope: c.scope, service: "ai_router", endpoint: "https://api.selectel.ru/aig/v1", endpointID: "selectel-ai-router-v1"}, ctx, request)
}

func chat(c *dataClient, ctx context.Context, request ChatRequest) (types.Result[ChatCompletion], error) {
	op := core.Operation{ID: c.service + ".v1.chat_completions.create", Service: c.service, APIGeneration: "v1", Method: http.MethodPost, Effect: "mutation", Auth: types.ProtocolSpecific, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422, 429}, FixedEndpoint: c.endpoint, EndpointID: c.endpointID, RequireProject: c.requireProject, Headers: http.Header{"Accept": {"application/json"}, "Content-Type": {"application/json"}}}
	if c.service == "ai_router" {
		op.ErrorProtocol = "ai_router"
	}
	var validation error
	if c.endpoint == "" {
		bound, e := c.runtime.BindDocumented(op, c.scope)
		if e != nil {
			validation = e
		} else {
			op = bound
		}
	}
	body, requestErr := chatBody(c, request, false)
	if requestErr != nil {
		validation = requestErr
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

type InferenceClient struct {
	runtime *core.Runtime
	scope   types.Scope
}

func BindInference(r *core.Runtime, s types.Scope) *InferenceClient { return &InferenceClient{r, s} }
func (c *InferenceClient) Chat(ctx context.Context, request ChatRequest) (types.Result[ChatCompletion], error) {
	return chat(&dataClient{runtime: c.runtime, scope: c.scope, service: "dedicated_inference", requireProject: true}, ctx, request)
}

func chatBody(c *dataClient, request ChatRequest, stream bool) ([]byte, error) {
	var validation error
	type wireMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	wire := struct {
		Model     string         `json:"model"`
		Messages  []wireMessage  `json:"messages"`
		Stream    bool           `json:"stream"`
		Reasoning *Reasoning     `json:"reasoning,omitempty"`
		MaxTokens *types.Integer `json:"max_tokens,omitempty"`
	}{Model: request.Model, Stream: stream, Reasoning: request.Reasoning, MaxTokens: request.MaxTokens, Messages: make([]wireMessage, 0, len(request.Messages))}
	if strings.TrimSpace(request.Model) == "" || len(request.Messages) == 0 || (!c.requireProject && c.scope.ProjectID != "") || (c.requireProject && c.scope.ProjectID == "") {
		validation = types.NewError(types.InvalidArgument)
	}
	if request.MaxTokens != nil {
		n, ok := new(big.Int).SetString(request.MaxTokens.String(), 10)
		if !ok || n.Sign() < 0 {
			validation = types.NewError(types.InvalidArgument)
		}
	}
	if request.Reasoning != nil && (c.service != "ai_router" || request.Reasoning.Effort != "low") {
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

	return body, validation
}
