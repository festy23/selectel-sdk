package filestorage

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"
)

type NetworkFilters struct {
	NameContains        *string `json:"name_contains,omitempty"`
	DescriptionContains *string `json:"description_contains,omitempty"`
}

func (c *Client) ListNetworks(ctx context.Context, filters NetworkFilters, options pagination.Options) (pagination.Collection[ShareNetwork], error) {
	op := core.Operation{ID: "filestorage.v2_73.networks.list", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[ShareNetwork]{}, e
	}
	query := url.Values{}
	for _, pair := range []struct {
		key   string
		value *string
	}{{"name~", filters.NameContains}, {"description~", filters.DescriptionContains}} {
		if pair.value != nil {
			if !utf8.ValidString(*pair.value) {
				return pagination.Collection[ShareNetwork]{}, types.NewError(types.InvalidArgument)
			}
			query.Set(pair.key, *pair.value)
		}
	}
	return offsetCollection(c, ctx, op, core.Request{Scope: c.scope, Path: "/share-networks/detail", Query: query}, filters, options, "share_networks", func(v *ShareNetwork, ep types.EndpointBinding) error {
		if v.ID == nil || *v.ID == "" || v.ProjectID == nil {
			return types.NewError(types.ProtocolError)
		}
		if *v.ProjectID != c.scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if v.Validate() != nil {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "filestorage", "share_network", *v.ID)
		return nil
	})
}

// offsetCollection follows the pinned common.limited offset slicing contract.
// Only an empty page terminates: the server can clamp limit below the request.
// Returned hrefs are never followed and do not select a credential destination.
func offsetCollection[T any](c *Client, ctx context.Context, op core.Operation, request core.Request, semantic any, options pagination.Options, field string, validate func(*T, types.EndpointBinding) error) (pagination.Collection[T], error) {
	plan, e := c.runtime.PagePlan(op.ID, op.APIGeneration, c.scope, op.EndpointID, semantic)
	if e != nil {
		return pagination.Collection[T]{}, e
	}
	fetch := func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[T], error) {
		offset := uint64(0)
		if cursor != "" {
			var e error
			offset, e = strconv.ParseUint(cursor, 10, 64)
			if e != nil {
				return pagination.SourcePage[T]{}, types.NewError(types.InvalidArgument)
			}
		}
		nextRequest := request
		nextRequest.Query = url.Values{}
		for key, values := range request.Query {
			nextRequest.Query[key] = append([]string(nil), values...)
		}
		nextRequest.Query.Set("offset", strconv.FormatUint(offset, 10))
		nextRequest.Query.Set("limit", strconv.FormatUint(uint64(size), 10))
		var reportedCount *types.Integer
		out, e := core.DoJSON(c.runtime, ctx, op, nextRequest, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]T], error) {
			var envelope map[string]json.RawMessage
			var items []T
			if json.Unmarshal(raw, &envelope) != nil || json.Unmarshal(envelope[field], &items) != nil || items == nil {
				return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			if countRaw, present := envelope["count"]; present {
				var count types.Integer
				if json.Unmarshal(countRaw, &count) != nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				reportedCount = &count
			}
			for i := range items {
				if e := validate(&items[i], ep); e != nil {
					return core.Decoded[[]T]{}, e
				}
			}
			return core.Decoded[[]T]{Data: items}, nil
		})
		if e != nil {
			return pagination.SourcePage[T]{Metadata: out.Metadata}, e
		}
		page := pagination.SourcePage[T]{Items: out.Data, Done: len(out.Data) == 0, Metadata: out.Metadata, Provider: pagination.ProviderMetadata{TotalCount: reportedCount}}
		if !page.Done {
			advance := uint64(len(out.Data))
			if offset > ^uint64(0)-advance {
				return pagination.SourcePage[T]{Metadata: out.Metadata}, types.NewError(types.ProtocolError)
			}
			page.Next = strconv.FormatUint(offset+advance, 10)
		}
		return page, nil
	}
	iterator, e := pagination.New(plan, options, fetch)
	if e != nil {
		return pagination.Collection[T]{}, e
	}
	return iterator.Collect(ctx)
}
