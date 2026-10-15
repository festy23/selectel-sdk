package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

func (c *Client) ListPredefinedJournalConfigs(ctx context.Context, options pagination.Options) (pagination.Collection[TechJournalLogsConfig], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_logs_tech_journal_predefined_configs.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[TechJournalLogsConfig]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/clusters/logs/tech-journal/predefined-configs"}, "", options, "predefinedLogConfigs", func(v *TechJournalLogsConfig, _ types.EndpointBinding) error { return v.Validate() })
	if e != nil {
		return pagination.Collection[TechJournalLogsConfig]{}, e
	}
	return iterator.Collect(ctx)
}

func (c *Client) GetJournalConfig(ctx context.Context, cluster types.ResourceRef, options types.CallOptions) (types.Result[GetTechJournalLogsConfigResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_logs_tech_journal_config.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/logs/tech-journal/config", Options: options, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateClusterAnchor(cluster, op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetTechJournalLogsConfigResponse], error) {
		var v GetTechJournalLogsConfigResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.LogConfig == nil {
			return core.Decoded[GetTechJournalLogsConfigResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GetTechJournalLogsConfigResponse]{Data: v}, nil
	})
}

func (c *Client) ListJournalPlans(ctx context.Context, cluster types.ResourceRef, options pagination.Options) (pagination.Collection[TechJournalLogPlan], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_logs_tech_journal_plans.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[TechJournalLogPlan]{}, e
	}
	if e := c.validateClusterAnchor(cluster, op); e != nil {
		return pagination.Collection[TechJournalLogPlan]{}, e
	}
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/logs/tech-journal/plans", Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint}, cluster, options, "logPlans", func(v *TechJournalLogPlan, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.ID == nil || *v.ID == "" {
			return types.NewError(types.ProtocolError)
		}
		if v.ClusterID != nil && *v.ClusterID != cluster.ID {
			return types.NewError(types.ScopeMismatch)
		}
		v.Ref = core.Ref(ep, "cloud_1c", "journal_plan", *v.ID)
		v.Ref.Parent = cloneResourceRef(&cluster)
		return nil
	})
	if e != nil {
		return pagination.Collection[TechJournalLogPlan]{}, e
	}
	return iterator.Collect(ctx)
}

func (c *Client) CreateJournalPlan(ctx context.Context, cluster types.ResourceRef, spec CreateTechJournalLogPlanBody, options types.CallOptions) (types.Result[CreateTechJournalLogPlanResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_logs_tech_journal_plans.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := wireBody(spec)
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/logs/tech-journal/plans", Body: body, Options: options, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateClusterAnchor(cluster, op); e != nil {
			return e
		}
		if bodyErr != nil || spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if spec.StartsAt != nil && spec.EndsAt != nil && !spec.StartsAt.Before(*spec.EndsAt) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[CreateTechJournalLogPlanResponse], error) {
		var identity struct {
			LogPlan *struct {
				ID        *string `json:"id"`
				ClusterID *string `json:"clusterId"`
			} `json:"logPlan"`
		}
		decoded := core.Decoded[CreateTechJournalLogPlanResponse]{}
		if json.Unmarshal(raw, &identity) != nil || identity.LogPlan == nil || identity.LogPlan.ID == nil || *identity.LogPlan.ID == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		if identity.LogPlan.ClusterID != nil && *identity.LogPlan.ClusterID != cluster.ID {
			return decoded, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "cloud_1c", "journal_plan", *identity.LogPlan.ID)
		ref.Parent = cloneResourceRef(&cluster)
		decoded.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.LogPlan.Ref = cloneResourceRef(ref)
		return decoded, nil
	})
	if out.Receipt != nil {
		known := false
		for _, ref := range out.Receipt.ResourceRefs {
			if ref.Kind == "journal_plan" {
				known = true
			}
		}
		if !known {
			out.Receipt.Recovery = "unavailable"
		}
	}
	return out, e
}

func (c *Client) JournalPlanHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	op := core.Operation{Service: "cloud_1c", APIGeneration: "v2", RequireProject: true, RequireLocation: true}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return nil, e
	}
	if e := c.validateClusterChild(ref, "journal_plan", op); e != nil {
		return nil, e
	}
	ref = *cloneResourceRef(&ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		call, cancel := context.WithTimeout(ctx, c.runtime.CallTimeout())
		defer cancel()
		listing, e := c.ListJournalPlans(call, *ref.Parent, pagination.Options{})
		if e != nil && types.ErrorCode(e) != types.Incomplete {
			return operations.Observation{}, e
		}
		for _, row := range listing.Items {
			if *row.ID == ref.ID {
				state, class := "", "unknown"
				if row.Status != nil {
					state = *row.Status
				}
				switch state {
				case "TECH_JOURNAL_LOG_PLAN_STATUS_RUNNING", "TECH_JOURNAL_LOG_PLAN_STATUS_FINISHED", "TECH_JOURNAL_LOG_PLAN_STATUS_CANCELLED", "TECH_JOURNAL_LOG_PLAN_STATUS_STOPPED":
					class = "succeeded"
				case "TECH_JOURNAL_LOG_PLAN_STATUS_ERRORED":
					class = "failed"
				case "TECH_JOURNAL_LOG_PLAN_STATUS_SCHEDULED":
					class = "pending"
				}
				return operations.Observation{Ref: ref, ProviderState: state, Classification: class, ObservedAt: listing.Metadata.ObservedAt, Metadata: listing.Metadata}, nil
			}
		}
		return operations.Observation{}, types.NewError(types.Incomplete)
	}
	return c.bindChildHandle(ref, receipt, read, map[string]operations.Goal{
		"journal_plan.running": {Success: map[string]bool{"TECH_JOURNAL_LOG_PLAN_STATUS_RUNNING": true}, Failure: map[string]bool{"TECH_JOURNAL_LOG_PLAN_STATUS_ERRORED": true, "TECH_JOURNAL_LOG_PLAN_STATUS_CANCELLED": true, "TECH_JOURNAL_LOG_PLAN_STATUS_STOPPED": true, "TECH_JOURNAL_LOG_PLAN_STATUS_FINISHED": true}, Pending: map[string]bool{"TECH_JOURNAL_LOG_PLAN_STATUS_SCHEDULED": true}},
		"journal_plan.stopped": {Success: map[string]bool{"TECH_JOURNAL_LOG_PLAN_STATUS_CANCELLED": true, "TECH_JOURNAL_LOG_PLAN_STATUS_STOPPED": true, "TECH_JOURNAL_LOG_PLAN_STATUS_FINISHED": true}, Failure: map[string]bool{"TECH_JOURNAL_LOG_PLAN_STATUS_ERRORED": true}, Pending: map[string]bool{"TECH_JOURNAL_LOG_PLAN_STATUS_SCHEDULED": true, "TECH_JOURNAL_LOG_PLAN_STATUS_RUNNING": true}},
	})
}

func (c *Client) DisableJournalPlan(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DisableTechJournalLogPlanResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_logs_tech_journal_plans_disable.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
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
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/logs/tech-journal/plans/" + url.PathEscape(ref.ID) + "/disable", Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if refErr != nil {
			return refErr
		}
		if bindErr != nil {
			return bindErr
		}
		return c.validateClusterChild(ref, "journal_plan", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[DisableTechJournalLogPlanResponse], error) {
		var v DisableTechJournalLogPlanResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return core.Decoded[DisableTechJournalLogPlanResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[DisableTechJournalLogPlanResponse]{Data: v, Refs: []types.ResourceRef{ref}}, nil
	})
}
