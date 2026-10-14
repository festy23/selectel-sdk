// Package iam manages account users and membership through Selectel IAM v1.
package iam

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
)

type Client struct {
	runtime       *core.Runtime
	scope         types.Scope
	CurrentGroups *CurrentGroups
	Users         *Users
	ServiceUsers  *ServiceUsers
	Groups        *Groups
}
type Users struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{runtime: r, scope: s, CurrentGroups: &CurrentGroups{r, s}, Users: &Users{r, s}, Groups: &Groups{r, s}, ServiceUsers: &ServiceUsers{r, s}}
}
func (u *Users) Iterate(o pagination.Options) (*pagination.Iterator[User], error) {
	op := core.Operation{ID: "iam.v1.users.list", Service: "iam", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-iam", FixedEndpoint: "https://api.selectel.ru", Success: []int{200}}
	return core.FixedIterator(u.runtime, op, core.Request{Scope: u.scope, Path: "/iam/v1/users"}, nil, o, "users", func(v *User, e types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(e, "iam", "user", *v.ID)
		}
		return nil
	})
}
func (u *Users) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[User], error) {
	it, e := u.Iterate(o)
	if e != nil {
		return pagination.Page[User]{}, e
	}
	return it.Next(ctx)
}
func (u *Users) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[User], error) {
	it, e := u.Iterate(o)
	if e != nil {
		return pagination.Collection[User]{Items: []User{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}

type Groups struct {
	runtime *core.Runtime
	scope   types.Scope
}

func operation(resource, action, method string, success int, rejected ...int) core.Operation {
	effect := "mutation"
	if method == "GET" {
		effect = "read"
	}
	return core.Operation{ID: "iam.v1." + resource + "." + action, Service: "iam", APIGeneration: "v1", Method: method, Effect: effect, Auth: types.IAMAccount, EndpointID: "selectel-iam", FixedEndpoint: "https://api.selectel.ru", Success: []int{success}, Rejected: append(rejected, 403), NoContentSuccess: []int{204}}
}
func resourcePath(resource, id string) string {
	segment := url.PathEscape(id)
	if id == "." || id == ".." {
		segment = strings.ReplaceAll(id, ".", "%2E")
	}
	return "/iam/v1/" + resource + "/" + segment
}
func exactRequest(scope types.Scope, ref types.ResourceRef, kind, path string, body []byte, o types.CallOptions) core.Request {
	return core.Request{Scope: scope, Path: path, Body: body, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if ref.Kind != kind || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}
}
func decodeResource[T any](kind, expected string, assign func(*T, *types.ResourceRef), validate func(T) error) core.Decoder[T] {
	return func(raw []byte, e types.EndpointBinding) (core.Decoded[T], error) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		id := expected
		if value, ok := fields["id"]; ok {
			var actual string
			if json.Unmarshal(value, &actual) != nil || actual == "" {
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
		ref := core.Ref(e, "iam", kind, id)
		out := core.Decoded[T]{Refs: []types.ResourceRef{*ref}}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		assign(&out.Data, ref)
		if err := validate(out.Data); err != nil {
			return out, err
		}
		return out, nil
	}
}
func noContent(_ []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
	return core.Decoded[struct{}]{}, nil
}
func (g *Groups) Create(ctx context.Context, in CreateGroupRequest, o types.CallOptions) (types.Result[DetailedGroup], error) {
	body, err := json.Marshal(in)
	if err != nil {
		return types.Result[DetailedGroup]{}, types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(g.runtime, ctx, operation("groups", "create", "POST", 200, 401, 409), core.Request{Scope: g.scope, Path: "/iam/v1/groups", Body: body, Options: o, Validate: func() error {
		if in.Name == nil || *in.Name == "" {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, decodeResource("group", "", func(v *DetailedGroup, r *types.ResourceRef) { v.Ref = r }, func(v DetailedGroup) error { return v.Validate() }))
}
func (g *Groups) AddUsers(ctx context.Context, ref types.ResourceRef, in ModifyGroupUsersRequest, o types.CallOptions) (types.Result[struct{}], error) {
	body, err := json.Marshal(in)
	if err != nil {
		return types.Result[struct{}]{}, types.NewError(types.InvalidArgument)
	}
	req := exactRequest(g.scope, ref, "group", resourcePath("groups", ref.ID)+"/users", body, o)
	v := req.Validate
	req.Validate = func() error {
		if len(in.KeystoneIDs) == 0 {
			return types.NewError(types.InvalidArgument)
		}
		for _, id := range in.KeystoneIDs {
			if id == "" {
				return types.NewError(types.InvalidArgument)
			}
		}
		return v()
	}
	return core.DoJSON(g.runtime, ctx, operation("groups_users", "add", "PUT", 204, 400, 401), req, noContent)
}

func (g *Groups) Iterate(o pagination.Options) (*pagination.Iterator[ListedGroup], error) {
	op := operation("groups", "list", "GET", 200)
	return core.FixedIterator(g.runtime, op, core.Request{Scope: g.scope, Path: "/iam/v1/groups"}, nil, o, "groups", func(v *ListedGroup, e types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(e, "iam", "group", *v.ID)
		}
		return nil
	})
}
func (g *Groups) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[ListedGroup], error) {
	it, e := g.Iterate(o)
	if e != nil {
		return pagination.Page[ListedGroup]{}, e
	}
	return it.Next(ctx)
}
func (g *Groups) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[ListedGroup], error) {
	it, e := g.Iterate(o)
	if e != nil {
		return pagination.Collection[ListedGroup]{Items: []ListedGroup{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (g *Groups) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[DetailedGroup], error) {
	return core.DoJSON(g.runtime, ctx, operation("groups", "get", "GET", 200), exactRequest(g.scope, ref, "group", resourcePath("groups", ref.ID), nil, o), decodeResource("group", ref.ID, func(v *DetailedGroup, r *types.ResourceRef) { v.Ref = r }, func(v DetailedGroup) error { return v.Validate() }))
}
func (g *Groups) Update(ctx context.Context, ref types.ResourceRef, in ModifyGroupRequest, o types.CallOptions) (types.Result[DetailedGroup], error) {
	body, err := json.Marshal(in)
	if err != nil {
		return types.Result[DetailedGroup]{}, types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(g.runtime, ctx, operation("groups", "update", "PATCH", 200, 401, 404, 409), exactRequest(g.scope, ref, "group", resourcePath("groups", ref.ID), body, o), decodeResource("group", ref.ID, func(v *DetailedGroup, r *types.ResourceRef) { v.Ref = r }, func(v DetailedGroup) error { return v.Validate() }))
}
func (g *Groups) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(g.runtime, ctx, operation("groups", "delete", "DELETE", 204, 400, 401, 404), exactRequest(g.scope, ref, "group", resourcePath("groups", ref.ID), nil, o), noContent)
}
func (g *Groups) RemoveUsers(ctx context.Context, ref types.ResourceRef, in ModifyGroupUsersRequest, o types.CallOptions) (types.Result[struct{}], error) {
	body, err := json.Marshal(in)
	if err != nil {
		return types.Result[struct{}]{}, types.NewError(types.InvalidArgument)
	}
	req := exactRequest(g.scope, ref, "group", resourcePath("groups", ref.ID)+"/users", body, o)
	v := req.Validate
	req.Validate = func() error {
		if len(in.KeystoneIDs) == 0 {
			return types.NewError(types.InvalidArgument)
		}
		for _, id := range in.KeystoneIDs {
			if id == "" {
				return types.NewError(types.InvalidArgument)
			}
		}
		return v()
	}
	return core.DoJSON(g.runtime, ctx, operation("groups_users", "remove", "DELETE", 204, 400, 401), req, noContent)
}

type ServiceUsers struct {
	runtime *core.Runtime
	scope   types.Scope
}

func encodeProtectedPassword(value any, password *types.ProtectedValue) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if password == nil {
		return raw, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	fields["password"], err = json.Marshal(string(password.Reveal()))
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return json.Marshal(fields)
}
func (u *ServiceUsers) Iterate(o pagination.Options) (*pagination.Iterator[ServiceUser], error) {
	return core.FixedIterator(u.runtime, operation("service_users", "list", "GET", 200), core.Request{Scope: u.scope, Path: "/iam/v1/service_users"}, nil, o, "users", func(v *ServiceUser, e types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(e, "iam", "service_user", *v.ID)
		}
		return nil
	})
}
func (u *ServiceUsers) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[ServiceUser], error) {
	it, e := u.Iterate(o)
	if e != nil {
		return pagination.Page[ServiceUser]{}, e
	}
	return it.Next(ctx)
}
func (u *ServiceUsers) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[ServiceUser], error) {
	it, e := u.Iterate(o)
	if e != nil {
		return pagination.Collection[ServiceUser]{Items: []ServiceUser{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (u *ServiceUsers) Create(ctx context.Context, in ServiceUserCreateRequest, o types.CallOptions) (types.Result[ServiceUser], error) {
	body, err := encodeProtectedPassword(in, in.Password)
	if err != nil {
		return types.Result[ServiceUser]{}, err
	}
	return core.DoJSON(u.runtime, ctx, operation("service_users", "create", "POST", 200, 400, 401, 404, 409), core.Request{Scope: u.scope, Path: "/iam/v1/service_users", Body: body, Options: o, Validate: func() error {
		if in.Validate() != nil || in.Name == nil || *in.Name == "" || in.Password == nil || in.Password.Empty() {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, decodeResource("service_user", "", func(v *ServiceUser, r *types.ResourceRef) { v.Ref = r }, func(v ServiceUser) error { return v.Validate() }))
}
func (u *ServiceUsers) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[DetailedServiceUser], error) {
	return core.DoJSON(u.runtime, ctx, operation("service_users", "get", "GET", 200), exactRequest(u.scope, ref, "service_user", resourcePath("service_users", ref.ID), nil, o), decodeResource("service_user", ref.ID, func(v *DetailedServiceUser, r *types.ResourceRef) { v.Ref = r }, func(v DetailedServiceUser) error { return v.Validate() }))
}
func (u *ServiceUsers) Update(ctx context.Context, ref types.ResourceRef, in ServiceUserUpdateRequest, o types.CallOptions) (types.Result[UpdatedServiceUser], error) {
	body, err := encodeProtectedPassword(in, in.Password)
	if err != nil {
		return types.Result[UpdatedServiceUser]{}, err
	}
	return core.DoJSON(u.runtime, ctx, operation("service_users", "update", "PATCH", 200, 400, 401, 404), exactRequest(u.scope, ref, "service_user", resourcePath("service_users", ref.ID), body, o), decodeResource("service_user", ref.ID, func(v *UpdatedServiceUser, r *types.ResourceRef) { v.Ref = r }, func(v UpdatedServiceUser) error { return v.Validate() }))
}
func (u *ServiceUsers) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(u.runtime, ctx, operation("service_users", "delete", "DELETE", 204, 401, 404), exactRequest(u.scope, ref, "service_user", resourcePath("service_users", ref.ID), nil, o), noContent)
}
func membershipRequest(scope types.Scope, ref types.ResourceRef, kind, resource string, in any, ids []string, o types.CallOptions) (core.Request, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return core.Request{}, types.NewError(types.InvalidArgument)
	}
	req := exactRequest(scope, ref, kind, resourcePath(resource, ref.ID)+"/groups", body, o)
	validate := req.Validate
	req.Validate = func() error {
		if len(ids) == 0 {
			return types.NewError(types.InvalidArgument)
		}
		for _, id := range ids {
			if id == "" {
				return types.NewError(types.InvalidArgument)
			}
		}
		return validate()
	}
	return req, nil
}
func (u *ServiceUsers) AddGroups(ctx context.Context, ref types.ResourceRef, in ModifyServiceUserGroupIDsRequest, o types.CallOptions) (types.Result[struct{}], error) {
	req, err := membershipRequest(u.scope, ref, "service_user", "service_users", in, in.GroupIDs, o)
	if err != nil {
		return types.Result[struct{}]{}, err
	}
	return core.DoJSON(u.runtime, ctx, operation("service_users_groups", "add", "PUT", 204, 400, 401, 404), req, noContent)
}
func (u *ServiceUsers) RemoveGroups(ctx context.Context, ref types.ResourceRef, in ModifyServiceUserGroupIDsRequest, o types.CallOptions) (types.Result[struct{}], error) {
	req, err := membershipRequest(u.scope, ref, "service_user", "service_users", in, in.GroupIDs, o)
	if err != nil {
		return types.Result[struct{}]{}, err
	}
	return core.DoJSON(u.runtime, ctx, operation("service_users_groups", "remove", "DELETE", 204, 400, 401, 404), req, noContent)
}

func (u *Users) Create(ctx context.Context, in UserCreateRequest, o types.CallOptions) (types.Result[User], error) {
	body, err := json.Marshal(in)
	if err != nil {
		return types.Result[User]{}, types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(u.runtime, ctx, operation("users", "create", "POST", 200, 400, 401, 404), core.Request{Scope: u.scope, Path: "/iam/v1/users", Body: body, Options: o, Validate: func() error {
		if in.Validate() != nil || in.Email == nil || *in.Email == "" {
			return types.NewError(types.InvalidArgument)
		}
		if in.AuthType != nil && *in.AuthType != "local" && *in.AuthType != "federated" {
			return types.NewError(types.InvalidArgument)
		}
		if in.AuthType != nil && *in.AuthType == "federated" && (in.Federation == nil || in.Federation.ID == nil || *in.Federation.ID == "" || in.Federation.ExternalID == nil || *in.Federation.ExternalID == "") {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, decodeResource("user", "", func(v *User, r *types.ResourceRef) { v.Ref = r }, func(v User) error { return v.Validate() }))
}
func (u *Users) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[DetailedUser], error) {
	return core.DoJSON(u.runtime, ctx, operation("users", "get", "GET", 200), exactRequest(u.scope, ref, "user", resourcePath("users", ref.ID), nil, o), decodeResource("user", ref.ID, func(v *DetailedUser, r *types.ResourceRef) { v.Ref = r }, func(v DetailedUser) error { return v.Validate() }))
}
func (u *Users) Update(ctx context.Context, ref types.ResourceRef, in UserUpdateRequest, o types.CallOptions) (types.Result[User], error) {
	body, err := json.Marshal(in)
	if err != nil {
		return types.Result[User]{}, types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(u.runtime, ctx, operation("users", "update", "PATCH", 200, 400, 401, 404), exactRequest(u.scope, ref, "user", resourcePath("users", ref.ID), body, o), decodeResource("user", ref.ID, func(v *User, r *types.ResourceRef) { v.Ref = r }, func(v User) error { return v.Validate() }))
}
func (u *Users) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(u.runtime, ctx, operation("users", "delete", "DELETE", 204, 400, 401, 404), exactRequest(u.scope, ref, "user", resourcePath("users", ref.ID), nil, o), noContent)
}
func (u *Users) ResendInvite(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(u.runtime, ctx, operation("users", "resend_invite", "POST", 204, 400, 401, 404), exactRequest(u.scope, ref, "user", resourcePath("users", ref.ID)+"/resend_invite", nil, o), noContent)
}
func (u *Users) AddGroups(ctx context.Context, ref types.ResourceRef, in ModifyUserGroupIDsRequest, o types.CallOptions) (types.Result[struct{}], error) {
	req, err := membershipRequest(u.scope, ref, "user", "users", in, in.GroupIDs, o)
	if err != nil {
		return types.Result[struct{}]{}, err
	}
	return core.DoJSON(u.runtime, ctx, operation("users_groups", "add", "PUT", 204, 400, 401, 404), req, noContent)
}
func (u *Users) RemoveGroups(ctx context.Context, ref types.ResourceRef, in ModifyUserGroupIDsRequest, o types.CallOptions) (types.Result[struct{}], error) {
	req, err := membershipRequest(u.scope, ref, "user", "users", in, in.GroupIDs, o)
	if err != nil {
		return types.Result[struct{}]{}, err
	}
	return core.DoJSON(u.runtime, ctx, operation("users_groups", "remove", "DELETE", 204, 400, 401, 404), req, noContent)
}

func (c *Client) CurrentUser(ctx context.Context, o types.CallOptions) (types.Result[CurrentUser], error) {
	return core.DoJSON(c.runtime, ctx, operation("me", "get", "GET", 200), core.Request{Scope: c.scope, Path: "/iam/v1/me", Options: o}, decodeResource("user", "", func(v *CurrentUser, r *types.ResourceRef) { v.Ref = r }, func(v CurrentUser) error { return v.Validate() }))
}

type CurrentGroups struct {
	runtime *core.Runtime
	scope   types.Scope
}

func (g *CurrentGroups) Iterate(o pagination.Options) (*pagination.Iterator[Group], error) {
	return core.FixedIterator(g.runtime, operation("me_groups", "list", "GET", 200), core.Request{Scope: g.scope, Path: "/iam/v1/me/groups"}, nil, o, "groups", func(v *Group, e types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(e, "iam", "group", *v.ID)
		}
		return nil
	})
}
func (g *CurrentGroups) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[Group], error) {
	it, e := g.Iterate(o)
	if e != nil {
		return pagination.Page[Group]{}, e
	}
	return it.Next(ctx)
}
func (g *CurrentGroups) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[Group], error) {
	it, e := g.Iterate(o)
	if e != nil {
		return pagination.Collection[Group]{Items: []Group{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
