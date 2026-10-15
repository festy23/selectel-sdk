// Package letsencrypt manages project-scoped Let's Encrypt certificates.
package letsencrypt

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client { return &Client{r, s} }
func (c *Client) operation(id string) (core.Operation, error) {
	op := core.Operation{ID: id, Service: "certificates", APIGeneration: "le_v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, Success: []int{200}}
	if id == "certificates.le_v1.certificates.issue" {
		op.Method = "POST"
		op.Effect = "mutation"
		op.Rejected = []int{400, 401, 403}
	} else if id == "certificates.le_v1.certificates.delete" {
		op.Method = "DELETE"
		op.Effect = "mutation"
		op.Success = []int{204}
		op.Rejected = []int{400, 401, 403, 404}
	} else if id != "certificates.le_v1.certificates.list" {
		return op, types.NewError(types.CapabilityUnavailable)
	}
	return c.runtime.BindDocumented(op, c.scope)
}
func (c *Client) valid(ref types.ResourceRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Scope != c.scope || ref.Service != "certificates" || ref.APIGeneration != "le_v1" || ref.Kind != "lets_encrypt_certificate" || ref.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.ID == "." || ref.ID == ".." || strings.ContainsAny(ref.ID, "\r\n\x00") {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func (c *Client) List(options pagination.Options) (*pagination.Iterator[Certificate], error) {
	op, err := c.operation("certificates.le_v1.certificates.list")
	if err != nil {
		return nil, err
	}
	plan, err := c.runtime.PagePlan(op.ID, op.APIGeneration, c.scope, op.EndpointID, struct{ EndpointURL string }{op.FixedEndpoint})
	if err != nil {
		return nil, err
	}
	plan.FixedSize = true
	return pagination.New(plan, options, func(ctx context.Context, wire string, _ uint) (pagination.SourcePage[Certificate], error) {
		if wire != "" {
			return pagination.SourcePage[Certificate]{}, types.NewError(types.InvalidArgument)
		}
		items := []Certificate{}
		result, err := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/", PinnedEndpoint: &types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.scope, APIGeneration: op.APIGeneration, Audience: "selectel"}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]Certificate], error) {
			var env struct {
				Items []json.RawMessage `json:"items"`
			}
			if json.Unmarshal(raw, &env) != nil || env.Items == nil {
				return core.Decoded[[]Certificate]{}, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			for _, row := range env.Items {
				var v Certificate
				if json.Unmarshal(row, &v) != nil || v.ID == nil || *v.ID == "" || seen[*v.ID] {
					return core.Decoded[[]Certificate]{}, types.NewError(types.ProtocolError)
				}
				if v.ProjectID != nil && *v.ProjectID != c.scope.ProjectID {
					return core.Decoded[[]Certificate]{}, types.NewError(types.ScopeMismatch)
				}
				v.Ref = *core.Ref(ep, "certificates", "lets_encrypt_certificate", *v.ID)
				if c.valid(v.Ref) != nil {
					return core.Decoded[[]Certificate]{}, types.NewError(types.ProtocolError)
				}
				seen[*v.ID] = true
				items = append(items, v)
			}
			return core.Decoded[[]Certificate]{Data: items}, nil
		})
		return pagination.SourcePage[Certificate]{Items: items, Done: err == nil, Metadata: result.Metadata}, err
	})
}

func (c *Client) Issue(ctx context.Context, spec IssueRequest, options IssueOptions, call types.CallOptions) (types.Result[Certificate], error) {
	op, bind := c.operation("certificates.le_v1.certificates.issue")
	body, marshal := json.Marshal(spec)
	query := url.Values{}
	if options.DNSV2 != nil {
		query.Set("dnsv2", strconv.FormatBool(*options.DNSV2))
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/issue", Query: query, Body: body, Options: call, Validate: func() error {
		if bind != nil {
			return bind
		}
		if marshal != nil || spec.Domains == nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[Certificate], error) {
		var identity struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"project_id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ID == nil || *identity.ID == "" {
			return core.Decoded[Certificate]{}, types.NewError(types.ProtocolError)
		}
		if identity.ProjectID != nil && *identity.ProjectID != c.scope.ProjectID {
			return core.Decoded[Certificate]{}, types.NewError(types.ScopeMismatch)
		}
		ref := *core.Ref(ep, "certificates", "lets_encrypt_certificate", *identity.ID)
		if c.valid(ref) != nil {
			return core.Decoded[Certificate]{}, types.NewError(types.ProtocolError)
		}
		out := core.Decoded[Certificate]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = ref
		return out, nil
	})
}

func (c *Client) Ref(id string) (types.ResourceRef, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "\r\n\x00") {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, err := c.operation("certificates.le_v1.certificates.list")
	if err != nil {
		return types.ResourceRef{}, err
	}
	return types.ResourceRef{Scope: c.scope, Service: "certificates", APIGeneration: "le_v1", Kind: "lets_encrypt_certificate", ID: id, EndpointID: op.EndpointID, EndpointURL: op.FixedEndpoint}, nil
}
func (c *Client) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	op, bind := c.operation("certificates.le_v1.certificates.delete")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, Options: options, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if bind != nil {
			return bind
		}
		return c.valid(ref)
	}}, func(_ []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
