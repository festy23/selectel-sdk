// Package iamcredentials manages IAM v1 S3 credentials with protected secret keys.
package iamcredentials

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
)

type Client struct{ V1 V1Client }
type V1Client struct {
	ServiceUsers *ServiceUsers
	Users        *Users
	Current      *Current
}
type ServiceUsers struct{ base binding }
type binding struct {
	runtime                                          *core.Runtime
	scope                                            types.Scope
	resource, parentKind, opResource, credentialKind string
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{V1: V1Client{ServiceUsers: &ServiceUsers{binding{r, s, "service_users", "service_user", "iam_service_users_credentials", "s3_service_user_credential"}}, Users: &Users{binding{r, s, "users", "user", "iam_users_credentials", "s3_user_credential"}}, Current: &Current{binding{r, s, "users", "current_user", "iam_users_me_credentials", "s3_current_user_credential"}}}}
}
func (b binding) operation(action, method string) core.Operation {
	effect := "mutation"
	status := 204
	if method == "GET" {
		effect = "read"
		status = 200
	}
	if method == "POST" {
		status = 201
	}
	rejected := []int{401, 403, 404}
	if method == "POST" {
		rejected = append(rejected, 400)
	}
	return core.Operation{ID: b.operationID(action), Service: "iam", APIGeneration: "v1", Method: method, Effect: effect, Auth: types.IAMAccount, EndpointID: "selectel-iam", FixedEndpoint: "https://api.selectel.ru", Success: []int{status}, Rejected: rejected, NoContentSuccess: []int{204}}
}
func segment(id string) string {
	value := url.PathEscape(id)
	if id == "." || id == ".." {
		value = strings.ReplaceAll(id, ".", "%2E")
	}
	return value
}
func (b binding) target(id string) (types.ResourceRef, error) {
	if id == "" || (b.parentKind == "user" && id == "me") {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := b.runtime.BindDocumented(b.operation("list", "GET"), b.scope)
	if e != nil {
		return types.ResourceRef{}, e
	}
	return types.ResourceRef{Scope: b.scope, Service: "iam", APIGeneration: "v1", Kind: b.parentKind, ID: id, EndpointID: op.EndpointID, EndpointURL: op.FixedEndpoint}, nil
}
func (u *ServiceUsers) Target(id string) (types.ResourceRef, error) { return u.base.target(id) }
func (b binding) parentPath(ref types.ResourceRef) string {
	return "/iam/v1/" + b.resource + "/" + segment(ref.ID) + "/credentials"
}
func pin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func (b binding) validateParent(ref types.ResourceRef) error {
	if ref.Validate() != nil || ref.Kind != b.parentKind || ref.Parent != nil || (b.parentKind == "current_user" && ref.ID != "me") || (b.parentKind == "user" && ref.ID == "me") {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Scope != b.scope || ref.Service != "iam" || ref.APIGeneration != "v1" {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func credentialRef(parent types.ResourceRef, kind, key string) *types.ResourceRef {
	copy := parent
	ref := parent
	ref.Kind = kind
	ref.ID = key
	ref.Parent = &copy
	return &ref
}
func (b binding) request(parent types.ResourceRef, body []byte, o types.CallOptions) core.Request {
	return core.Request{Scope: b.scope, Path: b.parentPath(parent), Body: body, Options: o, Refs: []types.ResourceRef{parent}, PinnedEndpoint: pin(parent), Validate: func() error { return b.validateParent(parent) }}
}
func (b binding) validateCredential(v *Credential, parent types.ResourceRef, project string) error {
	if v.Validate() != nil || v.AccessKey == nil || *v.AccessKey == "" {
		return types.NewError(types.ProtocolError)
	}
	if project != "" && v.ProjectID != nil && *v.ProjectID != project {
		return types.NewError(types.ScopeMismatch)
	}
	v.Ref = credentialRef(parent, b.credentialKind, *v.AccessKey)
	return nil
}
func (b binding) iterate(parent types.ResourceRef, o pagination.Options) (*pagination.Iterator[Credential], error) {
	if e := b.validateParent(parent); e != nil {
		return nil, e
	}
	op, e := b.runtime.BindDocumented(b.operation("list", "GET"), b.scope)
	if e != nil {
		return nil, e
	}
	return core.FixedIterator(b.runtime, op, b.request(parent, nil, types.CallOptions{}), parent, o, "credentials", func(v *Credential, _ types.EndpointBinding) error { return b.validateCredential(v, parent, "") })
}
func (u *ServiceUsers) Iterate(parent types.ResourceRef, o pagination.Options) (*pagination.Iterator[Credential], error) {
	return u.base.iterate(parent, o)
}
func (u *ServiceUsers) ListPage(ctx context.Context, parent types.ResourceRef, o pagination.Options) (pagination.Page[Credential], error) {
	it, e := u.Iterate(parent, o)
	if e != nil {
		return pagination.Page[Credential]{}, e
	}
	return it.Next(ctx)
}
func (u *ServiceUsers) Collect(ctx context.Context, parent types.ResourceRef, o pagination.Options) (pagination.Collection[Credential], error) {
	it, e := u.Iterate(parent, o)
	if e != nil {
		return pagination.Collection[Credential]{Items: []Credential{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (b binding) create(ctx context.Context, parent types.ResourceRef, in CreateRequest, o types.CallOptions) (types.Result[CreatedCredential], error) {
	body, e := json.Marshal(in)
	if e != nil {
		return types.Result[CreatedCredential]{}, types.NewError(types.InvalidArgument)
	}
	req := b.request(parent, body, o)
	validate := req.Validate
	req.Validate = func() error {
		if in.Name == nil || *in.Name == "" || in.ProjectID == nil || *in.ProjectID == "" {
			return types.NewError(types.InvalidArgument)
		}
		return validate()
	}
	return core.DoJSON(b.runtime, ctx, b.operation("create", "POST"), req, func(raw []byte, _ types.EndpointBinding) (core.Decoded[CreatedCredential], error) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return core.Decoded[CreatedCredential]{}, types.NewError(types.ProtocolError)
		}
		var key string
		if json.Unmarshal(fields["access_key"], &key) != nil || key == "" {
			return core.Decoded[CreatedCredential]{}, types.NewError(types.ProtocolError)
		}
		if project, ok := fields["project_id"]; ok {
			var id string
			if json.Unmarshal(project, &id) != nil {
				return core.Decoded[CreatedCredential]{}, types.NewError(types.ProtocolError)
			}
			if id != *in.ProjectID {
				return core.Decoded[CreatedCredential]{}, types.NewError(types.ScopeMismatch)
			}
		}
		ref := credentialRef(parent, b.credentialKind, key)
		out := core.Decoded[CreatedCredential]{Refs: []types.ResourceRef{*ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.SecretKey == nil || out.Data.SecretKey.Empty() {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = ref
		return out, nil
	})
}
func (u *ServiceUsers) Create(ctx context.Context, parent types.ResourceRef, in CreateRequest, o types.CallOptions) (types.Result[CreatedCredential], error) {
	return u.base.create(ctx, parent, in, o)
}
func (b binding) childRequest(ref types.ResourceRef, o types.CallOptions) core.Request {
	req := core.Request{Scope: b.scope, Path: "/iam/v1/" + b.resource, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Validate: func() error {
		if ref.Kind != b.credentialKind || ref.Parent == nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := b.validateParent(*ref.Parent); e != nil {
			return e
		}
		if ref.EndpointID != ref.Parent.EndpointID || ref.EndpointURL != ref.Parent.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}}
	if ref.Parent != nil {
		req.Path = b.parentPath(*ref.Parent) + "/" + segment(ref.ID)
	}
	return req
}
func (b binding) delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(b.runtime, ctx, b.operation("delete", "DELETE"), b.childRequest(ref, o), func(_ []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
func (u *ServiceUsers) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return u.base.delete(ctx, ref, o)
}

// operationID retains the canonical IDs established by the pinned IAM inventory
// in reviewed commit e1beb01, including its existing duplicate-path list suffixes.
func (b binding) operationID(action string) string {
	suffix := action
	if action == "list" && b.opResource == "iam_service_users_credentials" {
		suffix = "list_c90ef3c3"
	}
	if action == "list" && b.opResource == "iam_users_credentials" {
		suffix = "list_2138ba42"
	}
	if action == "search" {
		suffix = "list"
	}
	return "iam.v1." + b.opResource + "." + suffix
}

type Users struct{ base binding }
type Current struct{ base binding }

func (u *Users) Target(id string) (types.ResourceRef, error) { return u.base.target(id) }
func (u *Users) Iterate(parent types.ResourceRef, o pagination.Options) (*pagination.Iterator[Credential], error) {
	return u.base.iterate(parent, o)
}
func (u *Users) ListPage(ctx context.Context, parent types.ResourceRef, o pagination.Options) (pagination.Page[Credential], error) {
	it, e := u.Iterate(parent, o)
	if e != nil {
		return pagination.Page[Credential]{}, e
	}
	return it.Next(ctx)
}
func (u *Users) Collect(ctx context.Context, parent types.ResourceRef, o pagination.Options) (pagination.Collection[Credential], error) {
	it, e := u.Iterate(parent, o)
	if e != nil {
		return pagination.Collection[Credential]{Items: []Credential{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (u *Users) Create(ctx context.Context, parent types.ResourceRef, in CreateRequest, o types.CallOptions) (types.Result[CreatedCredential], error) {
	return u.base.create(ctx, parent, in, o)
}
func (u *Users) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return u.base.delete(ctx, ref, o)
}
func (b binding) get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Credential], error) {
	return core.DoJSON(b.runtime, ctx, b.operation("get", "GET"), b.childRequest(ref, o), func(raw []byte, _ types.EndpointBinding) (core.Decoded[Credential], error) {
		var v Credential
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.AccessKey == nil || *v.AccessKey == "" {
			return core.Decoded[Credential]{}, types.NewError(types.ProtocolError)
		}
		if *v.AccessKey != ref.ID {
			return core.Decoded[Credential]{}, types.NewError(types.ScopeMismatch)
		}
		copy := ref
		v.Ref = &copy
		return core.Decoded[Credential]{Data: v}, nil
	})
}
func (u *Users) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Credential], error) {
	return u.base.get(ctx, ref, o)
}
func (u *Current) Iterate(o pagination.Options) (*pagination.Iterator[Credential], error) {
	parent, e := u.base.target("me")
	if e != nil {
		return nil, e
	}
	return u.base.iterate(parent, o)
}
func (u *Current) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[Credential], error) {
	it, e := u.Iterate(o)
	if e != nil {
		return pagination.Page[Credential]{}, e
	}
	return it.Next(ctx)
}
func (u *Current) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[Credential], error) {
	it, e := u.Iterate(o)
	if e != nil {
		return pagination.Collection[Credential]{Items: []Credential{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (u *Current) Create(ctx context.Context, in CreateRequest, o types.CallOptions) (types.Result[CreatedCredential], error) {
	parent, e := u.base.target("me")
	if e != nil {
		return types.Result[CreatedCredential]{}, e
	}
	return u.base.create(ctx, parent, in, o)
}
func (u *Current) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Credential], error) {
	return u.base.get(ctx, ref, o)
}
func (u *Current) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return u.base.delete(ctx, ref, o)
}

type SearchFilters struct {
	ProjectID string
	UserID    *string
}

func (b binding) searchIterate(f SearchFilters, o pagination.Options) (*pagination.Iterator[SearchedCredential], error) {
	if f.ProjectID == "" {
		return nil, types.NewError(types.InvalidArgument)
	}
	op, e := b.runtime.BindDocumented(b.operation("search", "GET"), b.scope)
	if e != nil {
		return nil, e
	}
	plan, e := b.runtime.PagePlan(op.ID, "v1", b.scope, op.EndpointID, f)
	if e != nil {
		return nil, e
	}
	plan.FixedSize = true
	q := url.Values{"project_id": {f.ProjectID}}
	if f.UserID != nil {
		q.Set("user_id", *f.UserID)
	}
	return pagination.New(plan, o, func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[SearchedCredential], error) {
		result, err := core.DoJSON(b.runtime, ctx, op, core.Request{Scope: b.scope, Path: "/iam/v1/" + b.resource + "/credentials", Query: q}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[[]SearchedCredential], error) {
			var groups []SearchGroup
			if json.Unmarshal(raw, &groups) != nil || groups == nil {
				return core.Decoded[[]SearchedCredential]{}, types.NewError(types.ProtocolError)
			}
			items := make([]SearchedCredential, 0)
			for _, group := range groups {
				if group.Credentials == nil {
					return core.Decoded[[]SearchedCredential]{}, types.NewError(types.ProtocolError)
				}
				for _, v := range group.Credentials {
					if v.Validate() != nil || v.AccessKey == nil || *v.AccessKey == "" {
						return core.Decoded[[]SearchedCredential]{}, types.NewError(types.ProtocolError)
					}
					if v.ProjectID != nil && *v.ProjectID != f.ProjectID {
						return core.Decoded[[]SearchedCredential]{}, types.NewError(types.ScopeMismatch)
					}
					userID := ""
					if f.UserID != nil {
						userID = *f.UserID
					}
					if v.User != nil && v.User.ID != nil {
						if userID != "" && *v.User.ID != userID {
							return core.Decoded[[]SearchedCredential]{}, types.NewError(types.ScopeMismatch)
						}
						userID = *v.User.ID
					}
					if userID != "" {
						parent := types.ResourceRef{Scope: b.scope, Service: "iam", APIGeneration: "v1", Kind: b.parentKind, ID: userID, EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
						if b.validateParent(parent) != nil {
							return core.Decoded[[]SearchedCredential]{}, types.NewError(types.ProtocolError)
						}
						v.Ref = credentialRef(parent, b.credentialKind, *v.AccessKey)
					}
					items = append(items, v)
				}
			}
			return core.Decoded[[]SearchedCredential]{Data: items}, nil
		})
		return pagination.SourcePage[SearchedCredential]{Items: result.Data, Done: true, Metadata: result.Metadata}, err
	})
}
func (u *Users) SearchIterate(f SearchFilters, o pagination.Options) (*pagination.Iterator[SearchedCredential], error) {
	return u.base.searchIterate(f, o)
}
func (u *Users) SearchPage(ctx context.Context, f SearchFilters, o pagination.Options) (pagination.Page[SearchedCredential], error) {
	it, e := u.SearchIterate(f, o)
	if e != nil {
		return pagination.Page[SearchedCredential]{}, e
	}
	return it.Next(ctx)
}
func (u *Users) Search(ctx context.Context, f SearchFilters, o pagination.Options) (pagination.Collection[SearchedCredential], error) {
	it, e := u.SearchIterate(f, o)
	if e != nil {
		return pagination.Collection[SearchedCredential]{Items: []SearchedCredential{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (u *ServiceUsers) SearchIterate(f SearchFilters, o pagination.Options) (*pagination.Iterator[SearchedCredential], error) {
	return u.base.searchIterate(f, o)
}
func (u *ServiceUsers) SearchPage(ctx context.Context, f SearchFilters, o pagination.Options) (pagination.Page[SearchedCredential], error) {
	it, e := u.SearchIterate(f, o)
	if e != nil {
		return pagination.Page[SearchedCredential]{}, e
	}
	return it.Next(ctx)
}
func (u *ServiceUsers) Search(ctx context.Context, f SearchFilters, o pagination.Options) (pagination.Collection[SearchedCredential], error) {
	it, e := u.SearchIterate(f, o)
	if e != nil {
		return pagination.Collection[SearchedCredential]{Items: []SearchedCredential{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
