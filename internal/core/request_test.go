package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func staticSource() credentials.Source {
	return credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic-source", Revision: "revision", Scheme: credentials.StaticToken, Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Token: types.Protect([]byte("synthetic-private-token"))}, nil
	})
}
func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"X-Request-Id": {"req-synthetic"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func readOperation() Operation {
	return Operation{ID: "billing.v3.balances.get", Service: "billing", APIGeneration: "v3", Method: "GET", Effect: "read", Auth: types.StaticToken, EndpointID: "billing", FixedEndpoint: "https://api.example.test", Success: []int{200}}
}
func writeOperation() Operation {
	return Operation{ID: "projects.v2.projects.create", Service: "projects", APIGeneration: "v2", Method: "POST", Effect: "mutation", Auth: types.StaticToken, EndpointID: "projects", FixedEndpoint: "https://api.example.test", Success: []int{202}, Rejected: []int{400, 401, 403}}
}
func decodeObject(raw []byte, _ types.EndpointBinding) (Decoded[map[string]any], error) {
	var value map[string]any
	err := json.Unmarshal(raw, &value)
	return Decoded[map[string]any]{Data: value}, err
}
func request() Request {
	return Request{Scope: types.Scope{AccountID: "test-account"}, Path: "/resource", Body: []byte(`{"payload":"synthetic-private-marker"}`)}
}

func TestReadRetriesAndObserverCannotExposePayloadOrChangeResult(t *testing.T) {
	count, observed := 0, 0
	r, err := New(Config{AccountID: "test-account", Credentials: staticSource(), RetryBaseDelay: time.Millisecond, RetryMaxDelay: time.Millisecond, Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		count++
		if req.Header.Get("X-Token") == "" {
			t.Error("authentication missing")
		}
		if count < 3 {
			return reply(503, `{"message":"synthetic-private-marker"}`), nil
		}
		return reply(200, `{"ok":true}`), nil
	}), Observer: func(e Event) {
		observed++
		raw, _ := json.Marshal(e)
		if strings.Contains(string(raw), "private") {
			t.Error("observer leaked payload")
		}
		panic("synthetic observer failure")
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := DoJSON(r, context.Background(), readOperation(), request(), decodeObject)
	if err != nil || count != 3 || observed != 1 || result.Data["ok"] != true || result.Receipt != nil {
		t.Fatal("bounded read retry failed")
	}
}

func TestUnknownMutationIsNeverRetried(t *testing.T) {
	count := 0
	r, err := New(Config{AccountID: "test-account", Credentials: staticSource(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { count++; return nil, io.ErrUnexpectedEOF })})
	if err != nil {
		t.Fatal(err)
	}
	result, err := DoJSON(r, context.Background(), writeOperation(), request(), decodeObject)
	if count != 1 || types.ErrorCode(err) != types.UnknownOutcome || result.Receipt == nil || result.Receipt.Outcome != types.Unknown || result.Receipt.Attempts != 1 || result.Receipt.CompletedAt.IsZero() {
		t.Fatal("uncertain write was replayed or lost receipt")
	}
	data, _ := json.Marshal(err)
	if strings.Contains(string(data), "private") {
		t.Fatal("error leaked input")
	}
}

func TestMutationAcknowledgementSurvivesDecodeFailure(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		outcome types.Outcome
		code    types.Code
	}{{202, "invalid-json", types.Accepted, types.ProtocolError}, {400, `{"message":"synthetic-private-marker"}`, types.Rejected, types.ProviderError}, {503, "invalid-json", types.Unknown, types.UnknownOutcome}} {
		count := 0
		r, err := New(Config{AccountID: "test-account", Credentials: staticSource(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { count++; return reply(tc.status, tc.body), nil })})
		if err != nil {
			t.Fatal(err)
		}
		result, err := DoJSON(r, context.Background(), writeOperation(), request(), decodeObject)
		if count != 1 || types.ErrorCode(err) != tc.code || result.Receipt.Outcome != tc.outcome || result.Receipt.CompletedAt.IsZero() {
			t.Fatal("write outcome misclassified")
		}
	}
}

func TestEarlyFailureFinalizesReceiptWithoutCredentialsIO(t *testing.T) {
	lookups, observed := 0, 0
	source := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		lookups++
		return credentials.Material{}, errors.New("synthetic-private-marker")
	})
	r, err := New(Config{AccountID: "test-account", Credentials: source, Observer: func(Event) { observed++ }})
	if err != nil {
		t.Fatal(err)
	}
	in := request()
	in.Validate = func() error { return types.NewError(types.InvalidArgument) }
	result, err := DoJSON(r, context.Background(), writeOperation(), in, decodeObject)
	if types.ErrorCode(err) != types.InvalidArgument || lookups != 0 || observed != 1 || result.Receipt.Outcome != types.NotSent || result.Receipt.CredentialIdentity != "" || result.Receipt.CompletedAt.IsZero() {
		t.Fatal("early failure escaped finalizer")
	}
}

func TestRetryAfterRespectsTotalBudget(t *testing.T) {
	count := 0
	r, err := New(Config{AccountID: "test-account", Credentials: staticSource(), CallTimeout: 50 * time.Millisecond, Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		count++
		res := reply(429, "private error")
		res.Header.Set("Retry-After", "120")
		return res, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = DoJSON(r, context.Background(), readOperation(), request(), decodeObject)
	var e *types.SDKError
	if count != 1 || !errors.As(err, &e) || e.Code != types.RateLimited || e.Reason != "retry_budget_exhausted" || time.Since(start) > time.Second {
		t.Fatal("server backoff was ignored or budget extended")
	}
}

func TestCanceledMutationAfterSendRemainsUnknown(t *testing.T) {
	r, err := New(Config{AccountID: "test-account", Credentials: staticSource(), Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result, err := DoJSON(r, ctx, writeOperation(), request(), decodeObject)
	var e *types.SDKError
	if !errors.As(err, &e) || e.Code != types.UnknownOutcome || e.CauseCode != types.DeadlineExceeded || result.Receipt.Outcome != types.Unknown {
		t.Fatal("cancel was confused with remote abort")
	}
}

func TestResponseLimitDoesNotDiscardAcknowledgement(t *testing.T) {
	r, err := New(Config{AccountID: "test-account", Credentials: staticSource(), MaxJSONBytes: 8, Transport: transportFunc(func(*http.Request) (*http.Response, error) { return reply(202, strings.Repeat("x", 32)), nil })})
	if err != nil {
		t.Fatal(err)
	}
	result, err := DoJSON(r, context.Background(), writeOperation(), request(), decodeObject)
	if types.ErrorCode(err) != types.ResponseLimit || result.Receipt.Outcome != types.Accepted {
		t.Fatal("body limit lost accepted outcome")
	}
}
