package selectelsdk_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func source() credentials.Source {
	return credentials.SourceFunc(func(ctx context.Context, req types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "test-source", Revision: "test-revision", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.IAMPassword, Username: types.Protect([]byte("test-user")), Password: types.Protect([]byte("synthetic-password"))}, nil
	})
}
func auth(project string) types.AuthRequest {
	s := types.Scope{AccountID: "test-account", ProjectID: project}
	mode := types.IAMAccount
	if project != "" {
		mode = types.IAMProject
	}
	return types.AuthRequest{Scope: s, Mode: mode, Service: "compute", Audience: credentials.SelectelAudience}
}
func response(scope types.Scope, expiry time.Time, status int) *http.Response {
	token := map[string]any{"expires_at": expiry.UTC().Format(time.RFC3339Nano), "user": map[string]string{"id": "test-subject"}, "catalog": []any{}}
	if scope.ProjectID == "" {
		token["domain"] = map[string]string{"name": scope.AccountID}
	} else {
		token["project"] = map[string]any{"id": scope.ProjectID, "domain": map[string]string{"name": scope.AccountID}}
	}
	data, _ := json.Marshal(map[string]any{"token": token})
	return &http.Response{StatusCode: status, Header: http.Header{"X-Subject-Token": []string{"synthetic-token"}}, Body: io.NopCloser(strings.NewReader(string(data)))}
}
func requestedScope(req *http.Request) types.Scope {
	var body struct {
		Auth struct {
			Scope struct {
				Domain struct {
					Name string `json:"name"`
				} `json:"domain"`
				Project struct {
					ID string `json:"id"`
				} `json:"project"`
			} `json:"scope"`
		} `json:"auth"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	return types.Scope{AccountID: "test-account", ProjectID: body.Auth.Scope.Project.ID}
}

func TestConstructionAndViewsHaveNoIO(t *testing.T) {
	var calls atomic.Int32
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	p1, err := c.Project("one")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := c.Project("two")
	if err != nil {
		t.Fatal(err)
	}
	r, err := p1.Location("region", "pool")
	if err != nil {
		t.Fatal(err)
	}
	if c.Scope().ProjectID != "" || p1.Scope().ProjectID != "one" || p2.Scope().ProjectID != "two" || r.Scope().Pool != "pool" || calls.Load() != 0 {
		t.Fatal("construction mutated scope or performed I/O")
	}
	if _, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(nil)}); types.ErrorCode(err) != types.InvalidArgument {
		t.Fatal("nil source accepted")
	}
}

func TestIAMCacheIsScopedAndMetadataIsCopied(t *testing.T) {
	var calls atomic.Int32
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return response(requestedScope(req), time.Now().Add(time.Hour), 201), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.Authenticate(context.Background(), auth("one"))
	if err != nil {
		t.Fatal(err)
	}
	*first.ExpiresAt = time.Time{}
	again, err := c.Authenticate(context.Background(), auth("one"))
	if err != nil || again.ExpiresAt.IsZero() {
		t.Fatal("caller mutated cache metadata")
	}
	if _, err := c.Authenticate(context.Background(), auth("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Authenticate(context.Background(), auth("")); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("token cache crossed project/account scopes")
	}
	bad := auth("one")
	bad.Scope.AccountID = "different"
	if _, err := c.Authenticate(context.Background(), bad); types.ErrorCode(err) != types.ScopeMismatch || calls.Load() != 3 {
		t.Fatal("cross-account authorization performed")
	}
}

func TestConcurrentAuthUsesOneFlight(t *testing.T) {
	var calls atomic.Int32
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return response(requestedScope(req), time.Now().Add(time.Hour), 201), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 40)
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Authenticate(context.Background(), auth("one"))
			if err != nil {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("refresh was not single flight")
	}
}

func TestCanceledWaiterDoesNotCancelAuthInitiator(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-release:
			return response(requestedScope(req), time.Now().Add(time.Hour), 201), nil
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	owner := make(chan error, 1)
	go func() { _, err := c.Authenticate(context.Background(), auth("one")); owner <- err }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Authenticate(ctx, auth("one")); types.ErrorCode(err) != types.DeadlineExceeded {
		t.Fatal("waiter cancellation ignored")
	}
	close(release)
	if err := <-owner; err != nil {
		t.Fatal("waiter canceled initiator")
	}
}

func TestTokenScopeMismatchAndRedirectCannotEscape(t *testing.T) {
	for _, tc := range []struct {
		status int
		scope  types.Scope
		code   types.Code
	}{
		{201, types.Scope{AccountID: "different", ProjectID: "one"}, types.ScopeMismatch},
		{302, types.Scope{AccountID: "test-account", ProjectID: "one"}, types.ProtocolError},
		{403, types.Scope{AccountID: "test-account", ProjectID: "one"}, types.PermissionDenied},
	} {
		var calls atomic.Int32
		c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			r := response(tc.scope, time.Now().Add(time.Hour), tc.status)
			r.Header.Set("Location", "https://different.example.test/collect")
			return r, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Authenticate(context.Background(), auth("one")); types.ErrorCode(err) != tc.code {
			t.Fatal("unsafe auth response accepted")
		}
		if calls.Load() != 1 {
			t.Fatal("redirect or automatic retry performed")
		}
	}
}

func TestExternalTokenIntrospectionAndExpirationRace(t *testing.T) {
	for _, expiresDuringCall := range []bool{false, true} {
		expiry := time.Now().Add(time.Hour)
		if expiresDuringCall {
			expiry = time.Now().Add(15 * time.Millisecond)
		}
		s := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
			return credentials.Material{Identity: "test-external", Revision: "revision", Scheme: credentials.IAMToken, Binding: types.Scope{AccountID: "test-account", ProjectID: "one"}, Audience: credentials.SelectelAudience, Token: types.Protect([]byte("synthetic-external-token")), ExpiresAt: expiry}, nil
		})
		c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: s, Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != "GET" || req.Header.Get("X-Auth-Token") == "" || req.Header.Get("X-Subject-Token") == "" {
				t.Error("introspection request missing")
			}
			if expiresDuringCall {
				time.Sleep(25 * time.Millisecond)
			}
			return response(types.Scope{AccountID: "test-account", ProjectID: "one"}, time.Now().Add(time.Hour), 200), nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		meta, err := c.Authenticate(context.Background(), auth("one"))
		if expiresDuringCall {
			if types.ErrorCode(err) != types.Unauthenticated {
				t.Fatal("expired effective token accepted")
			}
		} else if err != nil || meta.Verification != "provider_verified" {
			t.Fatal("introspection failed")
		}
	}
}
