package billing

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type ProjectsRequest struct{ ProviderKeys []string }

func providerQuery(keys []string) (url.Values, error) {
	if len(keys) == 0 {
		return nil, types.NewError(types.InvalidArgument)
	}
	q := url.Values{}
	for _, key := range keys {
		if !member(key, "vpc", "serverless", "mks", "dbaas", "storage", "cdn", "vmware", "craas", "ones", "mobfarm", "ses") {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("provider_keys", key)
	}
	return q, nil
}
func (c *Client) Projects(ctx context.Context, request ProjectsRequest, options types.CallOptions) (types.Result[ProjectsResponse], error) {
	q, validationErr := providerQuery(request.ProviderKeys)
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		return validationErr
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[ProjectsResponse], error) {
		var v ProjectsResponse
		if json.Unmarshal(raw, &v) != nil || v.Data == nil {
			return core.Decoded[ProjectsResponse]{}, types.NewError(types.ProtocolError)
		}
		if e := v.Validate(); e != nil {
			return core.Decoded[ProjectsResponse]{}, e
		}
		return core.Decoded[ProjectsResponse]{Data: v}, nil
	}
	return core.DoJSON(c.runtime, ctx, operation("billing.v1.cloud_billing_statistic_projects.list", "v1"), core.Request{Scope: c.scope, Path: "/v1/cloud_billing/statistic/projects", Query: q, Options: options, Validate: validate}, decode)
}

type ObjectsRequest struct {
	ProviderKeys []string
	Locale       *string
}

func (c *Client) Objects(ctx context.Context, request ObjectsRequest, options types.CallOptions) (types.Result[ObjectsResponse], error) {
	q, validationErr := providerQuery(request.ProviderKeys)
	if request.Locale != nil {
		if !member(*request.Locale, "ru", "en") {
			validationErr = types.NewError(types.InvalidArgument)
		}
		if q != nil {
			q.Set("locale", *request.Locale)
		}
	}
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		return validationErr
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[ObjectsResponse], error) {
		var v ObjectsResponse
		if json.Unmarshal(raw, &v) != nil || v.Data == nil {
			return core.Decoded[ObjectsResponse]{}, types.NewError(types.ProtocolError)
		}
		if e := v.Validate(); e != nil {
			return core.Decoded[ObjectsResponse]{}, e
		}
		return core.Decoded[ObjectsResponse]{Data: v}, nil
	}
	return core.DoJSON(c.runtime, ctx, operation("billing.v1.cloud_billing_statistic_objects.list", "v1"), core.Request{Scope: c.scope, Path: "/v1/cloud_billing/statistic/objects", Query: q, Options: options, Validate: validate}, decode)
}
