package selectelsdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/festy23/selectel-sdk/cdn"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

const cdnResourceID = "6d19b7a2-a493-48fa-a9ec-aea7ba9ffa28"

func TestCDNResourceReadUsesProjectIAMAndProtectsNestedOriginSecrets(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/cdn/v3/resources/"+cdnResourceID || r.Header.Get("X-Auth-Token") == "" {
			t.Fatal("CDN endpoint/auth/scope")
		}
		return inventoryReply(200, `{"id":"`+cdnResourceID+`","name":"selected","active":false,"creation_ts":9007199254740993,"origin":{"aws":{"auth":{"access_key":"synthetic-private-access","secret_key":"synthetic-private-secret"}}},"headers":{"request":{"Authorization":"synthetic-private-header"}},"extra_provider_field":{"pem":"synthetic-private-extra"},"ref":{"id":"forged"}}`), nil
	}, 0)
	svc := projectView(t, c).CDN()
	ref, e := svc.Resource(cdnResourceID)
	if e != nil {
		t.Fatal(e)
	}
	out, e := svc.GetResource(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.Ref == nil || out.Data.Ref.Scope.ProjectID != "test-project" || out.Data.Ref.ID != cdnResourceID || out.Data.Active == nil || *out.Data.Active || out.Data.CreationTS.String() != "9007199254740993" {
		t.Fatal("typed CDN resource", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") || strings.Contains(fmt.Sprintf("%+v %#v", out, out), "synthetic-private") {
		t.Fatal("CDN protected output leaked")
	}
}

func TestCDNCacheTaskSubmissionUsesExplicitPathsAndRetainsUnknownOutcome(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/cdn/v3/cache/tasks" {
			t.Fatal("cache endpoint")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("wire JSON")
		}
		if string(body["paths"]) != `["/synthetic-private-path?token=value"]` || string(body["action_type"]) != `"single"` {
			t.Fatal("protected path missing on wire")
		}
		return nil, errors.New("synthetic-private-network")
	}, 0)
	action, kind := "preload", "single"
	domain := types.Protect([]byte("selected.example"))
	paths := []types.ProtectedValue{types.Protect([]byte("/synthetic-private-path?token=value"))}
	out, e := projectView(t, c).CDN().CreateCacheTask(context.Background(), cdn.Task{Action: &action, ActionType: &kind, Domain: &domain, Paths: &paths}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || out.Receipt.Attempts != 1 || calls != 1 {
		t.Fatal("cache replay/receipt", e)
	}
}

func TestCDNCacheTaskAcceptedRefAndObservationStayInCacheDomain(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return inventoryReply(200, `{"task_id":"cache-task1","extra":"synthetic-private-extra"}`), nil
		}
		if r.Method != "GET" || r.URL.Path != "/cdn/v3/cache/tasks/cache-task1" {
			t.Fatal("cache task observation changed target")
		}
		return inventoryReply(200, `{"task_id":"cache-task1","status":"ok","rate":[98],"task":{"action":"preload","domain":"selected.example","paths":["/synthetic-private-path"]}}`), nil
	}, 0)
	action := "preload"
	domain := types.Protect([]byte("selected.example"))
	svc := projectView(t, c).CDN()
	accepted, e := svc.CreateCacheTask(context.Background(), cdn.Task{Action: &action, Domain: &domain}, types.CallOptions{})
	if e != nil || accepted.Receipt == nil || accepted.Receipt.Outcome != types.Accepted || len(accepted.Receipt.ResourceRefs) != 1 || accepted.Data.Ref == nil || accepted.Data.Ref.Kind != "cache_task" {
		t.Fatal("missing accepted cache locator", e)
	}
	got, e := svc.GetCacheTask(context.Background(), *accepted.Data.Ref, types.CallOptions{})
	if e != nil || got.Data.Ref == nil || got.Data.Ref.ID != "cache-task1" || got.Data.Rate == nil || (*got.Data.Rate)[0].String() != "98" {
		t.Fatal("cache task observation", e)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("cache task paths leaked")
	}
}

func TestCDNResourceCreationPreservesNestedFalseAndProtectedWire(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/cdn/v3/resources" {
			t.Fatal("resource create endpoint")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["active"]) != "false" || !strings.Contains(string(body["headers"]), "synthetic-private-header") {
			t.Fatal("resource settings wire lost")
		}
		return inventoryReply(200, `{"status":"accept","resource_id":"`+cdnResourceID+`","task_id":"20180402162358692400"}`), nil
	}, 0)
	name := types.Protect([]byte("selected"))
	active := false
	headers := map[string]types.ProtectedValue{"Authorization": types.Protect([]byte("synthetic-private-header"))}
	out, e := projectView(t, c).CDN().CreateResource(context.Background(), cdn.ResourceSpec{Name: &name, Active: &active, Headers: &cdn.ResourceHeaders{Request: &headers}}, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].Kind != "resource" || out.Data.Ref == nil || out.Data.Ref.ID != cdnResourceID || out.Receipt.ProviderOperationRef != nil {
		t.Fatal("resource/configuration task domain", e)
	}
}

func TestCDNResourceUpdatesKeepPartialAndReplacementMethodsDistinct(t *testing.T) {
	methods := []string{}
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.Method)
		return inventoryReply(200, `{"status":"accept","resource_id":"`+cdnResourceID+`"}`), nil
	}, 0)
	svc := projectView(t, c).CDN()
	ref, _ := svc.Resource(cdnResourceID)
	name := types.Protect([]byte("selected"))
	disabled := false
	spec := cdn.ResourceSpec{Name: &name, Active: &disabled}
	a, e := svc.PatchResource(context.Background(), ref, spec, types.CallOptions{})
	if e != nil || a.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	a, e = svc.ReplaceResource(context.Background(), ref, spec, types.CallOptions{})
	if e != nil || a.Receipt.Outcome != types.Accepted || strings.Join(methods, ",") != "PATCH,PUT" {
		t.Fatal("update semantics", e)
	}
}

func TestCDNResourcesListIsBoundedAndStatusBatchKeepsExactCSVTargets(t *testing.T) {
	second := "6d19b7a2-a493-48fa-a9ec-aea7ba9ffa29"
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/cdn/v3/resources" {
			if r.URL.Query().Get("active") != "false" {
				t.Fatal("explicit inactive filter lost")
			}
			return inventoryReply(200, `[{"id":"`+cdnResourceID+`","name":"first"},{"id":"`+second+`","name":"second"}]`), nil
		}
		if r.URL.Query().Get("resource_id") != cdnResourceID+","+second {
			t.Fatal("batch CSV changed")
		}
		return inventoryReply(200, `[{"resource":"`+cdnResourceID+`","status":"Completed"},{"resource":"`+second+`","status":"Processing"}]`), nil
	}, 0)
	svc := projectView(t, c).CDN()
	inactive := false
	first, e := svc.ListResources(context.Background(), cdn.ListOptions{Active: &inactive}, pagination.Options{MaxItems: 1, PageSize: 2})
	if types.ErrorCode(e) != types.Incomplete || first.Complete || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatal("list bound/cursor", e)
	}
	resumed, e := svc.ListResources(context.Background(), cdn.ListOptions{Active: &inactive}, pagination.Options{PageSize: 2, Cursor: first.NextCursor})
	if e != nil || !resumed.Complete || len(resumed.Items) != 1 || resumed.Items[0].Ref.ID != second {
		t.Fatal("list replay continuation", e)
	}
	a, _ := svc.Resource(cdnResourceID)
	b, _ := svc.Resource(second)
	statuses, e := svc.GetResourcesStatus(context.Background(), []types.ResourceRef{a, b}, types.CallOptions{})
	if e != nil || len(statuses.Data) != 2 || statuses.Data[0].Ref.ID != cdnResourceID || calls != 3 {
		t.Fatal("status exact refs", e)
	}
}

func TestCDNCertificateCreateRevealsOnlyOnWireAndRetainsExactSequence(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		var body map[string]string
		if r.Method != "POST" || r.URL.Path != "/cdn/v3/certificate" || json.NewDecoder(r.Body).Decode(&body) != nil || body["private_key"] != "synthetic-private-key" {
			t.Fatal("certificate wire")
		}
		return inventoryReply(200, `{"status":0,"data":{"seq":9007199254740993},"message":"synthetic-private-provider"}`), nil
	}, 0)
	cert, key, comment := types.Protect([]byte("synthetic-private-pem")), types.Protect([]byte("synthetic-private-key")), types.Protect([]byte("selected"))
	out, e := projectView(t, c).CDN().CreateCertificate(context.Background(), cdn.CreateCertificateRequest{Certificate: &cert, PrivateKey: &key, Comment: &comment}, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Data.Ref == nil || out.Data.Ref.ID != "9007199254740993" {
		t.Fatal("certificate exact sequence", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("certificate response leaked")
	}
}

func TestCDNCertificateExactGetPatchDeleteAndProtectedDetails(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/cdn/v3/certificate/9007199254740993" {
			t.Fatal("certificate sequence rounded")
		}
		if r.Method == "GET" {
			if r.URL.Query().Get("details") != "true" {
				t.Fatal("details omitted")
			}
			return inventoryReply(200, `{"status":0,"data":[{"seq":9007199254740993,"chain_info":[],"unpublished_extra":"synthetic-private-extension"}]}`), nil
		}
		return inventoryReply(200, `{"status":0,"data":{"seq":9007199254740993}}`), nil
	}, 0)
	svc := projectView(t, c).CDN()
	seq, _ := types.ParseInteger("9007199254740993")
	ref, e := svc.Certificate(seq)
	if e != nil {
		t.Fatal(e)
	}
	details := true
	got, e := svc.GetCertificate(context.Background(), ref, cdn.CertificateOptions{Details: &details}, types.CallOptions{})
	if e != nil || got.Data.Data == nil || len(*got.Data.Data) != 1 || (*got.Data.Data)[0].Ref.ID != ref.ID {
		t.Fatal("certificate detail envelope", e)
	}
	comment := types.Protect([]byte("updated"))
	a, e := svc.UpdateCertificate(context.Background(), ref, cdn.UpdateCertificateRequest{Comment: &comment}, types.CallOptions{})
	if e != nil || a.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	removed, e := svc.DeleteCertificate(context.Background(), ref, types.CallOptions{})
	if e != nil || removed.Receipt.Outcome != types.Accepted || calls != 3 {
		t.Fatal("certificate removal", e)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("unpublished detail field leaked")
	}
}

func TestCDNCertificateListRetainsBoundedRowsAndEnvelope(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"status":0,"message":"synthetic-private-message","data":[{"seq":1},{"seq":9007199254740993}]}`), nil
	}, 0)
	out, e := projectView(t, c).CDN().ListCertificates(context.Background(), pagination.Options{MaxItems: 1, PageSize: 2})
	if types.ErrorCode(e) != types.Incomplete || len(out.Items) != 1 || out.NextCursor == nil || len(out.Envelopes) != 1 || out.Envelopes[0].Data != nil || out.Items[0].Ref.ID != "1" {
		t.Fatal("certificate collection/envelope bound", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("certificate envelope leaked")
	}
}

func TestCDNLetsEncryptWildcardAndResourceOrdersKeepDifferentLocators(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if r.URL.Path != "/cdn/v3/letsencrypt" {
				t.Fatal("wildcard path")
			}
			return inventoryReply(200, `{"status":0,"message":"accepted"}`), nil
		}
		if r.URL.Path != "/cdn/v3/letsencrypt/"+cdnResourceID {
			t.Fatal("resource order path")
		}
		if r.Method == "GET" {
			return inventoryReply(200, `{"status":0,"data":{"task_status":"processed"}}`), nil
		}
		return inventoryReply(200, `{"status":0}`), nil
	}, 0)
	svc := projectView(t, c).CDN()
	domains := []types.ProtectedValue{types.Protect([]byte("*.selected.example"))}
	wildcard, e := svc.OrderWildcardLetsEncrypt(context.Background(), cdn.LetsEncryptWildcardOrderRequest{Domains: &domains}, types.CallOptions{})
	if e != nil || wildcard.Receipt == nil || wildcard.Receipt.Outcome != types.Accepted || len(wildcard.Receipt.ResourceRefs) != 0 {
		t.Fatal("wildcard invented certificate locator", e)
	}
	ref, _ := svc.Resource(cdnResourceID)
	order, e := svc.OrderLetsEncrypt(context.Background(), ref, types.CallOptions{})
	if e != nil || len(order.Receipt.ResourceRefs) != 1 || order.Receipt.ResourceRefs[0].Kind != "resource" {
		t.Fatal("resource order locator", e)
	}
	status, e := svc.GetLetsEncryptStatus(context.Background(), ref, types.CallOptions{})
	if e != nil || status.Data.Data == nil || status.Data.Data.TaskStatus == nil || *status.Data.Data.TaskStatus != "processed" || calls != 3 {
		t.Fatal("LE observation", e)
	}
}

func TestCDNNodeListsRetainProtectedNetworksAndExplicitDomainFilter(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 && r.URL.Path != "/cdn/v3/nodes/ip2origin" {
			t.Fatal("origin nodes path")
		}
		if calls == 2 && (r.URL.Path != "/cdn/v3/nodes/ip2user" || r.URL.Query().Get("cdn-domain") != "selected.example") {
			t.Fatal("delivery node filter")
		}
		return inventoryReply(200, `{"status":"ok","data":[{"IPv4_subnet":"192.0.2.0/24","IPv6_subnet":"2001:db8::/32"}]}`), nil
	}, 0)
	svc := projectView(t, c).CDN()
	origins, e := svc.ListOriginNodes(context.Background(), pagination.Options{})
	if e != nil || !origins.Complete || len(origins.Items) != 1 || len(origins.Envelopes) != 1 {
		t.Fatal("origin nodes", e)
	}
	domain := types.Protect([]byte("selected.example"))
	delivery, e := svc.ListDeliveryNodes(context.Background(), cdn.DeliveryOptions{CDNDomain: &domain}, pagination.Options{})
	if e != nil || !delivery.Complete || len(delivery.Items) != 1 || calls != 2 {
		t.Fatal("delivery nodes", e)
	}
	raw, _ := json.Marshal(delivery)
	if strings.Contains(string(raw), "192.0.2") || strings.Contains(string(raw), "2001:db8") {
		t.Fatal("node addresses leaked")
	}
}

func TestCDNOnlyDocumentedMutationErrorsAreRejected(t *testing.T) {
	for _, cache := range []bool{false, true} {
		t.Run(fmt.Sprint(cache), func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(*http.Request) (*http.Response, error) { calls++; return inventoryReply(429, ""), nil }, 0)
			svc := projectView(t, c).CDN()
			var receipt *types.Receipt
			var e error
			if cache {
				action := "delete"
				domain := types.Protect([]byte("selected.example"))
				out, x := svc.CreateCacheTask(context.Background(), cdn.Task{Action: &action, Domain: &domain}, types.CallOptions{})
				receipt, e = out.Receipt, x
			} else {
				name := types.Protect([]byte("selected"))
				out, x := svc.CreateResource(context.Background(), cdn.ResourceSpec{Name: &name}, types.CallOptions{})
				receipt, e = out.Receipt, x
			}
			code, outcome := types.UnknownOutcome, types.Unknown
			if cache {
				code, outcome = types.RateLimited, types.Rejected
			}
			if types.ErrorCode(e) != code || receipt == nil || receipt.Outcome != outcome || calls != 1 {
				t.Fatal("unconfirmed no-apply assumption", e)
			}
		})
	}
}

func TestCDNHandlesUseOperationSpecificStatesAndDoNotCompleteOnMissingState(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/cache/tasks/") {
			return inventoryReply(200, `{"task_id":"cache-task1","status":"ok","rate":[98],"task":{"action":"preload","domain":"selected.example"}}`), nil
		}
		return inventoryReply(200, `{"resource":"`+cdnResourceID+`"}`), nil
	}, 0)
	svc := projectView(t, c).CDN()
	task, e := svc.CacheTask("cache-task1")
	if e != nil {
		t.Fatal(e)
	}
	h, e := c.Resource(task)
	if e != nil {
		t.Fatal(e)
	}
	done, e := h.Wait(context.Background(), "cache_processed")
	if e != nil || !done.Complete {
		t.Fatal("cache processed goal", e)
	}
	resource, _ := svc.Resource(cdnResourceID)
	h, e = c.Resource(resource)
	if e != nil {
		t.Fatal(e)
	}
	missing, e := h.Wait(context.Background(), "configuration_observed")
	if types.ErrorCode(e) != types.StateUnknown || missing.Complete {
		t.Fatal("missing state claimed configured", e)
	}
}

func TestCDNLocalMutationValidationRetainsNotSentReceipt(t *testing.T) {
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("local validation reached HTTP"); return nil, nil }, 0)
	svc := projectView(t, c).CDN()
	for _, spec := range []cdn.ResourceSpec{{}, {Name: func() *types.ProtectedValue { v := types.Protect([]byte("invalid/name")); return &v }()}} {
		out, e := svc.CreateResource(context.Background(), spec, types.CallOptions{})
		if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.Attempts != 0 {
			t.Fatal("local receipt", e)
		}
	}
	action := "unsupported"
	domain := types.Protect([]byte("selected.example"))
	out, e := svc.CreateCacheTask(context.Background(), cdn.Task{Action: &action, Domain: &domain}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.Attempts != 0 {
		t.Fatal("cache local receipt", e)
	}
}

func TestCDNMalformedAcceptedUpdateRetainsCallerReference(t *testing.T) {
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"resource_id":42,"status":"ok"}`), nil
	}, 0)
	svc := projectView(t, c).CDN()
	ref, _ := svc.Resource(cdnResourceID)
	name := types.Protect([]byte("selected"))
	out, e := svc.PatchResource(context.Background(), ref, cdn.ResourceSpec{Name: &name}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != ref.ID {
		t.Fatal("accepted update lost caller identity", e)
	}
}
