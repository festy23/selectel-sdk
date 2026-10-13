// Package audit queries bounded, account-scoped Selectel audit events.
package audit

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, scope types.Scope) *Client { return &Client{runtime: r, scope: scope} }

type QueryRequest struct {
	Start, End                          time.Time
	EventTypes, ProjectIDs, SourceTypes []string
	Direction                           *string
}

func (c *Client) Iterate(request QueryRequest, options pagination.Options) (*pagination.Iterator[Event], error) {
	if c.scope.ProjectID != "" {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if request.Start.IsZero() || request.End.IsZero() || !request.Start.Before(request.End) {
		return nil, types.NewError(types.InvalidArgument)
	}
	direction := ""
	if request.Direction != nil {
		direction = *request.Direction
		if direction != "forward" && direction != "backward" {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	body := struct {
		Start       string   `json:"event_saved_time_from"`
		End         string   `json:"event_saved_time_to"`
		EventTypes  []string `json:"event_types,omitzero"`
		ProjectIDs  []string `json:"project_ids,omitzero"`
		SourceTypes []string `json:"source_types,omitzero"`
	}{Start: request.Start.UTC().Format(time.RFC3339Nano), End: request.End.UTC().Format(time.RFC3339Nano), EventTypes: appendPreservingNil(request.EventTypes), ProjectIDs: appendPreservingNil(request.ProjectIDs), SourceTypes: appendPreservingNil(request.SourceTypes)}
	raw, e := json.Marshal(body)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	op := core.Operation{ID: "audit.v1.logs.query", Service: "audit", APIGeneration: "v1", Method: "POST", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-audit", FixedEndpoint: "https://api.selectel.ru/audit-logs", Success: []int{200}}
	semantic := struct {
		Body      any
		Direction string
	}{body, direction}
	plan, e := c.runtime.PagePlan(op.ID, "v1", c.scope, op.EndpointID, semantic)
	if e != nil {
		return nil, e
	}
	plan.WireMax = 1000
	fetch := func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[Event], error) {
		query := url.Values{"limit": {strconv.FormatUint(uint64(size), 10)}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		if direction != "" {
			query.Set("dir", direction)
		}
		next := ""
		provider := pagination.ProviderMetadata{}
		decode := func(bytes []byte, endpoint types.EndpointBinding) (core.Decoded[[]Event], error) {
			var response struct {
				Data       []Event `json:"data"`
				Pagination *struct {
					Next     *string        `json:"next_cursor"`
					Previous *string        `json:"prev_cursor"`
					Count    *types.Integer `json:"count"`
				} `json:"pagination"`
			}
			if json.Unmarshal(bytes, &response) != nil || response.Data == nil {
				return core.Decoded[[]Event]{}, types.NewError(types.ProtocolError)
			}
			for i := range response.Data {
				v := &response.Data[i]
				if err := v.Validate(); err != nil {
					return core.Decoded[[]Event]{}, err
				}
				if v.Resource != nil && v.Resource.AccountID != nil && *v.Resource.AccountID != c.scope.AccountID {
					return core.Decoded[[]Event]{}, types.NewError(types.ScopeMismatch)
				}
				if v.EventID != nil {
					v.Ref = core.Ref(endpoint, "audit", "event", *v.EventID)
				}
			}
			if response.Pagination != nil && response.Pagination.Next != nil {
				next = *response.Pagination.Next
			}
			if response.Pagination != nil {
				provider.ReportedCount = response.Pagination.Count
				if response.Pagination.Previous != nil {
					v := types.Protect([]byte(*response.Pagination.Previous))
					provider.PreviousToken = &v
				}
			}
			return core.Decoded[[]Event]{Data: response.Data}, nil
		}
		result, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v1/logs", Body: raw, Query: query}, decode)
		page := pagination.SourcePage[Event]{Items: result.Data, Done: next == "", Metadata: result.Metadata}
		if e == nil {
			page.Provider = provider
		}
		if next != "" {
			v := types.Protect([]byte(next))
			page.SensitiveNext = &v
		}
		return page, e
	}
	return pagination.New(plan, options, fetch)
}
func appendPreservingNil(value []string) []string {
	if value == nil {
		return nil
	}
	return append([]string{}, value...)
}
func (c *Client) Query(ctx context.Context, r QueryRequest, o pagination.Options) (pagination.Collection[Event], error) {
	it, e := c.Iterate(r, o)
	if e != nil {
		return pagination.Collection[Event]{Items: []Event{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Client) ListPage(ctx context.Context, r QueryRequest, o pagination.Options) (pagination.Page[Event], error) {
	it, e := c.Iterate(r, o)
	if e != nil {
		return pagination.Page[Event]{}, e
	}
	return it.Next(ctx)
}
