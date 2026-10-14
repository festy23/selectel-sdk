package volumes_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"github.com/festy23/selectel-sdk/volumes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLifecycleMetadataReplacementOnlineExtendAndDelete(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return response(202, `{"volume":{"id":"v","size":10,"status":"creating"}}`), nil
		case 2:
			if r.Method != "PUT" {
				t.Fatal(r.Method)
			}
			var b map[string]map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&b)
			if string(b["volume"]["metadata"]) != "{}" || string(b["volume"]["name"]) != `""` {
				t.Fatal(b)
			}
			return response(200, `{"volume":{"id":"v","size":10,"status":"available"}}`), nil
		case 3:
			if r.Method != "POST" || r.Header.Get("OpenStack-API-Version") != "volume 3.42" {
				t.Fatal(r.Method, r.Header)
			}
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != `{"os-extend":{"new_size":20}}` {
				t.Fatal(string(raw))
			}
			return response(202, ""), nil
		case 4:
			if r.Method != "GET" {
				t.Fatal(r.Method)
			}
			return response(200, `{"volume":{"id":"v","size":20,"status":"in-use","attachments":[{"id":"v","attachment_id":"att","server_id":"server"}]}}`), nil
		case 5:
			if r.Method != "DELETE" {
				t.Fatal(r.Method)
			}
			return response(202, ""), nil
		}
		t.Fatal(calls)
		return nil, nil
	})
	size, _ := types.ParseInteger("10")
	made, e := c.Create(context.Background(), volumes.CreateSpec{Size: &size}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	r := *made.Data.Ref
	empty := ""
	m := map[string]string{}
	updated, e := c.Update(context.Background(), r, volumes.UpdateSpec{Name: &empty, Metadata: &m}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	target, _ := types.ParseInteger("20")
	extended, e := c.Extend(context.Background(), r, volumes.ExtendSpec{NewSize: target, Attached: true}, types.CallOptions{})
	if e != nil || extended.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	got, e := c.Get(context.Background(), r, types.CallOptions{})
	if e != nil || got.Data.Attachments[0].ID != "v" || got.Data.Attachments[0].AttachmentID != "att" {
		t.Fatal(e)
	}
	deleted, e := c.Delete(context.Background(), r, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 5 {
		t.Fatal(e, calls)
	}
}
func TestUnknownMutationNeverReplaysAndAcceptedMalformedKeepsIdentity(t *testing.T) {
	calls := 0
	c := client(t, func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(202, `{"volume":{"id":"v","size":"bad"}}`), nil
		}
		return nil, io.ErrUnexpectedEOF
	})
	size, _ := types.ParseInteger("10")
	made, e := c.Create(context.Background(), volumes.CreateSpec{Size: &size}, types.CallOptions{})
	if e == nil || made.Receipt.Outcome != types.Accepted || len(made.Receipt.ResourceRefs) != 1 {
		t.Fatalf("%+v %v", made, e)
	}
	deleted, e := c.Delete(context.Background(), made.Receipt.ResourceRefs[0], types.CallOptions{})
	if e == nil || deleted.Receipt.Outcome != types.Unknown || calls != 2 {
		t.Fatalf("%+v %v calls=%d", deleted, e, calls)
	}
}
func TestListsAreBoundedAndTypesAndQoSPreserveExactValues(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.URL.Path {
		case "/v3/p/volumes/detail":
			if r.URL.Query().Get("marker") == "v1" {
				return response(200, `{"volumes":[{"id":"v2","size":2}]}`), nil
			}
			return response(200, `{"volumes":[{"id":"v1","size":1}],"volumes_links":[{"rel":"next","href":"https://cinder.test/v3/p/volumes/detail?marker=v1&limit=1"}]}`), nil
		case "/v3/p/types/default":
			return response(200, `{"volume_type":{"id":"type","name":"fast","is_public":false,"extra_specs":{"volume_backend_name":"ssd"}}}`), nil
		case "/v3/p/types/type":
			return response(200, `{"volume_type":{"id":"type","name":"fast"}}`), nil
		case "/v3/p/types":
			return response(200, `{"volume_types":[{"id":"type","name":"fast"}]}`), nil
		case "/v3/p/qos-specs/qos_limits":
			return response(200, `{"service_flag":true,"fast":{"volume_type_id":"type","qos_specs":{"iops":9007199254740993},"allow_user_qos":true,"full_qos_disk_type":true}}`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	one, e := c.Collect(context.Background(), pagination.Options{MaxItems: 1, PageSize: 1})
	if types.ErrorCode(e) != types.Incomplete || one.Complete || one.NextCursor == nil {
		t.Fatalf("%+v %v", one, e)
	}
	rest, e := c.Collect(context.Background(), pagination.Options{Cursor: one.NextCursor})
	if e != nil || !rest.Complete || rest.Items[0].ID != "v2" {
		t.Fatalf("%+v %v", rest, e)
	}
	typ, e := c.Types.Default(context.Background(), types.CallOptions{})
	if e != nil || typ.Data.Ref.ID != "type" {
		t.Fatal(e)
	}
	_, e = c.Types.Get(context.Background(), *typ.Data.Ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	all, e := c.Types.Collect(context.Background(), pagination.Options{})
	if e != nil || !all.Complete || len(all.Items) != 1 {
		t.Fatal(e)
	}
	qos, e := c.Types.QoSLimits(context.Background(), pagination.Options{})
	if e != nil || !qos.Complete || qos.Items[0].QoSSpecs["iops"].String() != "9007199254740993" {
		t.Fatalf("%+v %v", qos, e)
	}
}
func TestCinderStructuredQuotaAndOversizedRequestRemainDistinct(t *testing.T) {
	for _, quota := range []bool{false, true} {
		t.Run(map[bool]string{false: "request", true: "quota"}[quota], func(t *testing.T) {
			c := client(t, func(*http.Request) (*http.Response, error) {
				body := `{"overLimit":{"code":413,"message":"private-detail"}}`
				if quota {
					body = `{"overLimit":{"code":413,"message":"private-detail","retryAfter":"0"}}`
				}
				return response(413, body), nil
			})
			n, _ := types.ParseInteger("1")
			got, e := c.Create(context.Background(), volumes.CreateSpec{Size: &n}, types.CallOptions{})
			var se *types.SDKError
			want := types.InvalidArgument
			if quota {
				want = types.QuotaExceeded
			}
			if !errors.As(e, &se) || se.Code != want || got.Receipt.Outcome != types.Rejected || strings.Contains(e.Error(), "private-detail") {
				t.Fatalf("%+v %v", got, e)
			}
		})
	}
}

func TestWaitGoalsDoNotTreatInUseOrWrongSizeAsAvailable(t *testing.T) {
	calls := 0
	c := client(t, func(*http.Request) (*http.Response, error) {
		calls++
		return response(200, `{"volume":{"id":"v","size":20,"status":"in-use"}}`), nil
	})
	r := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "volumes", APIGeneration: "v3", Kind: "volume", ID: "v", EndpointID: "cinder", EndpointURL: "https://cinder.test/v3/p"}
	target, _ := types.ParseInteger("20")
	result, e := c.WaitExtended(context.Background(), r, target, true)
	if e != nil || !result.Complete || result.Last.Data.Status != "in-use" || calls != 1 {
		t.Fatalf("%+v %v", result, e)
	}
	target, _ = types.ParseInteger("21")
	result, e = c.WaitExtended(context.Background(), r, target, true)
	if types.ErrorCode(e) != types.OperationFailed || result.Complete {
		t.Fatalf("%+v %v", result, e)
	}
	h, e := c.Handle(r, nil)
	if e != nil {
		t.Fatal(e)
	}
	obs, e := h.Status(context.Background())
	if e != nil || obs.ProviderState != "in-use" || obs.Classification != "pending" {
		t.Fatalf("%+v %v", obs, e)
	}
}

func TestExactRefAndRequestValidationRejectBeforeHTTP(t *testing.T) {
	calls := 0
	c := client(t, func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
	r := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "volumes", APIGeneration: "v3", Kind: "volume", ID: "v", EndpointID: "cinder", EndpointURL: "https://cinder.test/v3/p"}
	for _, change := range []func(*types.ResourceRef){func(r *types.ResourceRef) { r.Scope.ProjectID = "other" }, func(r *types.ResourceRef) { r.Kind = "snapshot" }, func(r *types.ResourceRef) { r.Service = "compute" }, func(r *types.ResourceRef) { r.APIGeneration = "v1" }, func(r *types.ResourceRef) { r.EndpointID = "foreign" }} {
		wrong := r
		change(&wrong)
		got, e := c.Delete(context.Background(), wrong, types.CallOptions{})
		if e == nil || got.Receipt == nil || got.Receipt.Outcome != types.NotSent {
			t.Fatalf("%+v %v", got, e)
		}
	}
	n, _ := types.ParseInteger("0")
	got, e := c.Create(context.Background(), volumes.CreateSpec{Size: &n}, types.CallOptions{})
	if e == nil || got.Receipt.Outcome != types.NotSent {
		t.Fatal(e)
	}
	got, e = c.Update(context.Background(), r, volumes.UpdateSpec{}, types.CallOptions{})
	if e == nil || got.Receipt.Outcome != types.NotSent {
		t.Fatal(e)
	}
	if calls != 0 {
		t.Fatal(calls)
	}
}
func TestPaginationRejectsForeignLinkAndKeepsEarlierPages(t *testing.T) {
	calls := 0
	c := client(t, func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(200, `{"volumes":[{"id":"v","size":1}],"volumes_links":[{"rel":"next","href":"?marker=v"}]}`), nil
		}
		return response(200, `{"volumes":[],"volumes_links":[{"rel":"next","href":"https://foreign.test/v3/p/volumes/detail?marker=foreign"}]}`), nil
	})
	all, e := c.Collect(context.Background(), pagination.Options{})
	if e == nil || all.Complete || len(all.Items) != 1 || calls != 2 {
		t.Fatalf("%+v %v calls=%d", all, e, calls)
	}
}
func TestScopedAbsenceRequiresCompleteListAndUnknownStateIsNotReady(t *testing.T) {
	r := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "volumes", APIGeneration: "v3", Kind: "volume", ID: "v", EndpointID: "cinder", EndpointURL: "https://cinder.test/v3/p"}
	t.Run("hidden", func(t *testing.T) {
		c := client(t, func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "detail") {
				return response(200, `{"volumes":[{"id":"v","size":1}]}`), nil
			}
			return response(404, `{}`), nil
		})
		h, e := c.Handle(r, nil)
		if e != nil {
			t.Fatal(e)
		}
		got, e := h.Wait(context.Background(), "volume.absent")
		if types.ErrorCode(e) != types.PermissionDenied || got.Complete {
			t.Fatalf("%+v %v", got, e)
		}
	})
	t.Run("absent", func(t *testing.T) {
		c := client(t, func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "detail") {
				return response(200, `{"volumes":[]}`), nil
			}
			return response(404, `{}`), nil
		})
		h, e := c.Handle(r, nil)
		if e != nil {
			t.Fatal(e)
		}
		got, e := h.Wait(context.Background(), "volume.absent")
		if e != nil || !got.Complete {
			t.Fatalf("%+v %v", got, e)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		c := client(t, func(*http.Request) (*http.Response, error) {
			return response(200, `{"volume":{"id":"v","size":1,"status":"new-provider-state"}}`), nil
		})
		h, e := c.Handle(r, nil)
		if e != nil {
			t.Fatal(e)
		}
		got, e := h.Wait(context.Background(), "volume.available")
		if types.ErrorCode(e) != types.StateUnknown || got.Complete || got.Last.ProviderState != "new-provider-state" {
			t.Fatalf("%+v %v", got, e)
		}
	})
}

func TestImageSourceStillRequiresSizeBeforeHTTP(t *testing.T) {
	calls := 0
	c := client(t, func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
	got, e := c.Create(context.Background(), volumes.CreateSpec{ImageID: "image"}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || got.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("%+v %v calls=%d", got, e, calls)
	}
}

func TestIOPSMetadataUsesConfirmedProviderKeyAndExactDecimalString(t *testing.T) {
	c := client(t, func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"metadata":{"total_iops_sec":"9007199254740993"}`) {
			t.Fatal(string(raw))
		}
		return response(200, `{"volume":{"id":"v","size":10,"metadata":{"total_iops_sec":"9007199254740993","total_bytes_sec":"100000"}}}`), nil
	})
	r := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "volumes", APIGeneration: "v3", Kind: "volume", ID: "v", EndpointID: "cinder", EndpointURL: "https://cinder.test/v3/p"}
	m := map[string]string{"total_iops_sec": "9007199254740993"}
	got, e := c.Update(context.Background(), r, volumes.UpdateSpec{Metadata: &m}, types.CallOptions{})
	if e != nil || got.Data.Metadata["total_bytes_sec"] != "100000" {
		t.Fatalf("%+v %v", got, e)
	}
}
func TestQoSLimitsBoundsResumeWithoutInventedProviderPagination(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.RawQuery != "" {
			t.Fatal(r.URL)
		}
		return response(200, `{"b":{"volume_type_id":"b","qos_specs":{"iops":2}},"a":{"volume_type_id":"a","qos_specs":{"iops":1}}}`), nil
	})
	one, e := c.Types.QoSLimits(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || one.Complete || one.Items[0].VolumeTypeID != "a" || one.NextCursor == nil {
		t.Fatalf("%+v %v", one, e)
	}
	rest, e := c.Types.QoSLimits(context.Background(), pagination.Options{Cursor: one.NextCursor})
	if e != nil || !rest.Complete || len(rest.Items) != 1 || rest.Items[0].VolumeTypeID != "b" || calls != 2 {
		t.Fatalf("%+v %v calls=%d", rest, e, calls)
	}
}

func TestRootResourceAndResumeObserveExactCinderRefWithoutMutationReplay(t *testing.T) {
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(202, `{"volume":{"id":"v","size":10,"status":"creating"}}`), nil
		}
		if r.Method == "DELETE" {
			return nil, io.ErrUnexpectedEOF
		}
		if r.Method != "GET" || r.URL.Path != "/v3/p/volumes/v" {
			t.Fatal("replayed or selected other identity", r.Method, r.URL)
		}
		return response(200, `{"volume":{"id":"v","size":10,"status":"available"}}`), nil
	})
	size, _ := types.ParseInteger("10")
	created, e := c.Create(context.Background(), volumes.CreateSpec{Size: &size}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	h, e := root.Resource(*created.Data.Ref)
	if e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal("Resource I/O")
	}
	observation, e := h.Status(context.Background())
	if e != nil || observation.Ref.ID != "v" {
		t.Fatal(e)
	}
	accepted, e := root.Resume(*created.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	before := calls
	recovery, e := accepted.Recover(context.Background())
	if e != nil || recovery.Receipt.Outcome != types.Accepted || calls != before {
		t.Fatal("accepted recovery I/O", e)
	}
	deleted, e := c.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome {
		t.Fatal(e)
	}
	unknown, e := root.Resume(*deleted.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	before = calls
	got, e := unknown.Wait(context.Background(), "volume.absent")
	if types.ErrorCode(e) != types.UnknownOutcome || got.Complete || got.Receipt.Outcome != types.Unknown || calls != before+1 {
		t.Fatalf("%+v %v calls=%d", got, e, calls)
	}
	forged := *created.Receipt
	cap, e := root.Catalog().Describe("volumes.v3.volumes.get")
	if e != nil {
		t.Fatal(e)
	}
	forged.OperationID = cap.OperationID
	forged.ContractRevision = cap.ContractRevision
	before = calls
	if _, e = root.Resume(forged); e == nil || calls != before {
		t.Fatal("read receipt resumed", e)
	}
}

func TestQuotaFaultRequiresConfirmedCodeAndRateLimitRemainsSeparate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   types.Code
	}{
		{"wrong-code", 413, `{"overLimit":{"code":429,"retryAfter":"0"}}`, types.InvalidArgument},
		{"body-text", 413, `{"overLimit":{"code":413,"message":"quota exceeded retryAfter"}}`, types.InvalidArgument},
		{"rate-limit", 429, `{"overLimit":{"code":413,"retryAfter":"0"}}`, types.RateLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := client(t, func(*http.Request) (*http.Response, error) { calls++; return response(tc.status, tc.body), nil })
			n, _ := types.ParseInteger("1")
			got, e := c.Create(context.Background(), volumes.CreateSpec{Size: &n}, types.CallOptions{})
			if types.ErrorCode(e) != tc.want || got.Receipt.Outcome != types.Rejected || calls != 1 {
				t.Fatalf("%+v %v calls=%d", got, e, calls)
			}
		})
	}
}

func TestUpdateRejectsNullMetadataVariantAbsentFromSource(t *testing.T) {
	calls := 0
	c := client(t, func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
	r := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "volumes", APIGeneration: "v3", Kind: "volume", ID: "v", EndpointID: "cinder", EndpointURL: "https://cinder.test/v3/p"}
	var metadata map[string]string
	got, e := c.Update(context.Background(), r, volumes.UpdateSpec{Metadata: &metadata}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || got.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("%+v %v calls=%d", got, e, calls)
	}
}
