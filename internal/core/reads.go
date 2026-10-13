package core

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/endpoints"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"time"
)

func (r *Runtime) CollectionTimeout() time.Duration { return r.config.CollectionTimeout }
func (r *Runtime) MetricSampleLimit() uint          { return r.config.MaxMetricSamples }

// BindDocumented selects a published address or an exact explicit override.
// It performs no credential lookup or network request.
func (r *Runtime) BindDocumented(op Operation, scope types.Scope) (Operation, error) {
	if scope.Validate() != nil {
		return op, types.NewError(types.InvalidArgument)
	}
	if scope.AccountID != r.config.AccountID {
		return op, types.NewError(types.ScopeMismatch)
	}
	if (op.RequireProject && scope.ProjectID == "") || (op.RequireLocation && scope.Pool == "") {
		return op, types.NewError(types.InvalidArgument)
	}
	if scope.Region != "" || scope.Pool != "" {
		if _, err := endpoints.Location(scope.Region, scope.Pool); err != nil {
			return op, err
		}
	}
	var endpoint types.EndpointBinding
	var err error
	if override, ok := r.config.EndpointOverrides[types.EndpointKey{Service: op.Service, APIGeneration: op.APIGeneration, Scope: scope}]; ok {
		endpoint = override
	} else if op.Service == "registry" {
		endpoint, err = endpoints.Registry(scope, op.APIGeneration)
	} else {
		endpoint, err = endpoints.Resolve(op.Service, op.APIGeneration, scope)
	}
	if err != nil {
		return op, err
	}
	op.EndpointID, op.FixedEndpoint = endpoint.ID, endpoint.URL
	return op, nil
}

// FixedIterator binds APIs documented to return one complete provider page.
// Local truncation is represented by a replay cursor and skip, never success.
func FixedIterator[T any](r *Runtime, op Operation, request Request, semantic any, options pagination.Options, field string, validate func(*T, types.EndpointBinding) error) (*pagination.Iterator[T], error) {
	if request.Validate != nil {
		if err := request.Validate(); err != nil {
			return nil, err
		}
	}
	if request.Scope.AccountID != r.config.AccountID {
		return nil, types.NewError(types.ScopeMismatch)
	}
	plan, err := r.PagePlan(op.ID, op.APIGeneration, request.Scope, op.EndpointID, semantic)
	if err != nil {
		return nil, err
	}
	plan.FixedSize = true
	fetch := func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[T], error) {
		decode := func(raw []byte, endpoint types.EndpointBinding) (Decoded[[]T], error) {
			var envelope map[string]json.RawMessage
			if json.Unmarshal(raw, &envelope) != nil {
				return Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			var items []T
			if json.Unmarshal(envelope[field], &items) != nil || items == nil {
				return Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			for i := range items {
				if err := validate(&items[i], endpoint); err != nil {
					return Decoded[[]T]{}, err
				}
			}
			return Decoded[[]T]{Data: items}, nil
		}
		result, err := DoJSON(r, ctx, op, request, decode)
		return pagination.SourcePage[T]{Items: result.Data, Done: true, Metadata: result.Metadata}, err
	}
	return pagination.New(plan, options, fetch)
}

func Ref(endpoint types.EndpointBinding, service, kind, id string) *types.ResourceRef {
	if id == "" {
		return nil
	}
	return &types.ResourceRef{Scope: endpoint.Scope, Service: service, APIGeneration: endpoint.APIGeneration, Kind: kind, ID: id, EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
}
