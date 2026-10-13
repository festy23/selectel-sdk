package mks

import (
	"context"
	"encoding/json"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type RegistryIntegrations struct{ service }
type Configuration struct{ service }

func direct[T validatable](raw []byte, _ types.EndpointBinding) (core.Decoded[T], error) {
	var value T
	if json.Unmarshal(raw, &value) != nil {
		return core.Decoded[T]{}, types.NewError(types.ProtocolError)
	}
	if err := value.Validate(); err != nil {
		return core.Decoded[T]{}, err
	}
	return core.Decoded[T]{Data: value}, nil
}
func (c *RegistryIntegrations) Get(ctx context.Context, cluster types.ResourceRef) (types.Result[RegistriesIntegration], error) {
	return invoke(c.service, ctx, "mks.v2.clusters_registries.list", "GET", resourcePath(cluster)+"/registries", 200, &cluster, nil, nil, validateRef(cluster, "cluster", c.scope), direct[RegistriesIntegration])
}
func (c *RegistryIntegrations) Create(ctx context.Context, cluster types.ResourceRef, specs []RegistryIntegrationCreateSpec) (types.Result[RegistriesIntegration], error) {
	validation := validateRef(cluster, "cluster", c.scope)
	if specs == nil && validation == nil {
		validation = types.NewError(types.InvalidArgument)
	}
	for _, spec := range specs {
		if validation == nil {
			validation = inputError(spec)
		}
	}
	body, err := requestBody("registries", specs)
	if validation == nil {
		validation = err
	}
	return invoke(c.service, ctx, "mks.v2.clusters_registries.create", "POST", resourcePath(cluster)+"/registries", 201, &cluster, body, nil, validation, direct[RegistriesIntegration])
}
func (c *RegistryIntegrations) Replace(ctx context.Context, cluster types.ResourceRef, specs []RegistryIntegrationUpdateSpec) (types.Result[RegistriesIntegration], error) {
	validation := validateRef(cluster, "cluster", c.scope)
	if specs == nil && validation == nil {
		validation = types.NewError(types.InvalidArgument)
	}
	for _, spec := range specs {
		if validation == nil {
			validation = inputError(spec)
		}
	}
	body, err := requestBody("registries", specs)
	if validation == nil {
		validation = err
	}
	return invoke(c.service, ctx, "mks.v2.clusters_registries.replace", "PUT", resourcePath(cluster)+"/registries", 200, &cluster, body, nil, validation, direct[RegistriesIntegration])
}
func (c *RegistryIntegrations) DeleteAll(ctx context.Context, cluster types.ResourceRef) (types.Result[struct{}], error) {
	return invoke(c.service, ctx, "mks.v2.clusters_registries.delete", "DELETE", resourcePath(cluster)+"/registries", 204, &cluster, nil, nil, validateRef(cluster, "cluster", c.scope), empty)
}
func (c *RegistryIntegrations) Delete(ctx context.Context, cluster types.ResourceRef, registryID string) (types.Result[struct{}], error) {
	validation := validateRef(cluster, "cluster", c.scope)
	if registryID == "" && validation == nil {
		validation = types.NewError(types.InvalidArgument)
	}
	return invoke(c.service, ctx, "mks.v2.clusters_registries.delete_c297d03c", "DELETE", resourcePath(cluster)+"/registries/"+segment(registryID), 204, &cluster, nil, nil, validation, empty)
}
func (c *Configuration) AdmissionControllers(ctx context.Context) (types.Result[AdmissionControllersList], error) {
	return invoke(c.service, ctx, "mks.v2.admission_controllers.list", "GET", "/admission-controllers", 200, nil, nil, nil, nil, direct[AdmissionControllersList])
}
func (c *Configuration) KubernetesVersions(ctx context.Context) (types.Result[KubeVersionsList], error) {
	return invoke(c.service, ctx, "mks.v2.kubeversions.list", "GET", "/kubeversions", 200, nil, nil, nil, nil, direct[KubeVersionsList])
}
func (c *Configuration) FeatureGates(ctx context.Context) (types.Result[FeatureGatesList], error) {
	return invoke(c.service, ctx, "mks.v2.feature_gates.list", "GET", "/feature-gates", 200, nil, nil, nil, nil, direct[FeatureGatesList])
}
