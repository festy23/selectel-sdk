package mks

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Clusters) service() service { return service{runtime: c.runtime, scope: c.scope} }
func (c *Clusters) decoder(expected string) core.Decoder[DetailedCluster] {
	return decodeObject("cluster", "cluster", expected, nil, func(v *DetailedCluster, r types.ResourceRef) { v.Ref = &r }, func(v DetailedCluster) error {
		if *v.ProjectID != c.scope.ProjectID || *v.Pool != c.scope.Pool {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	})
}
func (c *Clusters) Create(ctx context.Context, spec ClusterCreateSpec) (types.Result[DetailedCluster], error) {
	validation := inputError(spec)
	if validation == nil && *spec.Pool != c.scope.Pool {
		validation = types.NewError(types.ScopeMismatch)
	}
	if validation == nil && !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,31}$`).MatchString(*spec.Name) {
		validation = types.NewError(types.InvalidArgument)
	}
	if validation == nil && spec.ClusterType != nil && *spec.ClusterType != "BASIC" && *spec.ClusterType != "HIGH_AVAILABILITY" && *spec.ClusterType != "HIGH_AVAILABILITY_MULTI_AZ" {
		validation = types.NewError(types.InvalidArgument)
	}
	if validation == nil {
		var software map[string]json.RawMessage
		if json.Unmarshal(spec.AdditionalSoftware.Reveal(), &software) != nil || software == nil {
			validation = types.NewError(types.InvalidArgument)
		}
	}
	body, err := requestBody("cluster", spec)
	if validation == nil {
		validation = err
	}
	return invoke(c.service(), ctx, "mks.v2.clusters.create", "POST", "/clusters", 201, nil, body, nil, validation, c.decoder(""))
}
func (c *Clusters) Get(ctx context.Context, ref types.ResourceRef) (types.Result[DetailedCluster], error) {
	return invoke(c.service(), ctx, "mks.v2.clusters.get", "GET", resourcePath(ref), 200, &ref, nil, nil, validateRef(ref, "cluster", c.scope), c.decoder(ref.ID))
}
func (c *Clusters) change(ctx context.Context, ref types.ResourceRef, spec ClusterUpdateSpec, method, action string) (types.Result[DetailedCluster], error) {
	validation := validateRef(ref, "cluster", c.scope)
	if validation == nil {
		validation = inputError(spec)
	}
	body, err := requestBody("cluster", spec)
	if validation == nil {
		validation = err
	}
	return invoke(c.service(), ctx, "mks.v2.clusters."+action, method, resourcePath(ref), 200, &ref, body, nil, validation, c.decoder(ref.ID))
}
func (c *Clusters) Replace(ctx context.Context, ref types.ResourceRef, spec ClusterUpdateSpec) (types.Result[DetailedCluster], error) {
	return c.change(ctx, ref, spec, "PUT", "replace")
}
func (c *Clusters) Update(ctx context.Context, ref types.ResourceRef, spec ClusterUpdateSpec) (types.Result[DetailedCluster], error) {
	return c.change(ctx, ref, spec, "PATCH", "update")
}
func (c *Clusters) Delete(ctx context.Context, ref types.ResourceRef) (types.Result[struct{}], error) {
	return invoke(c.service(), ctx, "mks.v2.clusters.delete", "DELETE", resourcePath(ref), 204, &ref, nil, nil, validateRef(ref, "cluster", c.scope), empty)
}
func (c *Clusters) Kubeconfig(ctx context.Context, ref types.ResourceRef) (types.Result[types.ProtectedValue], error) {
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[types.ProtectedValue], error) {
		if len(raw) == 0 {
			return core.Decoded[types.ProtectedValue]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[types.ProtectedValue]{Data: types.Protect(raw)}, nil
	}
	return invoke(c.service(), ctx, "mks.v2.clusters_kubeconfig.list", "GET", resourcePath(ref)+"/kubeconfig", 200, &ref, nil, nil, validateRef(ref, "cluster", c.scope), decode)
}
func (c *Clusters) RotateCertificates(ctx context.Context, ref types.ResourceRef) (types.Result[struct{}], error) {
	return invoke(c.service(), ctx, "mks.v2.clusters_rotate_certs.create", "POST", resourcePath(ref)+"/rotate-certs", 204, &ref, nil, nil, validateRef(ref, "cluster", c.scope), empty)
}
func (c *Clusters) upgrade(ctx context.Context, ref types.ResourceRef, kind string) (types.Result[DetailedCluster], error) {
	return invoke(c.service(), ctx, "mks.v2.clusters_upgrade_"+kind+"_version.create", "POST", resourcePath(ref)+"/upgrade-"+kind+"-version", 200, &ref, nil, nil, validateRef(ref, "cluster", c.scope), c.decoder(ref.ID))
}
func (c *Clusters) UpgradePatchVersion(ctx context.Context, ref types.ResourceRef) (types.Result[DetailedCluster], error) {
	return c.upgrade(ctx, ref, "patch")
}
func (c *Clusters) UpgradeMinorVersion(ctx context.Context, ref types.ResourceRef) (types.Result[DetailedCluster], error) {
	return c.upgrade(ctx, ref, "minor")
}

var clusterPending = map[string]bool{"PENDING_CREATE": true, "PENDING_UPDATE": true, "PENDING_UPGRADE": true, "PENDING_ROTATE_CERTS": true, "PENDING_DELETE": true, "PENDING_RESIZE": true, "PENDING_NODE_REINSTALL": true, "PENDING_UPGRADE_PATCH_VERSION": true, "PENDING_UPGRADE_MINOR_VERSION": true, "PENDING_UPDATE_NODEGROUP": true, "PENDING_UPGRADE_MASTERS_CONFIGURATION": true, "PENDING_REINSTALL_MASTERS": true, "PENDING_UPGRADE_CLUSTER_CONFIGURATION": true, "MAINTENANCE": true, "PENDING_NODES_UNSHELVE": true, "PENDING_UPDATE_REGISTRIES_INTEGRATION": true}

func observe(ref types.ResourceRef, state string, metadata types.Metadata, pending map[string]bool) operations.Observation {
	class := "unknown"
	if state == "ACTIVE" || state == "DONE" {
		class = "succeeded"
	} else if state == "ERROR" || state == "CANCELED" {
		class = "failed"
	} else if pending[state] {
		class = "pending"
	}
	return operations.Observation{Ref: ref, ProviderState: state, Classification: class, ObservedAt: metadata.ObservedAt, Metadata: metadata}
}
func deletedPending(pending map[string]bool) map[string]bool {
	out := map[string]bool{"ACTIVE": true}
	for state := range pending {
		out[state] = true
	}
	return out
}
func (c *Clusters) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if err := validateRef(ref, "cluster", c.scope); err != nil {
		return nil, err
	}
	ref = ownedRef(ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		v, err := c.Get(ctx, ref)
		if err != nil {
			return operations.Observation{}, err
		}
		return observe(ref, *v.Data.Status, v.Metadata, clusterPending), nil
	}
	return handle(c.service(), ref, receipt, read, map[string]operations.Goal{"cluster.active": {Success: map[string]bool{"ACTIVE": true}, Failure: map[string]bool{"ERROR": true}, Pending: clusterPending}, "cluster.deleted": {Failure: map[string]bool{"ERROR": true}, Pending: deletedPending(clusterPending), AbsentSuccess: true}})
}

type CreateAndWaitResult struct {
	Create types.Result[DetailedCluster]
	Wait   operations.WaitResult
}

func (c *Clusters) CreateAndWait(ctx context.Context, spec ClusterCreateSpec) (CreateAndWaitResult, error) {
	v, err := c.Create(ctx, spec)
	out := CreateAndWaitResult{Create: v}
	if err != nil {
		return out, err
	}
	h, err := c.Handle(*v.Data.Ref, v.Receipt)
	if err != nil {
		return out, err
	}
	out.Wait, err = h.Wait(ctx, "cluster.active")
	return out, err
}
