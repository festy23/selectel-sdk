package operations_test

import (
	"context"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"testing"
	"time"
)

func fixture(t *testing.T, receipt *types.Receipt, read operations.Read) *operations.Handle {
	t.Helper()
	r, e := core.New(core.Config{AccountID: "account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		t.Fatal("unexpected credentials IO")
		return credentials.Material{}, nil
	}), WaitTimeout: time.Second, PollInterval: time.Millisecond})
	if e != nil {
		t.Fatal(e)
	}
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "account", ProjectID: "project"}, Service: "compute", APIGeneration: "v2_1", Kind: "server", ID: "id", EndpointID: "ep", EndpointURL: "https://nova.test"}
	h, e := operations.Bind(r, ref, receipt, read, map[string]operations.Goal{"active": {Success: map[string]bool{"ACTIVE": true}, Failure: map[string]bool{"ERROR": true}, Pending: map[string]bool{"BUILD": true}}, "deleted": {AbsentSuccess: true}})
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func TestWaitUnknownStateAndFailureKeepAcceptedReceipt(t *testing.T) {
	for _, tc := range []struct {
		state string
		code  types.Code
	}{{"NEW_STATE", types.StateUnknown}, {"ERROR", types.OperationFailed}} {
		t.Run(tc.state, func(t *testing.T) {
			h := fixture(t, &types.Receipt{Outcome: types.Accepted}, func(context.Context) (operations.Observation, error) {
				return operations.Observation{ProviderState: tc.state}, nil
			})
			r, e := h.Wait(context.Background(), "active")
			if types.ErrorCode(e) != tc.code || r.Complete || r.Receipt.Outcome != types.Accepted || r.Last.ProviderState != tc.state {
				t.Fatal("state classification or receipt lost")
			}
		})
	}
}
func TestCancellationStopsWaitAndDoesNotUndoAcceptance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	h := fixture(t, &types.Receipt{Outcome: types.Accepted}, func(context.Context) (operations.Observation, error) {
		n++
		cancel()
		return operations.Observation{ProviderState: "BUILD"}, nil
	})
	r, e := h.Wait(ctx, "active")
	if types.ErrorCode(e) != types.Canceled || n != 1 || r.Last == nil || r.Receipt.Outcome != types.Accepted {
		t.Fatal("cancel changed mutation outcome")
	}
}
func TestForbiddenReadDoesNotProveDeletion(t *testing.T) {
	h := fixture(t, &types.Receipt{Outcome: types.Accepted}, func(context.Context) (operations.Observation, error) {
		return operations.Observation{}, types.NewError(types.PermissionDenied)
	})
	r, e := h.Wait(context.Background(), "deleted")
	if types.ErrorCode(e) != types.PermissionDenied || r.Complete {
		t.Fatal("403 treated as deleted")
	}
}
func TestKnownOutcomeRecoveryHasNoIO(t *testing.T) {
	n := 0
	h := fixture(t, &types.Receipt{Outcome: types.Accepted}, func(context.Context) (operations.Observation, error) { n++; return operations.Observation{}, nil })
	r, e := h.Recover(context.Background())
	if e != nil || n != 0 || r.ProofStatus != "conclusive" || r.Receipt.Outcome != types.Accepted {
		t.Fatal("accepted outcome unnecessarily looked up")
	}
	r.Receipt.Outcome = types.Unknown
	if h.Receipt().Outcome != types.Accepted {
		t.Fatal("receipt mutable")
	}
}
