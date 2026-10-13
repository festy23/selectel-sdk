// Package secrets manages project-scoped Secret Manager resources.
package secrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(runtime *core.Runtime, scope types.Scope) *Client {
	return &Client{runtime: runtime, scope: scope}
}

type Metadata struct {
	CreatedAtRaw *string `json:"created_at,omitempty"`
	Description  *string `json:"description,omitempty"`
}
type Key struct {
	Name     *string   `json:"name,omitempty"`
	Type     *string   `json:"type,omitempty"`
	Metadata *Metadata `json:"metadata,omitempty"`
}
type VersionMetadata struct {
	CreatedAtRaw *string        `json:"created_at,omitempty"`
	Current      *bool          `json:"is_current,omitempty"`
	ID           *types.Integer `json:"version_id,omitempty"`
}
type Versions struct {
	CreatedAtRaw *string           `json:"created_at,omitempty"`
	Description  *string           `json:"description,omitempty"`
	Name         *string           `json:"name,omitempty"`
	Versions     []VersionMetadata `json:"versions,omitzero"`
	Ref          types.ResourceRef `json:"ref"`
}
type Version struct {
	VersionMetadata
	Value types.ProtectedValue `json:"value"`
}
type Secret struct {
	CreatedAtRaw *string           `json:"created_at,omitempty"`
	Description  *string           `json:"description,omitempty"`
	Name         *string           `json:"name,omitempty"`
	Version      *Version          `json:"version,omitempty"`
	Ref          types.ResourceRef `json:"ref"`
}
type CreateRequest struct {
	Name        string
	Description *string
	Value       *types.ProtectedValue
}
type CreateVersionRequest struct {
	Name     string
	Value    *types.ProtectedValue
	Activate *bool
}
type UpdateRequest struct {
	Ref         types.ResourceRef
	Description *string
}

func operation(id, method string) core.Operation {
	effect := "read"
	success := []int{200}
	if method != "GET" {
		effect = "mutation"
	}
	if method == "PUT" || method == "DELETE" {
		success = []int{204}
	}
	return core.Operation{ID: id, Service: "secrets", APIGeneration: "v1", Method: method, Effect: effect, Auth: types.IAMProject, RequireProject: true, EndpointID: "selectel-secrets", FixedEndpoint: "https://cloud.api.selcloud.ru/secrets-manager", Success: success, Rejected: []int{401, 403}}
}
func keyPath(name string) (string, error) {
	if name == "" {
		return "", types.NewError(types.InvalidArgument)
	}
	if name == "." || name == ".." {
		return "/v1/" + strings.ReplaceAll(name, ".", "%2E"), nil
	}
	return "/v1/" + url.PathEscape(name), nil
}
func secretRef(endpoint types.EndpointBinding, name string) types.ResourceRef {
	return types.ResourceRef{Scope: endpoint.Scope, Service: "secrets", APIGeneration: "v1", Kind: "secret_key", ID: name, EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
}
func pinned(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
}
func validateKeyRef(ref types.ResourceRef, pathErr error) error {
	if pathErr != nil {
		return pathErr
	}
	if ref.Kind != "secret_key" || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}

func (c *Client) IterateMetadata(options pagination.Options) (*pagination.Iterator[Key], error) {
	plan, err := c.runtime.PagePlan("secrets.v1.secrets.list", "v1", c.scope, "selectel-secrets", struct{}{})
	if err != nil {
		return nil, err
	}
	plan.FixedSize = true
	fetch := func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[Key], error) {
		decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[[]Key], error) {
			var response struct {
				Keys []Key `json:"keys"`
			}
			if json.Unmarshal(raw, &response) != nil || response.Keys == nil {
				return core.Decoded[[]Key]{}, types.NewError(types.ProtocolError)
			}
			return core.Decoded[[]Key]{Data: response.Keys}, nil
		}
		result, err := core.DoJSON(c.runtime, ctx, operation("secrets.v1.secrets.list", "GET"), core.Request{Scope: c.scope, Path: "/v1/", Query: url.Values{"list": {"true"}}}, decode)
		return pagination.SourcePage[Key]{Items: result.Data, Done: true, Metadata: result.Metadata}, err
	}
	return pagination.New(plan, options, fetch)
}
func (c *Client) ListMetadata(ctx context.Context, options pagination.Options) (pagination.Collection[Key], error) {
	i, err := c.IterateMetadata(options)
	if err != nil {
		return pagination.Collection[Key]{Items: []Key{}, Complete: false, StopReason: "error"}, err
	}
	return i.Collect(ctx)
}
func (c *Client) ListPage(ctx context.Context, options pagination.Options) (pagination.Page[Key], error) {
	i, err := c.IterateMetadata(options)
	if err != nil {
		return pagination.Page[Key]{}, err
	}
	return i.Next(ctx)
}

func (c *Client) GetMetadata(ctx context.Context, name string) (types.Result[Versions], error) {
	path, pathErr := keyPath(name)
	decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Versions], error) {
		var value Versions
		if json.Unmarshal(raw, &value) != nil {
			return core.Decoded[Versions]{}, types.NewError(types.ProtocolError)
		}
		if value.Name != nil && *value.Name != name {
			return core.Decoded[Versions]{}, types.NewError(types.ScopeMismatch)
		}
		value.Ref = secretRef(endpoint, name)
		return core.Decoded[Versions]{Data: value}, nil
	}
	return core.DoJSON(c.runtime, ctx, operation("secrets.v1.versions.list", "GET"), core.Request{Scope: c.scope, Path: path + "/versions", Validate: func() error { return pathErr }}, decode)
}

func decodeSecret(name string, versionID *types.Integer) core.Decoder[Secret] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Secret], error) {
		var response struct {
			CreatedAtRaw *string `json:"created_at"`
			Description  *string `json:"description"`
			Name         *string `json:"name"`
			Version      *struct {
				VersionMetadata
				Value *string `json:"value"`
			} `json:"version"`
		}
		if json.Unmarshal(raw, &response) != nil {
			return core.Decoded[Secret]{}, types.NewError(types.ProtocolError)
		}
		ref := secretRef(endpoint, name)
		partial := core.Decoded[Secret]{Refs: []types.ResourceRef{ref}}
		if response.Name != nil && *response.Name != name {
			return core.Decoded[Secret]{}, types.NewError(types.ScopeMismatch)
		}
		if response.Version == nil || response.Version.Value == nil {
			return partial, types.NewError(types.ProtocolError)
		}
		if versionID != nil && (response.Version.ID == nil || response.Version.ID.String() != versionID.String()) {
			return core.Decoded[Secret]{}, types.NewError(types.ScopeMismatch)
		}
		bytes, err := base64.StdEncoding.DecodeString(*response.Version.Value)
		if err != nil {
			return partial, types.NewError(types.ProtocolError)
		}
		partial.Data = Secret{CreatedAtRaw: response.CreatedAtRaw, Description: response.Description, Name: response.Name, Version: &Version{VersionMetadata: response.Version.VersionMetadata, Value: types.Protect(bytes)}, Ref: ref}
		return partial, nil
	}
}
func (c *Client) ReadValue(ctx context.Context, name string) (types.Result[Secret], error) {
	path, err := keyPath(name)
	return core.DoJSON(c.runtime, ctx, operation("secrets.v1.secrets.get", "GET"), core.Request{Scope: c.scope, Path: path, Validate: func() error { return err }}, decodeSecret(name, nil))
}
func (c *Client) ReadVersion(ctx context.Context, name string, id types.Integer) (types.Result[Secret], error) {
	path, err := keyPath(name)
	if id.String() == "" {
		err = types.NewError(types.InvalidArgument)
	}
	return core.DoJSON(c.runtime, ctx, operation("secrets.v1.versions.get", "GET"), core.Request{Scope: c.scope, Path: path + "/versions/" + id.String(), Validate: func() error { return err }}, decodeSecret(name, &id))
}

func (c *Client) Create(ctx context.Context, request CreateRequest) (types.Result[Secret], error) {
	path, pathErr := keyPath(request.Name)
	body := map[string]any{}
	if request.Description != nil {
		body["description"] = *request.Description
	}
	if request.Value != nil {
		body["value"] = base64.StdEncoding.EncodeToString(request.Value.Reveal())
	}
	raw, _ := json.Marshal(body)
	return core.DoJSON(c.runtime, ctx, operation("secrets.v1.secrets.create", "POST"), core.Request{Scope: c.scope, Path: path, Body: raw, Validate: func() error { return pathErr }}, decodeSecret(request.Name, nil))
}
func (c *Client) CreateVersion(ctx context.Context, request CreateVersionRequest) (types.Result[Secret], error) {
	path, pathErr := keyPath(request.Name)
	body := map[string]any{}
	if request.Value != nil {
		body["value"] = base64.StdEncoding.EncodeToString(request.Value.Reveal())
	}
	raw, _ := json.Marshal(body)
	query := url.Values{}
	if request.Activate != nil {
		query.Set("activate", strconv.FormatBool(*request.Activate))
	}
	return core.DoJSON(c.runtime, ctx, operation("secrets.v1.versions.create", "POST"), core.Request{Scope: c.scope, Path: path + "/versions", Body: raw, Query: query, Validate: func() error { return pathErr }}, decodeSecret(request.Name, nil))
}
func emptyResponse(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
	if len(strings.TrimSpace(string(raw))) != 0 {
		return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
	}
	return core.Decoded[struct{}]{}, nil
}
func (c *Client) Update(ctx context.Context, request UpdateRequest) (types.Result[struct{}], error) {
	path, pathErr := keyPath(request.Ref.ID)
	pathErr = validateKeyRef(request.Ref, pathErr)
	body := map[string]any{}
	if request.Description != nil {
		body["description"] = *request.Description
	}
	raw, _ := json.Marshal(body)
	return core.DoJSON(c.runtime, ctx, operation("secrets.v1.secrets.replace", "PUT"), core.Request{Scope: c.scope, Path: path, Body: raw, Refs: []types.ResourceRef{request.Ref}, PinnedEndpoint: pinned(request.Ref), Validate: func() error { return pathErr }}, emptyResponse)
}
func (c *Client) Delete(ctx context.Context, ref types.ResourceRef) (types.Result[struct{}], error) {
	path, pathErr := keyPath(ref.ID)
	pathErr = validateKeyRef(ref, pathErr)
	return core.DoJSON(c.runtime, ctx, operation("secrets.v1.secrets.delete", "DELETE"), core.Request{Scope: c.scope, Path: path, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pinned(ref), Validate: func() error { return pathErr }}, emptyResponse)
}
func (c *Client) ActivateVersion(ctx context.Context, ref types.ResourceRef, id types.Integer) (types.Result[struct{}], error) {
	path, pathErr := keyPath(ref.ID)
	pathErr = validateKeyRef(ref, pathErr)
	if id.String() == "" {
		pathErr = types.NewError(types.InvalidArgument)
	}
	op := operation("secrets.v1.versions_activate.create", "POST")
	op.Success = []int{204}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/versions/" + id.String() + "/activate", Refs: []types.ResourceRef{ref}, PinnedEndpoint: pinned(ref), Validate: func() error { return pathErr }}, emptyResponse)
}
