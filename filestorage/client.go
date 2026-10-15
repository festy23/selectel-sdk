// Package filestorage binds the source-confirmed Selectel Manila subset to a
// pinned2.73 wire profile. It does not imply complete deployment API support.
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

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(runtime *core.Runtime, scope types.Scope) *Client {
	return &Client{runtime: runtime, scope: scope}
}

type TypeFilters struct {
	IsPublic   *bool             `json:"is_public,omitempty"`
	ExtraSpecs map[string]string `json:"extra_specs,omitempty"`
}

func (c *Client) ListTypes(ctx context.Context, filters TypeFilters, options pagination.Options) (pagination.Collection[ShareType], error) {
	op := core.Operation{ID: "filestorage.v2_73.types.list", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[ShareType]{}, e
	}
	query := url.Values{}
	if filters.IsPublic != nil {
		query.Set("is_public", strconv.FormatBool(*filters.IsPublic))
	}
	if filters.ExtraSpecs != nil {
		for key, value := range filters.ExtraSpecs {
			if !utf8.ValidString(key) || !utf8.ValidString(value) {
				return pagination.Collection[ShareType]{}, types.NewError(types.InvalidArgument)
			}
		}
		raw, e := json.Marshal(filters.ExtraSpecs)
		if e != nil {
			return pagination.Collection[ShareType]{}, types.NewError(types.InvalidArgument)
		}
		query.Set("extra_specs", string(raw))
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/types", Query: query}, filters, options, "share_types", func(v *ShareType, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.ID == nil || *v.ID == "" {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "filestorage", "share_type", *v.ID)
		return nil
	})
	if e != nil {
		return pagination.Collection[ShareType]{}, e
	}
	return iterator.Collect(ctx)
}
