// Package pagination provides bounded, scope-bound result iteration.
package pagination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/festy23/selectel-sdk/types"
)

type Cursor struct {
	SchemaVersion    int                   `json:"schema_version"`
	OperationID      string                `json:"operation_id"`
	ContractRevision string                `json:"contract_revision"`
	Scope            types.Scope           `json:"scope"`
	EndpointID       string                `json:"endpoint_id"`
	APIGeneration    string                `json:"api_generation"`
	Fingerprint      string                `json:"fingerprint"`
	WireCursor       string                `json:"wire_cursor,omitempty"`
	Skip             uint                  `json:"skip,omitempty"`
	SensitiveWire    *types.ProtectedValue `json:"sensitive_wire,omitempty"`
	Sensitive        bool                  `json:"sensitive,omitempty"`
}

type Options struct {
	PageSize, MaxItems, MaxPages uint
	Cursor                       *Cursor
}
type Page[T any] struct {
	Items      []T              `json:"items"`
	NextCursor *Cursor          `json:"next_cursor,omitempty"`
	Done       bool             `json:"done"`
	Metadata   types.Metadata   `json:"metadata"`
	Provider   ProviderMetadata `json:"provider,omitzero"`
}

// ProviderMetadata retains documented fields consumed by SDK pagination.
type ProviderMetadata struct {
	OCIReferrersIndex *types.ProtectedJSON  `json:"oci_referrers_index,omitempty"`
	OCIFiltersApplied *types.ProtectedValue `json:"oci_filters_applied,omitempty"`
	TotalCount        *types.Integer        `json:"totalCount,omitempty"`
	PreviousToken     *types.ProtectedValue `json:"prev_cursor,omitempty"`
	ReportedCount     *types.Integer        `json:"count,omitempty"`
	ForwardToken      *types.ProtectedValue `json:"nextForwardToken,omitempty"`
	BackwardToken     *types.ProtectedValue `json:"nextBackwardToken,omitempty"`
	SearchedStreams   []LogStreamStatus     `json:"searchedLogStreams,omitzero"`
}
type LogStreamStatus struct {
	Name     *string `json:"logStreamName,omitempty"`
	Complete *bool   `json:"searchedCompletely,omitempty"`
}
type Collection[T any] struct {
	Items         []T                `json:"items"`
	Complete      bool               `json:"complete"`
	StopReason    string             `json:"stop_reason"`
	NextCursor    *Cursor            `json:"next_cursor,omitempty"`
	Pages         uint               `json:"pages"`
	Metadata      types.Metadata     `json:"metadata"`
	ProviderPages []ProviderMetadata `json:"provider_pages,omitzero"`
}

// Source returns one provider page. Sensitive tokens remain explicit values.
// Done and Next must be obtained from the provider's documented end rule.
type SourcePage[T any] struct {
	Items         []T
	Next          string
	SensitiveNext *types.ProtectedValue
	Done          bool
	Metadata      types.Metadata
	Provider      ProviderMetadata
}
type Fetch[T any] func(context.Context, string, uint) (SourcePage[T], error)
type Plan struct {
	OperationID, ContractRevision, EndpointID, APIGeneration string
	Scope                                                    types.Scope
	SemanticRequest                                          any
	Defaults                                                 Options
	WireMax                                                  uint
	FixedSize                                                bool
	DynamicEndpoint                                          bool
	CollectionTimeout                                        time.Duration
}

type Iterator[T any] struct {
	mu             sync.Mutex
	plan           Plan
	options        Options
	fetch          Fetch[T]
	base           Cursor
	next           string
	skip           uint
	items, pages   uint
	terminal       error
	ended          bool
	last           *Cursor
	sensitiveInput bool
}

func New[T any](plan Plan, options Options, fetch Fetch[T]) (*Iterator[T], error) {
	if fetch == nil || plan.OperationID == "" || plan.ContractRevision == "" || (plan.EndpointID == "" && !plan.DynamicEndpoint) || plan.APIGeneration == "" || plan.CollectionTimeout <= 0 || plan.Scope.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if options.PageSize == 0 {
		options.PageSize = plan.Defaults.PageSize
	}
	if options.MaxItems == 0 {
		options.MaxItems = plan.Defaults.MaxItems
	}
	if options.MaxPages == 0 {
		options.MaxPages = plan.Defaults.MaxPages
	}
	if options.PageSize == 0 || options.PageSize > 1000 || options.MaxItems == 0 || options.MaxPages == 0 {
		return nil, types.NewError(types.InvalidArgument)
	}
	raw, err := json.Marshal(plan.SemanticRequest)
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	hash := sha256.Sum256(raw)
	base := Cursor{SchemaVersion: 1, OperationID: plan.OperationID, ContractRevision: plan.ContractRevision, Scope: plan.Scope, EndpointID: plan.EndpointID, APIGeneration: plan.APIGeneration, Fingerprint: hex.EncodeToString(hash[:])}
	i := &Iterator[T]{plan: plan, options: options, fetch: fetch, base: base}
	if options.Cursor != nil {
		c := options.Cursor
		if c.SchemaVersion != 1 {
			return nil, types.NewError(types.InvalidArgument)
		}
		if c.ContractRevision != base.ContractRevision {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if c.OperationID != base.OperationID || c.Scope != base.Scope || (base.EndpointID != "" && c.EndpointID != base.EndpointID) || c.EndpointID == "" || c.APIGeneration != base.APIGeneration || c.Fingerprint != base.Fingerprint {
			return nil, types.NewError(types.ScopeMismatch)
		}
		i.base.EndpointID = c.EndpointID
		i.next, i.skip = c.WireCursor, c.Skip
		i.sensitiveInput = c.Sensitive
		if c.SensitiveWire != nil {
			if c.WireCursor != "" {
				return nil, types.NewError(types.InvalidArgument)
			}
			i.next = string(c.SensitiveWire.Reveal())
			i.sensitiveInput = true
		}
		i.last = copyCursor(c)
	}
	return i, nil
}

func (i *Iterator[T]) Next(ctx context.Context) (Page[T], error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.terminal != nil {
		return Page[T]{NextCursor: copyCursor(i.last)}, i.terminal
	}
	if i.ended {
		return Page[T]{}, io.EOF
	}
	if ctx == nil {
		i.terminal = types.NewError(types.InvalidArgument)
		return Page[T]{}, i.terminal
	}
	if err := ctx.Err(); err != nil {
		i.terminal = types.ContextError(err)
		return Page[T]{NextCursor: copyCursor(i.last)}, i.terminal
	}
	if i.items >= i.options.MaxItems || i.pages >= i.options.MaxPages {
		i.terminal = types.NewError(types.Incomplete)
		return Page[T]{NextCursor: copyCursor(i.last)}, i.terminal
	}
	remaining := i.options.MaxItems - i.items
	size := min(i.options.PageSize, remaining)
	if i.plan.WireMax > 0 {
		size = min(size, i.plan.WireMax)
	}
	input := i.next
	page, err := i.fetch(ctx, input, size)
	i.pages++
	if err != nil {
		// A typed source may return a validated prefix before a malformed row.
		// Preserve only this view's bounded items, with a replay cursor that skips
		// the delivered prefix; the source error remains terminal for this iterator.
		if len(page.Items) > 0 {
			if i.base.EndpointID == "" && i.plan.DynamicEndpoint {
				i.base.EndpointID = page.Metadata.EndpointID
			}
			if page.Metadata.Scope != i.base.Scope || page.Metadata.EndpointID != i.base.EndpointID || page.Metadata.APIGeneration != i.base.APIGeneration {
				i.terminal = types.NewError(types.ScopeMismatch)
				return Page[T]{}, i.terminal
			}
			if i.skip > uint(len(page.Items)) {
				i.terminal = types.NewError(types.ProtocolError)
				return Page[T]{}, i.terminal
			}
			available := page.Items[i.skip:]
			n := min(uint(len(available)), remaining)
			cursor := i.base
			cursor.WireCursor = input
			cursor.Skip = i.skip + n
			if i.sensitiveInput {
				v := types.Protect([]byte(input))
				cursor.Sensitive = true
				cursor.SensitiveWire = &v
				cursor.WireCursor = ""
			}
			i.last = &cursor
			i.items += n
			i.terminal = err
			return Page[T]{Items: append([]T{}, available[:n]...), Metadata: page.Metadata, Provider: page.Provider, NextCursor: copyCursor(i.last)}, err
		}
		i.terminal = err
		return Page[T]{Metadata: page.Metadata, NextCursor: copyCursor(i.last)}, err
	}
	if i.base.EndpointID == "" && i.plan.DynamicEndpoint {
		i.base.EndpointID = page.Metadata.EndpointID
	}
	if page.Metadata.Scope != i.base.Scope || page.Metadata.EndpointID != i.base.EndpointID || page.Metadata.APIGeneration != i.base.APIGeneration {
		i.terminal = types.NewError(types.ScopeMismatch)
		return Page[T]{}, i.terminal
	}
	if i.skip > uint(len(page.Items)) {
		e := types.NewError(types.ProtocolError)
		e.Reason = "cursor_stale"
		i.terminal = e
		return Page[T]{}, e
	}
	available := page.Items[i.skip:]
	n := min(uint(len(available)), remaining)
	result := Page[T]{Items: append([]T{}, available[:n]...), Metadata: page.Metadata, Provider: page.Provider}
	i.items += n
	if n < uint(len(available)) {
		c := i.base
		c.WireCursor = input
		if i.sensitiveInput {
			v := types.Protect([]byte(input))
			c.SensitiveWire = &v
			c.Sensitive = true
			c.WireCursor = ""
		}
		c.Skip = i.skip + n
		i.last = &c
		i.skip = c.Skip
	} else {
		i.skip = 0
		i.next = page.Next
		i.sensitiveInput = page.SensitiveNext != nil
		if page.SensitiveNext != nil {
			i.next = string(page.SensitiveNext.Reveal())
		}
		if page.Done {
			i.ended = true
			i.last = nil
			result.Done = true
		} else {
			if i.next == input {
				e := types.NewError(types.ProtocolError)
				e.Reason = "cursor_no_progress"
				i.terminal = e
				return result, e
			}
			c := i.base
			c.WireCursor = page.Next
			if page.SensitiveNext != nil {
				v := types.Protect(page.SensitiveNext.Reveal())
				c.SensitiveWire = &v
				c.Sensitive = true
				c.WireCursor = ""
			}
			i.last = &c
		}
	}
	result.NextCursor = copyCursor(i.last)
	if !i.ended && (i.items >= i.options.MaxItems || i.pages >= i.options.MaxPages) {
		i.terminal = types.NewError(types.Incomplete)
		return result, i.terminal
	}
	return result, nil
}

func (i *Iterator[T]) Collect(ctx context.Context) (Collection[T], error) {
	out := Collection[T]{Items: []T{}, StopReason: "error"}
	if ctx == nil {
		return out, types.NewError(types.InvalidArgument)
	}
	call, cancel := context.WithTimeout(ctx, i.plan.CollectionTimeout)
	defer cancel()
	for {
		page, err := i.Next(call)
		out.Items = append(out.Items, page.Items...)
		if page.Provider.OCIReferrersIndex != nil || page.Provider.OCIFiltersApplied != nil || page.Provider.TotalCount != nil || page.Provider.PreviousToken != nil || page.Provider.ReportedCount != nil || page.Provider.ForwardToken != nil || page.Provider.BackwardToken != nil || page.Provider.SearchedStreams != nil {
			out.ProviderPages = append(out.ProviderPages, page.Provider)
		}
		if page.Metadata.EndpointID != "" {
			ids := append(append([]string(nil), out.Metadata.RequestIDs...), page.Metadata.RequestIDs...)
			out.Metadata = page.Metadata
			out.Metadata.RequestIDs = ids
		}
		out.NextCursor = page.NextCursor
		i.mu.Lock()
		out.Pages = i.pages
		items, pages := i.items, i.pages
		i.mu.Unlock()
		if err == io.EOF || page.Done {
			out.Complete = true
			out.StopReason = "end"
			return out, nil
		}
		if err != nil {
			switch types.ErrorCode(err) {
			case types.Incomplete:
				if items >= i.options.MaxItems {
					out.StopReason = "max_items"
				} else if pages >= i.options.MaxPages {
					out.StopReason = "max_pages"
				}
			case types.Canceled:
				out.StopReason = "canceled"
			case types.DeadlineExceeded:
				out.StopReason = "deadline"
			}
			return out, err
		}
	}
}

func copyCursor(c *Cursor) *Cursor {
	if c == nil {
		return nil
	}
	copy := *c
	if c.SensitiveWire != nil {
		v := types.Protect(c.SensitiveWire.Reveal())
		copy.SensitiveWire = &v
	}
	return &copy
}

func (i *Iterator[T]) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "[Pagination Iterator]") }

// Export explicitly discloses a sensitive continuation token for persistence.
func (c Cursor) Export() ([]byte, error) {
	type plain Cursor
	copy := c
	if copy.SensitiveWire != nil {
		copy.WireCursor = string(copy.SensitiveWire.Reveal())
		copy.SensitiveWire = nil
		copy.Sensitive = true
	}
	return json.Marshal(plain(copy))
}
func (c Cursor) MarshalJSON() ([]byte, error) {
	type plain Cursor
	copy := c
	if copy.Sensitive && copy.WireCursor != "" {
		v := types.Protect([]byte(copy.WireCursor))
		copy.SensitiveWire = &v
		copy.WireCursor = ""
	}
	return json.Marshal(plain(copy))
}

// Formatting, including an explicitly imported cursor, never discloses tokens.
func (c Cursor) Format(s fmt.State, _ rune) {
	raw, err := c.MarshalJSON()
	if err != nil {
		_, _ = io.WriteString(s, "[Pagination Cursor]")
		return
	}
	_, _ = s.Write(raw)
}
