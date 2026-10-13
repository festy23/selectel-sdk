package core

import (
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

func (r *Runtime) PagePlan(operationID, generation string, scope types.Scope, endpointID string, semantic any) (pagination.Plan, error) {
	capability, err := r.catalog.Describe(operationID)
	if err != nil {
		return pagination.Plan{}, err
	}
	if override, ok := r.config.EndpointOverrides[types.EndpointKey{Service: capability.Service, APIGeneration: generation, Scope: scope}]; ok {
		endpointID = override.ID
	}
	return pagination.Plan{OperationID: operationID, ContractRevision: capability.ContractRevision, Scope: scope, EndpointID: endpointID, APIGeneration: generation, SemanticRequest: semantic, Defaults: pagination.Options{PageSize: r.config.PageSize, MaxItems: r.config.MaxItems, MaxPages: r.config.MaxPages}, CollectionTimeout: r.config.CollectionTimeout}, nil
}
