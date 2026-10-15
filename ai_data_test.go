package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/ai"
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
