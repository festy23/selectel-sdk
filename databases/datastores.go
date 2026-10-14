package databases

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Datastores) operation(action, method string) (core.Operation, error) {
	op := core.Operation{ID: "databases.v1.datastores." + action, Service: "databases", APIGeneration: "v1", Method: method, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	if method == "POST" {
		op.Effect = "mutation"
		op.Success = []int{201}
		op.Rejected = []int{400, 401, 403}
	}
	return c.runtime.BindDocumented(op, c.scope)
}

func decodeDatastore(expectedID string) core.Decoder[Datastore] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Datastore], error) {
		var wrapper struct {
			Datastore json.RawMessage `json:"datastore"`
		}
		if json.Unmarshal(raw, &wrapper) != nil {
			return core.Decoded[Datastore]{}, types.NewError(types.ProtocolError)
		}
		var identity struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"project_id"`
		}
		if json.Unmarshal(wrapper.Datastore, &identity) != nil || string(wrapper.Datastore) == "null" {
			return core.Decoded[Datastore]{}, types.NewError(types.ProtocolError)
		}
		if identity.ProjectID != nil && *identity.ProjectID != endpoint.Scope.ProjectID {
			return core.Decoded[Datastore]{}, types.NewError(types.ScopeMismatch)
		}
		id := expectedID
		if identity.ID != nil {
			if *identity.ID == "" {
				return core.Decoded[Datastore]{}, types.NewError(types.ProtocolError)
			}
			if expectedID != "" && *identity.ID != expectedID {
				return core.Decoded[Datastore]{}, types.NewError(types.ScopeMismatch)
			}
			id = *identity.ID
		}
		ref := core.Ref(endpoint, "databases", "datastore", id)
		if ref == nil {
			return core.Decoded[Datastore]{}, types.NewError(types.ProtocolError)
		}
		out := core.Decoded[Datastore]{Refs: []types.ResourceRef{*ref}}
		var data Datastore
		if json.Unmarshal(wrapper.Datastore, &data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if err := data.Validate(); err != nil {
			return out, err
		}
		data.Ref = ref
		out.Data = data
		return out, nil
	}
}

// Create starts provisioning; acceptance does not mean the datastore is ready.
func (c *Datastores) Create(ctx context.Context, request CreateRequest, options types.CallOptions) (types.Result[Datastore], error) {
	op, bindErr := c.operation("create", "POST")
	project := c.scope.ProjectID
	originalProject := request.ProjectID
	request.ProjectID = &project
	body, bodyErr := createBody(request)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if originalProject != nil && *originalProject != c.scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores", Body: body, Options: options, Validate: validate}, decodeDatastore(""))
}

func (c *Datastores) validateRef(ref types.ResourceRef) error {
	if ref.Validate() != nil || ref.Kind != "datastore" || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Service != "databases" || ref.APIGeneration != "v1" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *Datastores) Get(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[Datastore], error) {
	op, bindErr := c.operation("get", "GET")
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref)
	}
	segment := url.PathEscape(ref.ID)
	if ref.ID == "." || ref.ID == ".." {
		segment = strings.ReplaceAll(ref.ID, ".", "%2E")
	}
	pin := &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/" + segment, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Validate: validate}, decodeDatastore(ref.ID))
}

// Handle observes an exact datastore. Readiness is the provider's ACTIVE state,
// not proof of SQL connectivity or application of an individual setting.
func (c *Datastores) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if err := c.validateRef(ref); err != nil {
		return nil, err
	}
	op, err := c.operation("get", "GET")
	if err != nil {
		return nil, err
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return nil, types.NewError(types.EndpointUnavailable)
	}
	if receipt != nil {
		capability, e := c.runtime.Catalog().Describe("databases.v1.datastores.create")
		if e != nil {
			return nil, e
		}
		if receipt.OperationID != capability.OperationID || receipt.ContractRevision != capability.ContractRevision {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || (receipt.Outcome != types.Accepted && receipt.Outcome != types.Unknown && receipt.Outcome != types.NotSent && receipt.Outcome != types.Rejected) {
			return nil, types.NewError(types.InvalidArgument)
		}
		if receipt.Scope != ref.Scope || receipt.Endpoint == nil || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.Scope != ref.Scope || receipt.Endpoint.APIGeneration != ref.APIGeneration || receipt.Endpoint.Audience != "selectel" || len(receipt.ResourceRefs) != 1 || receipt.ResourceRefs[0] != ref {
			return nil, types.NewError(types.ScopeMismatch)
		}
	}
	pending := map[string]bool{"PENDING_CREATE": true, "PENDING_UPDATE": true, "PENDING_DELETE": true, "RESIZING": true, "REBUILDING": true}
	failure := map[string]bool{"ERROR": true, "DEGRADED": true, "DISK_FULL": true, "DELETED": true}
	read := func(ctx context.Context) (operations.Observation, error) {
		result, e := c.Get(ctx, ref, types.CallOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		state := ""
		if result.Data.Status != nil {
			state = *result.Data.Status
		}
		classification := "unknown"
		if state == "ACTIVE" {
			classification = "succeeded"
		} else if failure[state] {
			classification = "failed"
		} else if pending[state] {
			classification = "pending"
		}
		return operations.Observation{Ref: ref, ProviderState: state, Classification: classification, ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, nil
	}
	return operations.Bind(c.runtime, ref, receipt, read, map[string]operations.Goal{"datastore.active": {Success: map[string]bool{"ACTIVE": true}, Failure: failure, Pending: pending}})
}
