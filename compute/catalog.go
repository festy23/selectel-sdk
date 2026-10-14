package compute

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

// FlavorSwap preserves Nova's numeric or legacy string swap representation.
type FlavorSwap struct{ types.Integer }

func (s *FlavorSwap) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return types.NewError(types.ProtocolError)
		}
		if text == "" {
			text = "0"
		}
		raw = []byte(text)
	}
	return json.Unmarshal(raw, &s.Integer)
}

type Flavors struct {
	runtime *core.Runtime
	scope   types.Scope
}
type FlavorFilters struct {
	SortKeys, SortDirs                         []string
	MinDisk, MinRAM                            int
	ChangesSince, SortDir, SortKey, AccessType string
}

func catalogOperation(id string) core.Operation {
	op := operation("list", "GET")
	op.ID = "compute.v2_1." + id
	op.Headers.Set("OpenStack-API-Version", "compute 2.93")
	op.Headers.Set("X-OpenStack-Nova-API-Version", "2.93")
	return op
}
func flavorQuery(f FlavorFilters) (url.Values, error) {
	if f.MinDisk < 0 || f.MinRAM < 0 || (f.SortDir != "" && f.SortDir != "asc" && f.SortDir != "desc") || (f.AccessType != "" && f.AccessType != "true" && f.AccessType != "false" && f.AccessType != "None") {
		return nil, types.NewError(types.InvalidArgument)
	}
	if (len(f.SortKeys) > 0 || len(f.SortDirs) > 0) && (f.SortKey != "" || f.SortDir != "") {
		return nil, types.NewError(types.InvalidArgument)
	}
	for _, d := range f.SortDirs {
		if d != "asc" && d != "desc" {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	if len(f.SortDirs) > len(f.SortKeys) {
		return nil, types.NewError(types.InvalidArgument)
	}
	q := url.Values{}
	for _, k := range f.SortKeys {
		if k == "" {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("sort_key", k)
	}
	for _, d := range f.SortDirs {
		q.Add("sort_dir", d)
	}
	for k, v := range map[string]string{"changes-since": f.ChangesSince, "sort_dir": f.SortDir, "sort_key": f.SortKey, "is_public": f.AccessType} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if f.MinDisk != 0 {
		q.Set("minDisk", strconv.Itoa(f.MinDisk))
	}
	if f.MinRAM != 0 {
		q.Set("minRam", strconv.Itoa(f.MinRAM))
	}
	return q, nil
}
func (f *Flavors) Iterate(filters FlavorFilters, o pagination.Options) (*pagination.Iterator[Flavor], error) {
	q, err := flavorQuery(filters)
	if err != nil {
		return nil, err
	}
	op := catalogOperation("flavors.list")
	plan, err := f.runtime.PagePlan(op.ID, "v2_1", f.scope, "", filters)
	if err != nil {
		return nil, err
	}
	plan.DynamicEndpoint = plan.EndpointID == ""
	expected := plan.EndpointID
	if o.Cursor != nil {
		expected = o.Cursor.EndpointID
	}
	fetch := func(ctx context.Context, marker string, size uint) (pagination.SourcePage[Flavor], error) {
		query := url.Values{}
		for k, v := range q {
			query[k] = append([]string(nil), v...)
		}
		query.Set("limit", strconv.FormatUint(uint64(size), 10))
		if marker != "" {
			query.Set("marker", marker)
		}
		next := ""
		result, e := core.DoJSON(f.runtime, ctx, op, core.Request{Scope: f.scope, Path: "/flavors/detail", Query: query, ExpectedEndpointID: expected}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[[]Flavor], error) {
			var body struct {
				Flavors []Flavor                     `json:"flavors"`
				Links   []struct{ Rel, Href string } `json:"flavors_links"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Flavors == nil {
				return core.Decoded[[]Flavor]{}, types.NewError(types.ProtocolError)
			}
			for i := range body.Flavors {
				v := &body.Flavors[i]
				if v.Validate() != nil || v.ID == nil || *v.ID == "" {
					return core.Decoded[[]Flavor]{}, types.NewError(types.ProtocolError)
				}
				v.Ref = core.Ref(endpoint, "compute", "flavor", *v.ID)
			}
			for _, link := range body.Links {
				if link.Rel != "next" {
					continue
				}
				if next != "" {
					return core.Decoded[[]Flavor]{}, types.NewError(types.ProtocolError)
				}
				target, e := url.Parse(link.Href)
				base, _ := url.Parse(strings.TrimRight(endpoint.URL, "/") + "/flavors/detail")
				if e != nil {
					return core.Decoded[[]Flavor]{}, types.NewError(types.ProtocolError)
				}
				target = base.ResolveReference(target)
				if target.Scheme != base.Scheme || target.Host != base.Host || target.EscapedPath() != base.EscapedPath() || target.User != nil || target.Fragment != "" {
					return core.Decoded[[]Flavor]{}, types.NewError(types.ProtocolError)
				}
				next = target.Query().Get("marker")
				if next == "" {
					return core.Decoded[[]Flavor]{}, types.NewError(types.ProtocolError)
				}
			}
			return core.Decoded[[]Flavor]{Data: body.Flavors}, nil
		})
		if e == nil {
			expected = result.Metadata.EndpointID
		}
		return pagination.SourcePage[Flavor]{Items: result.Data, Next: next, Done: next == "", Metadata: result.Metadata}, e
	}
	return pagination.New(plan, o, fetch)
}
func (f *Flavors) Collect(ctx context.Context, filters FlavorFilters, o pagination.Options) (pagination.Collection[Flavor], error) {
	it, e := f.Iterate(filters, o)
	if e != nil {
		return pagination.Collection[Flavor]{Items: []Flavor{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (f *Flavors) ListPage(ctx context.Context, filters FlavorFilters, o pagination.Options) (pagination.Page[Flavor], error) {
	it, e := f.Iterate(filters, o)
	if e != nil {
		return pagination.Page[Flavor]{}, e
	}
	return it.Next(ctx)
}

func validateFlavorRef(ref types.ResourceRef) error {
	if ref.Kind != "flavor" || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func (f *Flavors) Get(ctx context.Context, reference types.ResourceRef, o types.CallOptions) (types.Result[Flavor], error) {
	return core.DoJSON(f.runtime, ctx, catalogOperation("flavors.get"), core.Request{Scope: f.scope, Path: "/flavors/" + url.PathEscape(reference.ID), Refs: []types.ResourceRef{reference}, PinnedEndpoint: pin(reference), Options: o, Validate: func() error { return validateFlavorRef(reference) }}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Flavor], error) {
		var body struct {
			Flavor *Flavor `json:"flavor"`
		}
		if json.Unmarshal(raw, &body) != nil || body.Flavor == nil || body.Flavor.Validate() != nil || body.Flavor.ID == nil || *body.Flavor.ID == "" {
			return core.Decoded[Flavor]{}, types.NewError(types.ProtocolError)
		}
		if *body.Flavor.ID != reference.ID {
			return core.Decoded[Flavor]{}, types.NewError(types.ScopeMismatch)
		}
		body.Flavor.Ref = core.Ref(endpoint, "compute", "flavor", reference.ID)
		return core.Decoded[Flavor]{Data: *body.Flavor}, nil
	})
}
func (f *Flavors) ExtraSpecs(ctx context.Context, reference types.ResourceRef, o types.CallOptions) (types.Result[FlavorExtraSpecs], error) {
	return core.DoJSON(f.runtime, ctx, catalogOperation("flavors_extra_specs.get"), core.Request{Scope: f.scope, Path: "/flavors/" + url.PathEscape(reference.ID) + "/os-extra_specs", Refs: []types.ResourceRef{reference}, PinnedEndpoint: pin(reference), Options: o, Validate: func() error { return validateFlavorRef(reference) }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[FlavorExtraSpecs], error) {
		var body FlavorExtraSpecs
		if json.Unmarshal(raw, &body) != nil || body.ExtraSpecs == nil {
			return core.Decoded[FlavorExtraSpecs]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[FlavorExtraSpecs]{Data: body}, body.Validate()
	})
}
