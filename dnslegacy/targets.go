package dnslegacy

import (
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type domainTarget struct {
	ref  *types.ResourceRef
	name *types.ProtectedValue
}

func (t domainTarget) request(scope types.Scope, op core.Operation) (string, []types.ResourceRef, error) {
	if t.ref == nil {
		path, err := (DomainSelector{Name: t.name}).path()
		return path, nil, err
	}
	ref := *t.ref
	if ref.Validate() != nil || !decimalID.MatchString(ref.ID) {
		return "", nil, types.NewError(types.InvalidArgument)
	}
	if t.name != nil || ref.Scope != scope || ref.Service != "dns" || ref.APIGeneration != "v1" || ref.Kind != "domain" || ref.Parent != nil || ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return "", nil, types.NewError(types.ScopeMismatch)
	}
	return "/" + ref.ID, []types.ResourceRef{ref}, nil
}

type recordTarget struct {
	ref        *types.ResourceRef
	parentName *types.ProtectedValue
	id         string
}

func (t recordTarget) request(scope types.Scope, op core.Operation) (string, domainTarget, []types.ResourceRef, error) {
	if t.ref == nil {
		target := domainTarget{name: t.parentName}
		path, _, err := target.request(scope, op)
		if err == nil && !decimalID.MatchString(t.id) {
			err = types.NewError(types.InvalidArgument)
		}
		return path + "/records/" + t.id, target, nil, err
	}
	ref := *t.ref
	if ref.Validate() != nil || ref.Parent == nil || !decimalID.MatchString(ref.ID) {
		return "", domainTarget{}, nil, types.NewError(types.InvalidArgument)
	}
	target := domainTarget{ref: ref.Parent}
	path, _, err := target.request(scope, op)
	if err != nil {
		return "", target, nil, err
	}
	if ref.Scope != scope || ref.Service != "dns" || ref.APIGeneration != "v1" || ref.Kind != "record" || ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return "", target, nil, types.NewError(types.ScopeMismatch)
	}
	return path + "/records/" + ref.ID, target, []types.ResourceRef{ref}, nil
}
func (t recordTarget) recordID() string {
	if t.ref != nil {
		return t.ref.ID
	}
	return t.id
}

// DNS domains are root resources. Snapshot the one supported record parent;
// deeper/cyclic hierarchies remain intact for validation to reject before I/O.
func snapshotRecordRef(ref types.ResourceRef) types.ResourceRef {
	if ref.Parent != nil {
		parent := *ref.Parent
		ref.Parent = &parent
	}
	return ref
}

func snapshotProtected(value *types.ProtectedValue) *types.ProtectedValue {
	if value == nil {
		return nil
	}
	copy := types.Protect(value.Reveal())
	return &copy
}
