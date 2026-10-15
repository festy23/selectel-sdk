package selectelsdk_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

// Authentication only asserts the caller's binding; it never probes an AI
// endpoint, grants preview access, or substitutes an IAM/static token.
func TestAIBearerAuthentication(t *testing.T) {
	for _, service := range []string{"ai_router", "dedicated_inference", "token_factory"} {
		t.Run(service, func(t *testing.T) {
			var lookups, calls int
			scope := types.Scope{AccountID: "test-account"}
			client, err := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: credentials.SourceFunc(func(_ context.Context, req types.AuthRequest) (credentials.Material, error) {
				lookups++
				if req.Service != service || req.Mode != types.ProtocolSpecific || req.Scope != scope || req.Audience != credentials.SelectelAudience {
					t.Fatalf("unexpected auth request: %+v", req)
				}
				return credentials.Material{Identity: "ai-key", Revision: "one", Binding: scope, Audience: credentials.SelectelAudience, Scheme: credentials.Scheme("bearer_token"), Token: types.Protect([]byte("synthetic-ai-key"))}, nil
			}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.Authenticate(context.Background(), types.AuthRequest{Scope: scope, Mode: types.ProtocolSpecific, Service: service, Audience: credentials.SelectelAudience})
			if err != nil {
				t.Fatal(err)
			}
			if lookups != 1 || calls != 0 || result.Verification != "caller_asserted" || result.Scheme != "bearer_token" || result.Scope != scope {
				t.Fatalf("auth %+v lookups=%d calls=%d", result, lookups, calls)
			}
		})
	}
}

func TestAIBearerBindingAndExpiry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scheme  credentials.Scheme
		binding types.Scope
		expiry  time.Time
		code    types.Code
	}{
		{"static token is not bearer", credentials.StaticToken, types.Scope{AccountID: "test-account"}, time.Time{}, types.CredentialsUnavailable},
		{"foreign account", credentials.Scheme("bearer_token"), types.Scope{AccountID: "foreign"}, time.Time{}, types.ScopeMismatch},
		{"foreign project", credentials.Scheme("bearer_token"), types.Scope{AccountID: "test-account", ProjectID: "foreign"}, time.Time{}, types.ScopeMismatch},
		{"expired", credentials.Scheme("bearer_token"), types.Scope{AccountID: "test-account"}, time.Now().Add(-time.Hour), types.Unauthenticated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "key", Revision: "one", Binding: tc.binding, Audience: credentials.SelectelAudience, Scheme: tc.scheme, Token: types.Protect([]byte("synthetic-ai-key")), ExpiresAt: tc.expiry}, nil
			}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Authenticate(context.Background(), types.AuthRequest{Scope: types.Scope{AccountID: "test-account"}, Mode: types.ProtocolSpecific, Service: "ai_router", Audience: credentials.SelectelAudience})
			if types.ErrorCode(err) != tc.code || calls != 0 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
		})
	}
}

func TestAIBearerExplicitEnvironment(t *testing.T) {
	t.Setenv("SDK_AI_ACCOUNT_ID", "test-account")
	t.Setenv("SDK_AI_BEARER_TOKEN", "synthetic-ai-key")
	t.Setenv("SDK_AI_STATIC_TOKEN", "different-static-token")
	src, err := credentials.FromEnvironment("SDK_AI_")
	if err != nil {
		t.Fatal(err)
	}
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: src, Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected HTTP"); return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Authenticate(context.Background(), types.AuthRequest{Scope: types.Scope{AccountID: "test-account"}, Mode: types.ProtocolSpecific, Service: "token_factory", Audience: credentials.SelectelAudience})
	if err != nil || result.Scheme != "bearer_token" {
		t.Fatalf("metadata=%+v error=%v", result, err)
	}
}

func TestAIBearerExpiryIsCallerAssertedMetadata(t *testing.T) {
	expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key")), ExpiresAt: expiry}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected auth HTTP")
		return nil, context.Canceled
	})})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := c.Authenticate(context.Background(), types.AuthRequest{Scope: types.Scope{AccountID: "test-account"}, Mode: types.ProtocolSpecific, Service: "ai_router", Audience: credentials.SelectelAudience})
	if err != nil || metadata.ExpiresAt == nil || !metadata.ExpiresAt.Equal(expiry) || metadata.Verification != "caller_asserted" {
		t.Fatalf("metadata=%+v error=%v", metadata, err)
	}
}
