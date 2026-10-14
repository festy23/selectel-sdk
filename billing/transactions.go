package billing

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
)

// TransactionsRequest addresses one caller-selected offset page. Timestamps are
// preserved verbatim, including the source's timezone-free examples.
type TransactionsRequest struct {
	CreatedFrom, CreatedTo string
	Balances               *string
	Offset                 *types.Integer
	Limit                  uint
	WithoutRemoved         *bool
}

// TransactionsPage returns one bounded page without inferring a terminal page:
// the published API has no total count, next cursor or completion marker.
func (c *Client) TransactionsPage(ctx context.Context, request TransactionsRequest, options types.CallOptions) (types.Result[TransactionsResponse], error) {
	endpoint, endpointErr := c.runtime.ExplicitEndpoint("billing", "v2", c.scope)
	op := core.Operation{ID: "billing.v2.billing_transactions.list", Service: "billing", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.StaticToken, EndpointID: endpoint.ID, FixedEndpoint: endpoint.URL, Success: []int{200}}
	q := url.Values{"created_from": {request.CreatedFrom}, "created_to": {request.CreatedTo}, "limit": {strconv.FormatUint(uint64(request.Limit), 10)}}
	if request.Balances != nil {
		q.Set("balances", *request.Balances)
	}
	if request.Offset != nil {
		q.Set("offset", request.Offset.String())
	}
	if request.WithoutRemoved != nil {
		q.Set("without_removed", strconv.FormatBool(*request.WithoutRemoved))
	}
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		if request.Offset != nil {
			parsed, e := types.ParseInteger(request.Offset.String())
			if e != nil || len(parsed.String()) == 0 || parsed.String()[0] == '-' {
				return types.NewError(types.InvalidArgument)
			}
		}
		if request.Limit < 1 || request.Limit > 500 {
			return types.NewError(types.InvalidArgument)
		}
		for _, text := range []string{request.CreatedFrom, request.CreatedTo} {
			var value ProviderTimestamp
			raw, _ := json.Marshal(text)
			if json.Unmarshal(raw, &value) != nil {
				return types.NewError(types.InvalidArgument)
			}
		}
		return endpointErr
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[TransactionsResponse], error) {
		var value TransactionsResponse
		if json.Unmarshal(raw, &value) != nil {
			return core.Decoded[TransactionsResponse]{}, types.NewError(types.ProtocolError)
		}
		limited := len(value.Data) > int(request.Limit)
		if limited {
			value.Data = value.Data[:request.Limit]
		}
		if e := value.Validate(); e != nil {
			return core.Decoded[TransactionsResponse]{}, e
		}
		out := core.Decoded[TransactionsResponse]{Data: value, PartialRead: limited}
		if limited {
			return out, types.NewError(types.ResponseLimit)
		}
		return out, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/billing/transactions", Query: q, Options: options, Validate: validate}, decode)
}
