package projects

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type CreateRequest struct {
	Name           string  `json:"name"`
	Description    *string `json:"description,omitempty"`
	SkipQuotasInit *bool   `json:"skip_quotas_init,omitempty"`
}
type GetOptions struct {
	ExcludeQuotas *bool `json:"exclude_quotas,omitempty"`
}
type ThemeRequest struct {
	BrandColor *types.Nullable[string] `json:"brand_color,omitempty"`
	Color      *types.Nullable[string] `json:"color,omitempty"`
	Logo       *types.Nullable[string] `json:"logo,omitempty"`
}
type UpdateRequest struct {
	Ref         types.ResourceRef       `json:"-"`
	Name        *string                 `json:"name,omitempty"`
	Description *string                 `json:"description,omitempty"`
	CustomURL   *types.Nullable[string] `json:"custom_url,omitempty"`
	Theme       *ThemeRequest           `json:"theme,omitempty"`
}

func projectOperation(action, method string) core.Operation {
	op := core.Operation{ID: "projects.v2.projects." + action, Service: "projects", APIGeneration: "v2", Method: method, Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}}
	if method != "GET" {
		op.Effect = "mutation"
		op.Rejected = []int{400, 401, 409}
	}
	if method == "PATCH" {
		op.Success = []int{201}
	}
	if method == "DELETE" {
		op.Success = []int{204}
		op.Rejected = []int{400, 401}
	}
	if action == "get" || action == "delete" {
		op.ErrorCodes = map[int]types.Code{400: types.NotFound}
	}
	return op
}
func validateProjectRef(ref types.ResourceRef) error {
	if ref.Kind != "project" || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil // Core checks account, service, generation and the exact endpoint.
}
func (c *Client) validateAccount() error {
	if c.scope.ProjectID != "" {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func projectPin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
}
func projectPath(id string) string {
	segment := url.PathEscape(id)
	if id == "." || id == ".." {
		segment = strings.ReplaceAll(id, ".", "%2E")
	}
	return "/v2/projects/" + segment
}
func validText(value string, min, max int) bool {
	n := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && n >= min && n <= max
}
func validNullable(value *types.Nullable[string]) bool {
	if value == nil {
		return true
	}
	text, present := value.Value()
	return !present || utf8.ValidString(text)
}

// decodeProject preserves a trustworthy provider ID before decoding other fields.
func decodeProject[T any](expectedID string, assign func(*T, *types.ResourceRef), validate func(T) error) core.Decoder[T] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[T], error) {
		var envelope struct {
			Project json.RawMessage `json:"project"`
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil || json.Unmarshal(envelope.Project, &fields) != nil || fields == nil {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		id := expectedID
		if value, exists := fields["id"]; exists && string(value) != "null" {
			var responseID string
			if json.Unmarshal(value, &responseID) != nil || responseID == "" {
				return core.Decoded[T]{}, types.NewError(types.ProtocolError)
			}
			if expectedID != "" && expectedID != responseID {
				return core.Decoded[T]{}, types.NewError(types.ScopeMismatch)
			}
			id = responseID
		}
		ref := core.Ref(endpoint, "projects", "project", id)
		if ref == nil {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		out := core.Decoded[T]{Refs: []types.ResourceRef{*ref}}
		var value T
		if json.Unmarshal(envelope.Project, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if err := validate(value); err != nil {
			return out, err
		}
		assign(&value, ref)
		out.Data = value
		return out, nil
	}
}

func (c *Client) Create(ctx context.Context, request CreateRequest, options types.CallOptions) (types.Result[CreatedProject], error) {
	body, _ := json.Marshal(struct {
		Project CreateRequest `json:"project"`
	}{request})
	validate := func() error {
		if err := c.validateAccount(); err != nil {
			return err
		}
		if !validText(request.Name, 1, 64) || (request.Description != nil && !validText(*request.Description, 0, 255)) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	return core.DoJSON(c.runtime, ctx, projectOperation("create", "POST"), core.Request{Scope: c.scope, Path: "/v2/projects", Body: body, Options: options, Validate: validate}, decodeProject("", func(v *CreatedProject, ref *types.ResourceRef) { v.Ref = ref }, CreatedProject.Validate))
}
func (c *Client) Get(ctx context.Context, ref types.ResourceRef, filters GetOptions, options types.CallOptions) (types.Result[DetailedProject], error) {
	query := url.Values{}
	if filters.ExcludeQuotas != nil {
		query.Set("exclude_quotas", strconv.FormatBool(*filters.ExcludeQuotas))
	}
	validate := func() error {
		if err := c.validateAccount(); err != nil {
			return err
		}
		return validateProjectRef(ref)
	}
	return core.DoJSON(c.runtime, ctx, projectOperation("get", "GET"), core.Request{Scope: c.scope, Path: projectPath(ref.ID), Query: query, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: projectPin(ref), Validate: validate}, decodeProject(ref.ID, func(v *DetailedProject, r *types.ResourceRef) { v.Ref = r }, DetailedProject.Validate))
}
func (c *Client) Update(ctx context.Context, request UpdateRequest, options types.CallOptions) (types.Result[UpdatedProject], error) {
	body, _ := json.Marshal(struct {
		Project UpdateRequest `json:"project"`
	}{request})
	validate := func() error {
		if err := c.validateAccount(); err != nil {
			return err
		}
		if err := validateProjectRef(request.Ref); err != nil {
			return err
		}
		if (request.Name != nil && !validText(*request.Name, 1, 64)) || (request.Description != nil && !validText(*request.Description, 0, 255)) || !validNullable(request.CustomURL) {
			return types.NewError(types.InvalidArgument)
		}
		if request.Theme != nil && (!validNullable(request.Theme.BrandColor) || !validNullable(request.Theme.Color) || !validNullable(request.Theme.Logo)) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	return core.DoJSON(c.runtime, ctx, projectOperation("update", "PATCH"), core.Request{Scope: c.scope, Path: projectPath(request.Ref.ID), Body: body, Options: options, Refs: []types.ResourceRef{request.Ref}, PinnedEndpoint: projectPin(request.Ref), Validate: validate}, decodeProject(request.Ref.ID, func(v *UpdatedProject, r *types.ResourceRef) { v.Ref = r }, UpdatedProject.Validate))
}
func (c *Client) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	validate := func() error {
		if err := c.validateAccount(); err != nil {
			return err
		}
		return validateProjectRef(ref)
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(strings.TrimSpace(string(raw))) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	}
	return core.DoJSON(c.runtime, ctx, projectOperation("delete", "DELETE"), core.Request{Scope: c.scope, Path: projectPath(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: projectPin(ref), Validate: validate}, decode)
}
