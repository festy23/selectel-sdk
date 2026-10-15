package registry_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/registry"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type managementTransport func(*http.Request) (*http.Response, error)

func (f managementTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func managementResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func managementClient(t *testing.T, wire managementTransport) (*sdk.Client, *registry.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	c, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Scope != scope || r.Service != "registry" || r.Mode != types.IAMProject {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: managementTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			raw, _ := json.Marshal(map[string]any{"token": map[string]any{"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "user": map[string]string{"id": "subject"}, "catalog": []any{}, "project": map[string]any{"id": "p", "domain": map[string]string{"name": "a"}}}})
			v := managementResponse(200, string(raw))
			v.Header.Set("X-Subject-Token", "synthetic-token")
			return v, nil
		}
		if r.URL.Host != "cr.selcloud.ru" || r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal("wrong registry auth/endpoint", r.URL)
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("p")
	return c, p.Registry()
}
func TestGarbageCollectionUsesExistingLifecycleRefAndExplicitFalse(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.URL.Path {
		case "/api/v1/registries":
			return managementResponse(201, `{"id":"reg","name":"registry"}`), nil
		case "/api/v1/registries/reg/garbage-collection":
			if r.Method != "POST" || r.URL.Query().Get("delete-untagged") != "false" {
				t.Fatal(r.Method, r.URL)
			}
			return managementResponse(201, ""), nil
		case "/api/v1/registries/reg/garbage-collection/size":
			return managementResponse(200, `{"sizeNonReferenced":9007199254740993,"sizeUntagged":0,"sizeSummary":9007199254740993}`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	existing, e := c.V1.Registries.Create(context.Background(), registry.CreateRequest{Name: "registry"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	disabled := false
	out, e := c.V1.Registries.StartGarbageCollection(context.Background(), *existing.Data.Ref, registry.GarbageCollectionOptions{DeleteUntagged: &disabled}, types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted || out.Receipt.ResourceRefs[0].ID != "reg" {
		t.Fatal(e)
	}
	size, e := c.V1.Registries.GarbageCollectionSize(context.Background(), *existing.Data.Ref, types.CallOptions{})
	if e != nil || size.Data.SizeSummary.String() != "9007199254740993" || calls != 3 {
		t.Fatal(e, calls)
	}
}

func TestRepositoryExactParentAndLifecycleRemainCompatible(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/api/v1/registries" {
			return managementResponse(201, `{"id":"reg","name":"registry"}`), nil
		}
		if r.URL.EscapedPath() != "/api/v1/registries/reg/repositories/team%2Frepo" {
			t.Fatal(r.URL)
		}
		if r.Method == "DELETE" {
			return managementResponse(204, ""), nil
		}
		return managementResponse(200, `{"name":"team/repo","size":9007199254740993,"type":"image","updatedAt":null}`), nil
	})
	created, e := c.V1.Registries.Create(context.Background(), registry.CreateRequest{Name: "registry"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	repo, e := c.V1.Repositories().Ref(*created.Data.Ref, "team/repo")
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.V1.Repositories().Get(context.Background(), repo, types.CallOptions{})
	if e != nil || out.Data.Ref == nil || out.Data.Ref.Parent.ID != "reg" || out.Data.Size.String() != "9007199254740993" || out.Data.UpdatedAt == nil {
		t.Fatal(e)
	}
	if _, valid := out.Data.UpdatedAt.Value(); valid {
		t.Fatal("nullable timestamp lost")
	}
	removed, e := c.V1.Repositories().Delete(context.Background(), repo, types.CallOptions{})
	if e != nil || removed.Receipt.Outcome != types.Accepted || removed.Receipt.ResourceRefs[0].Parent.ID != "reg" {
		t.Fatal(e)
	}
	wrong := repo
	wrong.Parent = &types.ResourceRef{}
	_, e = c.V1.Repositories().Get(context.Background(), wrong, types.CallOptions{})
	if e == nil || calls != 3 {
		t.Fatal(e, calls)
	}
}

func TestCleanupPreservesFalseAndProtectedPerReferenceFailures(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/api/v1/registries" {
			return managementResponse(201, `{"id":"reg","name":"registry"}`), nil
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/registries/reg/repositories/repo/cleanup" {
			t.Fatal(r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"digests":[],"disable_gc":false,"tags":["release"]}` {
			t.Fatal(string(b))
		}
		return managementResponse(200, `{"deleted":[{"digest":"opaque-digest","tags":["release"]}],"failed":[{"digest":"other","error":"private-provider-failure","tags":[]}]}`), nil
	})
	created, _ := c.V1.Registries.Create(context.Background(), registry.CreateRequest{Name: "registry"}, types.CallOptions{})
	repo, _ := c.V1.Repositories().Ref(*created.Data.Ref, "repo")
	digests := []string{}
	tags := []string{"release"}
	disabled := false
	out, e := c.V1.Repositories().Cleanup(context.Background(), repo, registry.CleanupOptions{Digests: &digests, Tags: &tags, DisableGC: &disabled}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProviderError || out.Receipt.Outcome != types.Accepted || out.Data.Deleted == nil || len(*out.Data.Deleted) != 1 || out.Data.Failed == nil || len(*out.Data.Failed) != 1 || calls != 2 {
		t.Fatal(e, calls)
	}
	text := fmt.Sprintf("%+v %#v", out, &out)
	encoded, _ := json.Marshal(out)
	if strings.Contains(text, "private-provider-failure") || strings.Contains(string(encoded), "private-provider-failure") {
		t.Fatal("cleanup error disclosure")
	}
}

func TestOpaqueImageReferenceAndManifestParentsAreNotInterchanged(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/api/v1/registries" {
			return managementResponse(201, `{"id":"reg"}`), nil
		}
		if r.Method == "GET" {
			if r.URL.EscapedPath() != "/api/v1/registries/reg/repositories/repo/tag%20%2F%3F%23" {
				t.Fatal(r.URL)
			}
			return managementResponse(200, `{"digest":"opaque-resolved-digest","size":9007199254740993,"tags":["tag /?#"],"layers":[]}`), nil
		}
		if r.Method != "DELETE" || r.URL.Path != "/api/v1/registries/reg/repositories/repo/opaque-digest" {
			t.Fatal(r.Method, r.URL)
		}
		return managementResponse(204, ""), nil
	})
	created, _ := c.V1.Registries.Create(context.Background(), registry.CreateRequest{Name: "registry"}, types.CallOptions{})
	repo, _ := c.V1.Repositories().Ref(*created.Data.Ref, "repo")
	alias, e := c.V1.Images().Reference(repo, "tag /?#")
	if e != nil {
		t.Fatal(e)
	}
	layers, e := c.V1.Images().Layers(context.Background(), alias, types.CallOptions{})
	if e != nil || layers.Data.Ref.ID != alias.ID || layers.Data.Ref.Parent.ID != "repo" || layers.Data.Ref.Parent.Parent.ID != "reg" || *layers.Data.Digest != "opaque-resolved-digest" {
		t.Fatal(e)
	}
	denied, e := c.V1.Images().DeleteManifest(context.Background(), alias, types.CallOptions{})
	if e == nil || denied.Receipt.Outcome != types.NotSent || calls != 2 {
		t.Fatal(e, calls)
	}
	manifest, _ := c.V1.Images().Manifest(repo, "opaque-digest")
	deleted, e := c.V1.Images().DeleteManifest(context.Background(), manifest, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 3 {
		t.Fatal(e, calls)
	}
}

func TestRepositoryImagesAndTagsAreBoundedParentScopedLists(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if len(r.URL.RawQuery) != 0 {
			t.Fatal("unpaginated source received paging", r.URL)
		}
		switch r.URL.EscapedPath() {
		case "/api/v1/registries":
			return managementResponse(201, `{"id":"reg"}`), nil
		case "/api/v1/registries/reg/repositories/team%2Frepo/images":
			return managementResponse(200, `[{"digest":"sha256:first","tags":["latest"],"createdAt":"2025-01-02T03:04:05+03:00"},{"digest":"sha256:second"}]`), nil
		case "/api/v1/registries/reg/repositories/team%2Frepo/tags":
			return managementResponse(200, `["latest","a/b?#"]`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	ctx := context.Background()
	created, e := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	repo, e := c.V1.Repositories().Ref(*created.Data.Ref, "team/repo")
	if e != nil {
		t.Fatal(e)
	}
	first, e := c.V1.Images().ListPage(ctx, repo, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || len(first.Items) != 1 || first.Done || first.NextCursor == nil || first.NextCursor.Skip != 1 {
		t.Fatal(first, e)
	}
	if first.Items[0].Ref == nil || first.Items[0].Ref.Kind != "manifest" || first.Items[0].Ref.Parent.ID != "team/repo" || first.Items[0].CreatedAt.Format(time.RFC3339) != "2025-01-02T00:04:05Z" {
		t.Fatal(first.Items)
	}
	rest, e := c.V1.Images().Collect(ctx, repo, pagination.Options{Cursor: first.NextCursor})
	if e != nil || !rest.Complete || len(rest.Items) != 1 || rest.Items[0].Ref.ID != "sha256:second" {
		t.Fatal(rest, e)
	}
	tags, e := c.V1.Repositories().Tags(ctx, repo, pagination.Options{})
	if e != nil || !tags.Complete || len(tags.Items) != 2 || tags.Items[1] != "a/b?#" {
		t.Fatal(tags, e)
	}
	if calls != 4 {
		t.Fatal(calls)
	}
}

func TestIntegratedClustersRetainSourceCountWithoutInventingPagination(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/api/v1/registries" {
			return managementResponse(201, `{"id":"reg"}`), nil
		}
		if r.URL.Path != "/api/v1/registries/reg/clusters" || r.URL.RawQuery != "" {
			t.Fatal(r.URL)
		}
		return managementResponse(200, `{"clusters":[{"id":"cluster","name":"one","pool":"ru-9"}],"totalCount":1}`), nil
	})
	ctx := context.Background()
	v, e := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	list, e := c.V1.Registries.Clusters(ctx, *v.Data.Ref, pagination.Options{})
	if e != nil || !list.Complete || len(list.Items) != 1 || *list.Items[0].Pool != "ru-9" || list.ProviderPages[0].TotalCount.String() != "1" || calls != 2 {
		t.Fatal(list, e, calls)
	}
}

func TestRepositoryListV1PinsParentAndOriginalFilters(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/api/v1/registries" {
			return managementResponse(201, `{"id":"reg"}`), nil
		}
		q := r.URL.Query()
		if r.URL.Path != "/api/v1/registries/reg/repositories" || q.Get("search") != "team" || q.Get("sort_field") != "name" || q.Get("short") != "false" || q.Get("limit") != "1" {
			t.Fatal(r.URL)
		}
		if q.Get("offset") == "0" {
			return managementResponse(200, `[{"name":"team/repo","type":"image"}]`), nil
		}
		if q.Get("offset") == "1" {
			return managementResponse(200, `[]`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	ctx := context.Background()
	v, e := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	search, sort, short := "team", "name", false
	f := registry.RepositoryListOptions{Search: &search, SortField: &sort, Short: &short}
	it, e := c.V1.Repositories().Iterate(*v.Data.Ref, f, pagination.Options{PageSize: 1})
	if e != nil {
		t.Fatal(e)
	}
	search = "changed"
	sort = "size"
	short = true
	p, e := it.Next(ctx)
	if e != nil || len(p.Items) != 1 || p.Done || p.Items[0].Ref.Parent.ID != "reg" || p.Items[0].Ref.ID != "team/repo" {
		t.Fatal(p, e)
	}
	last, e := it.Next(ctx)
	if e != nil || !last.Done || calls != 3 {
		t.Fatal(last, e, calls)
	}
	_, e = c.V1.Repositories().Iterate(*v.Data.Ref, registry.RepositoryListOptions{Short: &short, SortField: &sort}, pagination.Options{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 3 {
		t.Fatal(e, calls)
	}
}

func TestRegistryV2ListsUseFilteredCountsAndGenerationParents(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		switch r.URL.EscapedPath() {
		case "/api/v2/registries":
			return managementResponse(200, `{"registries":[{"id":"reg","name":"r","createdAt":"2025-01-02T00:00:00Z","size":0,"status":"active"}],"totalCount":1}`), nil
		case "/api/v2/registries/reg/repositories":
			if q.Get("search") != "team" || q.Get("offset") != "0" {
				t.Fatal(r.URL)
			}
			return managementResponse(200, `{"repositories":[{"name":"team/repo"}],"totalCount":100,"filteredCount":1}`), nil
		case "/api/v2/registries/reg/repositories/team%2Frepo/images":
			if q.Get("search") != "stable" || q.Get("limit") != "2" {
				t.Fatal(r.URL)
			}
			return managementResponse(200, `{"images":[{"digest":"sha256:first","tags":["stable"]}],"totalCount":10,"filteredCount":1}`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	ctx := context.Background()
	rs, e := c.V2.Registries.Collect(ctx, registry.ListOptions{}, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
	search := "team"
	repos, e := c.V2.Repositories().Collect(ctx, *rs.Items[0].Ref, registry.RepositoryListOptions{Search: &search}, pagination.Options{PageSize: 2})
	if e != nil || !repos.Complete || len(repos.Items) != 1 || repos.ProviderPages[0].TotalCount.String() != "100" || repos.ProviderPages[0].ReportedCount.String() != "1" || repos.Items[0].Ref.APIGeneration != "v2" {
		t.Fatal(repos, e)
	}
	search = "stable"
	images, e := c.V2.Images().Collect(ctx, *repos.Items[0].Ref, registry.ImageListOptions{Search: &search}, pagination.Options{PageSize: 2})
	if e != nil || !images.Complete || len(images.Items) != 1 || images.Items[0].Ref.Parent.ID != "team/repo" || images.Items[0].Ref.Parent.Parent.ID != "reg" || calls != 3 {
		t.Fatal(images, e, calls)
	}
	wrong := *repos.Items[0].Ref
	wrong.APIGeneration = "v1"
	_, e = c.V2.Images().Iterate(wrong, registry.ImageListOptions{}, pagination.Options{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 3 {
		t.Fatal(e, calls)
	}
}

func TestExplicitNativeRegistryReadVariantsDoNotFallback(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.URL.Path {
		case "/api/v1/registries":
			return managementResponse(201, `{"id":"reg"}`), nil
		case "/api/v1/registries/reg/repositories":
			return managementResponse(204, ``), nil
		case "/api/v1/registries/reg/repositories/repo/tag":
			return managementResponse(200, `[{"digest":"sha256:layer","size":9007199254740993}]`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	ctx := context.Background()
	v, e := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.V1.Repositories().Collect(ctx, *v.Data.Ref, registry.RepositoryListOptions{}, pagination.Options{})
	if e == nil {
		t.Fatal("current API unexpectedly accepted native-only204")
	}
	native, e := c.V1.Repositories().Collect(ctx, *v.Data.Ref, registry.RepositoryListOptions{NativeEmpty204: true}, pagination.Options{})
	if e != nil || !native.Complete || len(native.Items) != 0 {
		t.Fatal(native, e)
	}
	repo, _ := c.V1.Repositories().Ref(*v.Data.Ref, "repo")
	image, _ := c.V1.Images().Reference(repo, "tag")
	_, e = c.V1.Images().Layers(ctx, image, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError {
		t.Fatal(e)
	}
	layers, e := c.V1.Images().NativeLayers(ctx, image, types.CallOptions{})
	if e != nil || len(layers.Data) != 1 || layers.Data[0].Size.String() != "9007199254740993" || calls != 5 {
		t.Fatal(layers, e, calls)
	}
}

func TestRegistryMalformedImageListPreservesValidatedPrefix(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return managementResponse(201, `{"id":"reg"}`), nil
		}
		return managementResponse(200, `[{"digest":"sha256:ok"},{"digest":"sha256:bad","createdAt":"naive"}]`), nil
	})
	ctx := context.Background()
	v, e := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	repo, _ := c.V1.Repositories().Ref(*v.Data.Ref, "repo")
	it, e := c.V1.Images().Iterate(repo, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
	page, e := it.Next(ctx)
	if types.ErrorCode(e) != types.ProtocolError || len(page.Items) != 1 || page.Done || page.Items[0].Ref.ID != "sha256:ok" {
		t.Fatal(page, e)
	}
	_, e = it.Next(ctx)
	if types.ErrorCode(e) != types.ProtocolError || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestRegistryResourceResumeObserveExactTargetsWithoutReplay(t *testing.T) {
	calls := 0
	root, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/registries":
			return managementResponse(201, `{"id":"reg"}`), nil
		case "DELETE /api/v1/registries/reg/repositories/repo/sha256:opaque":
			return nil, io.ErrUnexpectedEOF
		case "GET /api/v1/registries/reg/repositories/repo/sha256:opaque":
			return managementResponse(200, `{"digest":"sha256:opaque"}`), nil
		case "GET /api/v1/registries/reg/repositories/repo":
			return managementResponse(200, `{"name":"repo"}`), nil
		}
		t.Fatal(r.Method, r.URL)
		return nil, nil
	})
	ctx := context.Background()
	v, e := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	repo, _ := c.V1.Repositories().Ref(*v.Data.Ref, "repo")
	h, e := root.Resource(repo)
	if e != nil {
		t.Fatal(e)
	}
	obs, e := h.Status(ctx)
	if e != nil || obs.Classification != "succeeded" || obs.ProviderState != "present" || obs.Ref.ID != "repo" {
		t.Fatal(obs, e)
	}
	manifest, _ := c.V1.Images().Manifest(repo, "sha256:opaque")
	result, e := c.V1.Images().DeleteManifest(ctx, manifest, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || result.Receipt == nil || result.Receipt.Outcome != types.Unknown {
		t.Fatal(result, e)
	}
	h, e = root.Resume(*result.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	observed, e := h.Recover(ctx)
	if types.ErrorCode(e) != types.UnknownOutcome || observed.ProofStatus != "indeterminate" || observed.Observation == nil || observed.Observation.Ref.ID != "sha256:opaque" || calls != 4 {
		t.Fatal(observed, e, calls)
	}
	invalid := repo
	invalid.ID = ""
	notSent, e := c.V1.Repositories().Delete(ctx, invalid, types.CallOptions{})
	if e == nil || notSent.Receipt == nil {
		t.Fatal(notSent, e)
	}
	h, e = root.Resume(*notSent.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	terminal, e := h.Recover(ctx)
	if e != nil || terminal.ProofStatus != "conclusive" || calls != 4 {
		t.Fatal(terminal, e, calls)
	}
}

func TestRegistryManagementRejectsInvalidAncestryAndRetainsMutationReceipt(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return managementResponse(201, `{"id":"reg"}`), nil
		}
		return managementResponse(409, `{"error":"synthetic-provider-detail"}`), nil
	})
	ctx := context.Background()
	v, e := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	repo, _ := c.V1.Repositories().Ref(*v.Data.Ref, "repo")
	for _, change := range []func(*types.ResourceRef){func(r *types.ResourceRef) { r.Scope.ProjectID = "other" }, func(r *types.ResourceRef) { r.Service = "other" }, func(r *types.ResourceRef) { r.APIGeneration = "v2" }, func(r *types.ResourceRef) { r.Parent = nil }, func(r *types.ResourceRef) { r.EndpointURL = "https://wrong.example/api" }, func(r *types.ResourceRef) { r.ID = "" }} {
		bad := repo
		change(&bad)
		_, e = c.V1.Repositories().Get(ctx, bad, types.CallOptions{})
		if e == nil || calls != 1 {
			t.Fatal(e, calls)
		}
	}
	failed, e := c.V1.Repositories().Delete(ctx, repo, types.CallOptions{})
	if types.ErrorCode(e) != types.Conflict || failed.Receipt == nil || failed.Receipt.Outcome != types.Rejected || failed.Receipt.ResourceRefs[0].ID != "repo" || strings.Contains(fmt.Sprintf("%+v", e), "synthetic-provider-detail") || calls != 2 {
		t.Fatal(failed, e, calls)
	}
}

func TestRegistryImageIteratorOwnsParentSnapshot(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return managementResponse(201, `{"id":"reg"}`), nil
		}
		if r.URL.Path != "/api/v1/registries/reg/repositories/repo/images" {
			t.Fatal(r.URL)
		}
		return managementResponse(200, `[{"digest":"sha256:ok"}]`), nil
	})
	ctx := context.Background()
	v, _ := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	repo, _ := c.V1.Repositories().Ref(*v.Data.Ref, "repo")
	it, e := c.V1.Images().Iterate(repo, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
	repo.Parent.ID = "changed"
	page, e := it.Next(ctx)
	if e != nil || len(page.Items) != 1 || page.Items[0].Ref.Parent.Parent.ID != "reg" {
		t.Fatal(page, e)
	}
}

func TestRegistryRepositoryMalformedListRetainsPrefix(t *testing.T) {
	for _, generation := range []string{"v1", "v2"} {
		t.Run(generation, func(t *testing.T) {
			calls := 0
			_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/api/v1/registries" {
					return managementResponse(201, `{"id":"reg"}`), nil
				}
				if r.URL.Path == "/api/v2/registries" {
					return managementResponse(200, `{"registries":[{"id":"reg","name":"r","createdAt":"2025-01-02T00:00:00Z","size":0,"status":"active"}],"totalCount":1}`), nil
				}
				if generation == "v1" {
					return managementResponse(200, `[{"name":"ok"},{"name":"bad","size":"no"}]`), nil
				}
				return managementResponse(200, `{"repositories":[{"name":"ok"},{"name":"bad","size":"no"}],"totalCount":2,"filteredCount":2}`), nil
			})
			ctx := context.Background()
			if generation == "v1" {
				v, _ := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
				p, e := c.V1.Repositories().ListPage(ctx, *v.Data.Ref, registry.RepositoryListOptions{}, pagination.Options{})
				if types.ErrorCode(e) != types.ProtocolError || len(p.Items) != 1 || p.Items[0].Ref.ID != "ok" || p.Done {
					t.Fatal(p, e)
				}
			} else {
				v, e := c.V2.Registries.Collect(ctx, registry.ListOptions{}, pagination.Options{})
				if e != nil {
					t.Fatal(e)
				}
				p, e := c.V2.Repositories().ListPage(ctx, *v.Items[0].Ref, registry.RepositoryListOptions{}, pagination.Options{})
				if types.ErrorCode(e) != types.ProtocolError || len(p.Items) != 1 || p.Items[0].Ref.ID != "ok" || p.Done {
					t.Fatal(p, e)
				}
			}
			if calls != 2 {
				t.Fatal(calls)
			}
		})
	}
}

func TestRegistryFixedListsRetainPrefixOnIncompleteProviderData(t *testing.T) {
	for _, kind := range []string{"tags", "tags-null", "clusters"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return managementResponse(201, `{"id":"reg"}`), nil
				}
				if strings.HasPrefix(kind, "tags") {
					if kind == "tags-null" {
						return managementResponse(200, `["good",null]`), nil
					}
					return managementResponse(200, `["good",42]`), nil
				}
				return managementResponse(200, `{"clusters":[{"id":"one","name":"one","pool":"ru-9"}],"totalCount":2}`), nil
			})
			ctx := context.Background()
			v, _ := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
			if strings.HasPrefix(kind, "tags") {
				repo, _ := c.V1.Repositories().Ref(*v.Data.Ref, "repo")
				p, e := c.V1.Repositories().Tags(ctx, repo, pagination.Options{})
				if types.ErrorCode(e) != types.ProtocolError || len(p.Items) != 1 || p.Items[0] != "good" || p.Complete {
					t.Fatal(p, e)
				}
			} else {
				p, e := c.V1.Registries.Clusters(ctx, *v.Data.Ref, pagination.Options{})
				if types.ErrorCode(e) != types.Incomplete || len(p.Items) != 1 || p.Complete || p.ProviderPages[0].TotalCount.String() != "2" {
					t.Fatal(p, e)
				}
			}
			if calls != 2 {
				t.Fatal(calls)
			}
		})
	}
}

func TestRegistryHandleValidatesReceiptBindingBeforeObservation(t *testing.T) {
	calls := 0
	root, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return managementResponse(201, `{"id":"reg"}`), nil
		}
		return managementResponse(204, ``), nil
	})
	ctx := context.Background()
	v, _ := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	repo, _ := c.V1.Repositories().Ref(*v.Data.Ref, "repo")
	deleted, e := c.V1.Repositories().Delete(ctx, repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*types.Receipt){func(r *types.Receipt) { r.CallID = "" }, func(r *types.Receipt) { r.SchemaVersion = 99 }, func(r *types.Receipt) { r.Outcome = "invented" }, func(r *types.Receipt) { r.ContractRevision = "stale" }, func(r *types.Receipt) { r.Scope.ProjectID = "wrong" }, func(r *types.Receipt) { r.ResourceRefs = nil }} {
		bad := *deleted.Receipt
		change(&bad)
		if _, e = c.Handle(repo, &bad); e == nil {
			t.Fatal("forged receipt accepted", bad)
		}
	}
	wrong := repo
	wrong.ID = "other"
	if _, e = c.Handle(wrong, deleted.Receipt); e == nil {
		t.Fatal("foreign target accepted")
	}
	if _, e = root.Resume(*deleted.Receipt); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestRegistryCurrentRepositoryListRejectsEmpty200(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return managementResponse(201, `{"id":"reg"}`), nil
		}
		return managementResponse(200, ``), nil
	})
	ctx := context.Background()
	v, _ := c.V1.Registries.Create(ctx, registry.CreateRequest{Name: "r"}, types.CallOptions{})
	p, e := c.V1.Repositories().Collect(ctx, *v.Data.Ref, registry.RepositoryListOptions{}, pagination.Options{})
	if types.ErrorCode(e) != types.ProtocolError || p.Complete || calls != 2 {
		t.Fatal(p, e, calls)
	}
}

func TestRegistryV2EmptyPagesRespectOffsetAndShrinkingCounts(t *testing.T) {
	for _, kind := range []string{"repositories", "images"} {
		for _, tc := range []struct {
			name, offset string
			count        int
			rows         string
			code         types.Code
			shrink       bool
		}{
			{name: "beyond_count", offset: "100", count: 10, rows: "[]"},
			{name: "at_count", offset: "10", count: 10, rows: "[]"},
			{name: "empty_before_count", offset: "5", count: 10, rows: "[]", code: types.Incomplete},
			{name: "nonempty_overrun", offset: "100", count: 10, rows: `[{"name":"repo","digest":"sha256:one"}]`, code: types.ProtocolError},
			{name: "count_shrinks_between_pages", offset: "0", count: 3, rows: `[{"name":"one","digest":"sha256:one"},{"name":"two","digest":"sha256:two"}]`, shrink: true},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				requests := 0
				_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
					if r.URL.Path == "/api/v2/registries" {
						return managementResponse(200, `{"registries":[{"id":"reg","name":"r","createdAt":"2025-01-02T00:00:00Z","size":0,"status":"active"}],"totalCount":1}`), nil
					}
					requests++
					expected := "/api/v2/registries/reg/repositories"
					if kind == "images" {
						expected += "/repo/images"
					}
					if r.URL.Path != expected {
						t.Fatal(r.URL)
					}
					rows, count, offset := tc.rows, tc.count, tc.offset
					if tc.shrink && requests == 2 {
						rows, count, offset = "[]", 1, "2"
					}
					if r.URL.Query().Get("offset") != offset {
						t.Fatal(r.URL)
					}
					return managementResponse(200, fmt.Sprintf(`{"%s":%s,"totalCount":%d,"filteredCount":%d}`, kind, rows, count, count)), nil
				})
				ctx := context.Background()
				registries, e := c.V2.Registries.Collect(ctx, registry.ListOptions{}, pagination.Options{})
				if e != nil {
					t.Fatal(e)
				}
				parent := *registries.Items[0].Ref
				offset, _ := types.ParseInteger(tc.offset)
				check := func(complete bool, n int, counts []pagination.ProviderMetadata, e error) {
					t.Helper()
					if types.ErrorCode(e) != tc.code || complete != (tc.code == "") {
						t.Fatal("completion", complete, n, e)
					}
					if tc.shrink {
						if n != 2 || requests != 2 || len(counts) != 2 || counts[0].ReportedCount.String() != "3" || counts[1].ReportedCount.String() != "1" {
							t.Fatal(n, requests, counts)
						}
					} else if requests != 1 {
						t.Fatal(requests)
					}
				}
				if kind == "repositories" {
					out, e := c.V2.Repositories().Collect(ctx, parent, registry.RepositoryListOptions{Offset: &offset}, pagination.Options{PageSize: 2})
					check(out.Complete, len(out.Items), out.ProviderPages, e)
				} else {
					repo, e := c.V2.Repositories().Ref(parent, "repo")
					if e != nil {
						t.Fatal(e)
					}
					out, e := c.V2.Images().Collect(ctx, repo, registry.ImageListOptions{Offset: &offset}, pagination.Options{PageSize: 2})
					check(out.Complete, len(out.Items), out.ProviderPages, e)
				}
			})
		}
	}
}
