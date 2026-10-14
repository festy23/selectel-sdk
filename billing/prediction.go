package billing

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

// Prediction returns the provider estimate of remaining runtime in hours.
// It preserves null independently from an absent optional estimate.
func (c *Client) Prediction(ctx context.Context, options types.CallOptions) (types.Result[PredictionResponse], error) {
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[PredictionResponse], error) {
		var v PredictionResponse
		if json.Unmarshal(raw, &v) != nil || v.Data == nil {
			return core.Decoded[PredictionResponse]{}, types.NewError(types.ProtocolError)
		}
		if e := v.Validate(); e != nil {
			return core.Decoded[PredictionResponse]{}, e
		}
		return core.Decoded[PredictionResponse]{Data: v}, nil
	}
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
	return core.DoJSON(c.runtime, ctx, operation("billing.v2.billing_prediction.list", "v2"), core.Request{Scope: c.scope, Path: "/v2/billing/prediction", Options: options, Validate: validate}, decode)
}
