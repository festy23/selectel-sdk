// Package logs reads Selectel's documented CloudWatch-compatible API subset.
package logs

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, scope types.Scope) *Client { return &Client{runtime: r, scope: scope} }

type Group struct {
	Name                *string        `json:"logGroupName,omitempty"`
	Created             *types.Integer `json:"creationTime,omitempty"`
	RetentionDays       *types.Integer `json:"retentionInDays,omitempty"`
	FilterCount         *types.Integer `json:"metricFilterCount,omitempty"`
	ARN                 *string        `json:"arn,omitempty"`
	Bytes               *types.Integer `json:"storedBytes,omitempty"`
	KMSKeyID            *string        `json:"kmsKeyId,omitempty"`
	ProtectionStatus    *string        `json:"dataProtectionStatus,omitempty"`
	InheritedProperties []string       `json:"inheritedProperties,omitzero"`
	Class               *string        `json:"logGroupClass,omitempty"`
	GroupARN            *string        `json:"logGroupArn,omitempty"`
}
type Stream struct {
	Name          *string               `json:"logStreamName,omitempty"`
	Created       *types.Integer        `json:"creationTime,omitempty"`
	FirstEvent    *types.Integer        `json:"firstEventTimestamp,omitempty"`
	LastEvent     *types.Integer        `json:"lastEventTimestamp,omitempty"`
	LastIngestion *types.Integer        `json:"lastIngestionTime,omitempty"`
	SequenceToken *types.ProtectedValue `json:"uploadSequenceToken,omitempty"`
	ARN           *string               `json:"arn,omitempty"`
	Bytes         *types.Integer        `json:"storedBytes,omitempty"`
}
type Event struct {
	Timestamp *types.Integer        `json:"timestamp,omitempty"`
	Message   *types.ProtectedValue `json:"message,omitempty"`
	Ingested  *types.Integer        `json:"ingestionTime,omitempty"`
	Stream    *string               `json:"logStreamName,omitempty"`
	ID        *string               `json:"eventId,omitempty"`
}
type GroupsRequest struct {
	Prefix *string `json:"logGroupNamePrefix,omitempty"`
}
type StreamsRequest struct {
	Group      string  `json:"logGroupName"`
	Prefix     *string `json:"logStreamNamePrefix,omitempty"`
	OrderBy    *string `json:"orderBy,omitempty"`
	Descending *bool   `json:"descending,omitempty"`
}
type EventsRequest struct {
	Group, Stream string
	Start, End    time.Time
	StartFromHead *bool
}
type FilterRequest struct {
	Group                 string
	StreamNames           []string
	StreamPrefix, Pattern *string
	Start, End            time.Time
	Interleaved           *bool
}

var groupName = regexp.MustCompile(`^[.\-_/#A-Za-z0-9]+$`)

func validGroup(value string) bool {
	return len(value) > 0 && len(value) <= 512 && groupName.MatchString(value)
}
func validStream(value string) bool {
	return len(value) > 0 && len(value) <= 512 && !strings.ContainsAny(value, ":*")
}
func bounds(start, end time.Time) (int64, int64, error) {
	s, e := start.UnixMilli(), end.UnixMilli()
	if start.IsZero() || end.IsZero() || s < 0 || e < 0 || s >= e {
		return 0, 0, types.NewError(types.InvalidArgument)
	}
	return s, e, nil
}
func (c *Client) operation(id, action string) (core.Operation, error) {
	return c.runtime.BindDocumented(core.Operation{ID: id, Service: "logs", APIGeneration: "cloudwatch", Method: "POST", Effect: "read", Auth: types.S3SigV4, RequireProject: true, RequireLocation: true, Success: []int{200}, SigV4Service: "logs", ErrorProtocol: "cloudwatch", Headers: http.Header{"Content-Type": {"application/x-amz-json-1.1"}, "X-Amz-Target": {"Logs_20140328." + action}}}, c.scope)
}
func iterate[T any](c *Client, op core.Operation, body map[string]any, o pagination.Options, field string, wireMax uint, events bool) (*pagination.Iterator[T], error) {
	raw, e := json.Marshal(body)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	plan, e := c.runtime.PagePlan(op.ID, "cloudwatch", c.scope, op.EndpointID, json.RawMessage(raw))
	if e != nil {
		return nil, e
	}
	plan.WireMax = wireMax
	fetch := func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[T], error) {
		var request map[string]json.RawMessage
		_ = json.Unmarshal(raw, &request)
		request["limit"], _ = json.Marshal(size)
		if cursor != "" {
			request["nextToken"], _ = json.Marshal(cursor)
		}
		bytes, _ := json.Marshal(request)
		provider := pagination.ProviderMetadata{}
		next := ""
		done := false
		decode := func(data []byte, _ types.EndpointBinding) (core.Decoded[[]T], error) {
			var envelope map[string]json.RawMessage
			if json.Unmarshal(data, &envelope) != nil || envelope == nil {
				return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			items := []T{}
			if value, ok := envelope[field]; ok {
				if string(value) == "null" || json.Unmarshal(value, &items) != nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
			}
			key := "nextToken"
			if events {
				key = "nextForwardToken"
			}
			if value, ok := envelope[key]; ok {
				if json.Unmarshal(value, &next) != nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
			}
			if events {
				if next == "" {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				v := types.Protect([]byte(next))
				provider.ForwardToken = &v
				done = next == cursor
			} else {
				done = next == ""
			}
			if value, ok := envelope["nextBackwardToken"]; ok {
				var previous string
				if json.Unmarshal(value, &previous) != nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				v := types.Protect([]byte(previous))
				provider.BackwardToken = &v
			}
			if value, ok := envelope["searchedLogStreams"]; ok {
				if json.Unmarshal(value, &provider.SearchedStreams) != nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
			}
			return core.Decoded[[]T]{Data: items}, nil
		}
		r, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/", Body: bytes}, decode)
		page := pagination.SourcePage[T]{Items: r.Data, Done: done, Metadata: r.Metadata}
		if e == nil {
			page.Provider = provider
		}
		if !done {
			v := types.Protect([]byte(next))
			page.SensitiveNext = &v
		}
		return page, e
	}
	return pagination.New(plan, o, fetch)
}
func (c *Client) IterateGroups(r GroupsRequest, o pagination.Options) (*pagination.Iterator[Group], error) {
	if r.Prefix != nil && !validGroup(*r.Prefix) {
		return nil, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("logs.cloudwatch.log_groups.list", "DescribeLogGroups")
	if e != nil {
		return nil, e
	}
	body := map[string]any{}
	if r.Prefix != nil {
		body["logGroupNamePrefix"] = *r.Prefix
	}
	return iterate[Group](c, op, body, o, "logGroups", 50, false)
}
func (c *Client) Groups(ctx context.Context, r GroupsRequest, o pagination.Options) (pagination.Collection[Group], error) {
	it, e := c.IterateGroups(r, o)
	if e != nil {
		return pagination.Collection[Group]{Items: []Group{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Client) IterateStreams(r StreamsRequest, o pagination.Options) (*pagination.Iterator[Stream], error) {
	if !validGroup(r.Group) || (r.Prefix != nil && !validStream(*r.Prefix)) {
		return nil, types.NewError(types.InvalidArgument)
	}
	if r.OrderBy != nil && *r.OrderBy != "LogStreamName" && *r.OrderBy != "LastEventTime" {
		return nil, types.NewError(types.InvalidArgument)
	}
	if r.OrderBy != nil && *r.OrderBy == "LastEventTime" && r.Prefix != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("logs.cloudwatch.log_streams.list", "DescribeLogStreams")
	if e != nil {
		return nil, e
	}
	body := map[string]any{"logGroupName": r.Group}
	if r.Prefix != nil {
		body["logStreamNamePrefix"] = *r.Prefix
	}
	if r.OrderBy != nil {
		body["orderBy"] = *r.OrderBy
	}
	if r.Descending != nil {
		body["descending"] = *r.Descending
	}
	return iterate[Stream](c, op, body, o, "logStreams", 50, false)
}
func (c *Client) Streams(ctx context.Context, r StreamsRequest, o pagination.Options) (pagination.Collection[Stream], error) {
	it, e := c.IterateStreams(r, o)
	if e != nil {
		return pagination.Collection[Stream]{Items: []Stream{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Client) IterateEvents(r EventsRequest, o pagination.Options) (*pagination.Iterator[Event], error) {
	s, end, e := bounds(r.Start, r.End)
	if e != nil {
		return nil, e
	}
	if !validGroup(r.Group) || !validStream(r.Stream) {
		return nil, types.NewError(types.InvalidArgument)
	}
	if r.StartFromHead != nil && !*r.StartFromHead {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	op, e := c.operation("logs.cloudwatch.events.get", "GetLogEvents")
	if e != nil {
		return nil, e
	}
	body := map[string]any{"logGroupName": r.Group, "logStreamName": r.Stream, "startTime": s, "endTime": end, "startFromHead": true}
	return iterate[Event](c, op, body, o, "events", 10000, true)
}
func (c *Client) Events(ctx context.Context, r EventsRequest, o pagination.Options) (pagination.Collection[Event], error) {
	it, e := c.IterateEvents(r, o)
	if e != nil {
		return pagination.Collection[Event]{Items: []Event{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Client) IterateFilter(r FilterRequest, o pagination.Options) (*pagination.Iterator[Event], error) {
	s, end, e := bounds(r.Start, r.End)
	if e != nil {
		return nil, e
	}
	if !validGroup(r.Group) || len(r.StreamNames) > 100 || (r.StreamPrefix != nil && r.StreamNames != nil) {
		return nil, types.NewError(types.InvalidArgument)
	}
	for _, name := range r.StreamNames {
		if !validStream(name) {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	if r.StreamPrefix != nil && !validStream(*r.StreamPrefix) {
		return nil, types.NewError(types.InvalidArgument)
	}
	if r.Pattern != nil && len(*r.Pattern) > 1024 {
		return nil, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("logs.cloudwatch.events.filter", "FilterLogEvents")
	if e != nil {
		return nil, e
	}
	body := map[string]any{"logGroupName": r.Group, "startTime": s, "endTime": end}
	if r.StreamNames != nil {
		body["logStreamNames"] = append([]string{}, r.StreamNames...)
	}
	if r.StreamPrefix != nil {
		body["logStreamNamePrefix"] = *r.StreamPrefix
	}
	if r.Pattern != nil {
		body["filterPattern"] = *r.Pattern
	}
	if r.Interleaved != nil {
		body["interleaved"] = *r.Interleaved
	}
	return iterate[Event](c, op, body, o, "events", 10000, false)
}
func (c *Client) Filter(ctx context.Context, r FilterRequest, o pagination.Options) (pagination.Collection[Event], error) {
	it, e := c.IterateFilter(r, o)
	if e != nil {
		return pagination.Collection[Event]{Items: []Event{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
