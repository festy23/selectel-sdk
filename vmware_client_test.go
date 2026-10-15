package selectelsdk_test

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestVMwareVersionsNeverSendsCredentialsOrFollowsLoginURLs(t *testing.T) {
	lookups, calls := 0, 0
	scope := types.Scope{AccountID: "synthetic-account", ProjectID: "synthetic-project", Region: "spb", Pool: ""}
	endpoint := types.EndpointBinding{ID: "synthetic-vcd", URL: "https://synthetic.vcd.example", Scope: scope, APIGeneration: "v36_3", Audience: "selectel"}
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		lookups++
		return credentials.Material{}, nil
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "vmware", APIGeneration: "v36_3", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/versions" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Auth-Token") != "" {
			t.Fatal("credentialed/unexpected discovery")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/xml"}}, Body: io.NopCloser(strings.NewReader(`<SupportedVersions xmlns="http://www.vmware.com/vcloud/versions"><VersionInfo deprecated="false"><Version>36.3</Version><LoginUrl>https://foreign.example/api/sessions</LoginUrl></VersionInfo></SupportedVersions>`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project(scope.ProjectID)
	regional, _ := p.Location(scope.Region, scope.Pool)
	out, e := regional.VMware().Versions(context.Background(), types.CallOptions{})
	if e != nil || len(out.Data.VersionInfo) != 1 || *out.Data.VersionInfo[0].Version != "36.3" || out.Receipt != nil || lookups != 0 || calls != 1 {
		t.Fatalf("versions %+v %v lookup%d calls%d", out, e, lookups, calls)
	}
}

func TestVMwareVersionsRejectsForeignNamespaceAndDuplicateFields(t *testing.T) {
	for _, payload := range []string{
		`<SupportedVersions xmlns="http://www.vmware.com/vcloud/versions" xmlns:x="urn:foreign"><VersionInfo deprecated="false"><x:Version>36.3</x:Version></VersionInfo></SupportedVersions>`,
		`<SupportedVersions xmlns="http://www.vmware.com/vcloud/versions" xmlns:x="urn:foreign"><VersionInfo x:deprecated="false"><Version>36.3</Version></VersionInfo></SupportedVersions>`,
		`<SupportedVersions xmlns="http://www.vmware.com/vcloud/versions"><VersionInfo><Version>36.3</Version><Version>39.0</Version></VersionInfo></SupportedVersions>`,
		`<SupportedVersions xmlns="http://www.vmware.com/vcloud/versions"><VersionInfo>foreign-text<Version>36.3</Version></VersionInfo></SupportedVersions>`,
	} {
		scope := types.Scope{AccountID: "synthetic-account", ProjectID: "synthetic-project", Region: "spb"}
		endpoint := types.EndpointBinding{ID: "synthetic-vcd", URL: "https://synthetic.vcd.example", Scope: scope, APIGeneration: "v36_3", Audience: "selectel"}
		c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
			t.Fatal("discovery credential lookup")
			return credentials.Material{}, nil
		}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "vmware", APIGeneration: "v36_3", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
		})})
		if e != nil {
			t.Fatal(e)
		}
		p, _ := c.Project(scope.ProjectID)
		regional, _ := p.Location("spb", "")
		_, e = regional.VMware().Versions(context.Background(), types.CallOptions{})
		if types.ErrorCode(e) != types.ProtocolError {
			t.Fatalf("malformed discovery accepted: %v", e)
		}
	}
}
