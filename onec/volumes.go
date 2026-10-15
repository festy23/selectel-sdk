package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) ListVolumeSupportedRegions(ctx context.Context, options pagination.Options) (pagination.Collection[string], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_volumes_supported_regions.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[string]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/clusters/volumes/supported-regions"}, "", options, "regions", func(v *string, _ types.EndpointBinding) error {
		if *v == "" {
			return types.NewError(types.ProtocolError)
		}
		return nil
	})
	if e != nil {
		return pagination.Collection[string]{}, e
	}
	return iterator.Collect(ctx)
}

// VerifyVolumeSpec checks mount options; it does not mount a volume.
func (c *Client) VerifyVolumeSpec(ctx context.Context, spec VerifyClusterVolumeSpecRequest, options types.CallOptions) (types.Result[VerifyClusterVolumeSpecResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_volumes_verify.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := wireBody(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/volumes/verify", Body: body, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[VerifyClusterVolumeSpecResponse], error) {
		var v VerifyClusterVolumeSpecResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return core.Decoded[VerifyClusterVolumeSpecResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[VerifyClusterVolumeSpecResponse]{Data: v}, nil
	})
}

type VolumeListing struct {
	pagination.Collection[ClusterVolume]
	Envelopes []ListClusterVolumesResponse `json:"envelopes,omitempty"`
}

func (c *Client) volumeIterator(cluster types.ResourceRef, options pagination.Options, envelopes *[]ListClusterVolumesResponse) (*pagination.Iterator[ClusterVolume], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_volumes.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return nil, e
	}
	if e := c.validateClusterAnchor(cluster, op); e != nil {
		return nil, e
	}
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	plan, e := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, cluster)
	if e != nil {
		return nil, e
	}
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[ClusterVolume], error) {
		query := url.Values{"cursor.limit": {strconv.FormatUint(uint64(size), 10)}}
		if input != "" {
			query.Set("cursor.cursor", input)
		}
		out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/volumes", Query: query, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ListClusterVolumesResponse], error) {
			var v ListClusterVolumesResponse
			if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Volumes == nil {
				return core.Decoded[ListClusterVolumesResponse]{}, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			for i := range v.Volumes {
				row := &v.Volumes[i]
				if row.ID == nil || *row.ID == "" || seen[*row.ID] {
					return core.Decoded[ListClusterVolumesResponse]{}, types.NewError(types.ProtocolError)
				}
				if *row.Spec.ClusterID != cluster.ID {
					return core.Decoded[ListClusterVolumesResponse]{}, types.NewError(types.ScopeMismatch)
				}
				seen[*row.ID] = true
				row.Ref = core.Ref(ep, "cloud_1c", "cluster_volume", *row.ID)
				row.Ref.Parent = cloneResourceRef(&cluster)
			}
			return core.Decoded[ListClusterVolumesResponse]{Data: v}, nil
		})
		page := pagination.SourcePage[ClusterVolume]{Metadata: out.Metadata, Next: input}
		if e != nil {
			return page, e
		}
		page.Items = out.Data.Volumes
		out.Data.Volumes = nil
		if out.Data.Cursor != nil && out.Data.Cursor.Next != nil {
			page.Next = *out.Data.Cursor.Next
		}
		if envelopes != nil {
			*envelopes = append(*envelopes, out.Data)
		}
		return page, nil
	}
	return pagination.New(plan, options, fetch)
}

func (c *Client) ListClusterVolumes(ctx context.Context, cluster types.ResourceRef, options pagination.Options) (VolumeListing, error) {
	result := VolumeListing{}
	iterator, e := c.volumeIterator(cluster, options, &result.Envelopes)
	if e != nil {
		return result, e
	}
	result.Collection, e = iterator.Collect(ctx)
	return result, e
}

func scanVolumeMutation(raw []byte, expected string, cluster types.ResourceRef) (string, error) {
	var identity struct {
		Volume *struct {
			ID   *string `json:"id"`
			Spec *struct {
				ClusterID *string `json:"clusterId"`
			} `json:"spec"`
		} `json:"volume"`
		Cluster *struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"projectId"`
		} `json:"cluster"`
	}
	if json.Unmarshal(raw, &identity) != nil || identity.Volume == nil || identity.Volume.ID == nil || *identity.Volume.ID == "" {
		return "", types.NewError(types.ProtocolError)
	}
	if expected != "" && *identity.Volume.ID != expected || identity.Volume.Spec != nil && identity.Volume.Spec.ClusterID != nil && *identity.Volume.Spec.ClusterID != cluster.ID || identity.Cluster != nil && (identity.Cluster.ID != nil && *identity.Cluster.ID != cluster.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != cluster.Scope.ProjectID) {
		return "", types.NewError(types.ScopeMismatch)
	}
	return *identity.Volume.ID, nil
}

// MountClusterVolume records only an acknowledged exact child in its caller parent.
func (c *Client) MountClusterVolume(ctx context.Context, cluster types.ResourceRef, spec MountClusterVolumeBody, options types.CallOptions) (types.Result[MountClusterVolumeResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_volumes.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := wireBody(spec)
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/volumes", Body: body, Options: options, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateClusterAnchor(cluster, op); e != nil {
			return e
		}
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		if *spec.Spec.ClusterID != cluster.ID {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[MountClusterVolumeResponse], error) {
		id, e := scanVolumeMutation(raw, "", cluster)
		if e != nil {
			return core.Decoded[MountClusterVolumeResponse]{}, e
		}
		ref := core.Ref(ep, "cloud_1c", "cluster_volume", id)
		ref.Parent = cloneResourceRef(&cluster)
		decoded := core.Decoded[MountClusterVolumeResponse]{Refs: []types.ResourceRef{*ref}}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil || decoded.Data.Volume == nil || decoded.Data.Volume.Spec == nil || *decoded.Data.Volume.Spec.ClusterID != cluster.ID {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Volume.Ref = cloneResourceRef(ref)
		if decoded.Data.Cluster != nil {
			decoded.Data.Cluster.Ref = cloneResourceRef(&cluster)
		}
		return decoded, nil
	})
	if out.Receipt != nil {
		known := false
		for _, ref := range out.Receipt.ResourceRefs {
			if ref.Kind == "cluster_volume" {
				known = true
			}
		}
		if !known {
			out.Receipt.Recovery = "unavailable"
		}
	}
	return out, e
}

var volumePending = map[string]bool{"CLUSTER_VOLUME_STATUS_PENDING": true, "CLUSTER_VOLUME_STATUS_MOUNTING": true, "CLUSTER_VOLUME_STATUS_UNMOUNTING": true}

// ClusterVolumeHandle uses bounded native list pages and matches only an exact
// acknowledged/caller ID. A partial list never proves absence.
func (c *Client) ClusterVolumeHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	op := core.Operation{Service: "cloud_1c", APIGeneration: "v2", RequireProject: true, RequireLocation: true}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return nil, e
	}
	if e := c.validateClusterChild(ref, "cluster_volume", op); e != nil {
		return nil, e
	}
	ref = *cloneResourceRef(&ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		call, cancel := context.WithTimeout(ctx, c.runtime.CallTimeout())
		defer cancel()
		iterator, e := c.volumeIterator(*ref.Parent, pagination.Options{}, nil)
		if e != nil {
			return operations.Observation{}, e
		}
		for {
			page, e := iterator.Next(call)
			if usableObservationPage(e) {
				for _, row := range page.Items {
					if *row.ID == ref.ID {
						state := ""
						class := "unknown"
						if row.State != nil && row.State.Status != nil {
							state = *row.State.Status
						}
						switch state {
						case "CLUSTER_VOLUME_STATUS_ACTIVE":
							class = "succeeded"
						case "CLUSTER_VOLUME_STATUS_DELETED":
							class = "absent"
						case "CLUSTER_VOLUME_STATUS_ERRORED":
							class = "failed"
						default:
							if volumePending[state] {
								class = "pending"
							}
						}
						return operations.Observation{Ref: ref, ProviderState: state, Classification: class, ObservedAt: page.Metadata.ObservedAt, Metadata: page.Metadata}, nil
					}
				}
			}
			if e != nil {
				if e == io.EOF || usableObservationPage(e) {
					return operations.Observation{}, types.NewError(types.Incomplete)
				}
				return operations.Observation{}, e
			}
			if page.Done {
				return operations.Observation{}, types.NewError(types.Incomplete)
			}
		}
	}
	activePending := map[string]bool{}
	deletedPending := map[string]bool{"CLUSTER_VOLUME_STATUS_ACTIVE": true}
	for s := range volumePending {
		activePending[s] = true
		deletedPending[s] = true
	}
	return c.bindChildHandle(ref, receipt, read, map[string]operations.Goal{
		"cluster_volume.active":  {Success: map[string]bool{"CLUSTER_VOLUME_STATUS_ACTIVE": true}, Failure: map[string]bool{"CLUSTER_VOLUME_STATUS_ERRORED": true, "CLUSTER_VOLUME_STATUS_DELETED": true}, Pending: activePending},
		"cluster_volume.deleted": {Success: map[string]bool{"CLUSTER_VOLUME_STATUS_DELETED": true}, Failure: map[string]bool{"CLUSTER_VOLUME_STATUS_ERRORED": true}, Pending: deletedPending},
	})
}

func (c *Client) UnmountClusterVolume(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[UnmountClusterVolumeResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_volumes.delete", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodDelete, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	refErr := ref.Validate()
	if refErr == nil {
		ref = *cloneResourceRef(&ref)
	}
	cluster := types.ResourceRef{}
	if ref.Parent != nil {
		cluster = *ref.Parent
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/volumes/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if refErr != nil {
			return refErr
		}
		if bindErr != nil {
			return bindErr
		}
		return c.validateClusterChild(ref, "cluster_volume", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[UnmountClusterVolumeResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil {
			return core.Decoded[UnmountClusterVolumeResponse]{}, types.NewError(types.ProtocolError)
		}
		if identity.Cluster != nil && (identity.Cluster.ID != nil && *identity.Cluster.ID != cluster.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID) {
			return core.Decoded[UnmountClusterVolumeResponse]{}, types.NewError(types.ScopeMismatch)
		}
		var v UnmountClusterVolumeResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return core.Decoded[UnmountClusterVolumeResponse]{}, types.NewError(types.ProtocolError)
		}
		if v.Cluster != nil {
			v.Cluster.Ref = cloneResourceRef(&cluster)
		}
		return core.Decoded[UnmountClusterVolumeResponse]{Data: v, Refs: []types.ResourceRef{ref}}, nil
	})
}
