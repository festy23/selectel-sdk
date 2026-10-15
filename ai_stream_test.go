package selectelsdk_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/ai"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func TestAIRouterStreamRetainsProtectedEventsAndUsage(t *testing.T) {
	calls, closed := 0, 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"stream":true`) || r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Fatal("wrong stream request")
		}
		body := ": keepalive\n\ndata: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"synthetic-stream-output\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\ndata: [DONE]\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: &aiTrackedBody{Reader: strings.NewReader(body), close: func() { closed++ }}}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	var chunks []ai.ChatChunk
	result, err := c.Account().AIRouter().StreamChat(context.Background(), ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}}, ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 4096, MaxBytes: 8192}, func(chunk ai.ChatChunk) error {
		chunks = append(chunks, chunk)
		if chunk.Usage != nil {
			altered, _ := types.ParseInteger("999")
			*chunk.Usage.TotalTokens = altered
		}
		return nil
	})
	if err != nil || calls != 1 || closed != 1 || !result.Data.Complete || result.Data.Events != 2 || result.Data.Usage == nil || result.Data.Usage.TotalTokens.String() != "3" || result.Receipt == nil || result.Receipt.Outcome != types.Accepted {
		t.Fatalf("error=%v result=%+v calls=%d close=%d", err, result, calls, closed)
	}
	if len(chunks) != 2 || string(chunks[0].Choices[0].Delta.Content.Reveal()) != "synthetic-stream-output" {
		t.Fatal("stream content lost")
	}
}

type aiTrackedBody struct {
	io.Reader
	close func()
}

func (b *aiTrackedBody) Close() error { b.close(); return nil }

func TestAIRouterStreamBoundedFailuresPreserveAcceptedProgress(t *testing.T) {
	chunk := `data: {"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"synthetic-stream-output"}}]}` + "\n\n"
	for _, tc := range []struct {
		name, body, media string
		limit             ai.StreamOptions
		callback          bool
		code              types.Code
		events            uint
	}{
		{"missing terminal", chunk, "text/event-stream", ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 4096, MaxBytes: 8192}, false, types.ProtocolError, 1},
		{"malformed after prefix", chunk + "data: {}\n\n", "text/event-stream", ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 4096, MaxBytes: 8192}, false, types.ProtocolError, 1},
		{"event count", chunk + chunk + "data: [DONE]\n\n", "text/event-stream", ai.StreamOptions{MaxEvents: 1, MaxEventBytes: 4096, MaxBytes: 8192}, false, types.ResponseLimit, 1},
		{"event size", chunk, "text/event-stream", ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 32, MaxBytes: 8192}, false, types.ResponseLimit, 0},
		{"total bytes", chunk + chunk, "text/event-stream", ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 256, MaxBytes: 256}, false, types.ResponseLimit, 1},
		{"consumer stops", chunk, "text/event-stream", ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 4096, MaxBytes: 8192}, true, types.Incomplete, 1},
		{"wrong media", chunk, "application/json", ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 4096, MaxBytes: 8192}, false, types.ProtocolError, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, closed, observed := 0, 0, uint(0)
			c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
			}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {tc.media}}, Body: &aiTrackedBody{Reader: strings.NewReader(tc.body), close: func() { closed++ }}}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Account().AIRouter().StreamChat(context.Background(), ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}}, tc.limit, func(chunk ai.ChatChunk) error {
				observed++
				for _, diagnostic := range []string{fmt.Sprintf("%+v", chunk), fmt.Sprintf("%#v", chunk)} {
					if strings.Contains(diagnostic, "synthetic-stream-output") || strings.Contains(diagnostic, fmt.Sprint([]byte("synthetic-stream-output"))) || strings.Contains(diagnostic, fmt.Sprintf("%x", []byte("synthetic-stream-output"))) {
						t.Fatal("stream diagnostic leak")
					}
				}
				if tc.callback {
					return errors.New("synthetic-private-callback-error")
				}
				return nil
			})
			if types.ErrorCode(err) != tc.code || calls != 1 || closed != 1 || result.Data.Complete || result.Data.Events != tc.events || observed != tc.events || result.Receipt == nil || result.Receipt.Outcome != types.Accepted {
				t.Fatalf("error=%v result=%+v calls=%d close=%d observed=%d", err, result, calls, closed, observed)
			}
			if strings.Contains(fmt.Sprint(err), "synthetic-private-callback-error") {
				t.Fatal("callback diagnostics leaked")
			}
		})
	}
}

func TestAIRouterStreamGuardrailJSONDoesNotInvokeConsumer(t *testing.T) {
	calls, observed := 0, 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 422, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"guardrail_rejected"}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Account().AIRouter().StreamChat(context.Background(), ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}}, ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 4096, MaxBytes: 8192}, func(ai.ChatChunk) error { observed++; return nil })
	var e *types.SDKError
	if !errors.As(err, &e) || e.ProviderCode != "guardrail_rejected" || calls != 1 || observed != 0 || result.Receipt == nil || result.Receipt.Outcome != types.Rejected || result.Data.Complete {
		t.Fatalf("error=%v result=%+v calls=%d observed=%d", err, result, calls, observed)
	}
}

func TestAIRouterStreamCancellationClosesOwnedBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &aiBlockingBody{started: make(chan struct{}), closed: make(chan struct{})}
	var calls atomic.Int32
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var result types.Result[ai.StreamSummary]
	var callErr error
	go func() {
		defer close(done)
		result, callErr = c.Account().AIRouter().StreamChat(ctx, ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: "user", Content: types.Protect([]byte("question"))}}}, ai.StreamOptions{MaxEvents: 4, MaxEventBytes: 4096, MaxBytes: 8192}, func(ai.ChatChunk) error { t.Error("unexpected event"); return nil })
	}()
	select {
	case <-body.started:
	case <-time.After(2 * time.Second):
		t.Fatal("read not started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = body.Close()
		t.Fatal("cancellation failed to release owned stream")
	}
	if types.ErrorCode(callErr) != types.Canceled || calls.Load() != 1 || body.closes.Load() != 1 || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || result.Data.Complete {
		t.Fatalf("error=%v result=%+v calls=%d closes=%d", callErr, result, calls.Load(), body.closes.Load())
	}
}

type aiBlockingBody struct {
	started, closed      chan struct{}
	startOnce, closeOnce sync.Once
	closes               atomic.Int32
}

func (b *aiBlockingBody) Read([]byte) (int, error) {
	b.startOnce.Do(func() { close(b.started) })
	<-b.closed
	return 0, io.ErrClosedPipe
}
func (b *aiBlockingBody) Close() error {
	b.closes.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}
