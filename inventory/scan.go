// Package inventory scans explicitly selected read branches without expanding scope.
package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/festy23/selectel-sdk/compute"
	"github.com/festy23/selectel-sdk/databases"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/mks"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/projects"
	"github.com/festy23/selectel-sdk/registry"
	"github.com/festy23/selectel-sdk/types"
	"time"
)

type Client struct {
	runtime *core.Runtime
	account string
}

func Bind(r *core.Runtime, account string) *Client { return &Client{runtime: r, account: account} }

type Branch struct {
	Scope       types.Scope
	OperationID string
	Projects    *projects.ListOptions
	Servers     *servers.ListOpts
	Clusters    *mks.ListOptions
	Datastores  *databases.ListOptions
	Registries  *registry.ListOptions
	Pagination  pagination.Options
}
type Plan struct{ Branches []Branch }

// Exactly one typed collection is present for the selected branch.
type Items struct {
	Projects   []projects.Project    `json:"projects,omitzero"`
	Servers    []compute.Server      `json:"servers,omitzero"`
	Clusters   []mks.Cluster         `json:"clusters,omitzero"`
	Datastores []databases.Datastore `json:"datastores,omitzero"`
	Registries []registry.Registry   `json:"registries,omitzero"`
}
type BranchResult struct {
	Scope       types.Scope        `json:"scope"`
	OperationID string             `json:"operation_id"`
	Items       Items              `json:"items"`
	Complete    bool               `json:"complete"`
	StopReason  string             `json:"stop_reason"`
	Cursor      *pagination.Cursor `json:"cursor,omitempty"`
	Error       *types.SDKError    `json:"error,omitempty"`
	Metadata    types.Metadata     `json:"metadata"`
}
type Coverage struct {
	Scope              types.Scope `json:"scope"`
	OperationID        string      `json:"operation_id"`
	FiltersFingerprint string      `json:"filters_fingerprint"`
}
type Result struct {
	Branches     []BranchResult `json:"branches"`
	Coverage     []Coverage     `json:"coverage"`
	Complete     bool           `json:"complete"`
	ObservedFrom time.Time      `json:"observed_from"`
	ObservedTo   time.Time      `json:"observed_to"`
}
type collect func(context.Context) (BranchResult, error)

func errorValue(err error) *types.SDKError {
	if err == nil {
		return nil
	}
	var e *types.SDKError
	if errors.As(err, &e) {
		copy := *e
		return &copy
	}
	return types.ContextError(err)
}
func base[T any](b Branch, r pagination.Collection[T]) BranchResult {
	return BranchResult{Scope: b.Scope, OperationID: b.OperationID, Complete: r.Complete, StopReason: r.StopReason, Cursor: r.NextCursor, Metadata: r.Metadata}
}
func filters(b Branch) (any, int) {
	var value any
	count := 0
	if b.Projects != nil {
		value = *b.Projects
		count++
	}
	if b.Servers != nil {
		value = *b.Servers
		count++
	}
	if b.Clusters != nil {
		value = *b.Clusters
		count++
	}
	if b.Datastores != nil {
		value = *b.Datastores
		count++
	}
	if b.Registries != nil {
		value = *b.Registries
		count++
	}
	return value, count
}
func (c *Client) branch(b Branch) (collect, error) {
	if b.Scope.AccountID != c.account {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if b.Scope.Validate() != nil || b.OperationID == "" {
		return nil, types.NewError(types.InvalidArgument)
	}
	_, count := filters(b)
	if count > 1 {
		return nil, types.NewError(types.InvalidArgument)
	}
	capability, e := c.runtime.Catalog().Describe(b.OperationID)
	if e != nil {
		return nil, e
	}
	if capability.Effect != "read" {
		return nil, types.NewError(types.InvalidArgument)
	}
	switch b.OperationID {
	case "projects.v2.projects.list":
		if count == 1 && b.Projects == nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		f := projects.ListOptions{}
		if b.Projects != nil {
			f = *b.Projects
		}
		it, e := projects.Bind(c.runtime, b.Scope).Iterate(f, b.Pagination)
		if e != nil {
			return nil, e
		}
		return func(ctx context.Context) (BranchResult, error) {
			r, e := it.Collect(ctx)
			out := base(b, r)
			out.Items.Projects = r.Items
			return out, e
		}, nil
	case "compute.v2_1.servers.list":
		if count == 1 && b.Servers == nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		f := servers.ListOpts{}
		if b.Servers != nil {
			f = *b.Servers
		}
		it, e := compute.Bind(c.runtime, b.Scope).Servers.Iterate(f, b.Pagination)
		if e != nil {
			return nil, e
		}
		return func(ctx context.Context) (BranchResult, error) {
			r, e := it.Collect(ctx)
			for i := range r.Items {
				r.Items[i].AdminPassword = nil
				r.Items[i].UserData = nil
				r.Items[i].Server.AdminPass = ""
				r.Items[i].Server.Userdata = nil
			}
			out := base(b, r)
			out.Items.Servers = r.Items
			return out, e
		}, nil
	case "mks.v2.clusters.list":
		if count == 1 && b.Clusters == nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		f := mks.ListOptions{}
		it, e := mks.Bind(c.runtime, b.Scope).V2.Clusters.Iterate(f, b.Pagination)
		if e != nil {
			return nil, e
		}
		return func(ctx context.Context) (BranchResult, error) {
			r, e := it.Collect(ctx)
			out := base(b, r)
			out.Items.Clusters = r.Items
			return out, e
		}, nil
	case "databases.v1.datastores.list":
		if count == 1 && b.Datastores == nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		f := databases.ListOptions{}
		if b.Datastores != nil {
			f = *b.Datastores
		}
		it, e := databases.Bind(c.runtime, b.Scope).V1.Datastores.Iterate(f, b.Pagination)
		if e != nil {
			return nil, e
		}
		return func(ctx context.Context) (BranchResult, error) {
			r, e := it.Collect(ctx)
			for i := range r.Items {
				r.Items[i].Config = nil
				r.Items[i].Connection = nil
			}
			out := base(b, r)
			out.Items.Datastores = r.Items
			return out, e
		}, nil
	case "registry.v2.registries.list":
		if count == 1 && b.Registries == nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		f := registry.ListOptions{}
		if b.Registries != nil {
			f = *b.Registries
		}
		it, e := registry.Bind(c.runtime, b.Scope).V2.Registries.Iterate(f, b.Pagination)
		if e != nil {
			return nil, e
		}
		return func(ctx context.Context) (BranchResult, error) {
			r, e := it.Collect(ctx)
			out := base(b, r)
			out.Items.Registries = r.Items
			return out, e
		}, nil
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
}
func (c *Client) Scan(ctx context.Context, plan Plan) (out Result, err error) {
	out.ObservedFrom = time.Now().UTC()
	out.Branches = []BranchResult{}
	out.Coverage = []Coverage{}
	defer func() { out.ObservedTo = time.Now().UTC() }()
	if ctx == nil || len(plan.Branches) == 0 {
		return out, types.NewError(types.InvalidArgument)
	}
	collectors := make([]collect, len(plan.Branches))
	seen := map[string]bool{}
	for i, b := range plan.Branches {
		fn, e := c.branch(b)
		if e != nil {
			return out, e
		}
		collectors[i] = fn
		value, _ := filters(b)
		if value == nil {
			switch b.OperationID {
			case "projects.v2.projects.list":
				value = projects.ListOptions{}
			case "compute.v2_1.servers.list":
				value = servers.ListOpts{}
			case "mks.v2.clusters.list":
				value = mks.ListOptions{}
			case "databases.v1.datastores.list":
				value = databases.ListOptions{}
			case "registry.v2.registries.list":
				value = registry.ListOptions{}
			}
		}
		raw, e := json.Marshal(value)
		if e != nil {
			return out, types.NewError(types.InvalidArgument)
		}
		digest := sha256.Sum256(raw)
		fp := hex.EncodeToString(digest[:])
		keyBytes, _ := json.Marshal(Coverage{Scope: b.Scope, OperationID: b.OperationID, FiltersFingerprint: fp})
		key := string(keyBytes)
		if seen[key] {
			return out, types.NewError(types.InvalidArgument)
		}
		seen[key] = true
		out.Coverage = append(out.Coverage, Coverage{Scope: b.Scope, OperationID: b.OperationID, FiltersFingerprint: fp})
	}
	call, cancel := context.WithTimeout(ctx, c.runtime.CollectionTimeout())
	defer cancel()
	out.Complete = true
	for i, fn := range collectors {
		var branch BranchResult
		var e error
		if call.Err() != nil {
			b := plan.Branches[i]
			branch = BranchResult{Scope: b.Scope, OperationID: b.OperationID, StopReason: "not_started"}
			e = types.ContextError(call.Err())
		} else {
			branch, e = fn(call)
		}
		branch.Error = errorValue(e)
		if e != nil || !branch.Complete {
			out.Complete = false
		}
		out.Branches = append(out.Branches, branch)
	}
	if !out.Complete {
		return out, types.NewError(types.Incomplete)
	}
	return out, nil
}
