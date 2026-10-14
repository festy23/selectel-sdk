package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/iam"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestIAMUsersBoundedCollection(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method != "GET" || r.URL.String() != "https://api.selectel.ru/iam/v1/users" || r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"users":[{"id":"one","auth_type":"local"},{"id":"two","auth_type":"local"}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Account().IAM().Users.Collect(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Complete || got.Items[0].ID == nil || *got.Items[0].ID != "one" || got.Items[0].Ref == nil {
		t.Fatalf("unexpected collection: %+v", got)
	}
}

func TestIAMGroupCreateAndUnknownMembership(t *testing.T) {
	var membershipCalls int
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path == "/iam/v1/groups" && r.Method == "POST" {
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != `{"description":"","name":"ops"}` {
				t.Fatalf("body %s", raw)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"g1","name":"ops","users":[],"service_users":[]}`))}, nil
		}
		if r.URL.Path == "/iam/v1/groups/g1/users" && r.Method == "PUT" {
			membershipCalls++
			return nil, io.ErrUnexpectedEOF
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL)
		return nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	name, description := "ops", ""
	created, err := c.Account().IAM().Groups.Create(context.Background(), iam.CreateGroupRequest{Name: &name, Description: &description}, types.CallOptions{})
	if err != nil || created.Data.Ref == nil || created.Receipt == nil || created.Receipt.Outcome != types.Accepted {
		t.Fatalf("create %+v %v", created, err)
	}
	changed, err := c.Account().IAM().Groups.AddUsers(context.Background(), *created.Data.Ref, iam.ModifyGroupUsersRequest{KeystoneIDs: []string{"keystone-user"}}, types.CallOptions{})
	if types.ErrorCode(err) != types.UnknownOutcome || changed.Receipt == nil || changed.Receipt.Outcome != types.Unknown || len(changed.Receipt.ResourceRefs) != 1 || membershipCalls != 1 {
		t.Fatalf("membership %+v %v calls=%d", changed, err, membershipCalls)
	}
}

func TestIAMGroupLifecycleAndMembershipRemoval(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		body := `{"id":"g1","name":"ops","users":[],"service_users":[]}`
		status := 200
		if r.URL.Path == "/iam/v1/groups" {
			body = `{"groups":[{"id":"g1","users_count":2}]}`
		}
		if r.Method == "DELETE" {
			status = 204
			body = ""
		}
		if r.Method == "PATCH" {
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != `{"description":""}` {
				t.Fatalf("update body %s", raw)
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	api := c.Account().IAM().Groups
	listed, err := api.Collect(context.Background(), pagination.Options{})
	if err != nil || !listed.Complete || len(listed.Items) != 1 || listed.Items[0].Ref == nil {
		t.Fatalf("list %+v %v", listed, err)
	}
	ref := *listed.Items[0].Ref
	got, err := api.Get(context.Background(), ref, types.CallOptions{})
	if err != nil || got.Data.Ref == nil {
		t.Fatalf("get %+v %v", got, err)
	}
	empty := ""
	updated, err := api.Update(context.Background(), ref, iam.ModifyGroupRequest{Description: &empty}, types.CallOptions{})
	if err != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatalf("update %+v %v", updated, err)
	}
	removed, err := api.RemoveUsers(context.Background(), ref, iam.ModifyGroupUsersRequest{KeystoneIDs: []string{"k1"}}, types.CallOptions{})
	if err != nil || removed.Receipt.Outcome != types.Accepted {
		t.Fatalf("remove %+v %v", removed, err)
	}
	deleted, err := api.Delete(context.Background(), ref, types.CallOptions{})
	if err != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatalf("delete %+v %v", deleted, err)
	}
}

func TestIAMServiceUserLifecycleProtectsPasswordAndPreservesFalse(t *testing.T) {
	secret := "synthetic-service-password"
	var paths []string
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		paths = append(paths, r.Method+" "+r.URL.Path)
		status := 200
		body := `{"id":"su1","enabled":false,"name":"robot"}`
		if r.Method == "GET" && r.URL.Path == "/iam/v1/service_users" {
			body = `{"users":[{"id":"su1","enabled":false,"name":"robot"}]}`
		}
		if r.Method == "POST" || r.Method == "PATCH" {
			var fields map[string]any
			if json.NewDecoder(r.Body).Decode(&fields) != nil {
				t.Fatal("JSON")
			}
			if fields["password"] != secret || fields["enabled"] != false {
				t.Fatalf("protected request missing fields")
			}
		}
		if r.Method == "DELETE" || r.Method == "PUT" {
			status = 204
			body = ""
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	api := c.Account().IAM().ServiceUsers
	name := "robot"
	enabled := false
	password := types.Protect([]byte(secret))
	in := iam.ServiceUserCreateRequest{Name: &name, Enabled: &enabled, Password: &password}
	encoded, _ := json.Marshal(in)
	if strings.Contains(string(encoded), secret) || strings.Contains(fmt.Sprintf("%+v", in), secret) {
		t.Fatal("password disclosure")
	}
	created, err := api.Create(context.Background(), in, types.CallOptions{})
	if err != nil || created.Data.Ref == nil || created.Receipt.Outcome != types.Accepted {
		t.Fatalf("create %+v %v", created, err)
	}
	ref := *created.Data.Ref
	listed, err := api.Collect(context.Background(), pagination.Options{})
	if err != nil || !listed.Complete || len(listed.Items) != 1 {
		t.Fatalf("list %+v %v", listed, err)
	}
	got, err := api.Get(context.Background(), ref, types.CallOptions{})
	if err != nil || got.Data.Ref == nil {
		t.Fatalf("get %+v %v", got, err)
	}
	updated, err := api.Update(context.Background(), ref, iam.ServiceUserUpdateRequest{Enabled: &enabled, Password: &password}, types.CallOptions{})
	if err != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatalf("update %+v %v", updated, err)
	}
	for _, call := range []func() (types.Result[struct{}], error){func() (types.Result[struct{}], error) {
		return api.AddGroups(context.Background(), ref, iam.ModifyServiceUserGroupIDsRequest{GroupIDs: []string{"g1"}}, types.CallOptions{})
	}, func() (types.Result[struct{}], error) {
		return api.RemoveGroups(context.Background(), ref, iam.ModifyServiceUserGroupIDsRequest{GroupIDs: []string{"g1"}}, types.CallOptions{})
	}, func() (types.Result[struct{}], error) {
		return api.Delete(context.Background(), ref, types.CallOptions{})
	}} {
		out, e := call()
		if e != nil || out.Receipt.Outcome != types.Accepted {
			t.Fatalf("mutation %+v %v", out, e)
		}
	}
	if !slices.Equal(paths, []string{"POST /iam/v1/service_users", "GET /iam/v1/service_users", "GET /iam/v1/service_users/su1", "PATCH /iam/v1/service_users/su1", "PUT /iam/v1/service_users/su1/groups", "DELETE /iam/v1/service_users/su1/groups", "DELETE /iam/v1/service_users/su1"}) {
		t.Fatalf("calls %v", paths)
	}
}

func TestIAMUserLifecycleUsesDocumentedInviteAndMembership(t *testing.T) {
	var seen []string
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		seen = append(seen, r.Method+" "+r.URL.Path)
		status := 200
		body := `{"id":"u1","auth_type":"local","description":""}`
		if r.URL.Path == "/iam/v1/users" && r.Method == "POST" {
			var fields map[string]any
			_ = json.NewDecoder(r.Body).Decode(&fields)
			if fields["subscriptions_only"] != false || len(fields["subscriptions"].([]any)) != 0 || fields["email"] != "synthetic@example.invalid" {
				t.Fatal("create defaults changed")
			}
		}
		if r.Method == "DELETE" || r.Method == "PUT" || strings.HasSuffix(r.URL.Path, "resend_invite") {
			status = 204
			body = ""
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	api := c.Account().IAM().Users
	email := "synthetic@example.invalid"
	no := false
	created, err := api.Create(context.Background(), iam.UserCreateRequest{Email: &email, SubscriptionsOnly: &no, Subscriptions: []string{}}, types.CallOptions{})
	if err != nil || created.Data.Ref == nil {
		t.Fatalf("create %+v %v", created, err)
	}
	ref := *created.Data.Ref
	got, err := api.Get(context.Background(), ref, types.CallOptions{})
	if err != nil || got.Data.Ref == nil {
		t.Fatalf("get %+v %v", got, err)
	}
	empty := ""
	updated, err := api.Update(context.Background(), ref, iam.UserUpdateRequest{Description: &empty}, types.CallOptions{})
	if err != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatalf("update %+v %v", updated, err)
	}
	for _, call := range []func() (types.Result[struct{}], error){func() (types.Result[struct{}], error) {
		return api.AddGroups(context.Background(), ref, iam.ModifyUserGroupIDsRequest{GroupIDs: []string{"g1"}}, types.CallOptions{})
	}, func() (types.Result[struct{}], error) {
		return api.RemoveGroups(context.Background(), ref, iam.ModifyUserGroupIDsRequest{GroupIDs: []string{"g1"}}, types.CallOptions{})
	}, func() (types.Result[struct{}], error) {
		return api.ResendInvite(context.Background(), ref, types.CallOptions{})
	}, func() (types.Result[struct{}], error) {
		return api.Delete(context.Background(), ref, types.CallOptions{})
	}} {
		out, e := call()
		if e != nil || out.Receipt.Outcome != types.Accepted {
			t.Fatalf("mutation %+v %v", out, e)
		}
	}
	if !slices.Equal(seen, []string{"POST /iam/v1/users", "GET /iam/v1/users/u1", "PATCH /iam/v1/users/u1", "PUT /iam/v1/users/u1/groups", "DELETE /iam/v1/users/u1/groups", "POST /iam/v1/users/u1/resend_invite", "DELETE /iam/v1/users/u1"}) {
		t.Fatalf("calls %v", seen)
	}
}

func TestIAMCurrentUserAndGroups(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		body := `{"id":"u1","keystone_id":"k1","auth_type":"local"}`
		if r.URL.Path == "/iam/v1/me/groups" {
			body = `{"groups":[{"id":"g1","name":"ops"}]}`
		} else if r.URL.Path != "/iam/v1/me" {
			t.Fatalf("unexpected %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	me, err := c.Account().IAM().CurrentUser(context.Background(), types.CallOptions{})
	if err != nil || me.Data.Ref == nil || me.Data.Ref.ID != "u1" {
		t.Fatalf("me %+v %v", me, err)
	}
	groups, err := c.Account().IAM().CurrentGroups.Collect(context.Background(), pagination.Options{})
	if err != nil || !groups.Complete || len(groups.Items) != 1 || groups.Items[0].Ref == nil {
		t.Fatalf("groups %+v %v", groups, err)
	}
}

func TestIAMCollectionCursorBindsExactOverride(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "iam", APIGeneration: "v1", Scope: scope}: {ID: "explicit-iam", URL: "https://iam.example.invalid", Scope: scope, APIGeneration: "v1", Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Host != "iam.example.invalid" {
			t.Fatal("override not used")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"users":[{"id":"one"},{"id":"two"}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.Account().IAM().Users.ListPage(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || page.NextCursor == nil || page.NextCursor.EndpointID != "explicit-iam" {
		t.Fatalf("page %+v %v", page, err)
	}
	next, err := c.Account().IAM().Users.ListPage(context.Background(), pagination.Options{Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || *next.Items[0].ID != "two" || !next.Done {
		t.Fatalf("resume %+v %v", next, err)
	}
}

func TestIAMMutationPreservesAcceptedReferenceOnMalformedResponse(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"created-group","name":false}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	name := "ops"
	out, err := c.Account().IAM().Groups.Create(context.Background(), iam.CreateGroupRequest{Name: &name}, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "created-group" {
		t.Fatalf("lost acceptance/reference: %+v %v", out, err)
	}
}

func TestIAMRejectsForeignReferencesBeforeIO(t *testing.T) {
	var calls int
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })})
	if err != nil {
		t.Fatal(err)
	}
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "foreign"}, Service: "iam", APIGeneration: "v1", Kind: "user", ID: "u1", EndpointID: "selectel-iam", EndpointURL: "https://api.selectel.ru"}
	out, err := c.Account().IAM().Users.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(err) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("scope %+v %v calls=%d", out, err, calls)
	}
	ref.Scope.AccountID = "test-account"
	ref.Kind = "group"
	out, err = c.Account().IAM().Users.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(err) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("kind %+v %v calls=%d", out, err, calls)
	}
}

func TestIAMRejectionsAndForeignResponsesDoNotReplaceReferences(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		code    types.Code
		outcome types.Outcome
	}{{404, `{"message":"synthetic-secret-provider-message"}`, types.NotFound, types.Rejected}, {403, `{"message":"synthetic-secret-provider-message"}`, types.PermissionDenied, types.Rejected}, {200, `{"id":"foreign"}`, types.ScopeMismatch, types.Accepted}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "iam", APIGeneration: "v1", Kind: "user", ID: "u1", EndpointID: "selectel-iam", EndpointURL: "https://api.selectel.ru"}
			empty := ""
			out, e := c.Account().IAM().Users.Update(context.Background(), ref, iam.UserUpdateRequest{Description: &empty}, types.CallOptions{})
			if types.ErrorCode(e) != tc.code || out.Receipt == nil || out.Receipt.Outcome != tc.outcome || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "u1" {
				t.Fatalf("out %+v error %v", out, e)
			}
			if strings.Contains(fmt.Sprint(e), "synthetic-secret-provider-message") {
				t.Fatal("provider body disclosed")
			}
		})
	}
}

func TestIAMCatalogMapsCompleteUserAndMembershipSubset(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"users.list", "users.create", "users.get", "users.update", "users.delete", "users.resend_invite", "users_groups.add", "users_groups.remove", "service_users.list", "service_users.create", "service_users.get", "service_users.update", "service_users.delete", "service_users_groups.add", "service_users_groups.remove", "groups.list", "groups.create", "groups.get", "groups.update", "groups.delete", "groups_users.add", "groups_users.remove", "me.get", "me_groups.list"} {
		op, err := c.Catalog().Describe("iam.v1." + id)
		if err != nil || op.GoMethod == nil || op.Implementation.Status != "implemented" || op.APIGeneration != "v1" {
			t.Fatalf("catalog %s %+v %v", id, op, err)
		}
	}
}

func TestIAMCreationRejectsUnsupportedRoleScopeBeforeIO(t *testing.T) {
	var calls int
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })})
	if err != nil {
		t.Fatal(err)
	}
	email := "synthetic@example.invalid"
	scope := "all_accounts"
	out, err := c.Account().IAM().Users.Create(context.Background(), iam.UserCreateRequest{Email: &email, Roles: []iam.RoleRequest{{Scope: &scope}}}, types.CallOptions{})
	if types.ErrorCode(err) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("invalid variant %+v %v calls=%d", out, err, calls)
	}
}
