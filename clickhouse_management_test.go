package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/databases/clickhouse"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
)

const chID = "04c5bf32-bb4b-40b4-b0c7-3cb4eb1d99d1"

func chString(v string) *string { return &v }
func chInteger(t *testing.T, v string) *types.Integer {
	t.Helper()
	n, e := types.ParseInteger(v)
	if e != nil {
		t.Fatal(e)
	}
	return &n
}
func chSpec(t *testing.T) clickhouse.DatastoreCreateRequest {
	password := types.Protect([]byte("synthetic-private-password"))
	return clickhouse.DatastoreCreateRequest{Name: chString("analytics"), TypeID: chString(chID), SubnetID: chString(chID), Password: &password, NodeGroups: []clickhouse.NodeGroupCreateRequest{{Name: chString("data"), Role: chString("DATA"), Weight: chInteger(t, "0"), NodeCount: chInteger(t, "1"), Flavor: &clickhouse.FlavorForNodeGroupRequest{Type: chString("FLEXIBLE"), DiskType: chString("NETWORK_ULTRA"), RAM: chInteger(t, "900719925474099312345"), Disk: chInteger(t, "100"), VCPUs: chInteger(t, "2")}}}}
}
func TestClickHouseCreateUsesV2TypedRequestAndReceipt(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "ru-2.dbaas.selcloud.ru" || r.URL.Path != "/v2/datastores/clickhouse" || r.Method != "POST" {
			t.Fatal("generation binding", r.URL, r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"password":"synthetic-private-password"`) || !strings.Contains(string(raw), `"ram":900719925474099312345`) {
			t.Fatal("typed request mapping failed")
		}
		return inventoryReply(201, `{"id":"`+chID+`","project_id":"test-project","status":"CREATING","state":"PENDING","config":{"secret":"synthetic-private-config"}}`), nil
	}, 0)
	spec := chSpec(t)
	safe, _ := json.Marshal(spec)
	if strings.Contains(string(safe), "synthetic-private-password") {
		t.Fatal("password disclosed")
	}
	result, err := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores.Create(context.Background(), spec)
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || result.Data.Ref == nil || result.Data.Ref.APIGeneration != "v2" {
		t.Fatal("create receipt lost", err)
	}
	safe, _ = json.Marshal(result)
	if strings.Contains(string(safe), "synthetic-private-config") {
		t.Fatal("config disclosed")
	}
}
func TestClickHouseExactReadRejectsForeignGenerationAndResponse(t *testing.T) {
	foreign := false
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/datastores/clickhouse/"+chID || r.Method != "GET" {
			t.Fatal("exact path", r.URL)
		}
		id := chID
		if foreign {
			id = "550e8400-e29b-41d4-a716-446655440000"
		}
		return inventoryReply(200, `{"id":"`+id+`","project_id":"test-project","status":"ACTIVE","state":"RUNNING"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "msk", Pool: "ru-2"}, Service: "databases", APIGeneration: "v2", Kind: "clickhouse_datastore", ID: chID, EndpointID: "selectel-databases.v2:ru-2", EndpointURL: "https://ru-2.dbaas.selcloud.ru/v2"}
	result, err := service.Get(context.Background(), ref)
	if err != nil || result.Data.Ref == nil || result.Data.Ref.ID != chID {
		t.Fatal("exact get failed", err)
	}
	foreign = true
	_, err = service.Get(context.Background(), ref)
	if types.ErrorCode(err) != types.ScopeMismatch {
		t.Fatal("foreign response accepted", err)
	}
	ref.APIGeneration = "v1"
	_, err = service.Get(context.Background(), ref)
	if types.ErrorCode(err) != types.ScopeMismatch {
		t.Fatal("v1 ref accepted", err)
	}
}
func TestClickHouseCollectionsAndSettingsKeepLimitsAndNullableProtectedDefaults(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v2/datastores/clickhouse":
			return inventoryReply(200, `{"datastores":[{"id":"`+chID+`","project_id":"test-project","node_groups":[{"id":"node","node_count":900719925474099312345,"flavor":{"ram":900719925474099312345}}]},{"id":"550e8400-e29b-41d4-a716-446655440000"}]}`), nil
		case "/v2/datastores/clickhouse/configuration-parameters":
			return inventoryReply(200, `{"configuration-parameters":[{"id":"setting","default_value":"synthetic-private-setting","min":null,"max":"900719925474099312345","choices":null,"invalid_values":[]},{"id":"omitted"}]}`), nil
		default:
			t.Fatal(r.URL)
			return nil, nil
		}
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	first, err := service.Collect(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || first.Complete || first.NextCursor == nil || first.Items[0].NodeGroups[0].NodeCount.String() != "900719925474099312345" {
		t.Fatal("bounded collection", err)
	}
	rest, err := service.Collect(context.Background(), pagination.Options{Cursor: first.NextCursor})
	if err != nil || !rest.Complete || len(rest.Items) != 1 {
		t.Fatal("continuation lost", err)
	}
	settings, err := service.Settings(context.Background(), pagination.Options{})
	if err != nil || !settings.Complete || settings.Items[0].Min == nil || settings.Items[1].Min != nil {
		t.Fatal("settings presence lost", err)
	}
	raw, _ := json.Marshal(settings)
	if strings.Contains(string(raw), "synthetic-private-setting") || strings.Contains(fmt.Sprintf("%#v", settings), "0x73, 0x79, 0x6e") {
		t.Fatal("settings disclosed")
	}
}
func chRef() types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "msk", Pool: "ru-2"}, Service: "databases", APIGeneration: "v2", Kind: "clickhouse_datastore", ID: chID, EndpointID: "selectel-databases.v2:ru-2", EndpointURL: "https://ru-2.dbaas.selcloud.ru/v2"}
}

func TestClickHouseUpdateMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PATCH" || r.URL.Path != "/v2/datastores/clickhouse/"+chID+"" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"name":"renamed"}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+chID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	ref := chRef()
	result, err := service.Update(context.Background(), ref, clickhouse.DatastoreUpdateRequest{Name: chString("renamed")})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestClickHouseUpdatePasswordMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PATCH" || r.URL.Path != "/v2/datastores/clickhouse/"+chID+"/password" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"new_password":"synthetic-private-update"}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+chID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	ref := chRef()
	password := types.Protect([]byte("synthetic-private-update"))
	result, err := service.UpdatePassword(context.Background(), ref, clickhouse.DatastoreUpdatePasswordRequest{NewPassword: &password})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestClickHouseConfigureMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.URL.Path != "/v2/datastores/clickhouse/"+chID+"/config" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"config":{"limit":900719925474099312345}}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+chID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	ref := chRef()
	config := types.ProtectedJSON{ProtectedValue: types.Protect([]byte(`{"limit":900719925474099312345}`))}
	result, err := service.Configure(context.Background(), ref, clickhouse.DatastoreConfigRequest{Config: &config})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestClickHouseUpdateSecurityGroupsMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.URL.Path != "/v2/datastores/clickhouse/"+chID+"/security-groups" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"security_groups":[]}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+chID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	ref := chRef()
	result, err := service.UpdateSecurityGroups(context.Background(), ref, clickhouse.DatastoreSecurityGroupsRequest{SecurityGroups: []string{}})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestClickHouseEnableLoggingMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.URL.Path != "/v2/datastores/clickhouse/"+chID+"/log-platform" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"log_platform":{"log_group":"logs"}}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+chID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	ref := chRef()
	result, err := service.EnableLogging(context.Background(), ref, clickhouse.DatastoreLogPlatformRequest{LogPlatform: &clickhouse.DatastoreLogGroup{LogGroup: chString("logs")}})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestClickHouseDisableLoggingMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "DELETE" || r.URL.Path != "/v2/datastores/clickhouse/"+chID+"/log-platform" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(204, ""), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	ref := chRef()
	result, err := service.DisableLogging(context.Background(), ref)
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestClickHouseDeleteMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "DELETE" || r.URL.Path != "/v2/datastores/clickhouse/"+chID+"" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(204, ""), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores
	ref := chRef()
	result, err := service.Delete(context.Background(), ref)
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}
func TestClickHouseCreateRejectsScalarConfigBeforeResourceRequest(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) { t.Fatal("invalid request dispatched"); return nil, nil }, 0)
	spec := chSpec(t)
	config := types.ProtectedJSON{ProtectedValue: types.Protect([]byte(`"scalar"`))}
	spec.Config = &config
	_, err := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores.Create(context.Background(), spec)
	if types.ErrorCode(err) != types.InvalidArgument {
		t.Fatal("scalar config accepted", err)
	}
}
func TestClickHouseMutationKeepsUnknownAndAcceptedReceipts(t *testing.T) {
	for _, scenario := range []string{"lost-response", "malformed-accepted"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if scenario == "lost-response" {
					return nil, io.ErrUnexpectedEOF
				}
				return inventoryReply(201, `{"status":"CREATING"}`), nil
			}, 0)
			result, err := regionalView(t, c, "ru-2").Databases().V2.ClickHouse.Datastores.Create(context.Background(), chSpec(t))
			if result.Receipt == nil || calls != 1 {
				t.Fatal("receipt lost or replay", err)
			}
			if scenario == "lost-response" {
				if types.ErrorCode(err) != types.UnknownOutcome || result.Receipt.Outcome != types.Unknown {
					t.Fatal("unknown receipt lost", err)
				}
			} else {
				if types.ErrorCode(err) != types.ProtocolError || result.Receipt.Outcome != types.Accepted {
					t.Fatal("accepted receipt lost", err)
				}
			}
		})
	}
}
func TestClickHouseCatalogPinsElevenCompleteGenerationBindings(t *testing.T) {
	for action, method := range map[string]string{"create": "Create", "get": "Get", "list": "ListPage", "update": "Update", "delete": "Delete", "password": "UpdatePassword", "configure": "Configure", "security_groups": "UpdateSecurityGroups", "enable_logging": "EnableLogging", "disable_logging": "DisableLogging", "settings": "SettingsPage"} {
		capability, err := catalog.Default().Describe("databases.v2.clickhouse." + action)
		if err != nil || capability.Implementation.Status != "implemented" || capability.APIGeneration != "v2" || capability.GoMethod == nil || *capability.GoMethod != "clickhouse.Datastores."+method {
			t.Fatal("generation binding missing", action, err)
		}
	}
}
