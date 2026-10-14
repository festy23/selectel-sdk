package images

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

func memberRef(ep types.EndpointBinding, imageID, memberID string) *types.ResourceRef {
	r := core.Ref(ep, "images", "image_member", memberID)
	if r != nil {
		r.Parent = core.Ref(ep, "images", "image", imageID)
	}
	return r
}
func decodeMember(imageID, memberID string) core.Decoder[Member] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[Member], error) {
		out := core.Decoded[Member]{}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return out, types.NewError(types.ProtocolError)
		}
		base := map[string]json.RawMessage{}
		for _, key := range []string{"member_id", "image_id", "status", "created_at", "updated_at", "schema"} {
			if value, ok := fields[key]; ok {
				base[key] = value
			}
		}
		raw, _ = json.Marshal(base)

		var identity struct {
			MemberID string `json:"member_id"`
			ImageID  string `json:"image_id"`
		}
		if json.Unmarshal(raw, &identity) == nil && identity.MemberID != "" && identity.ImageID == imageID && (memberID == "" || memberID == identity.MemberID) {
			out.Refs = []types.ResourceRef{*memberRef(ep, imageID, identity.MemberID)}
		}
		var v Member
		if json.Unmarshal(raw, &v) != nil || v.MemberID == "" || v.ImageID == "" {
			return out, types.NewError(types.ProtocolError)
		}
		if v.ImageID != imageID || (memberID != "" && v.MemberID != memberID) {
			return out, types.NewError(types.ScopeMismatch)
		}
		v.Ref = memberRef(ep, imageID, v.MemberID)
		out.Data = v
		return out, nil
	}
}
func (m *Members) Create(ctx context.Context, image types.ResourceRef, project string, o types.CallOptions) (types.Result[Member], error) {
	target := types.ResourceRef{}
	if project != "" {
		target = *memberRef(*pin(image), image.ID, project)
	}
	raw, _ := json.Marshal(struct {
		Member string `json:"member"`
	}{project})
	return core.DoJSON(m.client.runtime, ctx, operation("members.create", "POST"), core.Request{Scope: m.client.scope, Path: "/v2/images/" + url.PathEscape(image.ID) + "/members", Body: raw, Refs: []types.ResourceRef{target}, PinnedEndpoint: pin(image), Options: o, Validate: func() error {
		if image.Scope != m.client.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if project == "" {
			return invalid()
		}
		return validateRef(image, "image")
	}}, decodeMember(image.ID, project))
}
func memberPath(r types.ResourceRef) string {
	parent := ""
	if r.Parent != nil {
		parent = r.Parent.ID
	}
	return "/v2/images/" + url.PathEscape(parent) + "/members/" + url.PathEscape(r.ID)
}
func (m *Members) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Member], error) {
	imageID := ""
	if r.Parent != nil {
		imageID = r.Parent.ID
	}
	return core.DoJSON(m.client.runtime, ctx, operation("members.get", "GET"), core.Request{Scope: m.client.scope, Path: memberPath(r), Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error { return validateRef(r, "image_member") }}, decodeMember(imageID, r.ID))
}
func (m *Members) Update(ctx context.Context, r types.ResourceRef, status string, o types.CallOptions) (types.Result[Member], error) {
	imageID := ""
	if r.Parent != nil {
		imageID = r.Parent.ID
	}
	raw, _ := json.Marshal(struct {
		Status string `json:"status"`
	}{status})
	return core.DoJSON(m.client.runtime, ctx, operation("members.update", "PUT"), core.Request{Scope: m.client.scope, Path: memberPath(r), Body: raw, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error {
		if status != "pending" && status != "accepted" && status != "rejected" {
			return invalid()
		}
		return validateRef(r, "image_member")
	}}, decodeMember(imageID, r.ID))
}
func (m *Members) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return core.DoJSON(m.client.runtime, ctx, operation("members.delete", "DELETE"), core.Request{Scope: m.client.scope, Path: memberPath(r), Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error { return validateRef(r, "image_member") }}, func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}
func (m *Members) Iterate(image types.ResourceRef, o pagination.Options) (*pagination.Iterator[Member], error) {
	if e := validateRef(image, "image"); e != nil {
		return nil, e
	}
	if image.Scope != m.client.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	plan, e := m.client.runtime.PagePlan("images.v2.members.list", "v2", m.client.scope, image.EndpointID, image)
	if e != nil {
		return nil, e
	}
	return pagination.New(plan, o, func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[Member], error) {
		result, e := core.DoJSON(m.client.runtime, ctx, operation("members.list", "GET"), core.Request{Scope: m.client.scope, Path: "/v2/images/" + url.PathEscape(image.ID) + "/members", Refs: []types.ResourceRef{image}, PinnedEndpoint: pin(image)}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]Member], error) {
			var body struct {
				Members []json.RawMessage `json:"members"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Members == nil {
				return core.Decoded[[]Member]{}, types.NewError(types.ProtocolError)
			}
			rows := []Member{}
			for _, raw := range body.Members {
				v, e := decodeMember(image.ID, "")(raw, ep)
				if e != nil {
					return core.Decoded[[]Member]{}, e
				}
				rows = append(rows, v.Data)
			}
			return core.Decoded[[]Member]{Data: rows}, nil
		})
		return pagination.SourcePage[Member]{Items: result.Data, Done: true, Metadata: result.Metadata}, e
	})
}
func (m *Members) Collect(ctx context.Context, image types.ResourceRef, o pagination.Options) (pagination.Collection[Member], error) {
	it, e := m.Iterate(image, o)
	if e != nil {
		return pagination.Collection[Member]{Items: []Member{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (m *Members) ListPage(ctx context.Context, image types.ResourceRef, o pagination.Options) (pagination.Page[Member], error) {
	it, e := m.Iterate(image, o)
	if e != nil {
		return pagination.Page[Member]{}, e
	}
	return it.Next(ctx)
}
