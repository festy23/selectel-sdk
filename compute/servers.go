// Package compute binds Nova v2.1 to the common SDK execution contract.
package compute

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct{ Servers *Servers }
type Servers struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, scope types.Scope) *Client {
	return &Client{Servers: &Servers{runtime: r, scope: scope}}
}

type Server struct {
	servers.Server
	AdminPassword *types.ProtectedValue `json:"adminPass,omitempty"`
	UserData      *types.ProtectedValue `json:"OS-EXT-SRV-ATTR:user_data,omitempty"`
	Ref           types.ResourceRef     `json:"ref"`
}

// Native options are reused, while sensitive inputs have explicit protected fields.
type CreateRequest struct {
	Options                 servers.CreateOpts
	AdminPassword, UserData *types.ProtectedValue
}

func (r CreateRequest) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "[VM Create Request]") }
func (r CreateRequest) MarshalJSON() ([]byte, error) { return []byte(`"[VM Create Request]"`), nil }
func operation(action, method string) core.Operation {
	effect := "read"
	success := []int{200}
	if method == "POST" {
		effect = "mutation"
		success = []int{202}
	}
	if method == "DELETE" {
		effect = "mutation"
		success = []int{204}
	}
	return core.Operation{ID: "compute.v2_1.servers." + action, Service: "compute", APIGeneration: "v2_1", Method: method, Effect: effect, Auth: types.IAMProject, ServiceType: "compute", RequireProject: true, RequireLocation: true, Success: success, Rejected: []int{400, 401, 403, 404}, Headers: http.Header{"OpenStack-API-Version": {"compute 2.1"}}}
}
func ref(endpoint types.EndpointBinding, id string) types.ResourceRef {
	return types.ResourceRef{Scope: endpoint.Scope, Service: "compute", APIGeneration: "v2_1", Kind: "server", ID: id, EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
}
func validateRef(r types.ResourceRef) error {
	if r.Kind != "server" || r.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func pin(r types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: r.EndpointID, URL: r.EndpointURL, Scope: r.Scope, APIGeneration: r.APIGeneration, Audience: "selectel"}
}
func decodeServer(raw json.RawMessage, endpoint types.EndpointBinding, expected string) (Server, error) {
	var native servers.Server
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &native) != nil || native.ID == "" {
		return Server{}, types.NewError(types.ProtocolError)
	}
	if expected != "" && expected != native.ID {
		return Server{}, types.NewError(types.ScopeMismatch)
	}
	if native.TenantID != "" && native.TenantID != endpoint.Scope.ProjectID {
		return Server{}, types.NewError(types.ScopeMismatch)
	}
	out := Server{Server: native, Ref: ref(endpoint, native.ID)}
	if native.AdminPass != "" {
		v := types.Protect([]byte(native.AdminPass))
		out.AdminPassword = &v
		out.Server.AdminPass = ""
	}
	if native.Userdata != nil {
		v := types.Protect([]byte(*native.Userdata))
		out.UserData = &v
		out.Server.Userdata = nil
	}
	return out, nil
}
func decodeOne(expected string) core.Decoder[Server] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Server], error) {
		var body struct {
			Server json.RawMessage `json:"server"`
		}
		if json.Unmarshal(raw, &body) != nil {
			return core.Decoded[Server]{}, types.NewError(types.ProtocolError)
		}
		var identity struct {
			ID      string `json:"id"`
			Project string `json:"tenant_id"`
		}
		partial := core.Decoded[Server]{}
		if json.Unmarshal(body.Server, &identity) == nil && identity.ID != "" {
			if (expected != "" && identity.ID != expected) || (identity.Project != "" && identity.Project != endpoint.Scope.ProjectID) {
				return partial, types.NewError(types.ScopeMismatch)
			}
			partial.Refs = []types.ResourceRef{ref(endpoint, identity.ID)}
		}
		v, err := decodeServer(body.Server, endpoint, expected)
		out := partial
		out.Data = v
		if err == nil {
			out.Refs = []types.ResourceRef{v.Ref}
		}
		return out, err
	}
}
func (s *Servers) Get(ctx context.Context, r types.ResourceRef) (types.Result[Server], error) {
	return core.DoJSON(s.runtime, ctx, operation("get", "GET"), core.Request{Scope: s.scope, Path: "/servers/" + url.PathEscape(r.ID), Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Validate: func() error { return validateRef(r) }}, decodeOne(r.ID))
}
func (s *Servers) Delete(ctx context.Context, r types.ResourceRef) (types.Result[struct{}], error) {
	return core.DoJSON(s.runtime, ctx, operation("delete", "DELETE"), core.Request{Scope: s.scope, Path: "/servers/" + url.PathEscape(r.ID), Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Validate: func() error { return validateRef(r) }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(strings.TrimSpace(string(raw))) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	})
}
func createBody(request CreateRequest) ([]byte, error) {
	opts := request.Options
	if opts.Name == "" || opts.FlavorRef == "" || (opts.ImageRef == "" && len(opts.BlockDevice) == 0) {
		return nil, types.NewError(types.InvalidArgument)
	}
	if opts.Min > 1 || opts.Max > 1 || opts.Min < 0 || opts.Max < 0 || len(opts.Tags) > 0 || opts.Hostname != "" || opts.HypervisorHostname != "" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if opts.AdminPass != "" || len(opts.UserData) > 0 || len(opts.Personality) > 0 {
		return nil, types.NewError(types.InvalidArgument)
	}
	networks, ok := opts.Networks.([]servers.Network)
	if !ok || len(networks) == 0 {
		return nil, types.NewError(types.InvalidArgument)
	}
	for _, n := range networks {
		if (n.UUID == "") == (n.Port == "") {
			return nil, types.NewError(types.InvalidArgument)
		}
		if n.Tag != "" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
	}
	boot := 0
	for _, b := range opts.BlockDevice {
		if b.Tag != "" || b.VolumeType != "" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if b.BootIndex == 0 {
			boot++
		}
		switch b.SourceType {
		case "image", "volume", "snapshot":
			if b.UUID == "" {
				return nil, types.NewError(types.InvalidArgument)
			}
		case "blank":
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
		if b.DestinationType != "" && b.DestinationType != "volume" && b.DestinationType != "local" {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	if opts.ImageRef == "" && boot != 1 || opts.ImageRef != "" && boot > 0 {
		return nil, types.NewError(types.InvalidArgument)
	}
	body, err := opts.ToServerCreateMap()
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	server, ok := body["server"].(map[string]any)
	if !ok {
		return nil, types.NewError(types.ProtocolError)
	}
	if request.AdminPassword != nil {
		server["adminPass"] = string(request.AdminPassword.Reveal())
	}
	if request.UserData != nil {
		server["user_data"] = base64.StdEncoding.EncodeToString(request.UserData.Reveal())
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return raw, nil
}
func (s *Servers) Create(ctx context.Context, r CreateRequest) (types.Result[Server], error) {
	raw, err := createBody(r)
	return core.DoJSON(s.runtime, ctx, operation("create", "POST"), core.Request{Scope: s.scope, Path: "/servers", Body: raw, Validate: func() error { return err }}, decodeOne(""))
}

func (s *Servers) Iterate(filters servers.ListOpts, options pagination.Options) (*pagination.Iterator[Server], error) {
	if s.scope.ProjectID == "" || (s.scope.Region == "" && s.scope.Pool == "") {
		return nil, types.NewError(types.InvalidArgument)
	}
	if filters.AllTenants || filters.TenantID != "" || filters.UserID != "" || filters.Tags != "" || filters.TagsAny != "" || filters.NotTags != "" || filters.NotTagsAny != "" || filters.IP6 != "" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if filters.Limit != 0 || filters.Marker != "" {
		return nil, types.NewError(types.InvalidArgument)
	}
	query, err := filters.ToServerListQuery()
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	values, err := url.ParseQuery(strings.TrimPrefix(query, "?"))
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	plan, err := s.runtime.PagePlan("compute.v2_1.servers.list", "v2_1", s.scope, "", filters)
	if err != nil {
		return nil, err
	}
	plan.DynamicEndpoint = plan.EndpointID == ""
	expectedEndpoint := plan.EndpointID
	if options.Cursor != nil {
		expectedEndpoint = options.Cursor.EndpointID
	}
	fetch := func(ctx context.Context, marker string, size uint) (pagination.SourcePage[Server], error) {
		q := url.Values{}
		for k, v := range values {
			q[k] = append([]string(nil), v...)
		}
		q.Set("limit", strconv.FormatUint(uint64(size), 10))
		if marker != "" {
			q.Set("marker", marker)
		}
		var next string
		decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[[]Server], error) {
			var body struct {
				Servers []json.RawMessage            `json:"servers"`
				Links   []struct{ Rel, Href string } `json:"servers_links"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Servers == nil {
				return core.Decoded[[]Server]{}, types.NewError(types.ProtocolError)
			}
			items := make([]Server, 0, len(body.Servers))
			for _, data := range body.Servers {
				v, e := decodeServer(data, endpoint, "")
				if e != nil {
					return core.Decoded[[]Server]{}, e
				}
				items = append(items, v)
			}
			for _, link := range body.Links {
				if link.Rel != "next" {
					continue
				}
				if next != "" {
					return core.Decoded[[]Server]{}, types.NewError(types.ProtocolError)
				}
				target, e := url.Parse(link.Href)
				base, _ := url.Parse(strings.TrimRight(endpoint.URL, "/") + "/servers/detail")
				if e != nil {
					return core.Decoded[[]Server]{}, types.NewError(types.ProtocolError)
				}
				target = base.ResolveReference(target)
				if target.Scheme != base.Scheme || target.Host != base.Host || target.EscapedPath() != base.EscapedPath() || target.User != nil || target.Fragment != "" {
					return core.Decoded[[]Server]{}, types.NewError(types.ProtocolError)
				}
				next = target.Query().Get("marker")
				if next == "" {
					return core.Decoded[[]Server]{}, types.NewError(types.ProtocolError)
				}
			}
			return core.Decoded[[]Server]{Data: items}, nil
		}
		result, e := core.DoJSON(s.runtime, ctx, operation("list", "GET"), core.Request{Scope: s.scope, Path: "/servers/detail", Query: q, ExpectedEndpointID: expectedEndpoint}, decode)
		if e == nil {
			expectedEndpoint = result.Metadata.EndpointID
		}
		return pagination.SourcePage[Server]{Items: result.Data, Next: next, Done: next == "", Metadata: result.Metadata}, e
	}
	return pagination.New(plan, options, fetch)
}
func (s *Servers) Collect(ctx context.Context, f servers.ListOpts, o pagination.Options) (pagination.Collection[Server], error) {
	it, e := s.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Server]{Items: []Server{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (s *Servers) ListPage(ctx context.Context, f servers.ListOpts, o pagination.Options) (pagination.Page[Server], error) {
	it, e := s.Iterate(f, o)
	if e != nil {
		return pagination.Page[Server]{}, e
	}
	return it.Next(ctx)
}

func states(values ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return m
}
func (s *Servers) Handle(r types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if validateRef(r) != nil || r.Service != "compute" || r.APIGeneration != "v2_1" || r.Scope != s.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if r.Scope.ProjectID == "" || (r.Scope.Region == "" && r.Scope.Pool == "") {
		return nil, types.NewError(types.InvalidArgument)
	}
	pending := states("BUILD", "HARD_REBOOT", "MIGRATING", "PASSWORD", "REBOOT", "REBUILD", "RESCUE", "RESIZE", "REVERT_RESIZE", "SHUTOFF", "SUSPENDED", "VERIFY_RESIZE")
	exists := states("ACTIVE", "DELETED", "SOFT_DELETED")
	for state := range pending {
		exists[state] = true
	}
	read := func(ctx context.Context) (operations.Observation, error) {
		result, e := s.Get(ctx, r)
		if e != nil {
			return operations.Observation{}, e
		}
		v := result.Data.Status
		classification := "unknown"
		if v == "ACTIVE" {
			classification = "succeeded"
		} else if v == "ERROR" || v == "DELETED" || v == "SOFT_DELETED" {
			classification = "failed"
		} else if pending[v] {
			classification = "pending"
		}
		return operations.Observation{Ref: r, ProviderState: v, Classification: classification, ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, nil
	}
	return operations.Bind(s.runtime, r, receipt, read, map[string]operations.Goal{"server.active": {Success: states("ACTIVE"), Failure: states("ERROR", "DELETED", "SOFT_DELETED"), Pending: pending}, "server.deleted": {Failure: states("ERROR"), Pending: exists, AbsentSuccess: true}})
}

type CreateAndWaitResult struct {
	Create types.Result[Server]
	Wait   operations.WaitResult
}

func (s *Servers) CreateAndWait(ctx context.Context, r CreateRequest) (CreateAndWaitResult, error) {
	created, e := s.Create(ctx, r)
	out := CreateAndWaitResult{Create: created}
	if e != nil {
		return out, e
	}
	h, e := s.Handle(created.Data.Ref, created.Receipt)
	if e != nil {
		return out, e
	}
	out.Wait, e = h.Wait(ctx, "server.active")
	return out, e
}
