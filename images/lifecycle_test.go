package images_test

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/images"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
)

func imageRef() types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "images", APIGeneration: "v2", Kind: "image", ID: "i", EndpointID: "glance", EndpointURL: "https://glance.test"}
}
func protectedJSON(t *testing.T, s string) *types.ProtectedJSON {
	t.Helper()
	var v types.ProtectedJSON
	if json.Unmarshal([]byte(s), &v) != nil {
		t.Fatal("test value")
	}
	return &v
}
func TestSharedImageMembershipUsesCallerViewAndExactParent(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "GET" && r.URL.Path == "/v2/images/i" {
			return response(200, `{"id":"i","owner":"other-owner","status":"active","visibility":"shared"}`), nil
		}
		if r.URL.Path != "/v2/images/i/members" && r.URL.Path != "/v2/images/i/members/target" {
			t.Fatal(r.URL)
		}
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		if r.Method == "GET" && r.URL.Path == "/v2/images/i/members" {
			return response(200, `{"members":[{"member_id":"target","image_id":"i","status":"pending"}]}`), nil
		}
		return response(200, `{"member_id":"target","image_id":"i","status":"pending","created_at":"2026-10-14T10:00:00.123","ref":"provider-extension"}`), nil
	})
	r := imageRef()
	shared, e := c.Get(context.Background(), r, types.CallOptions{})
	if e != nil || *shared.Data.Owner != "other-owner" || shared.Data.Ref.Scope.ProjectID != "p" {
		t.Fatalf("%+v %v", shared, e)
	}
	created, e := c.Members.Create(context.Background(), *shared.Data.Ref, "target", types.CallOptions{})
	if e != nil || created.Receipt.Outcome != types.Accepted || created.Data.Ref.Parent.ID != "i" || created.Data.Ref.ID != "target" || created.Data.Ref.Scope.ProjectID != "p" {
		t.Fatalf("%+v %v", created, e)
	}
	_, e = c.Members.Get(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Members.Update(context.Background(), *created.Data.Ref, "accepted", types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	all, e := c.Members.Collect(context.Background(), r, pagination.Options{})
	if e != nil || !all.Complete || len(all.Items) != 1 {
		t.Fatal(e)
	}
	_, e = c.Members.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil || calls != 6 {
		t.Fatal(e, calls)
	}
	wrong := *created.Data.Ref
	wrong.Parent = created.Data.Ref.Parent
	copyParent := *wrong.Parent
	copyParent.Scope.ProjectID = "foreign"
	wrong.Parent = &copyParent
	_, e = c.Members.Get(context.Background(), wrong, types.CallOptions{})
	if e == nil || calls != 6 {
		t.Fatal("foreign parent reached transport", e)
	}
}
func TestPatchPreservesExplicitValuesAndRejectsOwnerBeforeIO(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "PATCH" || r.Header.Get("Content-Type") != "application/openstack-images-v2.1-json-patch" {
			t.Fatal(r.Method, r.Header)
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"value":false`) || !strings.Contains(string(raw), `"path":"/custom~1key"`) {
			t.Fatal(string(raw))
		}
		return response(200, `{"id":"i","owner":"p","status":"queued"}`), nil
	})
	got, e := c.Update(context.Background(), imageRef(), []images.Patch{{Op: "replace", Path: "/protected", Value: protectedJSON(t, "false")}, {Op: "add", Path: "/custom~1key", Value: protectedJSON(t, `"private-marker"`)}}, types.CallOptions{})
	if e != nil || got.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	for _, path := range []string{"/owner", "/locations", "/status", "/os_glance_import_task", "/tags/0", "/owner~2"} {
		out, e := c.Update(context.Background(), imageRef(), []images.Patch{{Op: "replace", Path: path, Value: protectedJSON(t, `"foreign"`)}}, types.CallOptions{})
		if e == nil || out.Receipt.Outcome != types.NotSent || calls != 1 {
			t.Fatalf("%s %+v %v", path, out, e)
		}
	}
}

func TestImageListsKeepConfirmedFiltersAndBoundedCursor(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		if q.Get("owner") != "other" || q.Get("created_at") != "gte:2020-01-01T00:00:00Z" || q.Get("custom") != "private-marker" || q.Get("size_min") != "9007199254740993" || len(q["tag"]) != 2 {
			t.Fatal("filter mismatch")
		}
		if q.Get("marker") == "i" {
			return response(200, `{"images":[{"id":"second","owner":"other","status":"active"}]}`), nil
		}
		q.Set("marker", "i")
		return response(200, `{"images":[{"id":"i","owner":"other","status":"active"}],"next":"/v2/images?`+q.Encode()+`"}`), nil
	})
	n, _ := types.ParseInteger("9007199254740993")
	f := images.ListOptions{Owner: "other", CreatedAt: "gte:2020-01-01T00:00:00Z", SizeMin: &n, Tags: []string{"a", "b"}, Properties: map[string]types.ProtectedValue{"custom": types.Protect([]byte("private-marker"))}}
	one, e := c.Collect(context.Background(), f, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || one.Complete || one.NextCursor == nil {
		t.Fatalf("%+v %v", one, e)
	}
	changed := f
	changed.Properties = map[string]types.ProtectedValue{"custom": types.Protect([]byte("different"))}
	_, e = c.Collect(context.Background(), changed, pagination.Options{Cursor: one.NextCursor})
	if e == nil || calls != 1 {
		t.Fatal("changed protected filter accepted", e)
	}
	rest, e := c.Collect(context.Background(), f, pagination.Options{Cursor: one.NextCursor})
	if e != nil || !rest.Complete || len(rest.Items) != 1 || rest.Items[0].ID != "second" || calls != 2 {
		t.Fatalf("%+v %v", rest, e)
	}
}
func TestUnknownImageAndMembershipMutationNeverReplays(t *testing.T) {
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "GET" {
			if strings.Contains(r.URL.Path, "members") {
				return response(200, `{"member_id":"target","image_id":"i","status":"pending"}`), nil
			}
			return response(200, `{"id":"i","status":"active"}`), nil
		}
		return nil, io.ErrUnexpectedEOF
	})
	deleted, e := c.Delete(context.Background(), imageRef(), types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || deleted.Receipt.Outcome != types.Unknown || calls != 1 {
		t.Fatalf("%+v %v", deleted, e)
	}
	h, e := root.Resume(*deleted.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovery, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovery.ProofStatus != "indeterminate" || calls != 2 {
		t.Fatalf("%+v %v", recovery, e)
	}
	proposal, e := c.Members.Create(context.Background(), imageRef(), "target", types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || calls != 3 {
		t.Fatal(e)
	}
	member, e := root.Resume(*proposal.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	observed, e := member.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || observed.Observation.Ref.Parent.ID != "i" || calls != 4 {
		t.Fatalf("%+v %v", observed, e)
	}
}

func TestRequestedCreateIDRetainsExactAddressOnUnknownOutcome(t *testing.T) {
	id := "11111111-2222-3333-4444-555555555555"
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "POST" {
			return nil, io.ErrUnexpectedEOF
		}
		if r.Method != "GET" || r.URL.Path != "/v2/images/"+id {
			t.Fatal(r.Method, r.URL)
		}
		return response(200, `{"id":"`+id+`","status":"queued"}`), nil
	})
	got, e := c.Create(context.Background(), images.CreateSpec{ID: &id}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != id || got.Receipt.Outcome != types.Unknown {
		t.Fatalf("%+v %v", got, e)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	observed, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || observed.ProofStatus != "indeterminate" || calls != 2 {
		t.Fatalf("%+v %v", observed, e)
	}
}

func TestCustomPropertyNamedRefDoesNotCollideWithSDKReference(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		return response(200, `{"id":"i","owner":"p","ref":"provider-custom","Name":"case-sensitive-custom","created_at":"2026-10-14T10:00:00.123"}`), nil
	})
	got, e := c.Get(context.Background(), imageRef(), types.CallOptions{})
	if e != nil || got.Data.Ref.ID != "i" || got.Data.Name != nil {
		t.Fatalf("%+v %v", got, e)
	}
	var props map[string]string
	if json.Unmarshal(got.Data.Properties.Reveal(), &props) != nil || props["ref"] != "provider-custom" || props["Name"] != "case-sensitive-custom" {
		t.Fatal("custom property lost")
	}
	instant, ok := got.Data.CreatedAt.Instant()
	if ok || !instant.IsZero() || got.Data.CreatedAt.String() != "2026-10-14T10:00:00.123" {
		t.Fatal("invented timestamp offset")
	}
}

func TestCallerSelectedIDRejectsInvalidInputBeforeHTTPAndRetainsTargetOnMismatch(t *testing.T) {
	calls := 0
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		calls++
		return response(201, `{"id":"foreign","status":"queued"}`), nil
	})
	for _, id := range []string{"", "name-instead-of-uuid", "../foreign"} {
		got, e := c.Create(context.Background(), images.CreateSpec{ID: &id}, types.CallOptions{})
		if e == nil || got.Receipt.Outcome != types.NotSent || calls != 0 {
			t.Fatalf("%+v %v calls=%d", got, e, calls)
		}
	}
	id := "11111111-2222-3333-4444-555555555555"
	got, e := c.Create(context.Background(), images.CreateSpec{ID: &id}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != id || got.Data.Ref != nil {
		t.Fatalf("%+v %v", got, e)
	}
}

func TestCustomFilterCannotOverridePaginationControls(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected HTTP"); return nil, nil })
	for _, key := range []string{"marker", "limit", "sort_key", "sort_dir", "tag", "size_min", "member_status"} {
		_, e := c.Iterate(images.ListOptions{Properties: map[string]types.ProtectedValue{key: types.Protect([]byte("override"))}}, pagination.Options{})
		if e == nil {
			t.Fatal("reserved query filter accepted", key)
		}
	}
}

func TestBaseAttributeComparisonFiltersStaySeparateFromPropertyAndControlNames(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("checksum") != "abc" || r.URL.Query().Get("min_ram") != "0" {
			t.Fatal(r.URL)
		}
		return response(200, `{"images":[]}`), nil
	})
	got, e := c.Collect(context.Background(), images.ListOptions{Filters: map[images.FilterField]types.ProtectedValue{"checksum": types.Protect([]byte("abc")), "min_ram": types.Protect([]byte("0"))}}, pagination.Options{})
	if e != nil || !got.Complete {
		t.Fatal(e)
	}
}

func TestProtectedMemberListBoundsAndUnknownWaitState(t *testing.T) {
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/members") {
			return response(200, `{"members":[{"member_id":"one","image_id":"i","status":"pending"},{"member_id":"two","image_id":"i","status":"accepted"}]}`), nil
		}
		return response(200, `{"id":"i","status":"new-provider-state"}`), nil
	})
	one, e := c.Members.Collect(context.Background(), imageRef(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || one.Complete || one.NextCursor == nil {
		t.Fatalf("%+v %v", one, e)
	}
	rest, e := c.Members.Collect(context.Background(), imageRef(), pagination.Options{Cursor: one.NextCursor})
	if e != nil || !rest.Complete || len(rest.Items) != 1 || rest.Items[0].MemberID != "two" {
		t.Fatalf("%+v %v", rest, e)
	}
	h, e := root.Resource(imageRef())
	if e != nil {
		t.Fatal(e)
	}
	got, e := h.Wait(context.Background(), "image.active")
	if types.ErrorCode(e) != types.StateUnknown || got.Complete || got.Last.ProviderState != "new-provider-state" {
		t.Fatalf("%+v %v", got, e)
	}
}
func TestImageCollectionRejectsForeignNextAndKeepsEarlierPrefix(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(200, `{"images":[{"id":"i"}],"next":"?marker=i"}`), nil
		}
		return response(200, `{"images":[],"next":"https://foreign.test/v2/images?marker=x"}`), nil
	})
	got, e := c.Collect(context.Background(), images.ListOptions{}, pagination.Options{})
	if e == nil || got.Complete || len(got.Items) != 1 || calls != 2 {
		t.Fatalf("%+v %v", got, e)
	}
}
func TestMalformedAcceptedImageKeepsIdentityAndKnownNoIDRecovery(t *testing.T) {
	calls := 0
	root, c := clients(t, func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(201, `{"id":"i","size":"bad"}`), nil
		}
		return response(201, `{}`), nil
	})
	got, e := c.Create(context.Background(), images.CreateSpec{}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != "i" {
		t.Fatalf("%+v %v", got, e)
	}
	missing, e := c.Create(context.Background(), images.CreateSpec{}, types.CallOptions{})
	if e == nil || missing.Receipt.Outcome != types.Accepted || len(missing.Receipt.ResourceRefs) != 0 {
		t.Fatal(e)
	}
	h, e := root.Resume(*missing.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	proof, e := h.Recover(context.Background())
	if e != nil || proof.ProofStatus != "conclusive" || calls != 2 {
		t.Fatalf("%+v %v", proof, e)
	}
}

func TestVersionedCatalogEndpointDoesNotDuplicateV2(t *testing.T) {
	calls := 0
	_, c := clientsAt(t, func(r *http.Request) (*http.Response, error) {
		calls++
		want := "/proxy/v2/images"
		if r.Method == "GET" {
			want += "/i"
		}
		if r.URL.Path != want {
			t.Fatal(r.URL.Path)
		}
		if r.Method == "POST" {
			return response(201, `{"id":"i","status":"queued"}`), nil
		}
		return response(200, `{"id":"i","status":"active"}`), nil
	}, "https://glance.test/proxy/v2")
	got, e := c.Create(context.Background(), images.CreateSpec{}, types.CallOptions{})
	if e != nil || got.Data.Ref.EndpointURL != "https://glance.test/proxy/v2" {
		t.Fatalf("%+v %v", got, e)
	}
	_, e = c.Get(context.Background(), *got.Data.Ref, types.CallOptions{})
	if e != nil || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestProxyEndpointReanchorsKnownImageNextPath(t *testing.T) {
	calls := 0
	_, c := clientsAt(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/proxy/v2/images" {
			t.Fatal(r.URL.Path)
		}
		if r.URL.Query().Get("marker") == "i" {
			return response(200, `{"images":[]}`), nil
		}
		return response(200, `{"images":[{"id":"i"}],"next":"https://glance.test/v2/images?marker=i"}`), nil
	}, "https://glance.test/proxy/v2")
	got, e := c.Collect(context.Background(), images.ListOptions{}, pagination.Options{})
	if e != nil || !got.Complete || len(got.Items) != 1 || calls != 2 {
		t.Fatalf("%+v %v calls=%d", got, e, calls)
	}
}

func TestKnownRejectedAndNotSentMetadataReceiptsRecoverWithoutIO(t *testing.T) {
	for _, invalidID := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "not-sent"}[invalidID], func(t *testing.T) {
			calls := 0
			root, c := clients(t, func(*http.Request) (*http.Response, error) { calls++; return response(403, `{}`), nil })
			spec := images.CreateSpec{}
			if invalidID {
				empty := ""
				spec.ID = &empty
			}
			got, e := c.Create(context.Background(), spec, types.CallOptions{})
			if e == nil {
				t.Fatal("expected failure")
			}
			before := calls
			h, e := root.Resume(*got.Receipt)
			if e != nil {
				t.Fatal(e)
			}
			proof, e := h.Recover(context.Background())
			if e != nil || proof.ProofStatus != "conclusive" || proof.Receipt.Outcome != got.Receipt.Outcome || calls != before {
				t.Fatalf("%+v %v", proof, e)
			}
		})
	}
}

func TestPinnedSelectelImageQuotaAndRateLimitRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   types.Code
	}{{413, types.QuotaExceeded}, {429, types.RateLimited}} {
		calls := 0
		_, c := clients(t, func(*http.Request) (*http.Response, error) {
			calls++
			return response(tc.status, `{"message":"private-provider-message"}`), nil
		})
		got, e := c.Create(context.Background(), images.CreateSpec{}, types.CallOptions{})
		if types.ErrorCode(e) != tc.want || got.Receipt.Outcome != types.Rejected || calls != 1 || strings.Contains(e.Error(), "private-provider-message") {
			t.Fatalf("%+v %v", got, e)
		}
	}
}

func TestExplicitEmptyTagFilterRemainsPresent(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		values, ok := r.URL.Query()["tag"]
		if !ok || len(values) != 1 || values[0] != "" {
			t.Fatal("explicit empty tag omitted")
		}
		return response(200, `{"images":[]}`), nil
	})
	_, e := c.Collect(context.Background(), images.ListOptions{Tags: []string{""}}, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
}
