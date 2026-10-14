// Package selectelsdk provides scoped, typed access to documented Selectel APIs.
package selectelsdk

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"strings"

	"github.com/festy23/selectel-sdk/audit"
	"github.com/festy23/selectel-sdk/billing"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/compute"
	"github.com/festy23/selectel-sdk/databases"
	"github.com/festy23/selectel-sdk/iamcredentials"
	"github.com/festy23/selectel-sdk/internal/endpoints"
	"github.com/festy23/selectel-sdk/inventory"
	"github.com/festy23/selectel-sdk/logs"
	"github.com/festy23/selectel-sdk/metrics"
	"github.com/festy23/selectel-sdk/mks"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/projects"
	"github.com/festy23/selectel-sdk/registry"
	"github.com/festy23/selectel-sdk/secrets"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}
type AccountClient struct{ client *Client }
type ProjectClient struct{ client *Client }
type RegionalClient struct{ client *Client }

func (c *Client) Scope() types.Scope           { return c.scope }
func (c *Client) Catalog() catalog.Catalog     { return c.runtime.Catalog() }
func (c *Client) Inventory() *inventory.Client { return inventory.Bind(c.runtime, c.scope.AccountID) }
func (c *Client) Account() AccountClient {
	return AccountClient{client: &Client{runtime: c.runtime, scope: types.Scope{AccountID: c.scope.AccountID}}}
}
func (c *Client) Project(id string) (ProjectClient, error) {
	if id == "" {
		return ProjectClient{}, types.NewError(types.InvalidArgument)
	}
	return ProjectClient{client: &Client{runtime: c.runtime, scope: types.Scope{AccountID: c.scope.AccountID, ProjectID: id}}}, nil
}
func (p ProjectClient) Scope() types.Scope { return p.client.scope }
func (p ProjectClient) Secrets() *secrets.Client {
	return secrets.Bind(p.client.runtime, p.client.scope)
}
func (a AccountClient) Scope() types.Scope { return a.client.scope }
func (a AccountClient) Billing() *billing.Client {
	return billing.Bind(a.client.runtime, a.client.scope)
}
func (a AccountClient) Projects() *projects.Client {
	return projects.Bind(a.client.runtime, a.client.scope)
}
func (a AccountClient) Audit() *audit.Client { return audit.Bind(a.client.runtime, a.client.scope) }
func (p ProjectClient) Registry() *registry.Client {
	return registry.Bind(p.client.runtime, p.client.scope)
}
func (r RegionalClient) Registry() *registry.Client {
	return registry.Bind(r.client.runtime, r.client.scope)
}
func (r RegionalClient) MKS() *mks.Client { return mks.Bind(r.client.runtime, r.client.scope) }
func (r RegionalClient) Databases() *databases.Client {
	return databases.Bind(r.client.runtime, r.client.scope)
}
func (r RegionalClient) Scope() types.Scope { return r.client.scope }
func (r RegionalClient) Logs() *logs.Client { return logs.Bind(r.client.runtime, r.client.scope) }
func (r RegionalClient) Metrics() *metrics.Client {
	return metrics.Bind(r.client.runtime, r.client.scope)
}
func (r RegionalClient) Compute() *compute.Client {
	return compute.Bind(r.client.runtime, r.client.scope)
}

func (c *Client) Resource(ref types.ResourceRef) (*operations.Handle, error) {
	if ref.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if ref.Scope.AccountID != c.scope.AccountID {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if ref.Service == "compute" && ref.APIGeneration == "v2_1" && ref.Kind == "server" {
		return compute.Bind(c.runtime, ref.Scope).Servers.Handle(ref, nil)
	}
	return c.mksHandle(ref, nil)
}
func (c *Client) Resume(receipt types.Receipt) (*operations.Handle, error) {
	if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.OperationID == "" || receipt.ContractRevision == "" || receipt.Scope.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if receipt.Scope.AccountID != c.scope.AccountID {
		return nil, types.NewError(types.ScopeMismatch)
	}
	capability, err := c.Catalog().Describe(receipt.OperationID)
	if err != nil {
		return nil, err
	}
	if capability.ContractRevision != receipt.ContractRevision {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if receipt.Outcome != types.NotSent && receipt.Outcome != types.Rejected && receipt.Outcome != types.Accepted && receipt.Outcome != types.Unknown {
		return nil, types.NewError(types.InvalidArgument)
	}
	if len(receipt.ResourceRefs) != 1 || receipt.Endpoint == nil {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	ref := receipt.ResourceRefs[0]
	if ref.Scope != receipt.Scope || ref.EndpointID != receipt.Endpoint.ID || ref.EndpointURL != receipt.Endpoint.URL || ref.APIGeneration != receipt.Endpoint.APIGeneration || receipt.Endpoint.Scope != receipt.Scope || receipt.Endpoint.Audience != "selectel" {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if capability.Service != ref.Service {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if capability.Effect != "mutation" || capability.APIGeneration != ref.APIGeneration {
		return nil, types.NewError(types.InvalidArgument)
	}
	if ref.Service == "compute" {
		return compute.Bind(c.runtime, ref.Scope).Servers.Handle(ref, &receipt)
	}
	return c.mksHandle(ref, &receipt)
}

func (c *Client) mksHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if ref.Service != "mks" || ref.APIGeneration != "v2" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if receipt != nil {
		expected := "cluster"
		if strings.HasPrefix(receipt.OperationID, "mks.v2.clusters_nodegroups_nodes") {
			expected = "node"
		} else if strings.HasPrefix(receipt.OperationID, "mks.v2.clusters_nodegroups") && receipt.OperationID != "mks.v2.clusters_nodegroups.create" {
			expected = "nodegroup"
		}
		if ref.Kind != expected {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	m := mks.Bind(c.runtime, ref.Scope).V2
	switch ref.Kind {
	case "cluster":
		return m.Clusters.Handle(ref, receipt)
	case "nodegroup":
		return m.Nodegroups.Handle(ref, receipt)
	case "task":
		return m.Tasks.Handle(ref, receipt)
	case "node":
		return m.Nodes.Handle(ref, receipt)
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
}

func (p ProjectClient) Location(region, pool string) (RegionalClient, error) {
	if region == "" && pool == "" {
		return RegionalClient{}, types.NewError(types.InvalidArgument)
	}
	resolved, err := endpoints.Location(region, pool)
	if err != nil {
		return RegionalClient{}, err
	}
	region = resolved
	s := p.client.scope
	s.Region, s.Pool = region, pool
	return RegionalClient{client: &Client{runtime: p.client.runtime, scope: s}}, nil
}

func New(config Config) (*Client, error) {
	r, err := core.New(config)
	if err != nil {
		return nil, err
	}
	return &Client{runtime: r, scope: types.Scope{AccountID: config.AccountID}}, nil
}
func (c *Client) Authenticate(ctx context.Context, request types.AuthRequest) (types.AuthMetadata, error) {
	return c.runtime.Authenticate(ctx, request)
}

func (a AccountClient) S3Credentials() *iamcredentials.Client {
	return iamcredentials.Bind(a.client.runtime, a.client.scope)
}
