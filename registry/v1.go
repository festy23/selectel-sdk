package registry

import (
	"context"
	"encoding/json"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type V1Registries struct {
	runtime *core.Runtime
	scope   types.Scope
}
type CreateRequest struct {
	Name string `json:"name"`
}
type V1ListOptions struct {
	Search    *string        `json:"search,omitempty"`
	SortField *string        `json:"sort_field,omitempty"`
	SortType  *string        `json:"sort_type,omitempty"`
	Offset    *types.Integer `json:"offset,omitempty"`
}

func (c *V1Registries) operation(action, method string) (core.Operation, error) {
	op := core.Operation{ID: "registry.v1.registries." + action, Service: "registry", APIGeneration: "v1", Method: method, Effect: "read", Auth: types.IAMProject, RequireProject: true, Success: []int{200}}
	if method != "GET" {
		op.Effect = "mutation"
		op.Rejected = []int{401, 403}
	}
	if method == "POST" {
		op.Success = []int{201}
		op.Rejected = append(op.Rejected, 400, 409)
	}
	if method == "DELETE" {
		op.Success = []int{204}
	}
	if action == "list" {
		op.Success = []int{200, 204}
		op.NoContentSuccess = []int{204}
	}
	return c.runtime.BindDocumented(op, c.scope)
}
func registryPin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func registryPath(id string) string {
	segment := url.PathEscape(id)
	if id == "." || id == ".." {
		segment = strings.ReplaceAll(id, ".", "%2E")
	}
	return "/v1/registries/" + segment
}
func validateRegistryRef(ref types.ResourceRef) error {
	if ref.Kind != "registry" || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func decodeV1(expectedID string) core.Decoder[V1Registry] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[V1Registry], error) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return core.Decoded[V1Registry]{}, types.NewError(types.ProtocolError)
		}
		id := expectedID
		if field, exists := fields["id"]; exists && string(field) != "null" {
			var responseID string
			if json.Unmarshal(field, &responseID) != nil || responseID == "" {
				return core.Decoded[V1Registry]{}, types.NewError(types.ProtocolError)
			}
			if expectedID != "" && expectedID != responseID {
				return core.Decoded[V1Registry]{}, types.NewError(types.ScopeMismatch)
			}
			id = responseID
		}
		ref := core.Ref(endpoint, "registry", "registry", id)
		if ref == nil {
			return core.Decoded[V1Registry]{}, types.NewError(types.ProtocolError)
		}
		out := core.Decoded[V1Registry]{Refs: []types.ResourceRef{*ref}}
		var value V1Registry
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if err := value.Validate(); err != nil {
			return out, err
		}
		value.Ref = ref
		out.Data = value
		return out, nil
	}
}
func (c *V1Registries) Create(ctx context.Context, request CreateRequest, options types.CallOptions) (types.Result[V1Registry], error) {
	op, bindErr := c.operation("create", "POST")
	body, _ := json.Marshal(request)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if request.Name == "" || !utf8.ValidString(request.Name) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v1/registries", Body: body, Options: options, Validate: validate}, decodeV1(""))
}
func (c *V1Registries) Get(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[V1Registry], error) {
	op, bindErr := c.operation("get", "GET")
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		return validateRegistryRef(ref)
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: registryPath(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: registryPin(ref), Validate: validate}, decodeV1(ref.ID))
}
func (c *V1Registries) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	op, bindErr := c.operation("delete", "DELETE")
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		return validateRegistryRef(ref)
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(strings.TrimSpace(string(raw))) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: registryPath(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: registryPin(ref), Validate: validate}, decode)
}

func (c *V1Registries) Iterate(f V1ListOptions, o pagination.Options) (*pagination.Iterator[V1Registry], error) {
	if (f.SortField != nil && *f.SortField != "name" && *f.SortField != "createdAt" && *f.SortField != "size") || (f.SortType != nil && *f.SortType != "asc" && *f.SortType != "desc") || (f.Search != nil && !utf8.ValidString(*f.Search)) {
		return nil, types.NewError(types.InvalidArgument)
	}
	initial := "0"
	if f.Offset != nil {
		n, ok := new(big.Int).SetString(f.Offset.String(), 10)
		if !ok || n.Sign() < 0 {
			return nil, types.NewError(types.InvalidArgument)
		}
		initial = n.String()
	}
	op, err := c.operation("list", "GET")
	if err != nil {
		return nil, err
	}
	plan, err := c.runtime.PagePlan(op.ID, "v1", c.scope, op.EndpointID, f)
	if err != nil {
		return nil, err
	}
	filters := url.Values{}
	if f.Search != nil {
		filters.Set("search", *f.Search)
	}
	if f.SortField != nil {
		filters.Set("sort_field", *f.SortField)
	}
	if f.SortType != nil {
		filters.Set("sort_type", *f.SortType)
	}
	fetch := func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[V1Registry], error) {
		if cursor == "" {
			cursor = initial
		}
		offset, ok := new(big.Int).SetString(cursor, 10)
		if !ok || offset.Sign() < 0 {
			return pagination.SourcePage[V1Registry]{}, types.NewError(types.InvalidArgument)
		}
		query := url.Values{"limit": {strconv.FormatUint(uint64(size), 10)}, "offset": {offset.String()}}
		for key, values := range filters {
			query[key] = append([]string(nil), values...)
		}
		decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[[]V1Registry], error) {
			items := []V1Registry{}
			if len(strings.TrimSpace(string(raw))) != 0 && (json.Unmarshal(raw, &items) != nil || items == nil) {
				return core.Decoded[[]V1Registry]{}, types.NewError(types.ProtocolError)
			}
			for i := range items {
				if err := items[i].Validate(); err != nil {
					return core.Decoded[[]V1Registry]{}, err
				}
				if items[i].ID != nil {
					items[i].Ref = core.Ref(endpoint, "registry", "registry", *items[i].ID)
				}
			}
			return core.Decoded[[]V1Registry]{Data: items}, nil
		}
		result, err := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v1/registries", Query: query}, decode)
		page := pagination.SourcePage[V1Registry]{Items: result.Data, Metadata: result.Metadata}
		if err == nil {
			page.Done = len(result.Data) == 0
			if !page.Done {
				page.Next = new(big.Int).Add(offset, big.NewInt(int64(len(result.Data)))).String()
			}
		}
		return page, err
	}
	return pagination.New(plan, o, fetch)
}
func (c *V1Registries) ListPage(ctx context.Context, f V1ListOptions, o pagination.Options) (pagination.Page[V1Registry], error) {
	it, err := c.Iterate(f, o)
	if err != nil {
		return pagination.Page[V1Registry]{}, err
	}
	return it.Next(ctx)
}
func (c *V1Registries) Collect(ctx context.Context, f V1ListOptions, o pagination.Options) (pagination.Collection[V1Registry], error) {
	it, err := c.Iterate(f, o)
	if err != nil {
		return pagination.Collection[V1Registry]{Items: []V1Registry{}, StopReason: "error"}, err
	}
	return it.Collect(ctx)
}
