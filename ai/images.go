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

type ImageRequest struct {
	Model          string
	Prompt         types.ProtectedValue
	N              types.Integer
	ResponseFormat string
}
type ImageResult struct {
	Response ImagesResponse
	Complete bool
}

func (c *RouterClient) Images(ctx context.Context, request ImageRequest) (types.Result[ImageResult], error) {
	op := core.Operation{ID: "ai_router.v1.images_generations.create", Service: "ai_router", APIGeneration: "v1", Method: http.MethodPost, Effect: "mutation", Auth: types.ProtocolSpecific, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422, 429}, ErrorProtocol: "ai_router", FixedEndpoint: "https://api.selectel.ru/aig/v1", EndpointID: "selectel-ai-router-v1", Headers: http.Header{"Accept": {"application/json"}, "Content-Type": {"application/json"}}}
	var validation error
	count, valid := new(big.Int).SetString(request.N.String(), 10)
	if !valid || count.Sign() <= 0 || strings.TrimSpace(request.Model) == "" || request.Prompt.Empty() || (request.ResponseFormat != "url" && request.ResponseFormat != "b64_json") || c.scope.ProjectID != "" {
		validation = types.NewError(types.InvalidArgument)
	}
	wire := struct {
		Model          string        `json:"model"`
		Prompt         string        `json:"prompt"`
		N              types.Integer `json:"n"`
		ResponseFormat string        `json:"response_format"`
	}{request.Model, string(request.Prompt.Reveal()), request.N, request.ResponseFormat}
	body, e := json.Marshal(wire)
	if e != nil {
		validation = types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/images/generations", Body: body, Validate: func() error { return validation }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[ImageResult], error) {
		var out ImagesResponse
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return core.Decoded[ImageResult]{}, types.NewError(types.ProtocolError)
		}
		for _, image := range out.Data {
			if (request.ResponseFormat == "url" && image.URL == nil) || (request.ResponseFormat == "b64_json" && image.B64JSON == nil) {
				return core.Decoded[ImageResult]{}, types.NewError(types.ProtocolError)
			}
		}
		result := ImageResult{Response: out, Complete: count.IsUint64() && uint64(len(out.Data)) == count.Uint64()}
		if !result.Complete {
			return core.Decoded[ImageResult]{Data: result}, types.NewError(types.Incomplete)
		}
		return core.Decoded[ImageResult]{Data: result}, nil
	})
}
