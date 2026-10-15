package selectelsdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/ai"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func TestAIRouterChatProtectedDataAndReceipt(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Service != "ai_router" || r.Mode != types.ProtocolSpecific || r.Scope.ProjectID != "" {
			t.Fatalf("auth %+v", r)
		}
		return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-ai-key"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.selectel.ru/aig/v1/chat/completions" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-ai-key" || r.Header.Get("X-Token") != "" {
			t.Fatalf("wire URL=%s headers=%v", r.URL, r.Header)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"model":"synthetic-model","messages":[{"role":"user","content":"synthetic-prompt"}],"stream":false}` {
			t.Fatalf("body %s", raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"synthetic-completion","object":"chat.completion","created":1,"model":"synthetic-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"synthetic-output"}}],"usage":{"prompt_tokens":9007199254740993,"completion_tokens":2,"total_tokens":9007199254740995}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	req := ai.ChatRequest{Model: "synthetic-model", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("synthetic-prompt"))}}}
	result, err := c.Account().AIRouter().Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || result.Receipt.Attempts != 1 || len(result.Receipt.ResourceRefs) != 0 {
		t.Fatalf("result %+v", result)
	}
	if result.Data.Usage == nil || result.Data.Usage.PromptTokens.String() != "9007199254740993" || len(result.Data.Choices) != 1 || string(result.Data.Choices[0].Message.Content.Reveal()) != "synthetic-output" {
		t.Fatalf("data %+v", result.Data)
	}
	for _, v := range []any{req, result, result.Data} {
		raw, _ := json.Marshal(v)
		for _, s := range []string{string(raw), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v)} {
			if strings.Contains(s, "synthetic-prompt") || strings.Contains(s, "synthetic-output") {
				t.Fatalf("protected data leaked: %s", s)
			}
		}
	}
}

func TestAIRouterChatLocalValidationRetainsReceipt(t *testing.T) {
	var calls int
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		calls++
		return credentials.Material{}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Account().AIRouter().Chat(context.Background(), ai.ChatRequest{})
	if types.ErrorCode(err) != types.InvalidArgument || calls != 0 || result.Receipt == nil || result.Receipt.Outcome != types.NotSent || result.Receipt.Attempts != 0 || result.Receipt.ContractRevision == "" {
		t.Fatalf("error=%v result=%+v calls=%d", err, result, calls)
	}
}

func TestTokenFactoryChatUsesExactProjectKey(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
	var calls int
	c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Scope != scope || r.Service != "token_factory" {
			t.Fatalf("auth %+v", r)
		}
		return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://preview.tf.selcloud.ru/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Fatal("wrong model endpoint/auth")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"answer"}}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project(scope.ProjectID)
	result, err := p.TokenFactory().Chat(context.Background(), ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}})
	if err != nil || calls != 1 || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || result.Receipt.Scope != scope || result.Data.Usage != nil {
		t.Fatalf("error=%v result=%+v", err, result)
	}
}

func TestInferenceChatRequiresExplicitEndpoint(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprint(configured), func(t *testing.T) {
			var lookups, calls int
			endpoints := map[types.EndpointKey]types.EndpointBinding{}
			if configured {
				endpoints[types.EndpointKey{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}] = types.EndpointBinding{ID: "explicit-inference", URL: "https://inference.example.invalid/v1", APIGeneration: "v1", Scope: scope, Audience: credentials.SelectelAudience}
			}
			c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, EndpointOverrides: endpoints, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
				lookups++
				if r.Scope != scope || r.Service != "dedicated_inference" {
					t.Fatalf("auth %+v", r)
				}
				return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
			}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://inference.example.invalid/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
					t.Fatal("wrong model endpoint/auth")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"answer"}}]}`))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			p, _ := c.Project(scope.ProjectID)
			result, err := p.Inference().Chat(context.Background(), ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}})
			if !configured {
				if types.ErrorCode(err) != types.EndpointUnavailable || lookups != 0 || calls != 0 || result.Receipt == nil || result.Receipt.Outcome != types.NotSent {
					t.Fatalf("error=%v result=%+v lookups=%d calls=%d", err, result, lookups, calls)
				}
				return
			}
			if err != nil || calls != 1 || result.Receipt == nil || result.Receipt.Endpoint == nil || result.Receipt.Endpoint.ID != "explicit-inference" || result.Receipt.Outcome != types.Accepted {
				t.Fatalf("error=%v result=%+v", err, result)
			}
		})
	}
}

func TestAIRouterGuardrailRejectedBeforeSSE(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", ReadMaxAttempts: 5, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 422, Header: http.Header{"Content-Type": {"application/json"}, "X-Gateway-Request-Id": {"synthetic-request-id"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"guardrail_rejected","message":"synthetic-private-provider-message"}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Account().AIRouter().Chat(context.Background(), ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}})
	var e *types.SDKError
	if !errors.As(err, &e) || e.ProviderCode != "guardrail_rejected" || e.HTTPStatus != 422 || calls != 1 || result.Receipt == nil || result.Receipt.Outcome != types.Rejected || len(result.Metadata.RequestIDs) != 1 || result.Metadata.RequestIDs[0] != "synthetic-request-id" {
		t.Fatalf("error=%+v result=%+v calls=%d", err, result, calls)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", err, result), "synthetic-private-provider-message") {
		t.Fatal("provider content leaked")
	}
}

func TestAIModelPOSTNeverReplaysOrInventsRecovery(t *testing.T) {
	for _, service := range []string{"ai_router", "token_factory", "dedicated_inference"} {
		for _, tc := range []struct {
			name    string
			status  int
			body    string
			code    types.Code
			outcome types.Outcome
		}{
			{"connection lost", 0, "", types.UnknownOutcome, types.Unknown},
			{"accepted malformed", 200, `{"choices":[]}`, types.ProtocolError, types.Accepted},
			{"server failure", 500, `{"error":{"code":"internal_error","message":"synthetic-private-output"}}`, types.UnknownOutcome, types.Unknown},
			{"rate limited", 429, `{"error":"synthetic-private-output"}`, types.RateLimited, types.Rejected},
			{"redirect", 303, "", types.UnknownOutcome, types.Unknown},
		} {
			t.Run(service+"/"+tc.name, func(t *testing.T) {
				scope := types.Scope{AccountID: "test-account"}
				if service != "ai_router" {
					scope.ProjectID = "project-one"
				}
				var calls int
				cfg := sdk.Config{AccountID: scope.AccountID, ReadMaxAttempts: 5, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
					return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
				}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.Host == "foreign.invalid" {
						t.Fatal("followed redirect")
					}
					if tc.status == 0 {
						return nil, io.ErrUnexpectedEOF
					}
					return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": {"https://foreign.invalid/"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				})}
				if service == "dedicated_inference" {
					cfg.EndpointOverrides = map[types.EndpointKey]types.EndpointBinding{{Service: service, APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}
				}
				c, err := sdk.New(cfg)
				if err != nil {
					t.Fatal(err)
				}
				req := ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}}
				var result types.Result[ai.ChatCompletion]
				switch service {
				case "ai_router":
					result, err = c.Account().AIRouter().Chat(context.Background(), req)
				case "token_factory":
					p, _ := c.Project(scope.ProjectID)
					result, err = p.TokenFactory().Chat(context.Background(), req)
				case "dedicated_inference":
					p, _ := c.Project(scope.ProjectID)
					result, err = p.Inference().Chat(context.Background(), req)
				}
				if types.ErrorCode(err) != tc.code || calls != 1 || result.Receipt == nil || result.Receipt.Outcome != tc.outcome || result.Receipt.Attempts != 1 || result.Receipt.Recovery != "unavailable" || len(result.Receipt.ResourceRefs) != 0 {
					t.Fatalf("error=%v result=%+v calls=%d", err, result, calls)
				}
				if service == "ai_router" && tc.status == 500 {
					var known *types.SDKError
					if !errors.As(err, &known) || known.ProviderCode != "internal_error" {
						t.Fatalf("known provider code lost: %+v", err)
					}
				}
				if strings.Contains(fmt.Sprintf("%+v %#v", err, result), "synthetic-private-output") {
					t.Fatal("raw output leaked")
				}
			})
		}
	}
}

func TestAIDataCatalogPreservesProjectRequirements(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"token_factory", "dedicated_inference"} {
		capability, err := c.Catalog().Describe(service + ".v1.chat_completions.create")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(capability.ScopeRequirements, "project") || capability.Auth.Scheme != types.ProtocolSpecific || capability.Effect != "mutation" {
			t.Fatalf("catalog scope/auth/effect mismatch: %+v", capability)
		}
	}
}

func TestAIChatExplicitReasoningAndTokenBound(t *testing.T) {
	maximum, _ := types.ParseInteger("16")
	for _, service := range []string{"ai_router", "dedicated_inference"} {
		t.Run(service, func(t *testing.T) {
			scope := types.Scope{AccountID: "test-account"}
			if service == "dedicated_inference" {
				scope.ProjectID = "project-one"
			}
			c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("key"))}, nil
			}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				var wire struct {
					MaxTokens int `json:"max_tokens"`
					Reasoning *struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
				}
				if json.NewDecoder(r.Body).Decode(&wire) != nil || wire.MaxTokens != 16 {
					t.Fatal("explicit token limit lost")
				}
				if service == "ai_router" {
					if wire.Reasoning == nil || wire.Reasoning.Effort != "low" {
						t.Fatal("reasoning effort lost")
					}
				} else if wire.Reasoning != nil {
					t.Fatal("unrequested reasoning default")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"answer"}}]}`))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			req := ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}, MaxTokens: &maximum}
			if service == "ai_router" {
				req.Reasoning = &ai.Reasoning{Effort: "low"}
			}
			if service == "ai_router" {
				_, err = c.Account().AIRouter().Chat(context.Background(), req)
			} else {
				p, _ := c.Project(scope.ProjectID)
				_, err = p.Inference().Chat(context.Background(), req)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAIDataCatalogDoesNotEquateCompatibleSubsetWithCompleteAPI(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	for service, count := range map[string]int{"ai_router": 2, "dedicated_inference": 5, "token_factory": 1} {
		rows, err := c.Catalog().List(catalog.Filter{Service: service})
		if err != nil || len(rows) != count {
			t.Fatalf("%s count=%d error=%v", service, len(rows), err)
		}
		for _, row := range rows {
			if row.Implementation.Status != "partial" || row.Effect != "mutation" || row.GoMethod == nil || !slices.Contains(row.Limitations, "Narrow first-party entry-mode/compatible response subset; not whole OpenAI/Jina API support") {
				t.Fatalf("overclaimed %s: %+v", service, row)
			}
		}
	}
	summary := c.Catalog().Summary()
	if summary.InventoryComplete || summary.CoverageRatio != nil {
		t.Fatal("unknown denominator reported complete")
	}
}
