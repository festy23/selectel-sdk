package support

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
	"unicode/utf8"
)

type TicketFilters struct {
	Q                                 *types.ProtectedValue
	OnlyOpened, OnlyClosed, OnlyAbuse *bool
	AccessLevels                      *[]AccessLevelKeysEnum
	KeystoneProjects                  *[]types.ProtectedValue
}

func filtersQuery(spec TicketFilters) (url.Values, error) {
	query := url.Values{}
	var err error
	if spec.Q != nil {
		if !utf8.Valid(spec.Q.Reveal()) || len(spec.Q.Reveal()) == 0 {
			err = types.NewError(types.InvalidArgument)
		}
		query.Set("q", string(spec.Q.Reveal()))
	}
	for key, v := range map[string]*bool{"is_only_opened": spec.OnlyOpened, "is_only_closed": spec.OnlyClosed, "is_only_abuse": spec.OnlyAbuse} {
		if v != nil {
			query.Set(key, strconv.FormatBool(*v))
		}
	}
	if spec.AccessLevels != nil {
		for _, v := range *spec.AccessLevels {
			if v != "all_users" && v != "account_users" && v != "project_users" {
				err = types.NewError(types.InvalidArgument)
			}
			query.Add("access_levels", string(v))
		}
	}
	if spec.KeystoneProjects != nil {
		for _, v := range *spec.KeystoneProjects {
			if !utf8.Valid(v.Reveal()) || len(v.Reveal()) == 0 {
				err = types.NewError(types.InvalidArgument)
			}
			query.Add("keystone_projects", string(v.Reveal()))
		}
	}
	return query, err
}
func decodeTicketModel[T interface{ Validate() error }](raw []byte, _ types.EndpointBinding) (core.Decoded[T], error) {
	out := core.Decoded[T]{}
	if json.Unmarshal(raw, &out.Data) != nil {
		return out, types.NewError(types.ProtocolError)
	}
	return out, out.Data.Validate()
}
func (c *TicketsClient) Count(ctx context.Context, spec TicketFilters, o types.CallOptions) (types.Result[GetTicketsCountResponse], error) {
	op, bindErr := c.operation("tickets.v2.tickets.count", "GET", []int{400, 404})
	query, queryErr := filtersQuery(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/tickets/count", Query: query, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return queryErr
	}}, decodeTicketModel[GetTicketsCountResponse])
}

type CategoryOptions struct {
	SortBy, SortOrder *string
	OnlyActive        *bool
}
type ProductOptions struct {
	Parent            *types.ProtectedValue
	SortBy, SortOrder *string
}

func sortQuery(sortBy, order *string) (url.Values, error) {
	query := url.Values{}
	var err error
	if sortBy != nil {
		if !utf8.ValidString(*sortBy) || *sortBy == "" {
			err = types.NewError(types.InvalidArgument)
		}
		query.Set("sort_by", *sortBy)
	}
	if order != nil {
		if *order != "asc" && *order != "desc" {
			err = types.NewError(types.InvalidArgument)
		}
		query.Set("sort_by_order", *order)
	}
	return query, err
}
func fixedTickets[T interface{ Validate() error }](c *TicketsClient, ctx context.Context, id, path string, query url.Values, queryErr error, options pagination.Options) (pagination.Collection[T], error) {
	op, bindErr := c.operation(id, "GET", []int{400, 404})
	req := core.Request{Scope: c.scope, Path: path, Query: query, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return queryErr
	}}
	semantic := ""
	if query != nil {
		semantic = query.Encode()
	}
	it, e := core.FixedIterator(c.runtime, op, req, struct{ Path, Query string }{path, semantic}, options, "items", func(row *T, _ types.EndpointBinding) error { return (*row).Validate() })
	if e != nil {
		return pagination.Collection[T]{}, e
	}
	return it.Collect(ctx)
}
func (c *TicketsClient) Categories(ctx context.Context, spec CategoryOptions, options pagination.Options) (pagination.Collection[GetMultipleCategoriesItemResponse], error) {
	query, err := sortQuery(spec.SortBy, spec.SortOrder)
	if spec.OnlyActive != nil {
		query.Set("is_only_active", strconv.FormatBool(*spec.OnlyActive))
	}
	return fixedTickets[GetMultipleCategoriesItemResponse](c, ctx, "tickets.v2.categories.list", "/v2/categories", query, err, options)
}
func (c *TicketsClient) CategoryFields(ctx context.Context, category types.ProtectedValue, options pagination.Options) (pagination.Collection[GetCategoryFieldsItemResponse], error) {
	key := string(category.Reveal())
	var err error
	if !utf8.ValidString(key) || key == "" || key == "." || key == ".." {
		err = types.NewError(types.InvalidArgument)
	}
	return fixedTickets[GetCategoryFieldsItemResponse](c, ctx, "tickets.v2.category_fields.list", "/v2/categories/"+url.PathEscape(key)+"/fields", nil, err, options)
}
func (c *TicketsClient) Products(ctx context.Context, spec ProductOptions, options pagination.Options) (pagination.Collection[GetMultipleProductsItemResponse], error) {
	query, err := sortQuery(spec.SortBy, spec.SortOrder)
	if spec.Parent != nil {
		if !utf8.Valid(spec.Parent.Reveal()) || len(spec.Parent.Reveal()) == 0 {
			err = types.NewError(types.InvalidArgument)
		}
		query.Set("parent", string(spec.Parent.Reveal()))
	}
	return fixedTickets[GetMultipleProductsItemResponse](c, ctx, "tickets.v2.products.list", "/v2/products", query, err, options)
}
