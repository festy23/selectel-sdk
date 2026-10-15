package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
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
		payload := `{"serial":"serial-1","status":0,"present":true,"ready":true,"remoteConnectUrl":"synthetic-private-adb-url","owner":{"name":"synthetic-private-owner"},"phone":{"phoneNumber":"synthetic-private-phone"},"battery":{"level":42,"voltage":3.1415926535897932384626}}`
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
	for _, secret := range []string{"synthetic-private-adb-url", "synthetic-private-owner", "synthetic-private-phone", "3.1415926535897932384626"} {
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
