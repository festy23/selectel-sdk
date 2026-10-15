package onec

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type DomainListing struct {
	pagination.Collection[ClusterDomain]
	Envelopes []ListClusterDomainsResponse `json:"envelopes,omitempty"`
}

func (c *Client) domainIterator(cluster types.ResourceRef, options pagination.Options, envelopes *[]ListClusterDomainsResponse) (*pagination.Iterator[ClusterDomain], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_domains.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
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
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[ClusterDomain], error) {
		query := url.Values{"cursor.limit": {strconv.FormatUint(uint64(size), 10)}}
		if input != "" {
			query.Set("cursor.cursor", input)
		}
		out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/domains", Query: query, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ListClusterDomainsResponse], error) {
			var v ListClusterDomainsResponse
			if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.ClusterDomains == nil {
				return core.Decoded[ListClusterDomainsResponse]{}, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			for i := range v.ClusterDomains {
				row := &v.ClusterDomains[i]
				if row.ID == nil || *row.ID == "" || seen[*row.ID] {
					return core.Decoded[ListClusterDomainsResponse]{}, types.NewError(types.ProtocolError)
				}
				if *row.Spec.ClusterID != cluster.ID {
					return core.Decoded[ListClusterDomainsResponse]{}, types.NewError(types.ScopeMismatch)
				}
				seen[*row.ID] = true
				row.Ref = core.Ref(ep, "cloud_1c", "cluster_domain", *row.ID)
				row.Ref.Parent = cloneResourceRef(&cluster)
			}
			return core.Decoded[ListClusterDomainsResponse]{Data: v}, nil
		})
		page := pagination.SourcePage[ClusterDomain]{Metadata: out.Metadata, Next: input}
		if e != nil {
			return page, e
		}
		page.Items = out.Data.ClusterDomains
		out.Data.ClusterDomains = nil
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

func (c *Client) ListClusterDomains(ctx context.Context, cluster types.ResourceRef, options pagination.Options) (DomainListing, error) {
	result := DomainListing{}
	iterator, e := c.domainIterator(cluster, options, &result.Envelopes)
	if e != nil {
		return result, e
	}
	result.Collection, e = iterator.Collect(ctx)
	return result, e
}

func scanDomainMutation(raw []byte, expected string, cluster types.ResourceRef) (string, error) {
	var identity struct {
		Domain *struct {
			ID   *string `json:"id"`
			Spec *struct {
				ClusterID *string `json:"clusterId"`
			} `json:"spec"`
		} `json:"domain"`
		Cluster *struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"projectId"`
		} `json:"cluster"`
	}
	if json.Unmarshal(raw, &identity) != nil || identity.Domain == nil || identity.Domain.ID == nil || *identity.Domain.ID == "" {
		return "", types.NewError(types.ProtocolError)
	}
	if expected != "" && *identity.Domain.ID != expected || identity.Domain.Spec != nil && identity.Domain.Spec.ClusterID != nil && *identity.Domain.Spec.ClusterID != cluster.ID || identity.Cluster != nil && (identity.Cluster.ID != nil && *identity.Cluster.ID != cluster.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != cluster.Scope.ProjectID) {
		return "", types.NewError(types.ScopeMismatch)
	}
	return *identity.Domain.ID, nil
}

// RegisterClusterDomain requires the documented caller-managed DNS CNAME/TTL
// precondition; this method never changes DNS or validates public reachability.
func (c *Client) RegisterClusterDomain(ctx context.Context, cluster types.ResourceRef, spec RegisterClusterDomainBody, options types.CallOptions) (types.Result[RegisterClusterDomainResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_domains.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := wireBody(spec)
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/domains", Body: body, Options: options, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint, Validate: func() error {
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
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[RegisterClusterDomainResponse], error) {
		id, e := scanDomainMutation(raw, "", cluster)
		if e != nil {
			return core.Decoded[RegisterClusterDomainResponse]{}, e
		}
		ref := core.Ref(ep, "cloud_1c", "cluster_domain", id)
		ref.Parent = cloneResourceRef(&cluster)
		decoded := core.Decoded[RegisterClusterDomainResponse]{Refs: []types.ResourceRef{*ref}}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil || decoded.Data.Domain == nil || decoded.Data.Domain.Spec == nil || *decoded.Data.Domain.Spec.ClusterID != cluster.ID {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Domain.Ref = cloneResourceRef(ref)
		if decoded.Data.Cluster != nil {
			decoded.Data.Cluster.Ref = cloneResourceRef(&cluster)
		}
		return decoded, nil
	})
	if out.Receipt != nil {
		known := false
		for _, ref := range out.Receipt.ResourceRefs {
			if ref.Kind == "cluster_domain" {
				known = true
			}
		}
		if !known {
			out.Receipt.Recovery = "unavailable"
		}
	}
	return out, e
}

func (c *Client) validateClusterChild(ref types.ResourceRef, kind string, op core.Operation) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != kind || ref.Parent == nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.EndpointUnavailable)
	}
	return c.validateClusterAnchor(*ref.Parent, op)
}

var domainPending = map[string]bool{"CLUSTER_DOMAIN_STATUS_PENDING": true, "CLUSTER_DOMAIN_STATUS_CREATING": true, "CLUSTER_DOMAIN_STATUS_TRANSFERRING": true, "CLUSTER_DOMAIN_STATUS_MODIFYING": true, "CLUSTER_DOMAIN_STATUS_DELETING": true}

func usableObservationPage(e error) bool {
	if e == nil || types.ErrorCode(e) == types.Incomplete {
		return true
	}
	var sdkErr *types.SDKError
	return errors.As(e, &sdkErr) && sdkErr.Code == types.ProtocolError && sdkErr.Reason == "cursor_no_progress"
}

// ClusterDomainHandle uses bounded native list pages and matches only an exact
// acknowledged/caller ID. A partial list never proves absence.
func (c *Client) ClusterDomainHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	op := core.Operation{Service: "cloud_1c", APIGeneration: "v2", RequireProject: true, RequireLocation: true}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return nil, e
	}
	if e := c.validateClusterChild(ref, "cluster_domain", op); e != nil {
		return nil, e
	}
	ref = *cloneResourceRef(&ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		call, cancel := context.WithTimeout(ctx, c.runtime.CallTimeout())
		defer cancel()
		iterator, e := c.domainIterator(*ref.Parent, pagination.Options{}, nil)
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
						case "CLUSTER_DOMAIN_STATUS_ACTIVE":
							class = "succeeded"
						case "CLUSTER_DOMAIN_STATUS_DELETED":
							class = "absent"
						case "CLUSTER_DOMAIN_STATUS_ERRORED":
							class = "failed"
						default:
							if domainPending[state] {
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
	deletedPending := map[string]bool{"CLUSTER_DOMAIN_STATUS_ACTIVE": true}
	for s := range domainPending {
		activePending[s] = true
		deletedPending[s] = true
	}
	return c.bindChildHandle(ref, receipt, read, map[string]operations.Goal{
		"cluster_domain.active":  {Success: map[string]bool{"CLUSTER_DOMAIN_STATUS_ACTIVE": true}, Failure: map[string]bool{"CLUSTER_DOMAIN_STATUS_ERRORED": true, "CLUSTER_DOMAIN_STATUS_DELETED": true}, Pending: activePending},
		"cluster_domain.deleted": {Success: map[string]bool{"CLUSTER_DOMAIN_STATUS_DELETED": true}, Failure: map[string]bool{"CLUSTER_DOMAIN_STATUS_ERRORED": true}, Pending: deletedPending},
	})
}

func (c *Client) PatchClusterDomain(ctx context.Context, ref types.ResourceRef, spec ClusterDomainSpec, options types.CallOptions) (types.Result[PatchClusterDomainResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_domains.update", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPatch, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	refErr := ref.Validate()
	if refErr == nil {
		ref = *cloneResourceRef(&ref)
	}
	cluster := types.ResourceRef{}
	if ref.Parent != nil {
		cluster = *ref.Parent
	}
	body, bodyErr := wireBody(spec)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/domains/" + url.PathEscape(ref.ID), Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if refErr != nil {
			return refErr
		}
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateClusterChild(ref, "cluster_domain", op); e != nil {
			return e
		}
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		if *spec.ClusterID != cluster.ID {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[PatchClusterDomainResponse], error) {
		decoded := core.Decoded[PatchClusterDomainResponse]{Refs: []types.ResourceRef{ref}}
		if _, e := scanDomainMutation(raw, ref.ID, cluster); e != nil {
			return decoded, e
		}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil || decoded.Data.Domain == nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Domain.Ref = cloneResourceRef(&ref)
		if decoded.Data.Cluster != nil {
			decoded.Data.Cluster.Ref = cloneResourceRef(&cluster)
		}
		return decoded, nil
	})
}

func (c *Client) DeleteClusterDomain(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DeleteClusterDomainResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_domains.delete", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodDelete, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
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
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/domains/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if refErr != nil {
			return refErr
		}
		if bindErr != nil {
			return bindErr
		}
		return c.validateClusterChild(ref, "cluster_domain", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[DeleteClusterDomainResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil {
			return core.Decoded[DeleteClusterDomainResponse]{}, types.NewError(types.ProtocolError)
		}
		if identity.Cluster != nil && (identity.Cluster.ID != nil && *identity.Cluster.ID != cluster.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID) {
			return core.Decoded[DeleteClusterDomainResponse]{}, types.NewError(types.ScopeMismatch)
		}
		var v DeleteClusterDomainResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return core.Decoded[DeleteClusterDomainResponse]{}, types.NewError(types.ProtocolError)
		}
		if v.Cluster != nil {
			v.Cluster.Ref = cloneResourceRef(&cluster)
		}
		return core.Decoded[DeleteClusterDomainResponse]{Data: v, Refs: []types.ResourceRef{ref}}, nil
	})
}

// TransferClusterDomain requires the caller to update DNS to the target first.
// Only a returned domain explicitly naming that target establishes its new ref.
func (c *Client) TransferClusterDomain(ctx context.Context, ref, target types.ResourceRef, spec TransferClusterDomainBody, options types.CallOptions) (types.Result[TransferClusterDomainResponse], error) {
	op := core.Operation{ID: domainTransferOperation, Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	refErr, targetErr := ref.Validate(), target.Validate()
	if refErr == nil {
		ref = *cloneResourceRef(&ref)
	}
	if targetErr == nil {
		target = *cloneResourceRef(&target)
	}
	cluster := types.ResourceRef{}
	if ref.Parent != nil {
		cluster = *ref.Parent
	}
	body, bodyErr := wireBody(spec)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/domains/" + url.PathEscape(ref.ID) + "/transfer", Body: body, Options: options, Refs: []types.ResourceRef{ref, target}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if refErr != nil {
			return refErr
		}
		if targetErr != nil {
			return targetErr
		}
		if e := c.validateClusterChild(ref, "cluster_domain", op); e != nil {
			return e
		}
		if e := c.validateClusterAnchor(target, op); e != nil {
			return e
		}
		if spec.Validate() != nil || bodyErr != nil || target.ID == cluster.ID {
			return types.NewError(types.InvalidArgument)
		}
		if *spec.TargetClusterID != target.ID {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[TransferClusterDomainResponse], error) {
		decoded := core.Decoded[TransferClusterDomainResponse]{}
		var identity struct {
			Domain *struct {
				ID   *string `json:"id"`
				Spec *struct {
					ClusterID *string `json:"clusterId"`
				} `json:"spec"`
			} `json:"domain"`
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Domain == nil || identity.Domain.ID == nil || identity.Domain.Spec == nil || identity.Domain.Spec.ClusterID == nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		owner := cluster
		if *identity.Domain.ID != ref.ID {
			return decoded, types.NewError(types.ScopeMismatch)
		}
		switch *identity.Domain.Spec.ClusterID {
		case cluster.ID:
		case target.ID:
			owner = target
		default:
			return decoded, types.NewError(types.ScopeMismatch)
		}
		child := ref
		child.Parent = cloneResourceRef(&owner)
		decoded.Refs = []types.ResourceRef{child}
		var returnedCluster *types.ResourceRef
		if identity.Cluster != nil {
			if identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
				return decoded, types.NewError(types.ScopeMismatch)
			}
			if identity.Cluster.ID != nil {
				switch *identity.Cluster.ID {
				case cluster.ID:
					returnedCluster = &cluster
				case target.ID:
					returnedCluster = &target
				default:
					return decoded, types.NewError(types.ScopeMismatch)
				}
			}
		}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Domain.Ref = cloneResourceRef(&child)
		if decoded.Data.Cluster != nil {
			decoded.Data.Cluster.Ref = cloneResourceRef(returnedCluster)
		}
		return decoded, nil
	})
	if out.Receipt != nil {
		if _, err := domainTransferTarget(*out.Receipt); err != nil {
			out.Receipt.Recovery = "unavailable"
		}
	}
	return out, e
}
