package privatedns

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type ServiceListOptions struct {
	Project string
	types.CallOptions
}
type ZoneListOptions struct {
	Project, Domain string
	types.CallOptions
}
type RecordListOptions struct {
	Type, Domain string
	types.CallOptions
}

func iterator[T any](s *service, operation core.Operation, req core.Request, q url.Values, o pagination.Options, field string, validate func(*T, types.EndpointBinding) error) (*pagination.Iterator[T], error) {
	operation, e := s.runtime.BindDocumented(operation, s.scope)
	if e != nil {
		return nil, e
	}
	req.Query = q
	return core.FixedIterator(s.runtime, operation, req, struct {
		URL, Path string
		Query     url.Values
	}{operation.FixedEndpoint, req.Path, q}, o, field, validate)
}
func (c *Services) Iterate(in ServiceListOptions, o pagination.Options) (*pagination.Iterator[Service], error) {
	q := url.Values{}
	if in.Project != "" {
		q.Set("project", in.Project)
	}
	return iterator(c.service, op("services.list", "GET", 200), core.Request{Scope: c.service.scope, Path: "/services", Options: in.CallOptions, Validate: c.service.scopeValid}, q, o, "services", func(v *Service, ep types.EndpointBinding) error {
		if v.ID == nil || !validID(*v.ID) {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "private_dns", "service", *v.ID)
		return nil
	})
}
func (c *Services) ListPage(ctx context.Context, in ServiceListOptions, o pagination.Options) (pagination.Page[Service], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Page[Service]{}, e
	}
	return it.Next(ctx)
}
func (c *Services) Collect(ctx context.Context, in ServiceListOptions, o pagination.Options) (pagination.Collection[Service], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Collection[Service]{}, e
	}
	return it.Collect(ctx)
}
func (c *Zones) Iterate(in ZoneListOptions, o pagination.Options) (*pagination.Iterator[Zone], error) {
	q := url.Values{}
	if in.Project != "" {
		q.Set("project", in.Project)
	}
	if in.Domain != "" {
		q.Set("domain", in.Domain)
	}
	return iterator(c.service, op("zones.list", "GET", 200), core.Request{Scope: c.service.scope, Path: "/zones", Options: in.CallOptions, Validate: c.service.scopeValid}, q, o, "zones", func(v *Zone, ep types.EndpointBinding) error {
		if v.ID == nil || !validID(*v.ID) {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "private_dns", "zone", *v.ID)
		return nil
	})
}
func (c *Zones) ListPage(ctx context.Context, in ZoneListOptions, o pagination.Options) (pagination.Page[Zone], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Page[Zone]{}, e
	}
	return it.Next(ctx)
}
func (c *Zones) Collect(ctx context.Context, in ZoneListOptions, o pagination.Options) (pagination.Collection[Zone], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Collection[Zone]{}, e
	}
	return it.Collect(ctx)
}
func (c *Records) Iterate(r types.ResourceRef, in RecordListOptions, o pagination.Options) (*pagination.Iterator[RecordSet], error) {
	q := url.Values{}
	if in.Type != "" {
		q.Set("type", in.Type)
	}
	if in.Domain != "" {
		q.Set("domain", in.Domain)
	}
	req := exactRequest(c.service, r, "zone", in.CallOptions)
	req.Path += "/recordset"
	return iterator(c.service, op("records.list", "GET", 200), req, q, o, "records", func(v *RecordSet, _ types.EndpointBinding) error { return nil })
}
func (c *Records) ListPage(ctx context.Context, r types.ResourceRef, in RecordListOptions, o pagination.Options) (pagination.Page[RecordSet], error) {
	it, e := c.Iterate(r, in, o)
	if e != nil {
		return pagination.Page[RecordSet]{}, e
	}
	return it.Next(ctx)
}
func (c *Records) Collect(ctx context.Context, r types.ResourceRef, in RecordListOptions, o pagination.Options) (pagination.Collection[RecordSet], error) {
	it, e := c.Iterate(r, in, o)
	if e != nil {
		return pagination.Collection[RecordSet]{}, e
	}
	return it.Collect(ctx)
}
