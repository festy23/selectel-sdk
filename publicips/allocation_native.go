package publicips

import (
	"context"
	"github.com/festy23/selectel-sdk/types"
)

// AllocationNativeCreateRequest selects the pinned SDK region-only body.
// Provider-default quantity is not inferred and no fallback is attempted.
type AllocationNativeCreateRequest struct {
	ProjectID string
	Spec      AllocationNativeSpec
}

func (a *Allocations) CreateNative(ctx context.Context, req AllocationNativeCreateRequest, o types.CallOptions) (types.Result[[]Allocation], error) {
	return a.create(ctx, AllocationCreateRequest{ProjectID: req.ProjectID, Specs: []AllocationSpec{{Region: req.Spec.Region}}}, o, true)
}
