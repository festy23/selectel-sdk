package pagination_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

func TestSensitiveCursorExplicitExportAndResume(t *testing.T) {
	secret := types.Protect([]byte("synthetic-private-continuation"))
	fetch := func(_ context.Context, cursor string, _ uint) (pagination.SourcePage[int], error) {
		if cursor == "" {
			return pagination.SourcePage[int]{Items: []int{1}, SensitiveNext: &secret, Metadata: metadata()}, nil
		}
		if cursor != string(secret.Reveal()) {
			t.Fatal("continuation changed")
		}
		return pagination.SourcePage[int]{Items: []int{2, 3}, Done: true, Metadata: metadata()}, nil
	}
	i, e := pagination.New(plan(), pagination.Options{MaxItems: 1}, fetch)
	if e != nil {
		t.Fatal(e)
	}
	page, e := i.Next(context.Background())
	if types.ErrorCode(e) != types.Incomplete || page.NextCursor == nil {
		t.Fatal("cursor absent")
	}
	raw, _ := json.Marshal(page.NextCursor)
	if strings.Contains(string(raw), "private-continuation") || strings.Contains(fmt.Sprintf("%+v", page.NextCursor), "private-continuation") {
		t.Fatal("cursor leaked")
	}
	exported, e := page.NextCursor.Export()
	if e != nil || !strings.Contains(string(exported), "private-continuation") {
		t.Fatal("explicit export failed")
	}
	var imported pagination.Cursor
	if json.Unmarshal(exported, &imported) != nil {
		t.Fatal("export cannot resume")
	}
	if strings.Contains(fmt.Sprintf("%#v", imported), "private-continuation") {
		t.Fatal("imported cursor formatting leaked")
	}
	i, e = pagination.New(plan(), pagination.Options{Cursor: &imported, MaxItems: 1}, fetch)
	if e != nil {
		t.Fatal(e)
	}
	partial, e := i.Next(context.Background())
	if types.ErrorCode(e) != types.Incomplete || partial.NextCursor.SensitiveWire == nil || partial.NextCursor.Skip != 1 {
		t.Fatal("sensitive partial-page input lost")
	}
	i, e = pagination.New(plan(), pagination.Options{Cursor: partial.NextCursor}, fetch)
	if e != nil {
		t.Fatal(e)
	}
	rest, e := i.Collect(context.Background())
	if e != nil || !rest.Complete || len(rest.Items) != 1 || rest.Items[0] != 3 {
		t.Fatal("sensitive resume duplicated/lost records")
	}
}

func plan() pagination.Plan {
	return pagination.Plan{OperationID: "service.v1.items.list", ContractRevision: "revision", EndpointID: "endpoint", APIGeneration: "v1", Scope: types.Scope{AccountID: "account", ProjectID: "project"}, SemanticRequest: map[string]string{"filter": "selected"}, Defaults: pagination.Options{PageSize: 2, MaxItems: 100, MaxPages: 10}, CollectionTimeout: time.Second}
}
func metadata() types.Metadata {
	p := plan()
	return types.Metadata{Scope: p.Scope, EndpointID: p.EndpointID, APIGeneration: p.APIGeneration}
}

func TestBoundedPartialPageCanResume(t *testing.T) {
	fetch := func(context.Context, string, uint) (pagination.SourcePage[int], error) {
		return pagination.SourcePage[int]{Items: []int{1, 2, 3, 4, 5}, Done: true, Metadata: metadata()}, nil
	}
	i, err := pagination.New(plan(), pagination.Options{MaxItems: 2}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	first, err := i.Collect(context.Background())
	if types.ErrorCode(err) != types.Incomplete || first.Complete || len(first.Items) != 2 || first.StopReason != "max_items" || first.NextCursor == nil || first.NextCursor.Skip != 2 {
		t.Fatal("bounded prefix claimed complete")
	}
	i, err = pagination.New(plan(), pagination.Options{Cursor: first.NextCursor, MaxItems: 3}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	rest, err := i.Collect(context.Background())
	if err != nil || !rest.Complete || len(rest.Items) != 3 || rest.Items[0] != 3 || rest.NextCursor != nil {
		t.Fatal("resume duplicated or lost items")
	}
}

func TestCursorIsBoundToScopeEndpointAndRequest(t *testing.T) {
	calls := 0
	fetch := func(context.Context, string, uint) (pagination.SourcePage[int], error) {
		calls++
		return pagination.SourcePage[int]{Items: []int{1, 2}, Done: true, Metadata: metadata()}, nil
	}
	i, _ := pagination.New(plan(), pagination.Options{MaxItems: 1}, fetch)
	page, _ := i.Next(context.Background())
	for _, change := range []func(*pagination.Plan){func(p *pagination.Plan) { p.Scope.ProjectID = "other" }, func(p *pagination.Plan) { p.EndpointID = "other" }, func(p *pagination.Plan) { p.SemanticRequest = map[string]string{"filter": "other"} }} {
		p := plan()
		change(&p)
		if _, err := pagination.New(p, pagination.Options{Cursor: page.NextCursor}, fetch); types.ErrorCode(err) != types.ScopeMismatch {
			t.Fatal("foreign cursor accepted")
		}
	}
	if calls != 1 {
		t.Fatal("cursor validation performed I/O")
	}
}

func TestEmptyPageWithContinuationIsNotEnd(t *testing.T) {
	calls := 0
	fetch := func(_ context.Context, cursor string, _ uint) (pagination.SourcePage[int], error) {
		calls++
		if cursor == "" {
			return pagination.SourcePage[int]{Items: []int{}, Next: "next", Metadata: metadata()}, nil
		}
		return pagination.SourcePage[int]{Items: []int{7}, Done: true, Metadata: metadata()}, nil
	}
	i, _ := pagination.New(plan(), pagination.Options{}, fetch)
	out, err := i.Collect(context.Background())
	if err != nil || !out.Complete || calls != 2 || len(out.Items) != 1 {
		t.Fatal("empty page ended collection")
	}
	if _, err := i.Next(context.Background()); err != io.EOF || calls != 2 {
		t.Fatal("terminal iterator performed I/O")
	}
}

func TestPageLimitAndReadFailurePreservePartialItems(t *testing.T) {
	for _, fail := range []bool{false, true} {
		calls := 0
		fetch := func(_ context.Context, cursor string, _ uint) (pagination.SourcePage[int], error) {
			calls++
			if calls == 2 && fail {
				return pagination.SourcePage[int]{}, types.NewError(types.PermissionDenied)
			}
			return pagination.SourcePage[int]{Items: []int{calls}, Next: cursor + "n", Metadata: metadata()}, nil
		}
		options := pagination.Options{MaxPages: 2}
		if fail {
			options.MaxPages = 3
		}
		i, _ := pagination.New(plan(), options, fetch)
		out, err := i.Collect(context.Background())
		if out.Complete || err == nil || out.Pages != 2 || len(out.Items) == 0 {
			t.Fatal("partial data lost or hidden")
		}
		if fail && types.ErrorCode(err) != types.PermissionDenied {
			t.Fatal("read error converted to empty success")
		}
	}
}

func TestRepeatedCursorWithoutEndRuleFails(t *testing.T) {
	i, _ := pagination.New(plan(), pagination.Options{}, func(context.Context, string, uint) (pagination.SourcePage[int], error) {
		return pagination.SourcePage[int]{Items: []int{}, Next: "", Metadata: metadata()}, nil
	})
	if _, err := i.Next(context.Background()); types.ErrorCode(err) != types.ProtocolError {
		t.Fatal("no-progress cursor loop allowed")
	}
}
