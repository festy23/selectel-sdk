package filestorage

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"
)

// ShareFilters restrict observations within the explicit project view. Cross-
// project all_tenants/project_id selection is deliberately not inferred.
type ShareFilters struct {
	Status              *string               `json:"status,omitempty"`
	Name                *string               `json:"name,omitempty"`
	Description         *string               `json:"description,omitempty"`
	NameContains        *string               `json:"name~,omitempty"`
	DescriptionContains *string               `json:"description~,omitempty"`
	ShareServerID       *string               `json:"share_server_id,omitempty"`
	ShareTypeID         *string               `json:"share_type_id,omitempty"`
	SnapshotID          *string               `json:"snapshot_id,omitempty"`
	ShareNetworkID      *string               `json:"share_network_id,omitempty"`
	ShareGroupID        *string               `json:"share_group_id,omitempty"`
	ExportLocationID    *string               `json:"export_location_id,omitempty"`
	Host                *types.ProtectedValue `json:"host,omitempty"`
	ExportLocationPath  *types.ProtectedValue `json:"export_location_path,omitempty"`
	Metadata            *types.ProtectedJSON  `json:"metadata,omitempty"`
	ExtraSpecs          *types.ProtectedJSON  `json:"extra_specs,omitempty"`
	IsPublic            *bool                 `json:"is_public,omitempty"`
	WithCount           *bool                 `json:"with_count,omitempty"`
	IsSoftDeleted       *bool                 `json:"is_soft_deleted,omitempty"`
	SortKey             *string               `json:"sort_key,omitempty"`
	SortDirection       *string               `json:"sort_dir,omitempty"`
}

func (c *Client) ListShares(ctx context.Context, filters ShareFilters, options pagination.Options) (pagination.Collection[Share], error) {
	op := core.Operation{ID: "filestorage.v2_73.shares.list", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200, 202}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[Share]{}, e
	}
	query := url.Values{}
	for _, pair := range []struct {
		key   string
		value *string
	}{{"status", filters.Status}, {"name", filters.Name}, {"description", filters.Description}, {"name~", filters.NameContains}, {"description~", filters.DescriptionContains}, {"share_server_id", filters.ShareServerID}, {"share_type_id", filters.ShareTypeID}, {"snapshot_id", filters.SnapshotID}, {"share_network_id", filters.ShareNetworkID}, {"share_group_id", filters.ShareGroupID}, {"export_location_id", filters.ExportLocationID}, {"sort_key", filters.SortKey}, {"sort_dir", filters.SortDirection}} {
		if pair.value != nil {
			if !utf8.ValidString(*pair.value) {
				return pagination.Collection[Share]{}, types.NewError(types.InvalidArgument)
			}
			query.Set(pair.key, *pair.value)
		}
	}
	for _, pair := range []struct {
		key   string
		value *types.ProtectedValue
	}{{"host", filters.Host}, {"export_location_path", filters.ExportLocationPath}} {
		if pair.value != nil {
			raw := pair.value.Reveal()
			if !utf8.Valid(raw) {
				return pagination.Collection[Share]{}, types.NewError(types.InvalidArgument)
			}
			query.Set(pair.key, string(raw))
		}
	}
	for _, pair := range []struct {
		key   string
		value *types.ProtectedJSON
	}{{"metadata", filters.Metadata}, {"extra_specs", filters.ExtraSpecs}} {
		if pair.value != nil {
			raw := pair.value.Reveal()
			var value map[string]string
			if !utf8.Valid(raw) || json.Unmarshal(raw, &value) != nil || value == nil {
				return pagination.Collection[Share]{}, types.NewError(types.InvalidArgument)
			}
			encoded, e := json.Marshal(value)
			if e != nil {
				return pagination.Collection[Share]{}, types.NewError(types.InvalidArgument)
			}
			query.Set(pair.key, string(encoded))
		}
	}
	for _, pair := range []struct {
		key   string
		value *bool
	}{{"is_public", filters.IsPublic}, {"with_count", filters.WithCount}, {"is_soft_deleted", filters.IsSoftDeleted}} {
		if pair.value != nil {
			query.Set(pair.key, strconv.FormatBool(*pair.value))
		}
	}
	// Raw protected selectors participate only in the private cursor hash, never
	// in public cursor fields. URLs/links from the body never select an endpoint.
	return offsetCollection(c, ctx, op, core.Request{Scope: c.scope, Path: "/shares/detail", Query: query}, query.Encode(), options, "shares", func(v *Share, ep types.EndpointBinding) error {
		if v.ID == nil || *v.ID == "" || v.ProjectID == nil {
			return types.NewError(types.ProtocolError)
		}
		if *v.ProjectID != c.scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if v.Validate() != nil {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "filestorage", "share", *v.ID)
		return nil
	})
}
