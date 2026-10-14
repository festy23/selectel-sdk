// Package clickhouse exposes the documented DBaaS ClickHouse v2 generation.
package clickhouse

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"regexp"
	"strings"
)

type Client struct{ Datastores *Datastores }
type Datastores struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{Datastores: &Datastores{runtime: r, scope: s}}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validUUID(v *string) bool {
	if v == nil {
		return false
	}
	raw := *v
	if len(raw) == 45 && strings.EqualFold(raw[:9], "urn:uuid:") {
		raw = raw[9:]
	}
	if len(raw) == 38 && raw[0] == '{' && raw[37] == '}' {
		raw = raw[1:37]
	}
	if len(raw) == 32 {
		for _, c := range raw {
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
		return true
	}
	return uuidPattern.MatchString(raw)
}
func positive(v *types.Integer) bool {
	if v == nil {
		return false
	}
	n, ok := new(big.Int).SetString(v.String(), 10)
	return ok && n.Sign() > 0
}
func nonempty(v *string) bool { return v != nil && *v != "" }
func createValidation(v DatastoreCreateRequest) error {
	if v.Config != nil && !objectJSON(v.Config) {
		return types.NewError(types.InvalidArgument)
	}
	if v.Validate() != nil || !nonempty(v.Name) || v.Password == nil || v.Password.Empty() || !validUUID(v.TypeID) || !validUUID(v.SubnetID) || len(v.NodeGroups) == 0 {
		return types.NewError(types.InvalidArgument)
	}
	for _, g := range v.NodeGroups {
		if !nonempty(g.Name) || g.Role == nil || (*g.Role != "DATA" && *g.Role != "KEEPER") || !positive(g.NodeCount) || g.Flavor == nil {
			return types.NewError(types.InvalidArgument)
		}
		if (*g.Role == "DATA" && g.Weight == nil) || (*g.Role == "KEEPER" && g.Weight != nil) {
			return types.NewError(types.InvalidArgument)
		}
		f := g.Flavor
		if f.Type == nil {
			return types.NewError(types.InvalidArgument)
		}
		switch *f.Type {
		case "FIXED":
			if !validUUID(f.ID) {
				return types.NewError(types.InvalidArgument)
			}
		case "FLEXIBLE":
			if (f.ID != nil && *f.ID != "") || !positive(f.RAM) || !positive(f.Disk) || !positive(f.VCPUs) || f.DiskType == nil || (*f.DiskType != "LOCAL" && *f.DiskType != "NETWORK_ULTRA") {
				return types.NewError(types.InvalidArgument)
			}
		default:
			return types.NewError(types.InvalidArgument)
		}
	}
	return nil
}
func operation(c *Datastores, action, method string, status int) (core.Operation, error) {
	effect := "read"
	if method != "GET" {
		effect = "mutation"
	}
	return c.runtime.BindDocumented(core.Operation{ID: "databases.v2.clickhouse." + action, Service: "databases", APIGeneration: "v2", Method: method, Effect: effect, Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{status}}, c.scope)
}
func decodeDatastore(expected *types.ResourceRef) core.Decoder[DatastoreResponse] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[DatastoreResponse], error) {
		var value DatastoreResponse
		if json.Unmarshal(raw, &value) != nil || value.Validate() != nil {
			return core.Decoded[DatastoreResponse]{}, types.NewError(types.ProtocolError)
		}
		if value.ProjectID != nil && *value.ProjectID != endpoint.Scope.ProjectID {
			return core.Decoded[DatastoreResponse]{}, types.NewError(types.ScopeMismatch)
		}
		if expected != nil {
			if value.ID != nil && *value.ID != expected.ID {
				return core.Decoded[DatastoreResponse]{}, types.NewError(types.ScopeMismatch)
			}
			value.Ref = expected
		} else {
			if value.ID == nil || *value.ID == "" {
				return core.Decoded[DatastoreResponse]{}, types.NewError(types.ProtocolError)
			}
			value.Ref = core.Ref(endpoint, "databases", "clickhouse_datastore", *value.ID)
		}
		return core.Decoded[DatastoreResponse]{Data: value, Refs: []types.ResourceRef{*value.Ref}}, nil
	}
}
func (c *Datastores) Create(ctx context.Context, spec DatastoreCreateRequest) (types.Result[DatastoreResponse], error) {
	op, binding := operation(c, "create", "POST", 201)
	body, encode := requestBody("", spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/clickhouse", Body: body, Validate: func() error {
		if err := createValidation(spec); err != nil {
			return err
		}
		if encode != nil {
			return encode
		}
		return binding
	}}, decodeDatastore(nil))
}

func (c *Datastores) refValidation(ref types.ResourceRef) error {
	if ref.Scope != c.scope || ref.Service != "databases" || ref.APIGeneration != "v2" || ref.Kind != "clickhouse_datastore" || !validUUID(&ref.ID) || ref.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	op, e := operation(c, "get", "GET", 200)
	if e != nil {
		return e
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *Datastores) Get(ctx context.Context, ref types.ResourceRef) (types.Result[DatastoreResponse], error) {
	op, bind := operation(c, "get", "GET", 200)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/clickhouse/" + ref.ID, Refs: []types.ResourceRef{ref}, Validate: func() error {
		if e := c.refValidation(ref); e != nil {
			return e
		}
		return bind
	}}, decodeDatastore(&ref))
}
func (c *Datastores) Iterate(o pagination.Options) (*pagination.Iterator[DatastoreResponse], error) {
	op, e := operation(c, "list", "GET", 200)
	if e != nil {
		return nil, e
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/datastores/clickhouse"}, struct{}{}, o, "datastores", func(v *DatastoreResponse, ep types.EndpointBinding) error {
		if v.Validate() != nil {
			return types.NewError(types.ProtocolError)
		}
		if v.ProjectID != nil && *v.ProjectID != ep.Scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if v.ID != nil {
			v.Ref = core.Ref(ep, "databases", "clickhouse_datastore", *v.ID)
		}
		return nil
	})
}
func (c *Datastores) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[DatastoreResponse], error) {
	it, e := c.Iterate(o)
	if e != nil {
		return pagination.Page[DatastoreResponse]{}, e
	}
	return it.Next(ctx)
}
func (c *Datastores) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[DatastoreResponse], error) {
	it, e := c.Iterate(o)
	if e != nil {
		return pagination.Collection[DatastoreResponse]{Items: []DatastoreResponse{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Datastores) IterateSettings(o pagination.Options) (*pagination.Iterator[DatastoreConfigurationParameterResponse], error) {
	op, e := operation(c, "settings", "GET", 200)
	if e != nil {
		return nil, e
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/datastores/clickhouse/configuration-parameters"}, struct{}{}, o, "configuration-parameters", func(v *DatastoreConfigurationParameterResponse, _ types.EndpointBinding) error { return v.Validate() })
}
func (c *Datastores) SettingsPage(ctx context.Context, o pagination.Options) (pagination.Page[DatastoreConfigurationParameterResponse], error) {
	it, e := c.IterateSettings(o)
	if e != nil {
		return pagination.Page[DatastoreConfigurationParameterResponse]{}, e
	}
	return it.Next(ctx)
}
func (c *Datastores) Settings(ctx context.Context, o pagination.Options) (pagination.Collection[DatastoreConfigurationParameterResponse], error) {
	it, e := c.IterateSettings(o)
	if e != nil {
		return pagination.Collection[DatastoreConfigurationParameterResponse]{Items: []DatastoreConfigurationParameterResponse{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Datastores) change(ctx context.Context, ref types.ResourceRef, action, method, suffix string, status int, spec any, validation error) (types.Result[DatastoreResponse], error) {
	op, bind := operation(c, action, method, status)
	var body []byte
	var encode error
	if spec != nil {
		body, encode = requestBody("", spec)
	}
	decode := decodeDatastore(&ref)
	if status == 204 {
		decode = func(raw []byte, _ types.EndpointBinding) (core.Decoded[DatastoreResponse], error) {
			if len(raw) != 0 {
				return core.Decoded[DatastoreResponse]{}, types.NewError(types.ProtocolError)
			}
			return core.Decoded[DatastoreResponse]{}, nil
		}
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/clickhouse/" + ref.ID + suffix, Body: body, Refs: []types.ResourceRef{ref}, Validate: func() error {
		if validation != nil {
			return validation
		}
		if e := c.refValidation(ref); e != nil {
			return e
		}
		if encode != nil {
			return encode
		}
		return bind
	}}, decode)
}
func objectJSON(v *types.ProtectedJSON) bool {
	if v == nil {
		return false
	}
	var obj map[string]json.RawMessage
	return json.Unmarshal(v.Reveal(), &obj) == nil && obj != nil
}

func (c *Datastores) Update(ctx context.Context, ref types.ResourceRef, spec DatastoreUpdateRequest) (types.Result[DatastoreResponse], error) {
	var validation error
	if spec.Validate() != nil || !nonempty(spec.Name) {
		validation = types.NewError(types.InvalidArgument)
	}
	return c.change(ctx, ref, "update", "PATCH", "", 200, spec, validation)
}

func (c *Datastores) UpdatePassword(ctx context.Context, ref types.ResourceRef, spec DatastoreUpdatePasswordRequest) (types.Result[DatastoreResponse], error) {
	var validation error
	if spec.Validate() != nil || spec.NewPassword == nil || spec.NewPassword.Empty() {
		validation = types.NewError(types.InvalidArgument)
	}
	return c.change(ctx, ref, "password", "PATCH", "/password", 200, spec, validation)
}

func (c *Datastores) Configure(ctx context.Context, ref types.ResourceRef, spec DatastoreConfigRequest) (types.Result[DatastoreResponse], error) {
	var validation error
	if spec.Validate() != nil || !objectJSON(spec.Config) {
		validation = types.NewError(types.InvalidArgument)
	}
	return c.change(ctx, ref, "configure", "PUT", "/config", 200, spec, validation)
}

func (c *Datastores) UpdateSecurityGroups(ctx context.Context, ref types.ResourceRef, spec DatastoreSecurityGroupsRequest) (types.Result[DatastoreResponse], error) {
	var validation error
	if spec.Validate() != nil {
		validation = types.NewError(types.InvalidArgument)
	}
	for _, id := range spec.SecurityGroups {
		if !validUUID(&id) {
			validation = types.NewError(types.InvalidArgument)
		}
	}
	return c.change(ctx, ref, "security_groups", "PUT", "/security-groups", 200, spec, validation)
}

func (c *Datastores) EnableLogging(ctx context.Context, ref types.ResourceRef, spec DatastoreLogPlatformRequest) (types.Result[DatastoreResponse], error) {
	var validation error
	if spec.Validate() != nil || spec.LogPlatform == nil || !nonempty(spec.LogPlatform.LogGroup) {
		validation = types.NewError(types.InvalidArgument)
	}
	return c.change(ctx, ref, "enable_logging", "PUT", "/log-platform", 200, spec, validation)
}

func (c *Datastores) DisableLogging(ctx context.Context, ref types.ResourceRef) (types.Result[DatastoreResponse], error) {
	var validation error
	return c.change(ctx, ref, "disable_logging", "DELETE", "/log-platform", 204, nil, validation)
}

func (c *Datastores) Delete(ctx context.Context, ref types.ResourceRef) (types.Result[DatastoreResponse], error) {
	var validation error
	return c.change(ctx, ref, "delete", "DELETE", "", 204, nil, validation)
}
