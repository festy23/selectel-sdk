package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/iamcredentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServiceUserS3CredentialLifecycleProtectsSecret(t *testing.T) {
	secret := "synthetic-s3-secret-key"
	var seen []string
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal("account token missing")
		}
		seen = append(seen, r.Method+" "+r.URL.Path)
		status := 201
		body := `{"access_key":"ak1","project_id":"p1","secret_key":"synthetic-s3-secret-key"}`
		if r.Method == "POST" {
			var fields map[string]any
			_ = json.NewDecoder(r.Body).Decode(&fields)
			if len(fields) != 2 || fields["name"] != "synthetic" || fields["project_id"] != "p1" {
				t.Fatal("wrong credential request")
			}
		}
		if r.Method == "GET" {
			status = 200
			body = `{"credentials":[{"access_key":"ak1","project_id":"p1","name":"synthetic"}]}`
		}
		if r.Method == "DELETE" {
			status = 204
			body = ""
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	api := c.Account().S3Credentials().V1.ServiceUsers
	target, e := api.Target("su1")
	if e != nil {
		t.Fatal(e)
	}
	name, project := "synthetic", "p1"
	created, e := api.Create(context.Background(), target, iamcredentials.CreateRequest{Name: &name, ProjectID: &project}, types.CallOptions{})
	if e != nil || created.Receipt == nil || created.Receipt.Outcome != types.Accepted || created.Data.Ref == nil || created.Data.Ref.Parent == nil || created.Data.Ref.Parent.ID != "su1" || created.Data.SecretKey == nil || string(created.Data.SecretKey.Reveal()) != secret {
		t.Fatalf("create %+v %v", created, e)
	}
	raw, _ := json.Marshal(created)
	if strings.Contains(string(raw), secret) || strings.Contains(fmt.Sprintf("%+v", created), secret) {
		t.Fatal("credential secret disclosure")
	}
	listed, e := api.Collect(context.Background(), target, pagination.Options{})
	if e != nil || !listed.Complete || len(listed.Items) != 1 || listed.Items[0].Ref == nil {
		t.Fatalf("list %+v %v", listed, e)
	}
	deleted, e := api.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatalf("delete %+v %v", deleted, e)
	}
	if strings.Join(seen, "\n") != "POST /iam/v1/service_users/su1/credentials\nGET /iam/v1/service_users/su1/credentials\nDELETE /iam/v1/service_users/su1/credentials/ak1" {
		t.Fatalf("wire %v", seen)
	}
}

func TestUserAndCurrentS3CredentialContractsStayDistinct(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(fmt.Sprint(current), func(t *testing.T) {
			var paths []string
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				paths = append(paths, r.Method+" "+r.URL.Path)
				status := 200
				body := `{"access_key":"ak2","project_id":"p2"}`
				if r.Method == "POST" {
					status = 201
					body = `{"access_key":"ak2","project_id":"p2","secret_key":"synthetic-user-secret"}`
				}
				if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "credentials") {
					body = `{"credentials":[{"access_key":"ak2","project_id":"p2"}]}`
				}
				if r.Method == "DELETE" {
					status = 204
					body = ""
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			name, project := "synthetic", "p2"
			in := iamcredentials.CreateRequest{Name: &name, ProjectID: &project}
			var created types.Result[iamcredentials.CreatedCredential]
			var listed pagination.Collection[iamcredentials.Credential]
			var got types.Result[iamcredentials.Credential]
			var deleted types.Result[struct{}]
			id := "u2"
			if current {
				created, e = c.Account().S3Credentials().V1.Current.Create(context.Background(), in, types.CallOptions{})
				id = "me"
			} else {
				target, err := c.Account().S3Credentials().V1.Users.Target(id)
				if err != nil {
					t.Fatal(err)
				}
				created, e = c.Account().S3Credentials().V1.Users.Create(context.Background(), target, in, types.CallOptions{})
			}
			if e != nil || created.Data.Ref == nil {
				t.Fatalf("create %+v %v", created, e)
			}
			if current {
				listed, e = c.Account().S3Credentials().V1.Current.Collect(context.Background(), pagination.Options{})
			} else {
				listed, e = c.Account().S3Credentials().V1.Users.Collect(context.Background(), *created.Data.Ref.Parent, pagination.Options{})
			}
			if e != nil || !listed.Complete || len(listed.Items) != 1 {
				t.Fatalf("list %+v %v", listed, e)
			}
			if current {
				got, e = c.Account().S3Credentials().V1.Current.Get(context.Background(), *created.Data.Ref, types.CallOptions{})
			} else {
				got, e = c.Account().S3Credentials().V1.Users.Get(context.Background(), *created.Data.Ref, types.CallOptions{})
			}
			if e != nil || got.Data.Ref == nil {
				t.Fatalf("get %+v %v", got, e)
			}
			if current {
				deleted, e = c.Account().S3Credentials().V1.Current.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
			} else {
				deleted, e = c.Account().S3Credentials().V1.Users.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
			}
			if e != nil || deleted.Receipt.Outcome != types.Accepted {
				t.Fatalf("delete %+v %v", deleted, e)
			}
			base := "/iam/v1/users/" + id + "/credentials"
			if strings.Join(paths, "\n") != "POST "+base+"\nGET "+base+"\nGET "+base+"/ak2\nDELETE "+base+"/ak2" {
				t.Fatalf("wire %v", paths)
			}
		})
	}
}

func TestS3CredentialSearchFlattensAndBoundsProviderGroups(t *testing.T) {
	for _, service := range []bool{false, true} {
		t.Run(fmt.Sprint(service), func(t *testing.T) {
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				kind := "users"
				if service {
					kind = "service_users"
				}
				if r.Method != "GET" || r.URL.Path != "/iam/v1/"+kind+"/credentials" || r.URL.Query().Get("project_id") != "p1" || r.URL.Query().Get("user_id") != "id1" {
					t.Fatalf("search wire %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"credentials":[{"access_key":"ak1","project_id":"p1","user":{"id":"id1","name":"synthetic"}},{"access_key":"ak2","project_id":"p1","user":{"id":"id1"}}]}]`))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			id := "id1"
			filters := iamcredentials.SearchFilters{ProjectID: "p1", UserID: &id}
			var found pagination.Collection[iamcredentials.SearchedCredential]
			if service {
				found, e = c.Account().S3Credentials().V1.ServiceUsers.Search(context.Background(), filters, pagination.Options{MaxItems: 1})
			} else {
				found, e = c.Account().S3Credentials().V1.Users.Search(context.Background(), filters, pagination.Options{MaxItems: 1})
			}
			if types.ErrorCode(e) != types.Incomplete || found.Complete || len(found.Items) != 1 || found.NextCursor == nil || found.Items[0].Ref == nil || found.Items[0].Ref.Parent.ID != "id1" {
				t.Fatalf("search %+v %v", found, e)
			}
			var resumed pagination.Collection[iamcredentials.SearchedCredential]
			if service {
				resumed, e = c.Account().S3Credentials().V1.ServiceUsers.Search(context.Background(), filters, pagination.Options{Cursor: found.NextCursor})
			} else {
				resumed, e = c.Account().S3Credentials().V1.Users.Search(context.Background(), filters, pagination.Options{Cursor: found.NextCursor})
			}
			if e != nil || !resumed.Complete || len(resumed.Items) != 1 || *resumed.Items[0].AccessKey != "ak2" {
				t.Fatalf("resume %+v %v", resumed, e)
			}
		})
	}
}

func TestS3CredentialChangeRetainsIndependentStepOutcomes(t *testing.T) {
	var posts, deletes int
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method == "POST" {
			posts++
			return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_key":"new-key","project_id":"p1","secret_key":"synthetic-new-secret"}`))}, nil
		}
		if r.Method == "DELETE" {
			deletes++
			if !strings.HasSuffix(r.URL.Path, "/old-key") {
				t.Fatal("wrong key revoked")
			}
			return nil, io.ErrUnexpectedEOF
		}
		t.Fatal("unexpected automatic step")
		return nil, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	api := c.Account().S3Credentials().V1.ServiceUsers
	parent, e := api.Target("su1")
	if e != nil {
		t.Fatal(e)
	}
	name, project := "synthetic", "p1"
	created, e := api.Create(context.Background(), parent, iamcredentials.CreateRequest{Name: &name, ProjectID: &project}, types.CallOptions{})
	if e != nil || created.Data.Ref == nil {
		t.Fatalf("create %+v %v", created, e)
	}
	// The caller explicitly chooses the previously known old key. No helper
	// installs, adopts or revokes keys automatically after creation.
	old := *created.Data.Ref
	old.ID = "old-key"
	revoked, e := api.Delete(context.Background(), old, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || revoked.Receipt == nil || revoked.Receipt.Outcome != types.Unknown || len(revoked.Receipt.ResourceRefs) != 1 || revoked.Receipt.ResourceRefs[0].ID != "old-key" || created.Receipt.Outcome != types.Accepted || created.Data.Ref.ID != "new-key" || posts != 1 || deletes != 1 {
		t.Fatalf("step outcomes create=%+v revoke=%+v error=%v", created, revoked, e)
	}
}

func TestAcceptedS3CredentialRetainsKeyWhenSecretDecodeFails(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_key":"accepted-key","project_id":"p1","secret_key":42}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	api := c.Account().S3Credentials().V1.Users
	parent, e := api.Target("u1")
	if e != nil {
		t.Fatal(e)
	}
	name, project := "synthetic", "p1"
	out, e := api.Create(context.Background(), parent, iamcredentials.CreateRequest{Name: &name, ProjectID: &project}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[1].ID != "accepted-key" || out.Receipt.ResourceRefs[1].Parent == nil || out.Receipt.ResourceRefs[1].Parent.ID != "u1" {
		t.Fatalf("accepted key lost %+v %v", out, e)
	}
}

func TestS3CredentialReferencesRejectForeignParentAndCurrentLane(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })})
	if e != nil {
		t.Fatal(e)
	}
	parent, e := c.Account().S3Credentials().V1.Users.Target("u1")
	if e != nil {
		t.Fatal(e)
	}
	ref := parent
	ref.Kind = "s3_user_credential"
	ref.ID = "ak1"
	ref.Parent = &parent
	current, e := c.Account().S3Credentials().V1.Current.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || current.Receipt == nil || current.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("current adopted user reference %+v %v", current, e)
	}
	parent.EndpointURL = "https://foreign.example.invalid"
	parent.EndpointID = "foreign"
	deleted, e := c.Account().S3Credentials().V1.Users.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || deleted.Receipt == nil || deleted.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("foreign parent %+v %v", deleted, e)
	}
}

func TestS3CredentialCreateUsesAccountAuthAndRejectsForeignProjectResponse(t *testing.T) {
	var authRequests []types.AuthRequest
	creds := credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
		authRequests = append(authRequests, r)
		return source().Lookup(ctx, r)
	})
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: creds, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_key":"foreign-key","project_id":"foreign-project","secret_key":"synthetic-foreign-secret"}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	api := c.Account().S3Credentials().V1.Users
	target, e := api.Target("u1")
	if e != nil || len(authRequests) != 0 {
		t.Fatal("target construction used credentials")
	}
	name, project := "synthetic", "p1"
	out, e := api.Create(context.Background(), target, iamcredentials.CreateRequest{Name: &name, ProjectID: &project}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || len(authRequests) != 1 || authRequests[0].Mode != types.IAMAccount || authRequests[0].Scope.ProjectID != "" {
		t.Fatalf("project/account semantics %+v %v", out, e)
	}
}

func TestS3CredentialCatalogRetainsCanonicalIAMInventoryIDs(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if e != nil {
		t.Fatal(e)
	}
	ops, e := c.Catalog().List(catalog.Filter{Service: "iam"})
	if e != nil || len(ops) != 47 {
		t.Fatalf("IAM source inventory %d %v", len(ops), e)
	}
	expected := map[string]bool{"iam_service_users_credentials.create": true, "iam_service_users_credentials.delete": true, "iam_service_users_credentials.list": true, "iam_service_users_credentials.list_c90ef3c3": true, "iam_users_credentials.create": true, "iam_users_credentials.delete": true, "iam_users_credentials.get": true, "iam_users_credentials.list": true, "iam_users_credentials.list_2138ba42": true, "iam_users_me_credentials.create": true, "iam_users_me_credentials.delete": true, "iam_users_me_credentials.get": true, "iam_users_me_credentials.list": true}
	covered := 0
	for _, op := range ops {
		suffix := strings.TrimPrefix(op.OperationID, "iam.v1.")
		if expected[suffix] {
			covered++
			if op.Implementation.Status != "implemented" || op.GoMethod == nil || op.Auth.Scheme != types.IAMAccount || op.APIGeneration != "v1" || len(op.ScopeRequirements) != 1 || op.ScopeRequirements[0] != "account" || op.RequestBindingRef == nil || op.ResponseBindingRef == nil || op.ErrorBindingRef == nil {
				t.Fatalf("credential binding %+v", op)
			}
		} else if op.Implementation.Status != "unimplemented" {
			t.Fatalf("unrelated IAM coverage overclaimed %+v", op)
		}
	}
	if covered != 13 || c.Catalog().Summary().InventoryComplete || c.Catalog().Summary().CoverageRatio != nil {
		t.Fatalf("coverage %d", covered)
	}
}

func TestNamedS3CredentialUserTargetCannotSelectCurrentAlias(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("relative alias caused I/O"); return nil, nil })})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Account().S3Credentials().V1.Users.Target("me"); types.ErrorCode(e) != types.InvalidArgument {
		t.Fatalf("named target accepted current alias: %v", e)
	}
}

func TestS3CredentialSearchDoesNotAdoptRelativeCurrentAlias(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"credentials":[{"access_key":"ak1","project_id":"p1","user":{"id":"me"}}]}]`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Account().S3Credentials().V1.Users.Search(context.Background(), iamcredentials.SearchFilters{ProjectID: "p1"}, pagination.Options{})
	if types.ErrorCode(e) != types.ProtocolError || out.Complete {
		t.Fatalf("relative alias adopted %+v %v", out, e)
	}
}

func TestCredentialInventoryKeepsReviewedIAMUserDiscoveryID(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if e != nil {
		t.Fatal(e)
	}
	op, e := c.Catalog().Describe("iam.v1.users.list")
	if e != nil || op.Implementation.Status != "unimplemented" || op.GoMethod != nil {
		t.Fatalf("IAM canonical discovery changed %+v %v", op, e)
	}
}
