package selectelsdk_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/ai"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func TestAIRouterErrorBodyCancellationClosesOnceAndRetainsReceipt(t *testing.T) {
	for _, status := range []int{422, 500} {
		for _, stream := range []bool{false, true} {
			for _, deadline := range []bool{false, true} {
				t.Run(fmt.Sprintf("status%d/stream%t/deadline%t", status, stream, deadline), func(t *testing.T) {
					body := &aiBlockingBody{started: make(chan struct{}), closed: make(chan struct{})}
					var lookups, sends atomic.Int32
					budget := 2 * time.Second
					if deadline {
						budget = 100 * time.Millisecond
					}
					c, err := sdk.New(sdk.Config{AccountID: "test-account", CallTimeout: budget, TransferTimeout: budget, ReadMaxAttempts: 5, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
						lookups.Add(1)
						return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
					}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
						sends.Add(1)
						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
					})})
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					done := make(chan struct{})
					var receipt *types.Receipt
					var callErr error
					go func() {
						defer close(done)
						request := ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("synthetic-prompt"))}}}
						if stream {
							result, e := c.Account().AIRouter().StreamChat(ctx, request, ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 4096, MaxBytes: 8192}, func(ai.ChatChunk) error { t.Error("error JSON invoked consumer"); return nil })
							receipt, callErr = result.Receipt, e
						} else {
							result, e := c.Account().AIRouter().Chat(ctx, request)
							receipt, callErr = result.Receipt, e
						}
					}()
					select {
					case <-body.started:
					case <-time.After(time.Second):
						cancel()
						_ = body.Close()
						<-done
						t.Fatal("diagnostic body read did not start")
					}
					if !deadline {
						cancel()
					}
					select {
					case <-done:
					case <-time.After(time.Second):
						_ = body.Close()
						<-done
						t.Error("call stayed blocked after cancellation/deadline")
					}
					cause := types.Canceled
					if deadline {
						cause = types.DeadlineExceeded
					}
					outcome := types.Rejected
					code := cause
					if status == 500 {
						outcome = types.Unknown
						code = types.UnknownOutcome
					}
					var sdkErr *types.SDKError
					if !errors.As(callErr, &sdkErr) || sdkErr.Code != code || (status == 500 && sdkErr.CauseCode != cause) || sdkErr.HTTPStatus != status || receipt == nil || sdkErr.Receipt != receipt || receipt.Outcome != outcome || receipt.Attempts != 1 || receipt.ContractRevision == "" || len(receipt.ResourceRefs) != 0 || sends.Load() != 1 || lookups.Load() != 1 || body.closes.Load() != 1 {
						t.Fatalf("error=%+v receipt=%+v sends=%d lookups=%d closes=%d", callErr, receipt, sends.Load(), lookups.Load(), body.closes.Load())
					}
				})
			}
		}
	}
}
