// Package federations manages account-scoped SAML and OIDC federations.
package federations

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Client struct{ V1 V1Client }
type V1Client struct {
	runtime *core.Runtime
	scope   types.Scope
	SAML    *SAML
	OIDC    *OIDC
}
type SAML struct {
	GroupMappings *GroupMappings
	Certificates  *Certificates
	runtime       *core.Runtime
	scope         types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{V1: V1Client{runtime: r, scope: s, SAML: &SAML{runtime: r, scope: s, Certificates: &Certificates{r, s}, GroupMappings: &GroupMappings{r, s, "saml"}}, OIDC: &OIDC{runtime: r, scope: s, GroupMappings: &GroupMappings{r, s, "oidc"}}}}
}
func operation(resource, action, method string, status int, rejected ...int) core.Operation {
	effect := "mutation"
	if method == "GET" || method == "HEAD" {
		effect = "read"
	}
	return core.Operation{ID: "federations.v1." + resource + "." + action, Service: "federations", APIGeneration: "v1", Method: method, Effect: effect, Auth: types.IAMAccount, EndpointID: "selectel-federations", FixedEndpoint: "https://api.selectel.ru", Success: []int{status}, Rejected: rejected, NoContentSuccess: []int{204}}
}
func segment(id string) string {
	v := url.PathEscape(id)
	if id == "." || id == ".." {
		v = strings.ReplaceAll(id, ".", "%2E")
	}
	return v
}
func path(protocol, id string) string { return "/v1/federations/" + protocol + "/" + segment(id) }
func request(scope types.Scope, ref types.ResourceRef, kind, path string, body []byte, o types.CallOptions) core.Request {
	return core.Request{Scope: scope, Path: path, Body: body, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if ref.Kind != kind || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}
}
func decode[T any](kind, expected string, assign func(*T, *types.ResourceRef), validate func(T, types.EndpointBinding) error) core.Decoder[T] {
	return func(raw []byte, e types.EndpointBinding) (core.Decoded[T], error) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		if value, exists := fields["account_id"]; exists && string(value) != "null" {
			var account string
			if json.Unmarshal(value, &account) != nil {
				return core.Decoded[T]{}, types.NewError(types.ProtocolError)
			}
			if account != e.Scope.AccountID {
				return core.Decoded[T]{}, types.NewError(types.ScopeMismatch)
			}
		}
		id := expected
		if v, ok := fields["id"]; ok {
			var actual string
			if json.Unmarshal(v, &actual) != nil || actual == "" {
				return core.Decoded[T]{}, types.NewError(types.ProtocolError)
			}
			if expected != "" && actual != expected {
				return core.Decoded[T]{}, types.NewError(types.ScopeMismatch)
			}
			id = actual
		}
		if id == "" {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(e, "federations", kind, id)
		out := core.Decoded[T]{}
		assign(&out.Data, ref)
		out.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		assign(&out.Data, ref)
		if err := validate(out.Data, e); err != nil {
			return out, err
		}
		return out, nil
	}
}
func empty(_ []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
	return core.Decoded[struct{}]{}, nil
}
func checkAccount(id *string, e types.EndpointBinding) error {
	if id != nil && *id != e.Scope.AccountID {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func validateSAML(v SAMLFederation, e types.EndpointBinding) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return checkAccount(v.AccountID, e)
}
func (s *SAML) Iterate(o pagination.Options) (*pagination.Iterator[SAMLFederation], error) {
	return core.FixedIterator(s.runtime, operation("saml", "list", "GET", 200), core.Request{Scope: s.scope, Path: "/v1/federations/saml"}, nil, o, "federations", func(v *SAMLFederation, e types.EndpointBinding) error {
		if err := validateSAML(*v, e); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(e, "federations", "saml_federation", *v.ID)
		}
		return nil
	})
}
func (s *SAML) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[SAMLFederation], error) {
	it, e := s.Iterate(o)
	if e != nil {
		return pagination.Page[SAMLFederation]{}, e
	}
	return it.Next(ctx)
}
func (s *SAML) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[SAMLFederation], error) {
	it, e := s.Iterate(o)
	if e != nil {
		return pagination.Collection[SAMLFederation]{Items: []SAMLFederation{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (s *SAML) Create(ctx context.Context, in CreateSAMLRequest, o types.CallOptions) (types.Result[SAMLFederation], error) {
	body, e := json.Marshal(in)
	if e != nil {
		return types.Result[SAMLFederation]{}, types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(s.runtime, ctx, operation("saml", "create", "POST", 200, 400, 401, 403), core.Request{Scope: s.scope, Path: "/v1/federations/saml", Body: body, Options: o, Validate: func() error {
		if in.Name == nil || *in.Name == "" || in.Issuer == nil || *in.Issuer == "" || in.SSOURL == nil || *in.SSOURL == "" {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, decode("saml_federation", "", func(v *SAMLFederation, r *types.ResourceRef) { v.Ref = r }, validateSAML))
}
func (s *SAML) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[SAMLFederation], error) {
	return core.DoJSON(s.runtime, ctx, operation("saml", "get", "GET", 200), request(s.scope, ref, "saml_federation", path("saml", ref.ID), nil, o), decode("saml_federation", ref.ID, func(v *SAMLFederation, r *types.ResourceRef) { v.Ref = r }, validateSAML))
}
func (s *SAML) Update(ctx context.Context, ref types.ResourceRef, in UpdateSAMLRequest, o types.CallOptions) (types.Result[struct{}], error) {
	body, e := json.Marshal(in)
	if e != nil {
		return types.Result[struct{}]{}, types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(s.runtime, ctx, operation("saml", "update", "PATCH", 204, 401, 403, 404), request(s.scope, ref, "saml_federation", path("saml", ref.ID), body, o), empty)
}
func (s *SAML) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(s.runtime, ctx, operation("saml", "delete", "DELETE", 204, 401, 403, 404), request(s.scope, ref, "saml_federation", path("saml", ref.ID), nil, o), empty)
}

type OIDC struct {
	GroupMappings *GroupMappings
	runtime       *core.Runtime
	scope         types.Scope
}

func validateOIDC(v OIDCFederation, e types.EndpointBinding) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return checkAccount(v.AccountID, e)
}
func bodyWithProtected(value any, key string, secret *types.ProtectedValue) ([]byte, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if secret == nil {
		return raw, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	fields[key], e = json.Marshal(string(secret.Reveal()))
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return json.Marshal(fields)
}
func (s *OIDC) Iterate(o pagination.Options) (*pagination.Iterator[OIDCFederation], error) {
	return core.FixedIterator(s.runtime, operation("oidc", "list", "GET", 200), core.Request{Scope: s.scope, Path: "/v1/federations/oidc"}, nil, o, "federations", func(v *OIDCFederation, e types.EndpointBinding) error {
		if err := validateOIDC(*v, e); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(e, "federations", "oidc_federation", *v.ID)
		}
		return nil
	})
}
func (s *OIDC) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[OIDCFederation], error) {
	it, e := s.Iterate(o)
	if e != nil {
		return pagination.Page[OIDCFederation]{}, e
	}
	return it.Next(ctx)
}
func (s *OIDC) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[OIDCFederation], error) {
	it, e := s.Iterate(o)
	if e != nil {
		return pagination.Collection[OIDCFederation]{Items: []OIDCFederation{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (s *OIDC) Create(ctx context.Context, in CreateOIDCRequest, o types.CallOptions) (types.Result[OIDCFederation], error) {
	body, e := bodyWithProtected(in, "client_secret", in.ClientSecret)
	if e != nil {
		return types.Result[OIDCFederation]{}, e
	}
	return core.DoJSON(s.runtime, ctx, operation("oidc", "create", "POST", 200, 400, 401, 403), core.Request{Scope: s.scope, Path: "/v1/federations/oidc", Body: body, Options: o, Validate: func() error {
		if in.Validate() != nil || !validOIDCCreate(in) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, decode("oidc_federation", "", func(v *OIDCFederation, r *types.ResourceRef) { v.Ref = r }, validateOIDC))
}
func (s *OIDC) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[OIDCFederation], error) {
	return core.DoJSON(s.runtime, ctx, operation("oidc", "get", "GET", 200), request(s.scope, ref, "oidc_federation", path("oidc", ref.ID), nil, o), decode("oidc_federation", ref.ID, func(v *OIDCFederation, r *types.ResourceRef) { v.Ref = r }, validateOIDC))
}
func (s *OIDC) Update(ctx context.Context, ref types.ResourceRef, in UpdateOIDCRequest, o types.CallOptions) (types.Result[struct{}], error) {
	var secret *types.ProtectedValue
	if in.ClientSecret != nil {
		if value, present := in.ClientSecret.Value(); present {
			secret = &value
		}
	}
	body, e := bodyWithProtected(in, "client_secret", secret)
	if e != nil {
		return types.Result[struct{}]{}, e
	}
	req := request(s.scope, ref, "oidc_federation", path("oidc", ref.ID), body, o)
	v := req.Validate
	req.Validate = func() error {
		if !validOIDCUpdate(in) {
			return types.NewError(types.InvalidArgument)
		}
		return v()
	}
	return core.DoJSON(s.runtime, ctx, operation("oidc", "update", "PATCH", 204, 400, 401, 403, 404), req, empty)
}
func (s *OIDC) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(s.runtime, ctx, operation("oidc", "delete", "DELETE", 204, 401, 403, 404), request(s.scope, ref, "oidc_federation", path("oidc", ref.ID), nil, o), empty)
}

func publicOperation(resource, action, method string) core.Operation {
	op := operation(resource, action, method, 200)
	op.Auth = types.ProtocolSpecific
	if method == "HEAD" {
		op.NoContentSuccess = []int{200}
	}
	return op
}
func (s *SAML) Exists(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(s.runtime, ctx, publicOperation("saml", "exists", "HEAD"), request(s.scope, ref, "saml_federation", path("saml", ref.ID), nil, o), empty)
}
func (s *OIDC) Exists(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(s.runtime, ctx, authenticatedExistence("oidc"), request(s.scope, ref, "oidc_federation", path("oidc", ref.ID), nil, o), empty)
}
func (c V1Client) Preview(ctx context.Context, idOrAlias string, o types.CallOptions) (types.Result[Preview], error) {
	return core.DoJSON(c.runtime, ctx, publicOperation("preview", "get", "GET"), core.Request{Scope: c.scope, Path: "/v1/federations/" + segment(idOrAlias) + "/preview", Options: o, Validate: func() error {
		if idOrAlias == "" {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[Preview], error) {
		var v Preview
		if json.Unmarshal(raw, &v) != nil || v.ID == nil || *v.ID == "" || v.Validate() != nil {
			return core.Decoded[Preview]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[Preview]{Data: v}, nil
	})
}

func authenticatedExistence(protocol string) core.Operation {
	op := operation(protocol, "exists", "HEAD", 200)
	op.NoContentSuccess = []int{200}
	return op
}

type Certificates struct {
	runtime *core.Runtime
	scope   types.Scope
}

func certificatePath(ref types.ResourceRef) string {
	if ref.Parent == nil {
		return path("saml", "") + "/certificates/" + segment(ref.ID)
	}
	return path("saml", ref.Parent.ID) + "/certificates/" + segment(ref.ID)
}
func certificateRequest(scope types.Scope, ref types.ResourceRef, body []byte, o types.CallOptions) core.Request {
	req := request(scope, ref, "saml_certificate", certificatePath(ref), body, o)
	req.Validate = func() error {
		if ref.Kind != "saml_certificate" || ref.Parent == nil || ref.Parent.Kind != "saml_federation" || ref.Parent.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		if ref.Parent.Service != ref.Service || ref.Parent.APIGeneration != ref.APIGeneration || ref.Parent.EndpointID != ref.EndpointID || ref.Parent.EndpointURL != ref.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
	return req
}
func validateCertificate(v Certificate, e types.EndpointBinding, parent types.ResourceRef) error {
	if v.ID != nil && *v.ID == "" {
		return types.NewError(types.ProtocolError)
	}
	if err := v.Validate(); err != nil {
		return err
	}
	if err := checkAccount(v.AccountID, e); err != nil {
		return err
	}
	if v.FederationID != nil && *v.FederationID != parent.ID {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func certificateDecoder(parent types.ResourceRef, expected string) core.Decoder[Certificate] {
	base := decode("saml_certificate", expected, func(v *Certificate, r *types.ResourceRef) { copy := parent; r.Parent = &copy; v.Ref = r }, func(v Certificate, e types.EndpointBinding) error { return validateCertificate(v, e, parent) })
	return func(raw []byte, e types.EndpointBinding) (core.Decoded[Certificate], error) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return core.Decoded[Certificate]{}, types.NewError(types.ProtocolError)
		}
		if value, exists := fields["federation_id"]; exists && string(value) != "null" {
			var id string
			if json.Unmarshal(value, &id) != nil {
				return core.Decoded[Certificate]{}, types.NewError(types.ProtocolError)
			}
			if id != parent.ID {
				return core.Decoded[Certificate]{}, types.NewError(types.ScopeMismatch)
			}
		}
		return base(raw, e)
	}

}
func (c *Certificates) Iterate(parent types.ResourceRef, o pagination.Options) (*pagination.Iterator[Certificate], error) {
	req := request(c.scope, parent, "saml_federation", path("saml", parent.ID)+"/certificates", nil, types.CallOptions{})
	if parent.Validate() != nil || parent.Scope != c.scope || parent.Service != "federations" || parent.APIGeneration != "v1" {
		return nil, types.NewError(types.ScopeMismatch)
	}
	return core.FixedIterator(c.runtime, operation("saml_certificates", "list", "GET", 200), req, parent, o, "certificates", func(v *Certificate, e types.EndpointBinding) error {
		if err := validateCertificate(*v, e, parent); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(e, "federations", "saml_certificate", *v.ID)
			copy := parent
			v.Ref.Parent = &copy
		}
		return nil
	})
}
func (c *Certificates) ListPage(ctx context.Context, parent types.ResourceRef, o pagination.Options) (pagination.Page[Certificate], error) {
	it, e := c.Iterate(parent, o)
	if e != nil {
		return pagination.Page[Certificate]{}, e
	}
	return it.Next(ctx)
}
func (c *Certificates) Collect(ctx context.Context, parent types.ResourceRef, o pagination.Options) (pagination.Collection[Certificate], error) {
	it, e := c.Iterate(parent, o)
	if e != nil {
		return pagination.Collection[Certificate]{Items: []Certificate{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Certificates) Create(ctx context.Context, parent types.ResourceRef, in CreateCertificateRequest, o types.CallOptions) (types.Result[Certificate], error) {
	body, e := bodyWithProtected(in, "data", in.Data)
	if e != nil {
		return types.Result[Certificate]{}, e
	}
	req := request(c.scope, parent, "saml_federation", path("saml", parent.ID)+"/certificates", body, o)
	validation := req.Validate
	req.Validate = func() error {
		if in.Validate() != nil || in.Data == nil || in.Data.Empty() {
			return types.NewError(types.InvalidArgument)
		}
		return validation()
	}
	return core.DoJSON(c.runtime, ctx, operation("saml_certificates", "create", "POST", 200, 401, 403, 404), req, certificateDecoder(parent, ""))
}
func (c *Certificates) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Certificate], error) {
	parent := types.ResourceRef{}
	if ref.Parent != nil {
		parent = *ref.Parent
	}
	return core.DoJSON(c.runtime, ctx, operation("saml_certificates", "get", "GET", 200), certificateRequest(c.scope, ref, nil, o), certificateDecoder(parent, ref.ID))
}
func (c *Certificates) Update(ctx context.Context, ref types.ResourceRef, in UpdateCertificateRequest, o types.CallOptions) (types.Result[Certificate], error) {
	body, e := json.Marshal(in)
	if e != nil {
		return types.Result[Certificate]{}, types.NewError(types.InvalidArgument)
	}
	parent := types.ResourceRef{}
	if ref.Parent != nil {
		parent = *ref.Parent
	}
	return core.DoJSON(c.runtime, ctx, operation("saml_certificates", "update", "PATCH", 200, 401, 403, 404), certificateRequest(c.scope, ref, body, o), certificateDecoder(parent, ref.ID))
}
func (c *Certificates) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(c.runtime, ctx, operation("saml_certificates", "delete", "DELETE", 204, 401, 403, 404), certificateRequest(c.scope, ref, nil, o), empty)
}

type GroupMappings struct {
	runtime  *core.Runtime
	scope    types.Scope
	protocol string
}

var groupIDPattern = regexp.MustCompile(`^[A-z0-9-]{1,64}$`)

func validMapping(internal, external string) bool {
	return groupIDPattern.MatchString(internal) && utf8.ValidString(external) && utf8.RuneCountInString(external) >= 1 && utf8.RuneCountInString(external) <= 255
}
func (g *GroupMappings) Target(parent types.ResourceRef, internal, external string) (types.ResourceRef, error) {
	if parent.Validate() != nil || parent.Kind != g.protocol+"_federation" || parent.Parent != nil {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	if parent.Scope != g.scope || parent.Service != "federations" || parent.APIGeneration != "v1" {
		return types.ResourceRef{}, types.NewError(types.ScopeMismatch)
	}
	if !validMapping(internal, external) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	group := parent
	group.Kind = g.protocol + "_mapping_group"
	group.ID = internal
	copy := parent
	group.Parent = &copy
	ref := group
	ref.Kind = g.protocol + "_group_mapping"
	ref.ID = external
	ref.Parent = &group
	return ref, nil
}
func (g *GroupMappings) validateItem(v *GroupMapping, e types.EndpointBinding, parent types.ResourceRef) error {
	if v.Validate() != nil || v.InternalGroupID == nil || v.ExternalGroupID == nil || !validMapping(*v.InternalGroupID, *v.ExternalGroupID) {
		return types.NewError(types.ProtocolError)
	}
	ref, err := g.Target(parent, *v.InternalGroupID, *v.ExternalGroupID)
	if err != nil {
		return err
	}
	v.Ref = &ref
	return nil
}
func (g *GroupMappings) Iterate(parent types.ResourceRef, o pagination.Options) (*pagination.Iterator[GroupMapping], error) {
	req := request(g.scope, parent, g.protocol+"_federation", path(g.protocol, parent.ID)+"/group-mappings", nil, types.CallOptions{})
	if parent.Validate() != nil || parent.Scope != g.scope || parent.Service != "federations" || parent.APIGeneration != "v1" {
		return nil, types.NewError(types.ScopeMismatch)
	}
	return core.FixedIterator(g.runtime, operation(g.protocol+"_group_mappings", "list", "GET", 200), req, parent, o, "group_mappings", func(v *GroupMapping, e types.EndpointBinding) error { return g.validateItem(v, e, parent) })
}
func (g *GroupMappings) ListPage(ctx context.Context, parent types.ResourceRef, o pagination.Options) (pagination.Page[GroupMapping], error) {
	it, e := g.Iterate(parent, o)
	if e != nil {
		return pagination.Page[GroupMapping]{}, e
	}
	return it.Next(ctx)
}
func (g *GroupMappings) Collect(ctx context.Context, parent types.ResourceRef, o pagination.Options) (pagination.Collection[GroupMapping], error) {
	it, e := g.Iterate(parent, o)
	if e != nil {
		return pagination.Collection[GroupMapping]{Items: []GroupMapping{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}

// groupMappingsBody projects public DTOs onto the provider's request schema.
// Listed mappings also carry SDK refs, which are never provider request fields.
func groupMappingsBody(in GroupMappingsRequest) ([]byte, error) {
	type wireMapping struct {
		InternalGroupID *string `json:"internal_group_id,omitempty"`
		ExternalGroupID *string `json:"external_group_id,omitempty"`
	}
	var mappings []wireMapping
	if in.GroupMappings != nil {
		mappings = make([]wireMapping, len(in.GroupMappings))
		for i, item := range in.GroupMappings {
			mappings[i] = wireMapping{InternalGroupID: item.InternalGroupID, ExternalGroupID: item.ExternalGroupID}
		}
	}
	return json.Marshal(struct {
		GroupMappings []wireMapping `json:"group_mappings"`
	}{GroupMappings: mappings})
}
func (g *GroupMappings) Replace(ctx context.Context, parent types.ResourceRef, in GroupMappingsRequest, o types.CallOptions) (types.Result[GroupMappingsResponse], error) {
	body, e := groupMappingsBody(in)
	if e != nil {
		return types.Result[GroupMappingsResponse]{}, types.NewError(types.InvalidArgument)
	}
	req := request(g.scope, parent, g.protocol+"_federation", path(g.protocol, parent.ID)+"/group-mappings", body, o)
	v := req.Validate
	req.Validate = func() error {
		if in.GroupMappings == nil || len(in.GroupMappings) > 100 {
			return types.NewError(types.InvalidArgument)
		}
		for _, m := range in.GroupMappings {
			if m.InternalGroupID == nil || m.ExternalGroupID == nil || !validMapping(*m.InternalGroupID, *m.ExternalGroupID) {
				return types.NewError(types.InvalidArgument)
			}
		}
		return v()
	}
	status := 200
	if g.protocol == "oidc" {
		status = 204
	}
	return core.DoJSON(g.runtime, ctx, operation(g.protocol+"_group_mappings", "replace", "PUT", status, 400, 401, 403, 404), req, func(raw []byte, e types.EndpointBinding) (core.Decoded[GroupMappingsResponse], error) {
		if g.protocol == "oidc" {
			return core.Decoded[GroupMappingsResponse]{}, nil
		}
		var out GroupMappingsResponse
		if json.Unmarshal(raw, &out) != nil || out.GroupMappings == nil {
			return core.Decoded[GroupMappingsResponse]{}, types.NewError(types.ProtocolError)
		}
		for i := range out.GroupMappings {
			if err := g.validateItem(&out.GroupMappings[i], e, parent); err != nil {
				return core.Decoded[GroupMappingsResponse]{}, err
			}
		}
		return core.Decoded[GroupMappingsResponse]{Data: out}, nil
	})
}
func (g *GroupMappings) mappingRequest(ref types.ResourceRef, o types.CallOptions) core.Request {
	req := request(g.scope, ref, g.protocol+"_group_mapping", "/v1/federations/"+g.protocol, nil, o)
	req.Validate = func() error {
		if ref.Kind != g.protocol+"_group_mapping" || ref.Parent == nil || ref.Parent.Kind != g.protocol+"_mapping_group" || ref.Parent.Parent == nil || ref.Parent.Parent.Kind != g.protocol+"_federation" || ref.Parent.Parent.Parent != nil || !validMapping(ref.Parent.ID, ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
		for _, p := range []*types.ResourceRef{ref.Parent, ref.Parent.Parent} {
			if p.Service != ref.Service || p.APIGeneration != ref.APIGeneration || p.EndpointID != ref.EndpointID || p.EndpointURL != ref.EndpointURL {
				return types.NewError(types.ScopeMismatch)
			}
		}
		return nil
	}
	if ref.Parent != nil && ref.Parent.Parent != nil {
		req.Path = path(g.protocol, ref.Parent.Parent.ID) + "/group-mappings/" + segment(ref.Parent.ID) + "/external-groups/" + segment(ref.ID)
	}
	return req
}
func (g *GroupMappings) Put(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(g.runtime, ctx, operation(g.protocol+"_group_mappings", "put", "PUT", 204, 400, 401, 403, 404), g.mappingRequest(ref, o), empty)
}
func (g *GroupMappings) Exists(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(g.runtime, ctx, operation(g.protocol+"_group_mappings", "exists", "HEAD", 204), g.mappingRequest(ref, o), empty)
}
func (g *GroupMappings) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(g.runtime, ctx, operation(g.protocol+"_group_mappings", "delete", "DELETE", 204, 400, 401, 403, 404), g.mappingRequest(ref, o), empty)
}

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

func textBound(v *string, min, max int) bool {
	if v == nil {
		return true
	}
	n := utf8.RuneCountInString(*v)
	return utf8.ValidString(*v) && n >= min && n <= max
}
func secretBound(v *types.ProtectedValue) bool {
	if v == nil {
		return true
	}
	text := string(v.Reveal())
	return textBound(&text, 1, 255)
}
func sessionBound(v *types.Integer) bool {
	if v == nil {
		return true
	}
	n, ok := new(big.Int).SetString(v.String(), 10)
	return ok && n.Cmp(big.NewInt(1)) >= 0 && n.Cmp(big.NewInt(720)) <= 0
}
func validOIDCCreate(in CreateOIDCRequest) bool {
	return textBound(in.Name, 1, 255) && textBound(in.Description, 0, 255) && textBound(in.Issuer, 0, 4096) && textBound(in.ClientID, 1, 255) && secretBound(in.ClientSecret) && sessionBound(in.SessionMaxAgeHours) && (in.Alias == nil || aliasPattern.MatchString(*in.Alias))
}
func validOIDCUpdate(in UpdateOIDCRequest) bool {
	if !textBound(in.Name, 1, 255) || !textBound(in.ClientID, 1, 255) || !textBound(in.Issuer, 0, 4096) {
		return false
	}
	if in.Description != nil {
		if v, p := in.Description.Value(); p && !textBound(&v, 0, 255) {
			return false
		}
	}
	if in.Alias != nil {
		if v, p := in.Alias.Value(); p && !aliasPattern.MatchString(v) {
			return false
		}
	}
	if in.ClientSecret != nil {
		if v, p := in.ClientSecret.Value(); p && !secretBound(&v) {
			return false
		}
	}
	if in.SessionMaxAgeHours != nil {
		if v, p := in.SessionMaxAgeHours.Value(); p && !sessionBound(&v) {
			return false
		}
	}
	return true
}
