package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/databases/opensearch"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
)

const osID = "04c5bf32-bb4b-40b4-b0c7-3cb4eb1d99d1"

func osString(v string) *string { return &v }
func osInteger(t *testing.T, v string) *types.Integer {
	t.Helper()
	n, e := types.ParseInteger(v)
	if e != nil {
		t.Fatal(e)
	}
	return &n
}
func osSpec(t *testing.T) opensearch.DatastoreCreateRequest {
	p := types.Protect([]byte("synthetic-private-os-password"))
	return opensearch.DatastoreCreateRequest{Name: osString("search"), Password: &p, TypeID: osString(osID), SubnetID: osString(osID), NodeGroups: []opensearch.NodeGroupCreateRequest{{Name: osString("manager"), Role: osString("MANAGER"), NodeCount: osInteger(t, "1"), Flavor: &opensearch.FlavorForNodeGroupRequest{Type: osString("FLEXIBLE"), DiskType: osString("NETWORK_ULTRA"), Disk: osInteger(t, "100"), RAM: osInteger(t, "900719925474099312345"), VCPUs: osInteger(t, "2")}}}}
}
func TestOpenSearchCreateUsesOwnEngineAndCompleteTypedRequest(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Host != "ru-2.dbaas.selcloud.ru" || r.URL.Path != "/v2/datastores/opensearch" {
			t.Fatal("wrong engine", r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"role":"MANAGER"`) || !strings.Contains(string(raw), `"ram":900719925474099312345`) || !strings.Contains(string(raw), "synthetic-private-os-password") {
			t.Fatal("create mapping lost")
		}
		return inventoryReply(201, `{"id":"`+osID+`","project_id":"test-project","status":"CREATING","state":"UNKNOWN"}`), nil
	}, 0)
	spec := osSpec(t)
	safe, _ := json.Marshal(spec)
	if strings.Contains(string(safe), "synthetic-private-os-password") || strings.Contains(fmt.Sprintf("%#v", spec), "0x73, 0x79, 0x6e") || strings.Contains(fmt.Sprintf("%+v", spec), "115 121 110 116") {
		t.Fatal("protected password disclosed")
	}
	result, e := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores.Create(context.Background(), spec)
	if e != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || result.Data.Ref == nil || result.Data.Ref.Kind != "opensearch_datastore" {
		t.Fatal("create receipt/ref", e)
	}
}
func osRef() types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "msk", Pool: "ru-2"}, Service: "databases", APIGeneration: "v2", Kind: "opensearch_datastore", ID: osID, EndpointID: "selectel-databases.v2:ru-2", EndpointURL: "https://ru-2.dbaas.selcloud.ru/v2"}
}
func TestOpenSearchExactReadAndCollectionNeverAcceptOtherEngine(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/datastores/opensearch/"+osID {
			return inventoryReply(200, `{"id":"`+osID+`","project_id":"test-project","status":"ACTIVE","state":"RUNNING"}`), nil
		}
		if r.URL.Path == "/v2/datastores/opensearch" {
			return inventoryReply(200, `{"datastores":[{"id":"`+osID+`"},{"id":"550e8400-e29b-41d4-a716-446655440000"}]}`), nil
		}
		t.Fatal("other engine request", r.URL)
		return nil, nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	result, e := service.Get(context.Background(), osRef())
	if e != nil || result.Data.Ref.Kind != "opensearch_datastore" {
		t.Fatal(e)
	}
	foreign := osRef()
	foreign.Kind = "clickhouse_datastore"
	_, e = service.Get(context.Background(), foreign)
	if types.ErrorCode(e) != types.ScopeMismatch {
		t.Fatal("ClickHouse ref accepted", e)
	}
	first, e := service.Collect(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || first.Complete || first.NextCursor == nil {
		t.Fatal("limit lost", e)
	}
	rest, e := service.Collect(context.Background(), pagination.Options{Cursor: first.NextCursor})
	if e != nil || !rest.Complete || len(rest.Items) != 1 {
		t.Fatal("continuation lost", e)
	}
}

func TestOpenSearchUpdateMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PATCH" || r.URL.Path != "/v2/datastores/opensearch/"+osID+"" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"name":"renamed"}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+osID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	ref := osRef()
	result, err := service.Update(context.Background(), ref, opensearch.DatastoreUpdateRequest{Name: osString("renamed")})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestOpenSearchUpdatePasswordMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PATCH" || r.URL.Path != "/v2/datastores/opensearch/"+osID+"/password" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"new_password":"synthetic-private-update"}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+osID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	ref := osRef()
	password := types.Protect([]byte("synthetic-private-update"))
	result, err := service.UpdatePassword(context.Background(), ref, opensearch.DatastoreUpdatePasswordRequest{NewPassword: &password})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestOpenSearchUpdateSecurityGroupsMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.URL.Path != "/v2/datastores/opensearch/"+osID+"/security-groups" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"security_groups":["04c5bf32-bb4b-40b4-b0c7-3cb4eb1d99d1"]}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+osID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	ref := osRef()
	result, err := service.UpdateSecurityGroups(context.Background(), ref, opensearch.DatastoreSecurityGroupsRequest{SecurityGroups: []string{osID}})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestOpenSearchEnableLoggingMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.URL.Path != "/v2/datastores/opensearch/"+osID+"/log-platform" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"log_platform":{"log_group":"logs"}}` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(200, `{"id":"`+osID+`","project_id":"test-project"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	ref := osRef()
	result, err := service.EnableLogging(context.Background(), ref, opensearch.DatastoreLogPlatformRequest{LogPlatform: &opensearch.DatastoreLogGroup{LogGroup: osString("logs")}})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestOpenSearchDisableLoggingMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "DELETE" || r.URL.Path != "/v2/datastores/opensearch/"+osID+"/log-platform" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(204, ""), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	ref := osRef()
	result, err := service.DisableLogging(context.Background(), ref)
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}

func TestOpenSearchDeleteMapsSDKOperation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "DELETE" || r.URL.Path != "/v2/datastores/opensearch/"+osID+"" {
			t.Fatal("action binding", r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `` {
			t.Fatal("request mapping lost")
		}
		return inventoryReply(204, ""), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	ref := osRef()
	result, err := service.Delete(context.Background(), ref)
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("mutation receipt lost", err)
	}
}
func TestOpenSearchResumeAndWaitKeepExactGenerationWithoutReplay(t *testing.T) {
	mutations := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			mutations++
			return inventoryReply(201, `{"id":"`+osID+`","project_id":"test-project","status":"CREATING"}`), nil
		}
		if r.Method != "GET" || r.URL.Path != "/v2/datastores/opensearch/"+osID {
			t.Fatal("resume left exact engine", r.URL)
		}
		return inventoryReply(200, `{"id":"`+osID+`","project_id":"test-project","status":"ACTIVE","state":"RUNNING"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	created, e := service.Create(context.Background(), osSpec(t))
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(created.Receipt)
	var receipt types.Receipt
	if json.Unmarshal(raw, &receipt) != nil {
		t.Fatal("receipt serialize")
	}
	handle, e := service.Resume(receipt)
	if e != nil {
		t.Fatal(e)
	}
	waited, e := handle.Wait(context.Background(), "opensearch.active")
	if e != nil || !waited.Complete || waited.Last.Ref.Kind != "opensearch_datastore" || waited.Last.Ref.APIGeneration != "v2" || mutations != 1 {
		t.Fatal("resume/wait lost exact observation", e)
	}
	receipt.ResourceRefs[0].Kind = "clickhouse_datastore"
	if _, e = service.Resume(receipt); types.ErrorCode(e) != types.ScopeMismatch {
		t.Fatal("other engine receipt accepted", e)
	}
}
func TestOpenSearchInvalidEngineRoleAndEmptySecurityGroupsAreNotSent(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		t.Fatal("invalid engine input dispatched")
		return nil, nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	spec := osSpec(t)
	spec.NodeGroups[0].Role = osString("KEEPER")
	_, e := service.Create(context.Background(), spec)
	if types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal("ClickHouse role accepted", e)
	}
	_, e = service.UpdateSecurityGroups(context.Background(), osRef(), opensearch.DatastoreSecurityGroupsRequest{SecurityGroups: []string{}})
	if types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal("empty security groups accepted", e)
	}
}
func TestOpenSearchUnknownAndMalformedAcceptanceNeverReplay(t *testing.T) {
	for _, scenario := range []string{"lost", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if scenario == "lost" {
					return nil, io.ErrUnexpectedEOF
				}
				return inventoryReply(201, `{"status":"CREATING"}`), nil
			}, 0)
			service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
			result, e := service.Create(context.Background(), osSpec(t))
			if result.Receipt == nil || calls != 1 {
				t.Fatal("receipt lost/replay", e)
			}
			if scenario == "lost" {
				if types.ErrorCode(e) != types.UnknownOutcome || result.Receipt.Outcome != types.Unknown {
					t.Fatal(e)
				}
			} else {
				if types.ErrorCode(e) != types.ProtocolError || result.Receipt.Outcome != types.Accepted {
					t.Fatal(e)
				}
			}
			if _, e = service.Resume(*result.Receipt); types.ErrorCode(e) != types.RecoveryUnavailable {
				t.Fatal("unknown identity adopted", e)
			}
		})
	}
}
func TestOpenSearchCatalogHasNineSeparateEngineBindings(t *testing.T) {
	for action, method := range map[string]string{"create": "Create", "get": "Get", "list": "ListPage", "update": "Update", "delete": "Delete", "password": "UpdatePassword", "security_groups": "UpdateSecurityGroups", "enable_logging": "EnableLogging", "disable_logging": "DisableLogging"} {
		capability, e := catalog.Default().Describe("databases.v2.opensearch." + action)
		if e != nil || capability.Implementation.Status != "implemented" || capability.GoMethod == nil || *capability.GoMethod != "opensearch.Datastores."+method || capability.APIGeneration != "v2" {
			t.Fatal("source binding missing", action, e)
		}
	}
}
func TestOpenSearchDeletionWaitObservesExactAbsence(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/datastores/opensearch/"+osID {
			t.Fatal("wrong deletion observation", r.URL)
		}
		if r.Method == "DELETE" {
			return inventoryReply(204, ""), nil
		}
		return inventoryReply(404, `{"error":{"message":"synthetic-private-provider-message"}}`), nil
	}, 0)
	service := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Datastores
	deleted, e := service.Delete(context.Background(), osRef())
	if e != nil {
		t.Fatal(e)
	}
	handle, e := service.Resume(*deleted.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	result, e := handle.Wait(context.Background(), "opensearch.deleted")
	if e != nil || !result.Complete || result.Last.Classification != "absent" || result.Last.Ref.Kind != "opensearch_datastore" {
		t.Fatal("exact absence lost", e)
	}
}
