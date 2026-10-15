package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/filestorage"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFileStorageTypesUsePinnedMicroversionExactPoolAndPreserveFalse(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method != "GET" || r.URL.String() != "https://ru-9.cloud.api.selcloud.ru/share/v2/types" || r.Header.Get("X-OpenStack-Manila-API-Version") != "2.73" {
			t.Fatalf("types %s %s %s", r.Method, r.URL, r.Header.Get("X-OpenStack-Manila-API-Version"))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_types":[{"id":"type-1","name":"synthetic-type","description":"example","share_type_access:is_public":false,"is_default":false,"extra_specs":{"driver_handles_share_servers":"True"},"required_extra_specs":{"driver_handles_share_servers":"True"}}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	out, e := view.FileStorage().ListTypes(context.Background(), filestorage.TypeFilters{}, pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.ID != "type-1" || out.Items[0].Ref.Scope != view.Scope() || out.Items[0].Ref.Parent != nil || out.Items[0].ShareTypeAccessIsPublic == nil || *out.Items[0].ShareTypeAccessIsPublic || out.Items[0].IsDefault == nil || *out.Items[0].IsDefault {
		t.Fatalf("types %+v %v", out, e)
	}
	if out.Items[0].Ref.APIGeneration != "v2_73" || out.Items[0].Ref.Kind != "share_type" || out.Items[0].Ref.Service != "filestorage" {
		t.Fatal("wrong exact identity")
	}
}

func TestFileStorageTypeDescriptionPreservesNullButRejectsMissing(t *testing.T) {
	for _, description := range []string{`,"description":null`, ""} {
		t.Run(description, func(t *testing.T) {
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/identity/v3/auth/tokens" {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_types":[{"id":"type-1","name":"synthetic-type","share_type_access:is_public":false,"is_default":false,"extra_specs":{},"required_extra_specs":{}` + description + `}]}`))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			p, _ := c.Project("test-project")
			view, _ := p.Location("spb", "ru-9")
			out, e := view.FileStorage().ListTypes(context.Background(), filestorage.TypeFilters{}, pagination.Options{})
			if description == "" {
				if types.ErrorCode(e) != types.ProtocolError {
					t.Fatalf("missing %+v %v", out, e)
				}
				return
			}
			if e != nil || len(out.Items) != 1 || out.Items[0].Description == nil {
				t.Fatalf("null %+v %v", out, e)
			}
		})
	}
}

func TestFileStorageTypeFiltersAndCutoffKeepProtectedNullableDiagnostics(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Query().Get("is_public") != "false" || r.URL.Query().Get("extra_specs") != `{"driver_handles_share_servers":"True"}` {
			t.Fatalf("query %s", r.URL.RawQuery)
		}
		item := `{"id":"type-1","name":"synthetic-type","description":"synthetic-private-type-description","share_type_access:is_public":false,"is_default":false,"extra_specs":{"synthetic":"private-spec"},"required_extra_specs":{}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_types":[` + item + `,` + strings.Replace(item, "type-1", "type-2", 1) + `]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	public := false
	out, e := view.FileStorage().ListTypes(context.Background(), filestorage.TypeFilters{IsPublic: &public, ExtraSpecs: map[string]string{"driver_handles_share_servers": "True"}}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.NextCursor == nil {
		t.Fatalf("cutoff %+v %v", out, e)
	}
	description, valid := out.Items[0].Description.Value()
	if !valid || string(description.Reveal()) != "synthetic-private-type-description" {
		t.Fatal("description not preserved")
	}
	safe, _ := json.Marshal(out)
	diagnostics := string(safe) + fmt.Sprintf("%+v %#v %x", out.Items[0].Description, out.Items[0].Description, out.Items[0].Description)
	for _, secret := range []string{"synthetic-private-type-description", "private-spec"} {
		for _, encoding := range []string{secret, fmt.Sprint([]byte(secret)), fmt.Sprintf("%#v", []byte(secret)), fmt.Sprintf("%x", []byte(secret))} {
			if strings.Contains(diagnostics, encoding) {
				t.Fatal("protected nullable or specs leaked")
			}
		}
	}
}

func TestFileStorageNetworkListContinuesAfterServerClampedShortPage(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		offset := r.URL.Query().Get("offset")
		if r.URL.Path != "/share/v2/share-networks/detail" || r.URL.Query().Get("limit") != "3" {
			t.Fatalf("network %s", r.URL)
		}
		payload := `{"share_networks":[]}`
		if offset == "0" || offset == "1" {
			payload = `{"share_networks":[{"id":"network-` + offset + `","project_id":"test-project","name":null,"description":null,"created_at":"2022-01-01T00:00:00","updated_at":null,"status":"active","security_service_update_support":false,"network_allocation_update_support":false,"share_network_subnets":[]}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	out, e := view.FileStorage().ListNetworks(context.Background(), filestorage.NetworkFilters{}, pagination.Options{PageSize: 3})
	if e != nil || !out.Complete || calls != 3 || len(out.Items) != 2 || out.Items[0].Ref == nil || out.Items[0].Ref.ID != "network-0" || out.Items[0].Ref.Parent != nil || out.Items[1].Ref.Scope != view.Scope() {
		t.Fatalf("networks %+v %v calls%d", out, e, calls)
	}
}

func TestFileStorageNetworkPaginationRetainsEarlierPageButExcludesForeignProject(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		project := "test-project"
		if calls > 1 {
			project = "foreign-project"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_networks":[{"id":"network-1","project_id":"` + project + `","name":null,"description":null,"created_at":"2022-01-01T00:00:00","updated_at":null,"status":"active","security_service_update_support":false,"network_allocation_update_support":false,"share_network_subnets":[]}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	out, e := view.FileStorage().ListNetworks(context.Background(), filestorage.NetworkFilters{}, pagination.Options{PageSize: 1})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.Scope.ProjectID != "test-project" || calls != 2 {
		t.Fatalf("partial %+v %v", out, e)
	}
}
