package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/mobilefarm"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMobileFarmAvailableDevicesUsesRepeatedFieldsAndBoundedRootArray(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/mobfarm/api/v3/devices/available" || r.Method != "GET" || strings.Join(r.URL.Query()["fields"], ",") != "manufacturer,marketName" || r.Header.Get("X-Auth-Token") == "" {
			t.Fatalf("available wire %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"manufacturer":"synthetic-maker","marketName":"synthetic-one","count":2},{"manufacturer":"synthetic-maker","marketName":"synthetic-two","count":1}]`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	out, e := p.MobileFarm().AvailableDevices(context.Background(), []string{"manufacturer", "marketName"}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || out.Complete || len(out.Items) != 1 || *out.Items[0].MarketName != "synthetic-one" || calls != 1 {
		t.Fatalf("available %+v %v", out, e)
	}
}

func TestMobileFarmGetDevicePreservesExactRefAndProtectsAccessAndOwner(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/mobfarm/api/v3/devices/serial-1" || r.Method != "GET" {
			t.Fatalf("device wire %s", r.URL)
		}
		payload := `{"serial":"serial-1","status":0,"present":true,"ready":true,"remoteConnectUrl":"synthetic-private-adb-url","owner":{"name":"synthetic-private-owner"},"phone":{"phoneNumber":"synthetic-private-phone"},"reverseForwards":["synthetic-private-forward"],"battery":{"level":42,"voltage":3.1415926535897932384626}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project"}, Service: "mobile_farm", APIGeneration: "v3", Kind: "device", ID: "serial-1", EndpointID: "selectel-mobile-farm:v3", EndpointURL: "https://api.selectel.ru/mobfarm"}
	out, e := p.MobileFarm().GetDevice(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.Ref == nil || *out.Data.Ref != ref || string(out.Data.RemoteConnectURL.Reveal()) != "synthetic-private-adb-url" {
		t.Fatalf("device %+v %v", out, e)
	}
	safe, _ := json.Marshal(out.Data)
	for _, secret := range []string{"synthetic-private-adb-url", "synthetic-private-owner", "synthetic-private-phone", "synthetic-private-forward", "3.1415926535897932384626"} {
		if strings.Contains(string(safe), secret) || strings.Contains(fmt.Sprintf("%+v %#v", out.Data, out.Data), secret) {
			t.Fatal("device output leaked")
		}
	}
}

func TestMobileFarmListDevicesKeepsNativeSmallStatusShapeAndExactScopedSerial(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/mobfarm/api/v3/devices" || r.Method != "GET" {
			t.Fatalf("listdevices %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"serial":"serial-1","status":{"ACTIVE":0,"IN_USE":1,"BUSY":2,"MAINTENANCE":3,"ERROR":4},"owner":{"name":"synthetic-private-owner"}},{"serial":"serial-2"}]`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	out, e := p.MobileFarm().ListDevices(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0].Ref.ID != "serial-1" || out.Items[0].Ref.Scope.ProjectID != "test-project" || out.Items[0].Ref.Parent != nil || out.Items[0].Status.ACTIVE.String() != "0" {
		t.Fatalf("device inventory %+v %v", out, e)
	}
}

func TestMobileFarmADBListsUseExplicitAccountOrProjectView(t *testing.T) {
	for _, project := range []string{"", "test-project"} {
		t.Run("project="+project, func(t *testing.T) {
			var requested types.AuthRequest
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
				requested = r
				return source().Lookup(ctx, r)
			}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/identity/v3/auth/tokens" {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				if r.URL.Path != "/mobfarm/api/v3/keys/adb" || r.Method != "GET" {
					t.Fatalf("ADB list %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"fingerprint":"fingerprint-1","title":"synthetic-key"}]`))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			scoped := c.Account().MobileFarm()
			expected := types.IAMAccount
			if project != "" {
				p, _ := c.Project(project)
				scoped = p.MobileFarm()
				expected = types.IAMProject
			}
			out, e := scoped.ListADBKeys(context.Background(), pagination.Options{})
			if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref.ID != "fingerprint-1" || out.Items[0].Ref.Scope.ProjectID != project || requested.Mode != expected {
				t.Fatalf("ADB list %+v %v auth%+v", out, e, requested)
			}
		})
	}
}

func TestMobileFarmStoreADBKeyRetainsAcceptedFingerprintBeforeUnrelatedDecode(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/mobfarm/api/v3/keys/adb" || r.Method != "POST" || !strings.Contains(string(raw), `"publicKey":"synthetic-public-key"`) {
			t.Fatalf("store ADB wire %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"fingerprint":"fingerprint-1","title":42}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	var input mobilefarm.CreateADBKeyRequest
	_ = json.Unmarshal([]byte(`{"publicKey":"synthetic-public-key","title":"synthetic-title"}`), &input)
	safe, _ := json.Marshal(input)
	if strings.Contains(string(safe), "synthetic-public-key") {
		t.Fatal("request public key leaked")
	}
	out, e := c.Account().MobileFarm().StoreADBKey(context.Background(), input, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "fingerprint-1" || out.Receipt.ResourceRefs[0].Scope.ProjectID != "" {
		t.Fatalf("accepted ADB %+v %v", out, e)
	}
}

func TestMobileFarmADBDeleteValidatesEndpointBeforeCredentialsAndRetainsReceipt(t *testing.T) {
	lookups, calls := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
		lookups++
		return source().Lookup(ctx, r)
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.Method != "DELETE" || r.URL.Path != "/mobfarm/api/v3/keys/adb/fingerprint-1" {
			t.Fatalf("delete ADB %s", r.URL)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "mobile_farm", APIGeneration: "v3", Kind: "adb_key", ID: "fingerprint-1", EndpointID: "foreign-endpoint", EndpointURL: "https://api.selectel.ru/mobfarm"}
	out, e := c.Account().MobileFarm().DeleteADBKey(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || lookups != 0 || calls != 0 {
		t.Fatalf("ADB delete gate %+v %v", out, e)
	}
	ref.EndpointID = "selectel-mobile-farm:v3"
	out, e = c.Account().MobileFarm().DeleteADBKey(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "fingerprint-1" || calls != 1 {
		t.Fatalf("ADB delete %+v %v", out, e)
	}
}

func TestMobileFarmUserADBListRequiresAccountScopeAndRetainsCallerUserParent(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			if requestedScope(r).ProjectID != "" {
				t.Fatal("user admin fetched project token")
			}
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.URL.Path != "/mobfarm/api/v3/users/user-1/keys/adb" || r.Method != "GET" {
			t.Fatalf("user ADB path %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"fingerprint":"fingerprint-1","title":"synthetic-user-key"}]`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	user := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "mobile_farm", APIGeneration: "v3", Kind: "user", ID: "user-1", EndpointID: "selectel-mobile-farm:v3", EndpointURL: "https://api.selectel.ru/mobfarm"}
	p, _ := c.Project("test-project")
	if _, e := p.MobileFarm().ListUserADBKeys(context.Background(), user, pagination.Options{}); types.ErrorCode(e) != types.ScopeMismatch || calls != 0 {
		t.Fatalf("user scope gate %v", e)
	}
	out, e := c.Account().MobileFarm().ListUserADBKeys(context.Background(), user, pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref.Parent == nil || *out.Items[0].Ref.Parent != user {
		t.Fatalf("user ADB %+v %v", out, e)
	}
}

func TestMobileFarmUserADBStoreUnknownNeverReplaysOrInventsFingerprint(t *testing.T) {
	writes := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		writes++
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/mobfarm/api/v3/users/user-1/keys/adb" || r.Method != "POST" || !strings.Contains(string(raw), `"publicKey":"synthetic-user-public-key"`) {
			t.Fatalf("user ADB store %s", r.URL)
		}
		return nil, io.ErrUnexpectedEOF
	})})
	if e != nil {
		t.Fatal(e)
	}
	user := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "mobile_farm", APIGeneration: "v3", Kind: "user", ID: "user-1", EndpointID: "selectel-mobile-farm:v3", EndpointURL: "https://api.selectel.ru/mobfarm"}
	var input mobilefarm.CreateADBKeyRequest
	_ = json.Unmarshal([]byte(`{"publicKey":"synthetic-user-public-key"}`), &input)
	out, e := c.Account().MobileFarm().StoreUserADBKey(context.Background(), user, input, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != user || writes != 1 || out.Receipt.Recovery != "unavailable" {
		t.Fatalf("unknown user ADB %+v %v writes%d", out, e, writes)
	}
}

func TestMobileFarmUserADBDeleteRejectsForeignParentAndUsesExactNestedPath(t *testing.T) {
	lookups, calls := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
		lookups++
		return source().Lookup(ctx, r)
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.URL.Path != "/mobfarm/api/v3/users/user-1/keys/adb/fingerprint-1" || r.Method != "DELETE" {
			t.Fatalf("nested ADB delete %s", r.URL)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	user := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "foreign", APIGeneration: "v3", Kind: "user", ID: "user-1", EndpointID: "selectel-mobile-farm:v3", EndpointURL: "https://api.selectel.ru/mobfarm"}
	ref := user
	ref.Service = "mobile_farm"
	ref.Kind = "adb_key"
	ref.ID = "fingerprint-1"
	ref.Parent = &user
	out, e := c.Account().MobileFarm().DeleteUserADBKey(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || lookups != 0 || calls != 0 {
		t.Fatalf("parent gate %+v %v", out, e)
	}
	user.Service = "mobile_farm"
	out, e = c.Account().MobileFarm().DeleteUserADBKey(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].Parent.ID != "user-1" || calls != 1 {
		t.Fatalf("nested deletion %+v %v", out, e)
	}
}

func TestMobileFarmCatalogAuthAlternativesAreExplicitAndImmutable(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if e != nil {
		t.Fatal(e)
	}
	cap, e := c.Catalog().Describe("mobile_farm.v3.api_keys_adb.list")
	if e != nil || cap.Auth.Scheme != types.IAMAccount || len(cap.Auth.Alternatives) != 1 || cap.Auth.Alternatives[0] != types.IAMProject || len(cap.ScopeRequirements) != 1 || cap.ScopeRequirements[0] != "account" {
		t.Fatalf("auth capability %+v %v", cap, e)
	}
	cap.Auth.Alternatives[0] = types.Basic
	fresh, e := c.Catalog().Describe("mobile_farm.v3.api_keys_adb.list")
	if e != nil || fresh.Auth.Alternatives[0] != types.IAMProject {
		t.Fatal("caller mutated shared auth catalog")
	}
}

func TestMobileFarmADBInvalidSourceLengthsReturnNotSentBeforeCredentialIO(t *testing.T) {
	lookups, calls := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		lookups++
		return credentials.Material{}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if e != nil {
		t.Fatal(e)
	}
	for _, payload := range []string{`{"publicKey":""}`, `{"publicKey":"synthetic-key","title":""}`, `{"publicKey":"synthetic-key","title":"` + strings.Repeat("x", 256) + `"}`} {
		var input mobilefarm.CreateADBKeyRequest
		_ = json.Unmarshal([]byte(payload), &input)
		out, e := c.Account().MobileFarm().StoreADBKey(context.Background(), input, types.CallOptions{})
		if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || lookups != 0 || calls != 0 {
			t.Fatalf("source length gate %+v %v", out, e)
		}
	}
}

func TestMobileFarmProxyListExposesMetadataButNeverServerSecret(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/mobfarm/api/v3/keys/proxy" || r.Method != "GET" {
			t.Fatalf("proxy list %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"fingerprint":"proxy-1","name":"synthetic-token","active":false,"serials":[],"token":"synthetic-unexpected-secret"}]`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Account().MobileFarm().ListProxyTokens(context.Background(), pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref.ID != "proxy-1" || out.Items[0].Active == nil || *out.Items[0].Active || out.Items[0].Serials == nil {
		t.Fatalf("proxy inventory %+v %v", out, e)
	}
	safe, _ := json.Marshal(out)
	if strings.Contains(string(safe), "synthetic-unexpected-secret") {
		t.Fatal("unexpected server token escaped metadata DTO")
	}
}

func TestMobileFarmProxyCreationSecretOnlyCrossesExplicitRevealBoundary(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/mobfarm/api/v3/keys/proxy" || r.Method != "POST" || string(raw) != `{"name":"synthetic-token"}` {
			t.Fatalf("create proxy %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"fingerprint":"proxy-1","name":"synthetic-token","active":true,"token":"synthetic-created-secret"}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	var input mobilefarm.CreateProxyTokenRequest
	_ = json.Unmarshal([]byte(`{"name":"synthetic-token"}`), &input)
	out, e := c.Account().MobileFarm().CreateProxyToken(context.Background(), input, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "proxy-1" || string(out.Data.Token.Reveal()) != "synthetic-created-secret" {
		t.Fatalf("proxy create %+v %v", out, e)
	}
	safe, _ := json.Marshal(out)
	formatted := fmt.Sprintf("%v %+v %#v %x", out.Data.Token, out.Data.Token, out.Data.Token, out.Data.Token)
	for _, encoding := range []string{"synthetic-created-secret", fmt.Sprint([]byte("synthetic-created-secret")), fmt.Sprintf("%#v", []byte("synthetic-created-secret"))} {
		if strings.Contains(string(safe)+formatted, encoding) {
			t.Fatal("proxy secret escaped diagnostics")
		}
	}
}

func TestMobileFarmProxyDeleteRejectsForeignFingerprintAfterAcceptance(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/mobfarm/api/v3/keys/proxy/proxy-1" || r.Method != "DELETE" {
			t.Fatalf("proxy deletion %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"fingerprint":"foreign-proxy","name":"synthetic-token","active":false,"serials":[]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "mobile_farm", APIGeneration: "v3", Kind: "proxy_token", ID: "proxy-1", EndpointID: "selectel-mobile-farm:v3", EndpointURL: "https://api.selectel.ru/mobfarm"}
	out, e := c.Account().MobileFarm().DeleteProxyToken(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("proxy delete mismatch %+v %v", out, e)
	}
}

func mobileFarmTestClient(t *testing.T, wire transportFunc) *sdk.Client {
	t.Helper()
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func mobileFarmUserRef() types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "mobile_farm", APIGeneration: "v3", Kind: "user", ID: "user-1", EndpointID: "selectel-mobile-farm:v3", EndpointURL: "https://api.selectel.ru/mobfarm"}
}

func TestMobileFarmUserProxyListIsAccountScopedAndParentBound(t *testing.T) {
	calls := 0
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/mobfarm/api/v3/users/user-1/keys/proxy" {
			t.Fatalf("userproxylist %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"fingerprint":"proxy-1","name":"synthetic-token","active":true,"serials":[]}]`))}, nil
	}))
	user := mobileFarmUserRef()
	p, _ := c.Project("test-project")
	if _, e := p.MobileFarm().ListUserProxyTokens(context.Background(), user, pagination.Options{}); types.ErrorCode(e) != types.ScopeMismatch || calls != 0 {
		t.Fatalf("userproxy gate %v", e)
	}
	out, e := c.Account().MobileFarm().ListUserProxyTokens(context.Background(), user, pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref.Parent == nil || *out.Items[0].Ref.Parent != user {
		t.Fatalf("user proxy list %+v %v", out, e)
	}
}

func TestMobileFarmUserProxyCreateRetainsIdentityBeforeMalformedFields(t *testing.T) {
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/mobfarm/api/v3/users/user-1/keys/proxy" {
			t.Fatalf("create %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"fingerprint":"proxy-1","name":false,"token":"synthetic-secret"}`))}, nil
	}))
	var input mobilefarm.CreateProxyTokenRequest
	_ = json.Unmarshal([]byte(`{"name":"synthetic-token"}`), &input)
	user := mobileFarmUserRef()
	out, e := c.Account().MobileFarm().CreateUserProxyToken(context.Background(), user, input, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 {
		t.Fatalf("create %+v %v", out, e)
	}
	child := out.Receipt.ResourceRefs[1]
	if child.ID != "proxy-1" || child.Parent == nil || *child.Parent != user {
		t.Fatalf("identity %+v", child)
	}
}

func TestMobileFarmUserProxyDeletePreservesExplicitParent(t *testing.T) {
	calls := 0
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "DELETE" || r.URL.Path != "/mobfarm/api/v3/users/user-1/keys/proxy/proxy-1" {
			t.Fatalf("delete %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"fingerprint":"proxy-1","name":"synthetic-token","active":false,"serials":[]}`))}, nil
	}))
	user := mobileFarmUserRef()
	ref := user
	ref.Kind = "proxy_token"
	ref.ID = "proxy-1"
	ref.Parent = &user
	bad := ref
	foreign := user
	foreign.Service = "unrelated"
	bad.Parent = &foreign
	out, e := c.Account().MobileFarm().DeleteUserProxyToken(context.Background(), bad, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("gate %+v %v", out, e)
	}
	out, e = c.Account().MobileFarm().DeleteUserProxyToken(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Data.Ref == nil || out.Data.Ref.Parent == nil || *out.Data.Ref.Parent != user {
		t.Fatalf("delete %+v %v", out, e)
	}
}

func TestMobileFarmTicketProtectsSingleDeliveryValueWithoutResourceIdentity(t *testing.T) {
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/mobfarm/api/v2/auth/tickets" {
			t.Fatalf("ticket %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"success":true,"description":"synthetic-private-description","ticket":"synthetic-private-ticket"}`))}, nil
	}))
	p, _ := c.Project("test-project")
	out, e := p.MobileFarm().IssueTicket(context.Background(), types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 0 || string(out.Data.Ticket.Reveal()) != "synthetic-private-ticket" {
		t.Fatalf("ticket %+v %v", out, e)
	}
	safe, _ := json.Marshal(out)
	diagnostics := string(safe) + fmt.Sprintf("%+v %#v %x", out.Data, out.Data, out.Data.Ticket)
	for _, secret := range []string{"synthetic-private-ticket", "synthetic-private-description"} {
		if strings.Contains(diagnostics, secret) || strings.Contains(diagnostics, fmt.Sprint([]byte(secret))) || strings.Contains(diagnostics, fmt.Sprintf("%#v", []byte(secret))) {
			t.Fatal("ticket leaked")
		}
	}
}

func TestMobileFarmTicketUnknownNeverReplaysAndMissingProjectHasReceipt(t *testing.T) {
	calls := 0
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF }))
	out, e := c.Account().MobileFarm().IssueTicket(context.Background(), types.CallOptions{})
	if e == nil || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("gate %+v %v", out, e)
	}
	p, _ := c.Project("test-project")
	out, e = p.MobileFarm().IssueTicket(context.Background(), types.CallOptions{})
	if e == nil || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || len(out.Receipt.ResourceRefs) != 0 || out.Receipt.Recovery != "unavailable" {
		t.Fatalf("unknown %+v %v calls%d", out, e, calls)
	}
}

func TestMobileFarmFirewallListHasExactProjectRefsAndBoundedCollection(t *testing.T) {
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/mobfarm/api/v3/firewall-rules" {
			t.Fatalf("rules %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"id":"rule-1","action":"allow","address":"*","ports":"443"},{"id":"rule-2","action":"deny","address":"*","ports":"*"}]`))}, nil
	}))
	p, _ := c.Project("test-project")
	out, e := p.MobileFarm().ListFirewallRules(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.ID != "rule-1" || out.Items[0].Ref.Scope.ProjectID != "test-project" || out.Items[0].Ref.Parent != nil {
		t.Fatalf("rules %+v %v", out, e)
	}
}

func TestMobileFarmFirewallCreateValidatesRequiredFieldsAndRetainsAcceptedID(t *testing.T) {
	calls := 0
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/mobfarm/api/v3/firewall-rules" || !strings.Contains(string(body), `"address":"*"`) {
			t.Fatalf("create %s %s", r.URL, body)
		}
		return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"rule-1","action":"invalid","address":"*","ports":"443"}`))}, nil
	}))
	p, _ := c.Project("test-project")
	out, e := p.MobileFarm().CreateFirewallRule(context.Background(), mobilefarm.FirewallRuleRequest{}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("gate %+v %v", out, e)
	}
	var input mobilefarm.FirewallRuleRequest
	_ = json.Unmarshal([]byte(`{"action":"allow","address":"*","ports":"443"}`), &input)
	out, e = p.MobileFarm().CreateFirewallRule(context.Background(), input, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "rule-1" {
		t.Fatalf("accepted %+v %v", out, e)
	}
}

func TestMobileFarmFirewallUpdateUsesPutAndRejectsForeignAcceptedIdentity(t *testing.T) {
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || r.URL.Path != "/mobfarm/api/v3/firewall-rules/rule-1" || string(body) != `{"action":"deny"}` {
			t.Fatalf("update %s %s %s", r.Method, r.URL, body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"foreign","action":"deny","address":"*","ports":"*"}`))}, nil
	}))
	p, _ := c.Project("test-project")
	ref := mobileFarmUserRef()
	ref.Scope.ProjectID = "test-project"
	ref.Kind = "firewall_rule"
	ref.ID = "rule-1"
	var input mobilefarm.FirewallRuleRequest
	_ = json.Unmarshal([]byte(`{"action":"deny"}`), &input)
	out, e := p.MobileFarm().UpdateFirewallRule(context.Background(), ref, input, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("update %+v %v", out, e)
	}
}

func TestMobileFarmFirewallDeletePreservesAcceptanceButReportsUnknownResponseContract(t *testing.T) {
	calls := 0
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "DELETE" || r.URL.Path != "/mobfarm/api/v3/firewall-rules/rule-1" {
			t.Fatalf("delete %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"unknown":"synthetic-private-result"}`))}, nil
	}))
	p, _ := c.Project("test-project")
	ref := mobileFarmUserRef()
	ref.Scope.ProjectID = "test-project"
	ref.Kind = "firewall_rule"
	ref.ID = "rule-1"
	out, e := p.MobileFarm().DeleteFirewallRule(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.Incomplete || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || out.Data.HTTPStatus != 200 || string(out.Data.Body.Reveal()) != `{"unknown":"synthetic-private-result"}` {
		t.Fatalf("delete %+v %v", out, e)
	}
	safe, _ := json.Marshal(out)
	if strings.Contains(string(safe)+fmt.Sprintf("%+v %#v", out, out), "synthetic-private-result") {
		t.Fatal("unknown response leaked")
	}
}

func TestMobileFarmFirewallOrderPreservesCallerOrderAndAllTrustworthyAcceptedIDs(t *testing.T) {
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "PATCH" || r.URL.Path != "/mobfarm/api/v3/firewall-rules/order" || string(body) != `{"ids":["rule-2","rule-1"]}` {
			t.Fatalf("order %s %s", r.URL, body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"id":"rule-2","action":false},{"id":"rule-1","action":"allow","address":"*","ports":"443"}]`))}, nil
	}))
	p, _ := c.Project("test-project")
	a := mobileFarmUserRef()
	a.Scope.ProjectID = "test-project"
	a.Kind = "firewall_rule"
	a.ID = "rule-2"
	b := a
	b.ID = "rule-1"
	out, e := p.MobileFarm().ReorderFirewallRules(context.Background(), []types.ResourceRef{a, b}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[0].ID != "rule-2" || out.Receipt.ResourceRefs[1].ID != "rule-1" {
		t.Fatalf("order %+v %v", out, e)
	}
}

func TestMobileFarmAssignDevicePreservesUnknownResponseWithoutInventedAssociation(t *testing.T) {
	calls := 0
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/mobfarm/api/v3/users/devices" || string(body) != `{"serial":"serial-1","timeout":32400000}` {
			t.Fatalf("assign %s %s", r.URL, body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"opaque":"synthetic-private-assignment"}`))}, nil
	}))
	p, _ := c.Project("test-project")
	ref := mobileFarmUserRef()
	ref.Scope.ProjectID = "test-project"
	ref.Kind = "device"
	ref.ID = "serial-1"
	var input mobilefarm.AddUserDeviceRequest
	_ = json.Unmarshal([]byte(`{"serial":"serial-1","timeout":32400000}`), &input)
	out, e := p.MobileFarm().AssignDevice(context.Background(), ref, input, types.CallOptions{})
	if types.ErrorCode(e) != types.Incomplete || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || out.Data.HTTPStatus != 200 {
		t.Fatalf("assign %+v %v", out, e)
	}
	safe, _ := json.Marshal(out)
	if strings.Contains(string(safe)+fmt.Sprintf("%+v %#v", out, out), "synthetic-private-assignment") {
		t.Fatal("assignment leaked")
	}
}

func TestMobileFarmReleaseDevicePreservesAcceptedPartialAndUnknownNeverReplays(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			calls := 0
			c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "DELETE" || r.URL.Path != "/mobfarm/api/v3/users/devices/serial-1" {
					t.Fatalf("release %s", r.URL)
				}
				if !accepted {
					return nil, io.ErrUnexpectedEOF
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"unknown":true}`))}, nil
			}))
			p, _ := c.Project("test-project")
			ref := mobileFarmUserRef()
			ref.Scope.ProjectID = "test-project"
			ref.Kind = "device"
			ref.ID = "serial-1"
			out, e := p.MobileFarm().ReleaseDevice(context.Background(), ref, types.CallOptions{})
			outcome := types.Unknown
			if accepted {
				outcome = types.Accepted
			}
			if e == nil || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != outcome || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || out.Receipt.Recovery != "unavailable" {
				t.Fatalf("release %+v %v", out, e)
			}
			if accepted && types.ErrorCode(e) != types.Incomplete {
				t.Fatalf("accepted error %v", e)
			}
		})
	}
}

func TestMobileFarmStopRemoteConnectIsAcceptanceOnlyAndNeverOpensReturnedLocation(t *testing.T) {
	calls := 0
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "DELETE" || r.URL.Path != "/mobfarm/api/v3/users/devices/serial-1/remote-connect" {
			t.Fatalf("stop %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Location": []string{"https://unconfirmed.invalid/synthetic-private"}}, Body: io.NopCloser(strings.NewReader(`{"remoteConnectUrl":"synthetic-private-url"}`))}, nil
	}))
	p, _ := c.Project("test-project")
	ref := mobileFarmUserRef()
	ref.Scope.ProjectID = "test-project"
	ref.Kind = "device"
	ref.ID = "serial-1"
	out, e := p.MobileFarm().StopRemoteConnect(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.Incomplete || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("stop %+v %v", out, e)
	}
}

func TestMobileFarmStartRemoteConnectReturnsProtectedURLWithoutConnecting(t *testing.T) {
	calls := 0
	c := mobileFarmTestClient(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/mobfarm/api/v3/users/devices/serial-1/remote-connect" {
			t.Fatalf("start %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"remoteConnectUrl":"synthetic-private-adb-url"}`))}, nil
	}))
	p, _ := c.Project("test-project")
	ref := mobileFarmUserRef()
	ref.Scope.ProjectID = "test-project"
	ref.Kind = "device"
	ref.ID = "serial-1"
	out, e := p.MobileFarm().StartRemoteConnect(context.Background(), ref, types.CallOptions{})
	if e != nil || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || out.Data.RemoteConnectURL == nil || string(out.Data.RemoteConnectURL.Reveal()) != "synthetic-private-adb-url" {
		t.Fatalf("start %+v %v", out, e)
	}
	safe, _ := json.Marshal(out)
	diagnostics := string(safe) + fmt.Sprintf("%+v %#v %x", out, out, out.Data.RemoteConnectURL)
	for _, v := range []string{"synthetic-private-adb-url", fmt.Sprint([]byte("synthetic-private-adb-url")), fmt.Sprintf("%#v", []byte("synthetic-private-adb-url"))} {
		if strings.Contains(diagnostics, v) {
			t.Fatal("remote URL leaked")
		}
	}
}

func TestMobileFarmDeviceAccessMutationsValidateExactRefBeforeCredentials(t *testing.T) {
	credentialCalls, wireCalls := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		credentialCalls++
		return credentials.Material{}, io.ErrUnexpectedEOF
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { wireCalls++; return nil, io.ErrUnexpectedEOF })})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	ref := mobileFarmUserRef()
	ref.Scope.ProjectID = "test-project"
	ref.Kind = "device"
	ref.ID = "serial-1"
	ref.EndpointURL = "https://unconfirmed.invalid"
	var input mobilefarm.AddUserDeviceRequest
	_ = json.Unmarshal([]byte(`{"serial":"serial-1"}`), &input)
	calls := []struct {
		name string
		call func() (*types.Receipt, error)
	}{
		{"assign", func() (*types.Receipt, error) {
			out, e := p.MobileFarm().AssignDevice(context.Background(), ref, input, types.CallOptions{})
			return out.Receipt, e
		}},
		{"release", func() (*types.Receipt, error) {
			out, e := p.MobileFarm().ReleaseDevice(context.Background(), ref, types.CallOptions{})
			return out.Receipt, e
		}},
		{"start", func() (*types.Receipt, error) {
			out, e := p.MobileFarm().StartRemoteConnect(context.Background(), ref, types.CallOptions{})
			return out.Receipt, e
		}},
		{"stop", func() (*types.Receipt, error) {
			out, e := p.MobileFarm().StopRemoteConnect(context.Background(), ref, types.CallOptions{})
			return out.Receipt, e
		}},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			receipt, e := call.call()
			if types.ErrorCode(e) != types.EndpointUnavailable || receipt == nil || receipt.Outcome != types.NotSent || receipt.ContractRevision == "" || credentialCalls != 0 || wireCalls != 0 {
				t.Fatalf("gate %+v %v creds%d wire%d", receipt, e, credentialCalls, wireCalls)
			}
		})
	}
}
