package dnslegacy

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

// DomainListOptions preserves native offset/limit and filters. No terminal rule
// or continuation token is inferred from array length, even for an empty page.
type DomainListOptions struct {
	Limit, Offset      *types.Integer
	Filter, ChangeDate *types.ProtectedValue
	ShowIPs            *bool
	Tags, SortBy       []types.ProtectedValue
}

func protectedQuery(q url.Values, key string, value *types.ProtectedValue) error {
	if value == nil {
		return nil
	}
	raw := value.Reveal()
	if !utf8.Valid(raw) {
		return types.NewError(types.InvalidArgument)
	}
	q.Set(key, string(raw))
	return nil
}
func listBounds(q url.Values, limit, offset *types.Integer, o pagination.Options) error {
	if o.Cursor != nil {
		return types.NewError(types.CapabilityUnavailable)
	}
	if limit != nil {
		if !decimalID.MatchString(limit.String()) || limit.String() == "0" || o.PageSize != 0 {
			return types.NewError(types.InvalidArgument)
		}
		q.Set("limit", limit.String())
	} else if o.PageSize != 0 {
		q.Set("limit", strconv.FormatUint(uint64(o.PageSize), 10))
	}
	if offset != nil {
		if !decimalID.MatchString(offset.String()) {
			return types.NewError(types.InvalidArgument)
		}
		q.Set("offset", offset.String())
	}
	return nil
}
func repeatedQuery(q url.Values, key string, values []types.ProtectedValue, sort bool) error {
	for _, value := range values {
		raw := value.Reveal()
		if !utf8.Valid(raw) {
			return types.NewError(types.InvalidArgument)
		}
		text := string(raw)
		if sort {
			dot := strings.LastIndexByte(text, '.')
			if dot < 1 || (text[dot+1:] != "asc" && text[dot+1:] != "desc") {
				return types.NewError(types.InvalidArgument)
			}
		}
		q.Add(key, text)
	}
	return nil
}
func (c *Domains) ListPage(ctx context.Context, f DomainListOptions, o pagination.Options) (pagination.Page[Domain], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.secrets.list", Service: "dns", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.StaticToken, Success: []int{200}}, c.scope)
	q := url.Values{}
	queryErr := listBounds(q, f.Limit, f.Offset, o)
	for _, pair := range []struct {
		key   string
		value *types.ProtectedValue
	}{{"filter", f.Filter}, {"change_date", f.ChangeDate}} {
		if e := protectedQuery(q, pair.key, pair.value); queryErr == nil {
			queryErr = e
		}
	}
	if f.ShowIPs != nil {
		q.Set("show_ips", strconv.FormatBool(*f.ShowIPs))
	}
	if e := repeatedQuery(q, "tags", f.Tags, false); queryErr == nil {
		queryErr = e
	}
	if e := repeatedQuery(q, "sort_by", f.SortBy, true); queryErr == nil {
		queryErr = e
	}
	return boundedNativePage(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/", Query: q, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return queryErr
	}}, o, decodeDomain(c.scope, domainTarget{}))
}

// boundedNativePage returns one native array page. It preserves only a validated
// bounded prefix and never creates a cursor without a source continuation rule.
func boundedNativePage[T any](r *core.Runtime, ctx context.Context, op core.Operation, request core.Request, o pagination.Options, decode core.Decoder[T]) (pagination.Page[T], error) {
	if ctx == nil {
		return pagination.Page[T]{}, types.NewError(types.InvalidArgument)
	}
	plan, err := r.PagePlan(op.ID, op.APIGeneration, request.Scope, op.EndpointID, nil)
	if err != nil {
		return pagination.Page[T]{}, err
	}
	maxItems := o.MaxItems
	if maxItems == 0 {
		maxItems = plan.Defaults.MaxItems
	}
	call, cancel := context.WithTimeout(ctx, r.CollectionTimeout())
	defer cancel()
	var itemErr error
	decoded := func(raw []byte, ep types.EndpointBinding) (core.Decoded[pagination.Page[T]], error) {
		out := core.Decoded[pagination.Page[T]]{Data: pagination.Page[T]{Items: []T{}}}
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) != nil || rows == nil {
			return out, types.NewError(types.ProtocolError)
		}
		for _, row := range rows {
			if uint(len(out.Data.Items)) >= maxItems {
				e := types.NewError(types.Incomplete)
				e.Reason = "item_limit"
				itemErr = e
				break
			}
			value, e := decode(row, ep)
			if e != nil {
				itemErr = e
				break
			}
			out.Data.Items = append(out.Data.Items, value.Data)
		}
		return out, nil
	}
	result, err := core.DoJSON(r, call, op, request, decoded)
	page := result.Data
	page.Metadata = result.Metadata
	if err != nil {
		return page, err
	}
	if call.Err() != nil {
		return page, types.ContextError(call.Err())
	}
	if itemErr == nil {
		e := types.NewError(types.Incomplete)
		e.Reason = "terminal_rule_unknown"
		itemErr = e
	}
	if e, ok := itemErr.(*types.SDKError); ok {
		e.OperationID = op.ID
		e.Scope = request.Scope
		e.EndpointID = result.Metadata.EndpointID
		e.HTTPStatus = 200
	}
	return page, itemErr
}

type RecordListOptions struct {
	Limit, Offset                       *types.Integer
	Filter, Content, Search, ChangeDate *types.ProtectedValue
	RecordTypes, SortBy                 []types.ProtectedValue
}

func (c *Records) ListPage(ctx context.Context, parent types.ResourceRef, f RecordListOptions, o pagination.Options) (pagination.Page[Record], error) {
	return c.listPage(ctx, domainTarget{ref: &parent}, f, o)
}
func (c *Records) ListPageByName(ctx context.Context, name types.ProtectedValue, f RecordListOptions, o pagination.Options) (pagination.Page[Record], error) {
	return c.listPage(ctx, domainTarget{name: &name}, f, o)
}
func (c *Records) listPage(ctx context.Context, target domainTarget, f RecordListOptions, o pagination.Options) (pagination.Page[Record], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.records.list", Service: "dns", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.StaticToken, Success: []int{200}}, c.scope)
	path, _, parentErr := target.request(c.scope, op)
	q := url.Values{}
	queryErr := listBounds(q, f.Limit, f.Offset, o)
	for _, pair := range []struct {
		key   string
		value *types.ProtectedValue
	}{{"filter", f.Filter}, {"content", f.Content}, {"search", f.Search}, {"change_date", f.ChangeDate}} {
		if e := protectedQuery(q, pair.key, pair.value); queryErr == nil {
			queryErr = e
		}
	}
	if e := repeatedQuery(q, "record_types", f.RecordTypes, false); queryErr == nil {
		queryErr = e
	}
	if e := repeatedQuery(q, "sort_by", f.SortBy, true); queryErr == nil {
		queryErr = e
	}
	return boundedNativePage(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/records", Query: q, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if parentErr != nil {
			return parentErr
		}
		return queryErr
	}}, o, decodeRecord(c.scope, target, ""))
}
