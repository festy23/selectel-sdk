package support

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"reflect"
	"strconv"
	"unicode/utf8"
)

type LegacyListOptions struct {
	OnlyOpened, OnlyClosed *bool
	Q                      *types.ProtectedValue
	Page                   *types.Integer
	SortBy, SortOrder      *string
	AccessLevel            *AccessLevel
	KeystoneProjects       *KeystoneProjects
}
type LegacyTicketListing struct {
	pagination.Collection[TicketItem]
	Envelopes []TicketPage `json:"envelopes,omitempty"`
}

func legacyListQuery(spec LegacyListOptions) (url.Values, error) {
	q, err := sortQuery(spec.SortBy, spec.SortOrder)
	if spec.SortBy != nil {
		switch *spec.SortBy {
		case "created_at", "updated_at", "number", "summary", "status":
		default:
			err = types.NewError(types.InvalidArgument)
		}
	}
	for key, v := range map[string]*bool{"only_opened": spec.OnlyOpened, "only_closed": spec.OnlyClosed} {
		if v != nil {
			q.Set(key, strconv.FormatBool(*v))
		}
	}
	if spec.Q != nil {
		raw := spec.Q.Reveal()
		if !utf8.Valid(raw) || len(raw) == 0 {
			err = types.NewError(types.InvalidArgument)
		}
		q.Set("q", string(raw))
	}
	if spec.AccessLevel != nil {
		v := *spec.AccessLevel
		if v != "account_users" && v != "project_users" && v != "all_users" {
			err = types.NewError(types.InvalidArgument)
		}
		q.Set("access_level", string(v))
	}
	if spec.KeystoneProjects != nil {
		encoded, e := wireValue(reflect.ValueOf(*spec.KeystoneProjects))
		if e != nil {
			err = e
		} else if e := validateWireModel(*spec.KeystoneProjects, encoded); e != nil {
			err = e
		}
		for _, v := range *spec.KeystoneProjects {
			q.Add("keystone_projects", string(v))
		}
	}
	q.Set("page", "1")
	if spec.Page != nil {
		if e := int32Value(*spec.Page); e != nil {
			err = e
		}
		n, ok := new(big.Int).SetString(spec.Page.String(), 10)
		if !ok || n.Sign() < 1 {
			err = types.NewError(types.InvalidArgument)
		}
		q.Set("page", spec.Page.String())
	}
	return q, err
}
func (c *LegacyClient) List(ctx context.Context, spec LegacyListOptions, options pagination.Options) (LegacyTicketListing, error) {
	result := LegacyTicketListing{}
	op, bindErr := c.operation("tickets.v1.tickets.list", "GET", []int{400})
	query, queryErr := legacyListQuery(spec)
	if bindErr != nil {
		return result, bindErr
	}
	if queryErr != nil {
		return result, queryErr
	}
	plan, e := c.runtime.PagePlan(op.ID, "v1", c.scope, op.EndpointID, query.Encode())
	if e != nil {
		return result, e
	}
	plan.WireMax = 1000
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[TicketItem], error) {
		pageNumber := query.Get("page")
		if input != "" {
			pageNumber = input
		}
		q := url.Values{}
		for k, v := range query {
			q[k] = append([]string(nil), v...)
		}
		q.Set("page", pageNumber)
		q.Set("items_per_page", strconv.FormatUint(uint64(size), 10))
		out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/tickets", Query: q}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[TicketPage], error) {
			decoded := core.Decoded[TicketPage]{}
			if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Items == nil {
				return decoded, types.NewError(types.ProtocolError)
			}
			if decoded.Data.Page != nil && decoded.Data.Page.String() != pageNumber {
				return decoded, types.NewError(types.ScopeMismatch)
			}
			for i := range *decoded.Data.Items {
				row := &(*decoded.Data.Items)[i]
				if row.Number != nil {
					row.Ref = core.Ref(ep, "tickets", "ticket", row.Number.String())
				}
			}
			return decoded, decoded.Data.Validate()
		})
		source := pagination.SourcePage[TicketItem]{Metadata: out.Metadata, Next: input}
		if e != nil {
			return source, e
		}
		source.Items = *out.Data.Items
		source.Provider.TotalCount = out.Data.ItemCount
		if out.Data.PageCount != nil {
			total, ok := new(big.Int).SetString(out.Data.PageCount.String(), 10)
			current, valid := new(big.Int).SetString(pageNumber, 10)
			if !ok || !valid || total.Sign() < 0 {
				return source, types.NewError(types.ProtocolError)
			}
			if current.Cmp(total) == 0 || total.Sign() == 0 && len(source.Items) == 0 {
				source.Done = true
			} else if current.Cmp(total) < 0 {
				source.Next = new(big.Int).Add(current, big.NewInt(1)).String()
			}
		}
		out.Data.Items = nil
		result.Envelopes = append(result.Envelopes, out.Data)
		return source, nil
	}
	it, e := pagination.New(plan, options, fetch)
	if e != nil {
		return result, e
	}
	result.Collection, e = it.Collect(ctx)
	return result, e
}
