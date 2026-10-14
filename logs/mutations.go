package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) mutation(id, action string) (core.Operation, error) {
	return c.runtime.BindDocumented(core.Operation{ID: id, Service: "logs", APIGeneration: "cloudwatch", Method: "POST", Effect: "mutation", Auth: types.S3SigV4, RequireProject: true, RequireLocation: true, Success: []int{200}, Rejected: []int{400, 401, 403, 429}, SigV4Service: "logs", ErrorProtocol: "cloudwatch", Headers: http.Header{"Content-Type": {"application/x-amz-json-1.1"}, "X-Amz-Target": {"Logs_20140328." + action}}}, c.scope)
}
func (c *Client) refError(ref types.ResourceRef, kind string) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Scope != c.scope || ref.Service != "logs" || ref.APIGeneration != "cloudwatch" || ref.Kind != kind {
		return types.NewError(types.ScopeMismatch)
	}
	selected, e := c.runtime.BindDocumented(core.Operation{Service: "logs", APIGeneration: "cloudwatch", RequireProject: true, RequireLocation: true}, c.scope)
	if e != nil {
		return e
	}
	if ref.EndpointID != selected.EndpointID || ref.EndpointURL != selected.FixedEndpoint {
		return types.NewError(types.ScopeMismatch)
	}
	if kind == "log_group" {
		if ref.Parent != nil || !validGroup(ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
	} else {
		if !validStream(ref.ID) || !utf8.ValidString(ref.ID) || ref.Parent == nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := c.refError(*ref.Parent, "log_group"); e != nil {
			return e
		}
		if ref.Parent.EndpointID != ref.EndpointID || ref.Parent.EndpointURL != ref.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}
func pinned(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func copyRef(ref types.ResourceRef) types.ResourceRef {
	if ref.Parent != nil {
		p := copyRef(*ref.Parent)
		ref.Parent = &p
	}
	return ref
}
func acknowledgement(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return object(raw)
}
func object(raw []byte) error {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return types.NewError(types.ProtocolError)
	}
	return nil
}

// CreateGroup creates the exact caller-named group. Acceptance is not ownership or readiness proof.
func (c *Client) CreateGroup(ctx context.Context, name string, o types.CallOptions) (types.Result[types.ResourceRef], error) {
	op, bindErr := c.mutation("logs.cloudwatch.log_groups.create", "CreateLogGroup")
	body, _ := json.Marshal(struct {
		Name string `json:"logGroupName"`
	}{name})
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/", Body: body, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if !validGroup(name) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[types.ResourceRef], error) {
		ref := *core.Ref(ep, "logs", "log_group", name)
		return core.Decoded[types.ResourceRef]{Data: ref, Refs: []types.ResourceRef{ref}}, acknowledgement(raw)
	})
}

// CreateStream creates a stream under the explicitly selected group.
func (c *Client) CreateStream(ctx context.Context, group types.ResourceRef, name string, o types.CallOptions) (types.Result[types.ResourceRef], error) {
	op, bindErr := c.mutation("logs.cloudwatch.log_streams.create", "CreateLogStream")
	body, _ := json.Marshal(struct {
		Group string `json:"logGroupName"`
		Name  string `json:"logStreamName"`
	}{group.ID, name})
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/", Body: body, Options: o, Refs: []types.ResourceRef{group}, PinnedEndpoint: pinned(group), Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.refError(group, "log_group"); e != nil {
			return e
		}
		if !validStream(name) || !utf8.ValidString(name) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[types.ResourceRef], error) {
		ref := *core.Ref(ep, "logs", "log_stream", name)
		p := copyRef(group)
		ref.Parent = &p
		return core.Decoded[types.ResourceRef]{Data: ref, Refs: []types.ResourceRef{ref}}, acknowledgement(raw)
	})
}

// DeleteGroup deletes its streams and events too. An active producer may recreate names.
func (c *Client) DeleteGroup(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return c.remove(ctx, ref, "log_group", o)
}

// DeleteStream deletes the stream's events; this does not stop its producer.
func (c *Client) DeleteStream(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return c.remove(ctx, ref, "log_stream", o)
}
func (c *Client) remove(ctx context.Context, ref types.ResourceRef, kind string, o types.CallOptions) (types.Result[struct{}], error) {
	id, action := "logs.cloudwatch.log_groups.delete", "DeleteLogGroup"
	body := map[string]string{"logGroupName": ref.ID}
	if kind == "log_stream" {
		id, action = "logs.cloudwatch.log_streams.delete", "DeleteLogStream"
		if ref.Parent != nil {
			body["logGroupName"] = ref.Parent.ID
		}
		body["logStreamName"] = ref.ID
	}
	op, bindErr := c.mutation(id, action)
	raw, _ := json.Marshal(body)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/", Body: raw, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pinned(ref), Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, kind)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, acknowledgement(raw)
	})
}

// PutEvents submits one bounded ordered batch. HTTP acceptance does not mean every
// event was ingested: inspect RejectedLogEventsInfo. Unknown writes never replay.
func (c *Client) PutEvents(ctx context.Context, stream types.ResourceRef, events []InputLogEvent, o types.CallOptions) (types.Result[PutLogEventsResponse], error) {
	op, bindErr := c.mutation("logs.cloudwatch.events.put", "PutLogEvents")
	var raw []byte
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.refError(stream, "log_stream"); e != nil {
			return e
		}
		if len(events) < 1 || len(events) > 10000 {
			return types.NewError(types.InvalidArgument)
		}
		type wireEvent struct {
			Timestamp int64  `json:"timestamp"`
			Message   string `json:"message"`
		}
		wire := make([]wireEvent, 0, len(events))
		var total, first, last int64
		for i, event := range events {
			if event.Timestamp == nil || event.Message == nil {
				return types.NewError(types.InvalidArgument)
			}
			timestamp, e := strconv.ParseInt(event.Timestamp.String(), 10, 64)
			if e != nil || timestamp < 0 {
				return types.NewError(types.InvalidArgument)
			}
			message := event.Message.Reveal()
			if !utf8.Valid(message) || len(message) < 1 {
				return types.NewError(types.InvalidArgument)
			}
			total += int64(len(message)) + 26
			if total > 1048576 {
				return types.NewError(types.InvalidArgument)
			}
			if i == 0 {
				first = timestamp
			}
			if i > 0 && timestamp < last {
				return types.NewError(types.InvalidArgument)
			}
			last = timestamp
			wire = append(wire, wireEvent{timestamp, string(message)})
		}
		if last-first > 86400000 {
			return types.NewError(types.InvalidArgument)
		}
		var err error
		raw, err = json.Marshal(struct {
			Group  string      `json:"logGroupName"`
			Stream string      `json:"logStreamName"`
			Events []wireEvent `json:"logEvents"`
		}{stream.Parent.ID, stream.ID, wire})
		return err
	}
	// Encoding needs the reveal boundary, but validation errors still pass core's receipt finalizer.
	validation := validate()
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/", Body: raw, Options: o, Refs: []types.ResourceRef{stream}, PinnedEndpoint: pinned(stream), Validate: func() error { return validation }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[PutLogEventsResponse], error) {
		var out PutLogEventsResponse
		if object(raw) != nil || json.Unmarshal(raw, &out) != nil {
			return core.Decoded[PutLogEventsResponse]{}, types.NewError(types.ProtocolError)
		}
		if rejected := out.RejectedLogEventsInfo; rejected != nil {
			for _, index := range []*types.Integer{rejected.TooNewLogEventStartIndex, rejected.TooOldLogEventEndIndex, rejected.ExpiredLogEventEndIndex} {
				if index == nil {
					continue
				}
				n, e := strconv.ParseInt(index.String(), 10, 64)
				if e != nil || n < 0 || n >= int64(len(events)) {
					return core.Decoded[PutLogEventsResponse]{Data: out}, types.NewError(types.ProtocolError)
				}
			}
		}
		return core.Decoded[PutLogEventsResponse]{Data: out}, nil
	})
}
