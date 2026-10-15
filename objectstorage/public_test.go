package objectstorage_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/objectstorage"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProjectStorageQuotaPreservesExplicitNullAndExactInteger(t *testing.T) {
	calls := 0
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "spb", Pool: "ru-1"}
	root, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Service != "object_storage" || r.Mode != types.IAMProject || r.Scope != scope {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "s", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			if r.Method != "GET" || r.Header.Get("X-Subject-Token") != "synthetic-token" {
				t.Fatal("token validation")
			}
			raw, _ := json.Marshal(map[string]any{"token": map[string]any{"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "user": map[string]string{"id": "subject"}, "catalog": []any{}, "project": map[string]any{"id": "p", "domain": map[string]string{"name": "a"}}}})
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Subject-Token": []string{"synthetic-token"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		}
		calls++
		if r.URL.String() != "https://api.ru-1.storage.selcloud.ru/v2/account/quotas" || r.Method != "PUT" || r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal(r.Method, r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"quota_bytes":null}` {
			t.Fatal(string(b))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"quota_bytes":9007199254740993}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := root.Project("p")
	r, _ := p.Location("spb", "ru-1")
	out, e := r.ObjectStorage().Account.SetQuota(context.Background(), objectstorage.SetQuotaOptions{QuotaBytes: types.Null[types.Integer]()}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted {
		t.Fatal(calls, out.Receipt)
	}
	wire, _ := json.Marshal(out.Data)
	if string(wire) != `{"quota_bytes":9007199254740993}` {
		t.Fatal(string(wire))
	}
}

func TestStorageAccountReadsStayProjectBoundAndKeepNullQuota(t *testing.T) {
	_, c := client(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal(r.Method)
		}
		body := `{"quota_bytes":null}`
		if r.URL.Path == "/v2/account/info" {
			body = `{"bytes_used":9007199254740993,"containers_count":2,"object_count":3,"quota_bytes":null,"status":"active","storage_policies_stats":{"policy-0":{"bytes_used":9007199254740993,"containers_count":1,"object_count":3}}}`
		} else if r.URL.Path != "/v2/account/quotas" {
			t.Fatal(r.URL)
		}
		return response(200, body), nil
	})
	quota, e := c.Account.GetQuota(context.Background(), types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if quota.Data.QuotaBytes == nil {
		t.Fatal("explicit null lost")
	}
	if _, value := quota.Data.QuotaBytes.Value(); value {
		t.Fatal("null turned into value")
	}
	info, e := c.Account.Info(context.Background(), types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if info.Data.BytesUsed == nil || info.Data.BytesUsed.String() != "9007199254740993" || info.Data.QuotaBytes == nil {
		t.Fatal("stats/null mapping")
	}
}
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func client(t *testing.T, wire transport) (*sdk.Client, *objectstorage.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "spb", Pool: "ru-1"}
	root, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Scope != scope || r.Service != "object_storage" || r.Mode != types.IAMProject {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			if r.Method != "GET" {
				t.Fatal(r.Method)
			}
			raw, _ := json.Marshal(map[string]any{"token": map[string]any{"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "user": map[string]string{"id": "subject"}, "catalog": []any{}, "project": map[string]any{"id": "p", "domain": map[string]string{"name": "a"}}}})
			v := response(200, string(raw))
			v.Header.Set("X-Subject-Token", "synthetic-token")
			return v, nil
		}
		if r.URL.Host != "api.ru-1.storage.selcloud.ru" || r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal("wrong scope/auth endpoint", r.URL)
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := root.Project("p")
	r, _ := p.Location("spb", "ru-1")
	return root, r.ObjectStorage()
}

func TestNamedManagementContainerDomainsKeepScopeAndExactPath(t *testing.T) {
	calls := 0
	_, c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.EscapedPath() != "/v2/containers/folder%20+name/domains" {
			t.Fatal(r.URL)
		}
		if r.Method == "PUT" {
			b, _ := io.ReadAll(r.Body)
			if string(b) != `{"domain_name":"files.example"}` {
				t.Fatal(string(b))
			}
			return response(204, ""), nil
		}
		return response(200, `{"domains":["files.example"]}`), nil
	})
	ref, e := c.Containers.Ref("folder +name")
	if e != nil {
		t.Fatal(e)
	}
	name := "files.example"
	added, e := c.Containers.AddDomain(context.Background(), ref, objectstorage.AddDomainOptions{DomainName: &name}, types.CallOptions{})
	if e != nil || added.Receipt.Outcome != types.Accepted || len(added.Receipt.ResourceRefs) != 1 {
		t.Fatal(e)
	}
	domains, e := c.Containers.Domains(context.Background(), ref, types.CallOptions{})
	if e != nil || domains.Data.Domains == nil || len(*domains.Data.Domains) != 1 {
		t.Fatal(e)
	}
	ref.Scope.ProjectID = "other"
	_, e = c.Containers.Domains(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestDeleteDomainsPreservesEmptyArrayAndCertificateSchemaDoesNotInventDetails(t *testing.T) {
	calls := 0
	_, c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "DELETE" {
			b, _ := io.ReadAll(r.Body)
			if string(b) != `{"domains":[]}` {
				t.Fatal(string(b))
			}
			return response(204, ""), nil
		}
		if r.URL.Path != "/v2/containers/bucket/domains/certs" {
			t.Fatal(r.URL)
		}
		return response(200, `{"domains":["files.example"]}`), nil
	})
	ref, _ := c.Containers.Ref("bucket")
	empty := []string{}
	deleted, e := c.Containers.DeleteDomains(context.Background(), ref, objectstorage.DeleteDomainsOptions{Domains: &empty}, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	certs, e := c.Containers.DomainsAndCertificates(context.Background(), ref, types.CallOptions{})
	if e != nil || certs.Data.Domains == nil || len(*certs.Data.Domains) != 1 || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestContainerOptionsPreserveExplicitFalseEmptyZeroWithoutDefaults(t *testing.T) {
	_, c := client(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/containers/bucket/options" {
			t.Fatal(r.URL)
		}
		if r.Method == "PUT" {
			b, _ := io.ReadAll(r.Body)
			if string(b) != `{"general":{"cache_control":"","default_delete_after":0},"quota":{"max_object_count":0},"web":{"listing_enabled":false}}` {
				t.Fatal(string(b))
			}
		}
		return response(200, `{"general":{"cache_control":"","default_delete_after":0,"type":"private"},"quota":{"max_bytes_used":9007199254740993,"max_object_count":0},"web":{"listing_enabled":false}}`), nil
	})
	ref, _ := c.Containers.Ref("bucket")
	empty := ""
	zero, _ := types.ParseInteger("0")
	disabled := false
	updated, e := c.Containers.SetOptions(context.Background(), ref, objectstorage.SetContainerOptions{General: &objectstorage.ContainerGeneralOptionsBodySwagger{CacheControl: &empty, DefaultDeleteAfter: &zero}, Quota: &objectstorage.ContainerQuotaOptionsBodySwagger{MaxObjectCount: &zero}, Web: &objectstorage.ContainerWebOptionsBodySwagger{ListingEnabled: &disabled}}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	read, e := c.Containers.Options(context.Background(), ref, types.CallOptions{})
	if e != nil || read.Data.Web == nil || read.Data.Web.ListingEnabled == nil || *read.Data.Web.ListingEnabled || read.Data.Quota.MaxBytesUsed.String() != "9007199254740993" {
		t.Fatal(e)
	}
}

func TestLogExportCreateUsesProtectedFilterWireAndExactTaskRef(t *testing.T) {
	calls := 0
	_, c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "POST" {
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"filters":{"private":"filter-secret","big":9007199254740993}`) || strings.Contains(string(b), "REDACTED") {
				t.Fatal("protected filter not serialized at request boundary")
			}
			return response(200, `{"task":{"id":"job","created":"2026-10-15T12:00:00","data":{"filters":{"private":"filter-secret"}}}}`), nil
		}
		if r.URL.Path != "/v2/logs/job" {
			t.Fatal(r.URL)
		}
		return response(200, `{"task":{"id":"job","finished":"2026-10-15T12:10:00","response":{"objects":["logs/export.zip"]}}}`), nil
	})
	filter := types.ProtectedJSON{ProtectedValue: types.Protect([]byte(`{"private":"filter-secret","big":9007199254740993}`))}
	target := "bucket"
	kind := "export"
	created, e := c.LogExports.Create(context.Background(), objectstorage.CreateExportOptions{Type: &kind, Data: &objectstorage.CreateExportOptionsData{Container: &target, Filters: &filter}}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if created.Data.Ref == nil || created.Data.Ref.ID != "job" || created.Receipt.Outcome != types.Accepted || len(created.Receipt.ResourceRefs) != 1 {
		t.Fatal("missing task receipt/ref")
	}
	got, e := c.LogExports.Get(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil || got.Data.Task == nil || *got.Data.Task.Finished != "2026-10-15T12:10:00" || calls != 2 {
		t.Fatal(e, calls)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", created, &created), "filter-secret") {
		t.Fatal("filter disclosure")
	}
}

func TestCancelLogExportUsesExactTaskAndRetainsReceipt(t *testing.T) {
	calls := 0
	_, c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/v2/logs/job/cancel" {
			t.Fatal(r.Method, r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		if len(b) != 0 {
			t.Fatal("cancel body invented")
		}
		return response(200, `{"task":{"id":"job","updated":"2026-10-15T13:00:00"}}`), nil
	})
	ref, _ := c.LogExports.Ref("job")
	out, e := c.LogExports.Cancel(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted || out.Data.Ref.ID != "job" || calls != 1 {
		t.Fatal(e, calls)
	}
}

func TestLogExportPaginationUsesExactOffsetAndEmptyPageEnd(t *testing.T) {
	calls := 0
	_, c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/v2/logs" || r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("order_by") != "created desc" {
			t.Fatal(r.URL)
		}
		switch calls {
		case 1:
			if r.URL.Query().Get("offset") != "9007199254740993" {
				t.Fatal(r.URL)
			}
			return response(200, `{"tasks":[{"task":{"id":"first"}}]}`), nil
		case 2:
			if r.URL.Query().Get("offset") != "9007199254740994" {
				t.Fatal(r.URL)
			}
			return response(200, `{"tasks":[{"task":{"id":"second"}}]}`), nil
		case 3:
			if r.URL.Query().Get("offset") != "9007199254740995" {
				t.Fatal(r.URL)
			}
			return response(200, `{"tasks":[]}`), nil
		}
		t.Fatal("extra page")
		return nil, nil
	})
	offset, _ := types.ParseInteger("9007199254740993")
	order := "created desc"
	out, e := c.LogExports.Collect(context.Background(), objectstorage.ExportListOptions{Offset: &offset, OrderBy: &order}, pagination.Options{PageSize: 2, MaxPages: 4, MaxItems: 10})
	if e != nil || !out.Complete || len(out.Items) != 2 || out.Items[1].Ref.ID != "second" || calls != 3 {
		t.Fatal(e, out.Complete, calls)
	}
}

func TestLogExportMalformedCollectionKeepsValidPrefix(t *testing.T) {
	calls := 0
	_, c := client(t, func(*http.Request) (*http.Response, error) {
		calls++
		return response(200, `{"tasks":[{"task":{"id":"valid"}},{"task":{"id":""}}]}`), nil
	})
	out, e := c.LogExports.Collect(context.Background(), objectstorage.ExportListOptions{}, pagination.Options{PageSize: 2, MaxItems: 5, MaxPages: 3})
	if types.ErrorCode(e) != types.ProtocolError || out.Complete || len(out.Items) != 1 || out.Items[0].Ref.ID != "valid" || out.NextCursor == nil || calls != 1 {
		t.Fatal(e, len(out.Items), out.NextCursor, calls)
	}
}

func TestExportMutationsRetainKnownIdentityAndDoNotReplayUnknown(t *testing.T) {
	for _, test := range []struct {
		name, body string
		unknown    bool
	}{{"known malformed", `{"task":{"id":"job","data":{"delete_after":"bad"}}}`, false}, {"no identity", `{"task":{"created":"naive"}}`, false}, {"unknown", "", true}} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			_, c := client(t, func(*http.Request) (*http.Response, error) {
				calls++
				if test.unknown {
					return nil, io.ErrUnexpectedEOF
				}
				return response(200, test.body), nil
			})
			out, e := c.LogExports.Create(context.Background(), objectstorage.CreateExportOptions{}, types.CallOptions{})
			if calls != 1 || out.Receipt == nil {
				t.Fatal(e, calls)
			}
			if test.unknown {
				if out.Receipt.Outcome != types.Unknown || e == nil || len(out.Receipt.ResourceRefs) != 0 {
					t.Fatal(e)
				}
			} else if test.name == "known malformed" {
				if types.ErrorCode(e) != types.ProtocolError || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "job" {
					t.Fatal(e)
				}
			} else if out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 0 {
				t.Fatal(e)
			}
		})
	}
}

func TestOptionsAndQuotaBadInputsOrResponsesAreStructured(t *testing.T) {
	calls := 0
	_, c := client(t, func(*http.Request) (*http.Response, error) { calls++; return response(200, `null`), nil })
	neg, _ := types.ParseInteger("-1")
	tooBig, _ := types.ParseInteger("18446744073709551616")
	for _, n := range []types.Integer{neg, tooBig, {}} {
		out, e := c.Account.SetQuota(context.Background(), objectstorage.SetQuotaOptions{QuotaBytes: types.Value(n)}, types.CallOptions{})
		if types.ErrorCode(e) != types.InvalidArgument || out.Receipt.Outcome != types.NotSent || calls != 0 {
			t.Fatal(e, calls)
		}
	}
	_, e := c.Account.GetQuota(context.Background(), types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || calls != 1 {
		t.Fatal(e, calls)
	}
}

func TestManagementKnownQuotaReceiptResumeIsConclusiveWithoutReference(t *testing.T) {
	calls := 0
	root, c := client(t, func(*http.Request) (*http.Response, error) {
		calls++
		return response(200, `{"quota_bytes":null}`), nil
	})
	set, e := c.Account.SetQuota(context.Background(), objectstorage.SetQuotaOptions{QuotaBytes: types.Null[types.Integer]()}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	handle, e := root.Resume(*set.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	observed, e := handle.Recover(context.Background())
	if e != nil || observed.ProofStatus != "conclusive" || calls != 1 {
		t.Fatal(e, calls)
	}
}

func TestRootResourceAndResumeObserveOnlyExactExportAndNeverReplay(t *testing.T) {
	gets := 0
	root, c := client(t, func(r *http.Request) (*http.Response, error) {
		gets++
		if r.Method != "GET" || r.URL.Path != "/v2/logs/job" {
			t.Fatal(r.Method, r.URL)
		}
		return response(200, `{"task":{"id":"job","finished":"2026-10-15T12:00:00"}}`), nil
	})
	ref, _ := c.LogExports.Ref("job")
	handle, e := root.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	status, e := handle.Status(context.Background())
	if e != nil || status.Classification != "succeeded" || status.ProviderState != "present" || gets != 1 {
		t.Fatal(e, gets)
	}
	cap, _ := root.Catalog().Describe("object_storage.v2.log_exports.cancel")
	receipt := types.Receipt{SchemaVersion: 1, CallID: "synthetic", OperationID: cap.OperationID, ContractRevision: cap.ContractRevision, Scope: ref.Scope, Outcome: types.Unknown, Recovery: "resource", ResourceRefs: []types.ResourceRef{ref}, Endpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: "v2", Audience: "selectel"}}
	resumed, e := root.Resume(receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := resumed.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovered.ProofStatus != "indeterminate" || gets != 2 {
		t.Fatal(e, gets)
	}
	receipt.ResourceRefs = nil
	_, e = root.Resume(receipt)
	if types.ErrorCode(e) != types.RecoveryUnavailable || gets != 2 {
		t.Fatal(e, gets)
	}
	receipt.Outcome = types.Accepted
	known, e := root.Resume(receipt)
	if e != nil {
		t.Fatal(e)
	}
	r, e := known.Recover(context.Background())
	if e != nil || r.ProofStatus != "conclusive" || gets != 2 {
		t.Fatal(e, gets)
	}
	receipt.ContractRevision = "old"
	_, e = root.Resume(receipt)
	if types.ErrorCode(e) != types.CapabilityUnavailable || gets != 2 {
		t.Fatal(e, gets)
	}
}

func TestNotSentQuotaReceiptHasRevisionAndNeedsNoRecoveryIO(t *testing.T) {
	calls := 0
	root, c := client(t, func(*http.Request) (*http.Response, error) { calls++; t.Fatal("invalid quota HTTP"); return nil, nil })
	n, _ := types.ParseInteger("-1")
	out, e := c.Account.SetQuota(context.Background(), objectstorage.SetQuotaOptions{QuotaBytes: types.Value(n)}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt.ContractRevision == "" || out.Receipt.Outcome != types.NotSent {
		t.Fatal(e)
	}
	handle, e := root.Resume(*out.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	r, e := handle.Recover(context.Background())
	if e != nil || r.ProofStatus != "conclusive" || calls != 0 {
		t.Fatal(e, calls)
	}
}

func TestManagementInputSourceEnumsFailBeforeIO(t *testing.T) {
	calls := 0
	_, c := client(t, func(*http.Request) (*http.Response, error) { calls++; return response(200, `{}`), nil })
	ref, _ := c.Containers.Ref("bucket")
	bad := "invented"
	code, _ := types.ParseInteger("500")
	for _, input := range []objectstorage.SetContainerOptions{{General: &objectstorage.ContainerGeneralOptionsBodySwagger{Type: &bad}}, {Web: &objectstorage.ContainerWebOptionsBodySwagger{ErrorStatusCode: &code}}} {
		out, e := c.Containers.SetOptions(context.Background(), ref, input, types.CallOptions{})
		if types.ErrorCode(e) != types.InvalidArgument || out.Receipt.Outcome != types.NotSent || calls != 0 {
			t.Fatal(e, calls)
		}
	}
}

func TestManagementErrorMappingRedirectAndCursorBinding(t *testing.T) {
	for _, test := range []struct {
		status int
		code   types.Code
	}{{400, types.InvalidArgument}, {403, types.PermissionDenied}, {404, types.UnknownOutcome}, {422, types.InvalidArgument}, {307, types.UnknownOutcome}} {
		t.Run(strconv.Itoa(test.status), func(t *testing.T) {
			calls := 0
			_, c := client(t, func(*http.Request) (*http.Response, error) {
				calls++
				out := response(test.status, `{"code":"private-code","message":"private-provider-message"}`)
				out.Header.Set("Location", "https://foreign.example")
				return out, nil
			})
			ref, _ := c.Containers.Ref("bucket")
			out, e := c.Containers.SetOptions(context.Background(), ref, objectstorage.SetContainerOptions{}, types.CallOptions{})
			if types.ErrorCode(e) != test.code || out.Receipt == nil || calls != 1 || strings.Contains(fmt.Sprint(e), "private-provider-message") {
				t.Fatal(e, calls)
			}
			if test.status != 307 && test.status != 404 && out.Receipt.Outcome != types.Rejected {
				t.Fatal(out.Receipt.Outcome)
			}
		})
	}
	_, c := client(t, func(*http.Request) (*http.Response, error) {
		return response(200, `{"tasks":[{"task":{"id":"a"}}]}`), nil
	})
	first, e := c.LogExports.ListPage(context.Background(), objectstorage.ExportListOptions{}, pagination.Options{PageSize: 1, MaxItems: 2, MaxPages: 3})
	if e != nil || first.NextCursor == nil {
		t.Fatal(e)
	}
	order := "changed"
	_, e = c.LogExports.Iterate(objectstorage.ExportListOptions{OrderBy: &order}, pagination.Options{Cursor: first.NextCursor})
	if types.ErrorCode(e) != types.ScopeMismatch {
		t.Fatal(e)
	}
}
