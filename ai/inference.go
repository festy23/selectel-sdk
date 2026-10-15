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

type CompletionRequest struct {
	Model       string
	Prompt      types.ProtectedValue
	Temperature *types.Number
	MaxTokens   *types.Integer
}

func (c *InferenceClient) operation(resource string) (core.Operation, error) {
	return c.runtime.BindDocumented(core.Operation{ID: "dedicated_inference.v1." + resource + ".create", Service: "dedicated_inference", APIGeneration: "v1", Method: http.MethodPost, Effect: "mutation", Auth: types.ProtocolSpecific, RequireProject: true, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422, 429}, Headers: http.Header{"Accept": {"application/json"}, "Content-Type": {"application/json"}}}, c.scope)
}
func (c *InferenceClient) Complete(ctx context.Context, request CompletionRequest) (types.Result[Completion], error) {
	op, validation := c.operation("completions")
	if strings.TrimSpace(request.Model) == "" {
		validation = types.NewError(types.InvalidArgument)
	}
	if request.MaxTokens != nil {
		n, valid := new(big.Int).SetString(request.MaxTokens.String(), 10)
		if !valid || n.Sign() < 0 {
			validation = types.NewError(types.InvalidArgument)
		}
	}
	wire := struct {
		Model       string         `json:"model"`
		Prompt      string         `json:"prompt"`
		Temperature *types.Number  `json:"temperature,omitempty"`
		MaxTokens   *types.Integer `json:"max_tokens,omitempty"`
	}{request.Model, string(request.Prompt.Reveal()), request.Temperature, request.MaxTokens}
	body, e := json.Marshal(wire)
	if e != nil {
		validation = types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/completions", Body: body, Validate: func() error { return validation }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[Completion], error) {
		var out Completion
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil || *out.Object != "text_completion" || len(out.Choices) == 0 {
			return core.Decoded[Completion]{}, types.NewError(types.ProtocolError)
		}
		for _, choice := range out.Choices {
			switch *choice.FinishReason {
			case "stop", "length", "content_filter":
			default:
				return core.Decoded[Completion]{}, types.NewError(types.ProtocolError)
			}
		}
		return core.Decoded[Completion]{Data: out}, nil
	})
}

type EmbeddingRequest struct {
	Model          string
	Input          []types.ProtectedValue
	EncodingFormat string
}
type EmbeddingResult struct {
	Response EmbeddingsResponse
	Complete bool
}

func (c *InferenceClient) Embed(ctx context.Context, request EmbeddingRequest) (types.Result[EmbeddingResult], error) {
	op, validation := c.operation("embeddings")
	if strings.TrimSpace(request.Model) == "" || len(request.Input) == 0 || request.EncodingFormat != "float" {
		validation = types.NewError(types.InvalidArgument)
	}
	input := make([]string, 0, len(request.Input))
	for _, text := range request.Input {
		input = append(input, string(text.Reveal()))
	}
	wire := struct {
		Model          string   `json:"model"`
		Input          []string `json:"input"`
		EncodingFormat string   `json:"encoding_format"`
	}{request.Model, input, request.EncodingFormat}
	body, e := json.Marshal(wire)
	if e != nil {
		validation = types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/embeddings", Body: body, Validate: func() error { return validation }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[EmbeddingResult], error) {
		var out EmbeddingsResponse
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil || *out.Object != "list" {
			return core.Decoded[EmbeddingResult]{}, types.NewError(types.ProtocolError)
		}
		seen := map[string]bool{}
		for _, embedding := range out.Data {
			index, valid := new(big.Int).SetString(embedding.Index.String(), 10)
			if !valid || index.Sign() < 0 || !index.IsUint64() || index.Uint64() >= uint64(len(request.Input)) || seen[embedding.Index.String()] || *embedding.Object != "embedding" {
				return core.Decoded[EmbeddingResult]{}, types.NewError(types.ProtocolError)
			}
			seen[embedding.Index.String()] = true
		}
		result := EmbeddingResult{Response: out, Complete: len(out.Data) == len(request.Input)}
		if !result.Complete {
			return core.Decoded[EmbeddingResult]{Data: result}, types.NewError(types.Incomplete)
		}
		return core.Decoded[EmbeddingResult]{Data: result}, nil
	})
}

type RerankRequest struct {
	Model     string
	Query     types.ProtectedValue
	Documents []types.ProtectedValue
}

func (c *InferenceClient) Rerank(ctx context.Context, request RerankRequest) (types.Result[RerankResponse], error) {
	op, validation := c.operation("rerank")
	if strings.TrimSpace(request.Model) == "" || len(request.Documents) == 0 {
		validation = types.NewError(types.InvalidArgument)
	}
	documents := make([]string, 0, len(request.Documents))
	for _, document := range request.Documents {
		documents = append(documents, string(document.Reveal()))
	}
	wire := struct {
		Model     string   `json:"model"`
		Query     string   `json:"query"`
		Documents []string `json:"documents"`
	}{request.Model, string(request.Query.Reveal()), documents}
	body, e := json.Marshal(wire)
	if e != nil {
		validation = types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/rerank", Body: body, Validate: func() error { return validation }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[RerankResponse], error) {
		var out RerankResponse
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil || (out.Object != nil && *out.Object != "list") {
			return core.Decoded[RerankResponse]{}, types.NewError(types.ProtocolError)
		}
		seen := map[string]bool{}
		for _, entry := range out.Results {
			index, valid := new(big.Int).SetString(entry.Index.String(), 10)
			if !valid || index.Sign() < 0 || !index.IsUint64() || index.Uint64() >= uint64(len(request.Documents)) || seen[entry.Index.String()] {
				return core.Decoded[RerankResponse]{}, types.NewError(types.ProtocolError)
			}
			seen[entry.Index.String()] = true
		}
		return core.Decoded[RerankResponse]{Data: out}, nil
	})
}
