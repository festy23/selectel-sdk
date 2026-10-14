package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/dedicated"
	"github.com/festy23/selectel-sdk/types"
)

func TestDedicatedPowerAcceptedReceiptAndExplicitFalse(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/servers/v2/power/r1" || r.Method != "PUT" {
			t.Fatal("wrong dedicated endpoint/effect")
		}
		var body struct {
			PowerState *bool `json:"power_state"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.PowerState == nil || *body.PowerState {
			t.Fatal("explicit power-off lost")
		}
		return inventoryReply(200, `{"task_id":"task1","result":{},"status":"success"}`), nil
	}, 0)
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	out, err := c.Account().Dedicated().SetServerPowerState(context.Background(), ref, dedicated.PowerSwitchStatus{PowerState: managementPtr(false)}, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Data.TaskRef.ID != "task1" {
		t.Fatal("power receipt/task ref lost", err)
	}
}

func TestDedicatedResponseStatusSelectsExactQueuedAndTypedSchemas(t *testing.T) {
	for _, status := range []int{200, 202} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/servers/v2/resource/r1" {
					t.Fatal("wrong resource path")
				}
				if status == 202 {
					return inventoryReply(202, `{"task_id":"t1","result":{},"status":"processing"}`), nil
				}
				return inventoryReply(200, `{"task_id":"t1","result":`+dedicatedResourceFixture+`}`), nil
			}, 0)
			ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
			out, err := c.Account().Dedicated().GetResourceDetails(context.Background(), ref, types.CallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if status == 202 {
				if out.Data.Queued == nil || out.Data.Data != nil {
					t.Fatal("202 decoded as completed DTO")
				}
			} else if out.Data.Queued != nil || out.Data.Data == nil || *out.Data.Data.Result.UUID != "r1" || out.Data.Data.Result.Ref.ID != "r1" {
				t.Fatal("typed resource result lost")
			}
		})
	}
}

const dedicatedResourceFixture = `{"uuid":"r1","state":"active","project_uuid":"test-project","billing":{"currency":"RUB","current_price_plan":{"name":"monthly","period":1,"type":"month","uuid":"plan1"},"next_price_plan":{"name":"monthly","period":1,"type":"month","uuid":"plan1"}},"config_name":"synthetic","hw_uuid":"hw1","is_not_prolonged":false,"is_primary":true,"is_processing":false,"is_single_prolonged":false,"location_uuid":"loc1","owner_id":100,"paid_till":100,"previous_state":"deploy","quantity":1,"service_type":"server","service_uuid":"service1","tags":[]}`

func TestDedicatedMutationUsesHTTPStatusForSameBodyAndTask303NeverRedirects(t *testing.T) {
	for _, status := range []int{200, 202} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				return inventoryReply(status, `{"task_id":"t1","result":`+dedicatedResourceFixture+`}`), nil
			}, 0)
			ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
			out, e := c.Account().Dedicated().UpdateResource(context.Background(), ref, dedicated.ResourceUpdateModel{}, types.CallOptions{})
			if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Receipt.Attempts != 1 {
				t.Fatal("status-specific accepted receipt lost", e)
			}
			if status == 202 {
				if out.Data.Queued == nil || out.Data.Data != nil {
					t.Fatal("same body 202 interpreted as completed")
				}
			} else if out.Data.Queued != nil || out.Data.Data == nil {
				t.Fatal("same body 200 interpreted as queued")
			}
		})
	}
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/servers/v2/result/t1" {
			t.Fatal("redirect followed with credentials")
		}
		reply := inventoryReply(303, `{"task_id":"t1","status":"processing","result":{"private_payload":"synthetic-task-secret"}}`)
		reply.Header.Set("Location", "https://unconfirmed.example/receive")
		return reply, nil
	}, 0)
	task := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "task", ID: "t1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	out, e := c.Account().Dedicated().GetTask(context.Background(), task, types.CallOptions{})
	if e != nil || calls != 1 || out.Data.TaskRef.ID != "t1" || fmt.Sprintf("%+v", out.Data.Data.Result) != "[REDACTED]" {
		t.Fatal("task 303 exact observation failed", e)
	}
}

func TestDedicatedProjectTransferKeepsOriginalReceiptWithoutAdoptingTargetScope(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"task_id":"t1","result":`+strings.ReplaceAll(dedicatedResourceFixture, "test-project", "target-project")+`}`), nil
	}, 0)
	svc := projectView(t, c).Dedicated()
	ref := types.ResourceRef{Scope: projectView(t, c).Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	out, e := svc.UpdateResourceProject(context.Background(), ref, dedicated.ResourceUpdateProjectModel{ProjectUUID: managementPtr("target-project")}, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Receipt.ResourceRefs[0].Scope.ProjectID != "test-project" || out.Data.Data == nil || *out.Data.Data.Result.ProjectUUID != "target-project" || out.Data.Data.Result.Ref != nil {
		t.Fatal("transfer inferred ownership in new scope or lost original receipt", e)
	}
}

func TestDedicatedOSInstallPreservesProtectedNullableInputAndNativePartitionMap(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/servers/v2/boot/os/r1" {
			t.Fatal("wrong installation effect")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["cloud_init_user_data"]) != `"synthetic-sensitive-userdata"` {
			t.Fatal("protected request data not encoded at HTTP boundary")
		}
		var partitions map[string]json.RawMessage
		if json.Unmarshal(body["partitions_config"], &partitions) != nil || partitions["root"] == nil || partitions["*"] != nil {
			t.Fatal("native partition shape silently wrapped")
		}
		var node map[string]json.RawMessage
		_ = json.Unmarshal(partitions["root"], &node)
		if string(node["name"]) != `"synthetic-partition-name"` {
			t.Fatal("protected map value not revealed at wire boundary")
		}
		return inventoryReply(200, `{"task_id":"install1","result":{"arch":"x86_64","os_template":"ubuntu","version":"24.04","userhostname":"host","password":null,"reinstall":1,"partitions_config":{"root":{"type":"filesystem","fstype":"ext4","mount":"/"}}}}`), nil
	}, 0)
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	spec := dedicated.OSConfigInput{Arch: managementPtr("x86_64"), OsTemplate: managementPtr("ubuntu"), Version: managementPtr("24.04"), Userhostname: managementPtr(types.Protect([]byte("host"))), CloudInitUserData: dedicated.Value(types.Protect([]byte("synthetic-sensitive-userdata"))), PartitionsConfig: dedicated.NativePartitions(map[string]dedicated.PartitionsConfigNodeModel{"root": {Type: managementPtr("filesystem"), Fstype: managementPtr("ext4"), Mount: managementPtr("/"), Name: managementPtr(types.Protect([]byte("synthetic-partition-name")))}})}
	for _, pattern := range []string{"%v", "%+v", "%#v"} {
		if fmt.Sprintf(pattern, spec.CloudInitUserData) != "[REDACTED]" {
			t.Fatal("nullable wrapper leaked bytes")
		}
	}
	rawDiagnostic, _ := json.Marshal(spec)
	if strings.Contains(string(rawDiagnostic), "synthetic-partition-name") || strings.Contains(fmt.Sprintf("%#v", spec.PartitionsConfig), "synthetic-partition-name") {
		t.Fatal("partition diagnostic leaked")
	}
	out, e := c.Account().Dedicated().InstallServerOSConfiguration(context.Background(), ref, spec, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Data.TaskRef.ID != "install1" {
		t.Fatal("installation receipt lost", e)
	}
	if _, present := out.Data.Data.Result.Password.Value(); present {
		t.Fatal("null password replaced by value")
	}
	if out.Data.Data.Result.PartitionsConfig.Native["root"].Type == nil {
		t.Fatal("typed native partition response lost")
	}
}

func TestDedicatedPowerWaitUsesConfirmedStableFieldGoalAndPreservesReceipt(t *testing.T) {
	reads, mutations := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), PollInterval: time.Millisecond, WaitTimeout: time.Second, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method == "PUT" {
			mutations++
			return inventoryReply(200, `{"task_id":"power1","result":{}}`), nil
		}
		reads++
		body := `{"task_id":"read1","result":{"driver_status":{"power_state":"power off","target_power_state":"power on"}}}`
		if reads > 1 {
			body = `{"task_id":"read2","result":{"driver_status":{"power_state":"power on","target_power_state":""}}}`
		}
		return inventoryReply(200, body), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	svc := c.Account().Dedicated()
	changed, e := svc.SetServerPowerState(context.Background(), ref, dedicated.PowerSwitchStatus{PowerState: managementPtr(true)}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	handle, e := svc.PowerHandle(ref, changed.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	waited, e := handle.Wait(context.Background(), "dedicated.power.on")
	if e != nil || !waited.Complete || reads != 2 || mutations != 1 || waited.Receipt.Outcome != types.Accepted || waited.Last.Ref.ID != "r1" {
		t.Fatal("stable power wait/receipt failed", e)
	}
}

func TestDedicatedBillingCalculationIsReadAndPreservesExactProviderAmount(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/servers/v2/service/server/service1/billing" {
			t.Fatal("calculation confused with order")
		}
		return inventoryReply(200, `{"task_id":"calc1","result":{"currency":"RUB","current_price_plan":{"name":"monthly","period":1,"type":"month","uuid":"plan1"},"next_price_plan":{"name":"monthly","period":1,"type":"month","uuid":"plan1"},"price":{"amount_due":9007199254740993.25,"debt":0,"discount":0,"discount_detail":{"discounts_from_campaigns":{},"price_plan":0,"resource":0,"user":0},"future_price":9007199254740993.25,"plan_price":9007199254740993.25,"setup_fee":0}}}`), nil
	}, 0)
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "service", ID: "service1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	out, e := c.Account().Dedicated().CalculateServerBilling(context.Background(), ref, dedicated.ServiceBillingCheckModel{LocationUUID: managementPtr("location1"), PricePlanUUID: managementPtr("plan1")}, types.CallOptions{})
	if e != nil || out.Receipt != nil || out.Data.Data.Result.Price.AmountDue.String() != "9007199254740993.25" || *out.Data.Data.Result.Currency != "RUB" {
		t.Fatal("calculation mutated or rounded money", e)
	}
}

func TestDedicatedUnknownMutationRetainsExactRefsWithoutReplay(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF }, 0)
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	out, e := c.Account().Dedicated().RebootServer(context.Background(), ref, dedicated.PowerReboot{Reboot: managementPtr(true)}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || out.Receipt.ResourceRefs[0].ID != "r1" {
		t.Fatal("unknown reboot replayed or lost exact ref", e)
	}
}

func TestDedicatedResourceCancellationWaitIsNonRenewingNotHardwareAbsence(t *testing.T) {
	reads := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), PollInterval: time.Millisecond, WaitTimeout: time.Second, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method == "DELETE" {
			var body struct {
				Immediately *bool `json:"immediately"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Immediately == nil || *body.Immediately {
				t.Fatal("cancel became immediate deletion")
			}
			return inventoryReply(200, `{"task_id":"cancel1","result":{}}`), nil
		}
		reads++
		state := "active"
		if reads > 1 {
			state = "expiring"
		}
		return inventoryReply(200, `{"task_id":"read1","result":`+strings.ReplaceAll(dedicatedResourceFixture, `"state":"active"`, `"state":"`+state+`"`)+`}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	svc := c.Account().Dedicated()
	changed, e := svc.CancelResource(context.Background(), ref, dedicated.ResourceCancelModel{Immediately: managementPtr(false)}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	handle, e := svc.ResourceHandle(ref, changed.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	waited, e := handle.Wait(context.Background(), "dedicated.resource.non_renewing")
	if e != nil || !waited.Complete || reads != 2 || waited.Last.ProviderState != "expiring" || waited.Receipt.Outcome != types.Accepted {
		t.Fatal("non-renewing wait changed cancellation meaning", e)
	}
}

func TestDedicatedOSWaitDoesNotSucceedForUnknownInstallFlag(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"task_id":"read1","result":{"arch":"x86_64","os_template":"ubuntu","version":"24.04","userhostname":"host","password":null,"reinstall":7,"is_processing":false}}`), nil
	}, 0)
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
	handle, e := c.Account().Dedicated().OSHandle(ref, nil)
	if e != nil {
		t.Fatal(e)
	}
	waited, e := handle.Wait(context.Background(), "dedicated.os.installation_finished")
	if types.ErrorCode(e) != types.StateUnknown || waited.Complete || waited.Last.ProviderState != "7" {
		t.Fatal("unknown install flag invented success", e)
	}
}

func TestDedicatedPageQueryAndAcceptedCreateRetainLaterTrustworthyIDs(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("blockstorage_connect_ready") != "false" {
				t.Fatal("page/false query changed")
			}
			return inventoryReply(200, `{"task_id":"page1","page":2,"limit":1,"item_count":500,"result":[`+dedicatedResourceFixture+`]}`), nil
		}
		return inventoryReply(200, `{"task_id":"order1","result":[{"uuid":false},{"uuid":"r1","project_uuid":"test-project"}]}`), nil
	}, 0)
	page, _ := types.ParseInteger("2")
	limit, _ := types.ParseInteger("1")
	out, e := c.Account().Dedicated().ListResources(context.Background(), dedicated.ListResourcesQuery{Page: &page, Limit: &limit, BlockstorageConnectReady: managementPtr(false)}, types.CallOptions{})
	if e != nil || len(out.Data.Data.Result) != 1 || out.Data.Data.ItemCount.String() != "500" || out.Data.Data.Result[0].Ref.ID != "r1" {
		t.Fatal("bounded page lost source count/ref", e)
	}
	created, e := c.Account().Dedicated().CreateServerResource(context.Background(), dedicated.ResourceStartServerModel{Arch: managementPtr("x86_64"), LocationUUID: managementPtr("loc1"), OsTemplate: managementPtr("noos"), PricePlanUUID: managementPtr("plan1"), ServiceUUID: managementPtr("service1"), UserDesc: managementPtr(types.Protect([]byte("synthetic"))), Userhostname: managementPtr(types.Protect([]byte("host"))), Version: managementPtr("none")}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || created.Receipt == nil || created.Receipt.Outcome != types.Accepted || len(created.Receipt.ResourceRefs) != 2 || created.Receipt.ResourceRefs[1].ID != "r1" {
		t.Fatal("accepted rows lost later trustworthy identity", e)
	}
}

func TestDedicatedExistingResourceParentIsRetainedAndForeignParentRejected(t *testing.T) {
	for _, parentID := range []string{"primary1", "foreign-primary"} {
		t.Run(parentID, func(t *testing.T) {
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				body := strings.TrimSuffix(dedicatedResourceFixture, "}") + `,"primary_uuid":"` + parentID + `"}`
				return inventoryReply(200, `{"task_id":"update1","result":`+body+`}`), nil
			}, 0)
			parent := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "primary1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
			ref := parent
			ref.ID = "r1"
			ref.Parent = &parent
			out, e := c.Account().Dedicated().UpdateResource(context.Background(), ref, dedicated.ResourceUpdateModel{}, types.CallOptions{})
			if parentID == "primary1" {
				if e != nil || out.Data.Data.Result.Ref.Parent == nil || out.Data.Data.Result.Ref.Parent.ID != "primary1" {
					t.Fatal("existing exact parent lost", e)
				}
			} else if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt.Outcome != types.Accepted || out.Receipt.ResourceRefs[0].Parent.ID != "primary1" || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[1].ID != "update1" {
				t.Fatal("foreign returned parent adopted", e)
			}
		})
	}
}

func TestDedicatedPrimaryUUIDIsMetadataWithoutCallerParent(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, list := range []bool{false, true} {
			t.Run(fmt.Sprintf("project=%v/list=%v", project, list), func(t *testing.T) {
				c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
					row := strings.TrimSuffix(dedicatedResourceFixture, "}") + `,"primary_uuid":"owner-resource"}`
					if list {
						row = "[" + row + "]"
					}
					return inventoryReply(200, `{"task_id":"t1","result":`+row+`}`), nil
				}, 0)
				svc := c.Account().Dedicated()
				scope := c.Account().Scope()
				if project {
					svc = projectView(t, c).Dedicated()
					scope = projectView(t, c).Scope()
				}
				if list {
					out, e := svc.ListResources(context.Background(), dedicated.ListResourcesQuery{}, types.CallOptions{})
					if e != nil || out.Data.Data.Result[0].Ref.Parent != nil || out.Data.Data.Result[0].PrimaryUUID == nil || *out.Data.Data.Result[0].PrimaryUUID != "owner-resource" {
						t.Fatal("inferred list parent", e)
					}
				} else {
					ref := types.ResourceRef{Scope: scope, Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
					out, e := svc.GetResourceDetails(context.Background(), ref, types.CallOptions{})
					if e != nil || out.Data.Data.Result.Ref.Parent != nil || out.Data.Data.Result.PrimaryUUID == nil || *out.Data.Data.Result.PrimaryUUID != "owner-resource" {
						t.Fatal("inferred get parent", e)
					}
				}
			})
		}
	}
}

func TestDedicatedHandleStatusAndMandatoryGoalFields(t *testing.T) {
	cases := []struct {
		kind, fields, state, classification string
		invalid                             bool
	}{
		{"power", `"power_state":"power on","target_power_state":""`, "power on", "succeeded", false},
		{"power", `"power_state":"power off","target_power_state":"power on"`, "power off->power on", "pending", false},
		{"power", `"power_state":"provider-new","target_power_state":""`, "provider-new", "unknown", false},
		{"power", `"power_state":"power on"`, "", "", true},
		{"power", `"power_state":"power on","target_power_state":null`, "", "", true},
		{"power", `"target_power_state":""`, "", "", true},
		{"os", `"reinstall":0,"is_processing":false`, "0", "succeeded", false},
		{"os", `"reinstall":0,"is_processing":true`, "processing", "pending", false},
		{"os", `"reinstall":7,"is_processing":false`, "7", "unknown", false},
		{"os", `"reinstall":0`, "", "", true},
		{"os", `"reinstall":0,"is_processing":null`, "", "", true},
		{"os", `"is_processing":false`, "", "", true},
		{"resource", `active`, "active", "succeeded", false},
		{"resource", `deploy`, "deploy", "pending", false},
		{"resource", `blocked`, "blocked", "failed", false},
		{"resource", `expiring`, "expiring", "succeeded", false},
		{"resource", `provider-new`, "provider-new", "unknown", false},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				result := `{"driver_status":{` + tc.fields + `}}`
				if tc.kind == "os" {
					result = `{"arch":"x86_64","os_template":"ubuntu","version":"24.04","userhostname":"host","password":null,` + tc.fields + `}`
				}
				if tc.kind == "resource" {
					result = strings.Replace(dedicatedResourceFixture, `"state":"active"`, `"state":"`+tc.fields+`"`, 1)
				}
				return inventoryReply(200, `{"task_id":"read1","result":`+result+`}`), nil
			}, 0)
			svc := c.Account().Dedicated()
			ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "dedicated", APIGeneration: "v2", Kind: "resource", ID: "r1", EndpointID: "selectel-dedicated.v2:global", EndpointURL: "https://api.selectel.ru/servers/v2"}
			h, e := svc.PowerHandle(ref, nil)
			goal := "dedicated.power.on"
			if tc.kind == "os" {
				h, e = svc.OSHandle(ref, nil)
				goal = "dedicated.os.installation_finished"
			}
			if tc.kind == "resource" {
				h, e = svc.ResourceHandle(ref, nil)
			}
			if e != nil {
				t.Fatal(e)
			}
			out, e := h.Status(context.Background())
			if tc.invalid {
				if types.ErrorCode(e) != types.ProtocolError {
					t.Fatal("missing goal field accepted", e)
				}
				waited, e := h.Wait(context.Background(), goal)
				if types.ErrorCode(e) != types.ProtocolError || waited.Complete {
					t.Fatal("partial observation completed", e)
				}
				return
			}
			if e != nil || out.ProviderState != tc.state || out.Classification != tc.classification {
				t.Fatalf("status state=%q classification=%q error=%v", out.ProviderState, out.Classification, e)
			}
		})
	}
}
