package mkslegacy

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) ListKubeVersions(ctx context.Context, options pagination.Options) (pagination.Collection[KubeVersion], error) {
	op := core.Operation{ID: "mks.v1.kubeversions.list", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[KubeVersion]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/kubeversions"}, struct{}{}, options, "kube_versions", func(v *KubeVersion, _ types.EndpointBinding) error { return v.Validate() })
	if e != nil {
		return pagination.Collection[KubeVersion]{}, e
	}
	return iterator.Collect(ctx)
}

func (c *Client) ListAdmissionControllers(ctx context.Context, options pagination.Options) (pagination.Collection[AdmissionControllers], error) {
	op := core.Operation{ID: "mks.v1.admission_controllers.list", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[AdmissionControllers]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/admission-controllers"}, struct{}{}, options, "admission_controllers", func(v *AdmissionControllers, _ types.EndpointBinding) error { return v.Validate() })
	if e != nil {
		return pagination.Collection[AdmissionControllers]{}, e
	}
	return iterator.Collect(ctx)
}

func (c *Client) ListFeatureGates(ctx context.Context, options pagination.Options) (pagination.Collection[FeatureGates], error) {
	op := core.Operation{ID: "mks.v1.feature_gates.list", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[FeatureGates]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/feature-gates"}, struct{}{}, options, "feature_gates", func(v *FeatureGates, _ types.EndpointBinding) error { return v.Validate() })
	if e != nil {
		return pagination.Collection[FeatureGates]{}, e
	}
	return iterator.Collect(ctx)
}
