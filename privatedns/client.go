// Package privatedns binds the regional Private DNS API, independently of public DNS/PTR.
package privatedns

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
	"unicode/utf8"
)

type Client struct {
	Services *Services
	Zones    *Zones
	Records  *Records
}
type service struct {
	runtime *core.Runtime
	scope   types.Scope
}
type Services struct{ service *service }
type Zones struct{ service *service }
type Records struct{ service *service }
type Empty struct{}

func Bind(r *core.Runtime, s types.Scope) *Client {
	x := &service{r, s}
	return &Client{Services: &Services{x}, Zones: &Zones{x}, Records: &Records{x}}
}
func op(action, method string, success int) core.Operation {
	effect := "read"
	if method != "GET" {
		effect = "mutation"
	}
	return core.Operation{ID: "private_dns.v1." + action, Service: "private_dns", APIGeneration: "v1", Method: method, Effect: effect, ErrorProtocol: "private-dns", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{success}, Rejected: rejectedStatuses[action], ErrorCodes: map[int]types.Code{400: types.InvalidArgument}}
}
func (s *service) scopeValid() error {
	if s.scope.Validate() != nil || s.scope.ProjectID == "" || s.scope.Pool == "" {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func (s *service) valid(r types.ResourceRef, kind string) error {
	if e := s.scopeValid(); e != nil {
		return e
	}
	if r.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if r.Scope != s.scope || r.Service != "private_dns" || r.APIGeneration != "v1" || r.Kind != kind || r.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	if !validID(r.ID) {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func pin(r types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: r.EndpointID, URL: r.EndpointURL, Scope: r.Scope, APIGeneration: r.APIGeneration, Audience: "selectel"}
}
func decodeIdentity(raw []byte, field, kind, expected string, ep types.EndpointBinding) (json.RawMessage, *types.ResourceRef, error) {
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) != nil {
		return nil, nil, types.NewError(types.ProtocolError)
	}
	obj := env[field]
	var id struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(obj, &id) != nil || id.ID == "" || !validID(id.ID) {
		return nil, nil, types.NewError(types.ProtocolError)
	}
	if expected != "" && id.ID != expected {
		return nil, nil, types.NewError(types.ScopeMismatch)
	}
	return obj, core.Ref(ep, "private_dns", kind, id.ID), nil
}
func serviceDecoder(expected string) core.Decoder[ServiceDetails] {
	return func(raw []byte, ep types.EndpointBinding) (d core.Decoded[ServiceDetails], e error) {
		obj, r, e := decodeIdentity(raw, "service", "service", expected, ep)
		if e != nil {
			return d, e
		}
		d.Refs = []types.ResourceRef{*r}
		if json.Unmarshal(obj, &d.Data) != nil {
			return d, types.NewError(types.ProtocolError)
		}
		d.Data.Ref = r
		return d, nil
	}
}
func uuidValid(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, c := range v {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
func (c *Services) Create(ctx context.Context, in ServiceCreate, o types.CallOptions) (types.Result[ServiceDetails], error) {
	body, e := json.Marshal(in.wire())
	return do(c.service, ctx, op("services.create", "POST", 201), core.Request{Scope: c.service.scope, Path: "/services", Body: body, Options: o, Validate: func() error {
		if e != nil {
			return e
		}
		if e := c.service.scopeValid(); e != nil {
			return e
		}
		if !uuidValid(in.NetworkID) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, serviceDecoder(""))
}
func path(kind, id string) string { return "/" + kind + "/" + url.PathEscape(id) }
func emptyDecoder(_ []byte, _ types.EndpointBinding) (core.Decoded[Empty], error) {
	return core.Decoded[Empty]{}, nil
}
func exactRequest(s *service, r types.ResourceRef, kind string, o types.CallOptions) core.Request {
	return core.Request{Scope: s.scope, Path: path(kind+"s", r.ID), Options: o, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Validate: func() error { return s.valid(r, kind) }}
}
func (c *Services) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[ServiceDetails], error) {
	return do(c.service, ctx, op("services.get", "GET", 200), exactRequest(c.service, r, "service", o), serviceDecoder(r.ID))
}
func (c *Services) Update(ctx context.Context, r types.ResourceRef, in ServiceUpdate, o types.CallOptions) (types.Result[Empty], error) {
	req := exactRequest(c.service, r, "service", o)
	req.Body, _ = json.Marshal(in.wire())
	return do(c.service, ctx, op("services.update", "PUT", 204), req, emptyDecoder)
}
func (c *Services) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return do(c.service, ctx, op("services.delete", "DELETE", 204), exactRequest(c.service, r, "service", o), emptyDecoder)
}
func zoneDecoder(expected string) core.Decoder[ZoneDetails] {
	return func(raw []byte, ep types.EndpointBinding) (d core.Decoded[ZoneDetails], e error) {
		obj, r, e := decodeIdentity(raw, "zone", "zone", expected, ep)
		if e != nil {
			return d, e
		}
		d.Refs = []types.ResourceRef{*r}
		if json.Unmarshal(obj, &d.Data) != nil {
			return d, types.NewError(types.ProtocolError)
		}
		d.Data.Ref = r
		return d, nil
	}
}
func (c *Zones) Create(ctx context.Context, in ZoneCreate, o types.CallOptions) (types.Result[ZoneDetails], error) {
	body, e := json.Marshal(in.wire())
	return do(c.service, ctx, op("zones.create", "POST", 201), core.Request{Scope: c.service.scope, Path: "/zones", Body: body, Options: o, Validate: func() error {
		if e != nil {
			return e
		}
		if e := c.service.scopeValid(); e != nil {
			return e
		}
		if !validText(in.Name, 1, maxZoneName) || !validText(in.Domain, 1, maxZoneDomain) || negative(in.TTL, 0) {
			return types.NewError(types.InvalidArgument)
		}
		if in.Records != nil {
			if len(*in.Records) > maxCreateRecords {
				return types.NewError(types.InvalidArgument)
			}
			for _, v := range *in.Records {
				if e := recordValid(v); e != nil {
					return e
				}
			}
		}
		return nil
	}}, zoneDecoder(""))
}
func (c *Zones) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[ZoneDetails], error) {
	return do(c.service, ctx, op("zones.get", "GET", 200), exactRequest(c.service, r, "zone", o), zoneDecoder(r.ID))
}
func (c *Zones) Update(ctx context.Context, r types.ResourceRef, in ZoneUpdate, o types.CallOptions) (types.Result[ZoneDetails], error) {
	req := exactRequest(c.service, r, "zone", o)
	req.Body, _ = json.Marshal(in.wire())
	prev := req.Validate
	req.Validate = func() error {
		if e := prev(); e != nil {
			return e
		}
		if in.Name != nil && (!validText(*in.Name, 0, maxZoneName)) || negative(in.TTL, 0) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	return do(c.service, ctx, op("zones.update", "PUT", 200), req, zoneDecoder(r.ID))
}
func (c *Zones) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return do(c.service, ctx, op("zones.delete", "DELETE", 204), exactRequest(c.service, r, "zone", o), emptyDecoder)
}
func recordsDecoder(raw []byte, _ types.EndpointBinding) (d core.Decoded[[]RecordSet], e error) {
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env["records"], &d.Data) != nil || d.Data == nil {
		return d, types.NewError(types.ProtocolError)
	}
	return d, nil
}
func (c *Records) Replace(ctx context.Context, r types.ResourceRef, in RecordSetReplace, o types.CallOptions) (types.Result[[]RecordSet], error) {
	req := exactRequest(c.service, r, "zone", o)
	req.Path += "/recordset"
	req.Body, _ = json.Marshal(in.wire())
	prev := req.Validate
	req.Validate = func() error {
		if e := prev(); e != nil {
			return e
		}
		if in.Set != nil {
			if len(*in.Set) > maxReplace {
				return types.NewError(types.InvalidArgument)
			}
			for _, v := range *in.Set {
				if e := recordValid(v); e != nil {
					return e
				}
			}
		}
		if in.Delete != nil {
			if len(*in.Delete) > maxDelete {
				return types.NewError(types.InvalidArgument)
			}
			for _, v := range *in.Delete {
				if v.Type == "" || v.Domain == "" || !validText(v.Domain, 0, maxRecordDomain) {
					return types.NewError(types.InvalidArgument)
				}
			}
		}
		return nil
	}
	return do(c.service, ctx, op("records.replace", "PUT", 200), req, recordsDecoder)
}
func (c *Records) PatchValues(ctx context.Context, r types.ResourceRef, in RecordSetPatch, o types.CallOptions) (types.Result[[]RecordSet], error) {
	req := exactRequest(c.service, r, "zone", o)
	req.Path += "/recordset"
	req.Body, _ = json.Marshal(in.wire())
	prev := req.Validate
	req.Validate = func() error {
		if e := prev(); e != nil {
			return e
		}
		if in.Records != nil {
			for _, v := range *in.Records {
				if v.Delete != nil {
					for _, item := range *v.Delete {
						if !utf8.Valid(item.Reveal()) {
							return types.NewError(types.InvalidArgument)
						}
					}
				}
				if e := recordValid(RecordSetAdd{Type: v.Type, Domain: v.Domain, TTL: v.TTL, Values: v.Add}); e != nil {
					return e
				}
			}
		}
		return nil
	}
	return do(c.service, ctx, op("records.patch", "PATCH", 200), req, recordsDecoder)
}
func recordValid(v RecordSetAdd) error {
	if v.Values != nil {
		for _, item := range *v.Values {
			if !utf8.Valid(item.Reveal()) {
				return types.NewError(types.InvalidArgument)
			}
		}
	}
	if !recordTypes[v.Type] || v.Domain == "" || !validText(v.Domain, 0, maxRecordDomain) || negative(v.TTL, -1) || (v.Values != nil && (len(*v.Values) > maxValues || (v.Type == "CNAME" && len(*v.Values) > 1))) {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}

func do[T any](s *service, ctx context.Context, o core.Operation, req core.Request, decoder core.Decoder[T]) (types.Result[T], error) {
	bound, e := s.runtime.BindDocumented(o, s.scope)
	if e != nil {
		old := req.Validate
		req.Validate = func() error {
			if old != nil {
				if err := old(); err != nil {
					return err
				}
			}
			return e
		}
	}
	return core.DoJSON(s.runtime, ctx, bound, req, decoder)
}

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, "/?#")
}
