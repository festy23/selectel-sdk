package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/logs"
	"github.com/festy23/selectel-sdk/types"
)

func TestLogsLifecycleMutationsUseScopedSignedNamesAndReceipts(t *testing.T) {
	actions := []string{}
	c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) {
		raw := verifyLogSignature(t, r)
		if r.URL.Host != "ru-1.logs.selcloud.ru" || r.URL.Path != "/" || r.Method != "POST" {
			t.Fatal("wrong wire endpoint")
		}
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) != nil || string(body["logGroupName"]) != `"group"` {
			t.Fatal("wrong group")
		}
		action := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "Logs_20140328.")
		actions = append(actions, action)
		if strings.Contains(action, "Stream") && string(body["logStreamName"]) != `"stream"` {
			t.Fatal("wrong stream")
		}
		return inventoryReply(200, `{}`), nil
	}, 0, 0)
	svc := regionalView(t, c, "ru-1").Logs()
	group, e := svc.CreateGroup(context.Background(), "group", types.CallOptions{})
	if e != nil || group.Receipt == nil || group.Receipt.Outcome != types.Accepted || group.Data.Kind != "log_group" {
		t.Fatal("create group", e)
	}
	stream, e := svc.CreateStream(context.Background(), group.Data, "stream", types.CallOptions{})
	if e != nil || stream.Data.Parent == nil || stream.Data.Parent.ID != "group" || stream.Receipt.Outcome != types.Accepted {
		t.Fatal("create stream", e)
	}
	deleted, e := svc.DeleteStream(context.Background(), stream.Data, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || deleted.Receipt.ResourceRefs[0].Parent.ID != "group" {
		t.Fatal("delete stream", e)
	}
	deleted, e = svc.DeleteGroup(context.Background(), group.Data, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatal("delete group", e)
	}
	if strings.Join(actions, ",") != "CreateLogGroup,CreateLogStream,DeleteLogStream,DeleteLogGroup" {
		t.Fatal("wrong effects")
	}
}

func TestLogsWriteProtectsRawEventsAndRetainsPartialRejection(t *testing.T) {
	c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) {
		raw := verifyLogSignature(t, r)
		if r.Header.Get("X-Amz-Target") != "Logs_20140328.PutLogEvents" || !strings.Contains(string(raw), `"message":"synthetic-private-message"`) || !strings.Contains(string(raw), `"timestamp":1234567890123`) {
			t.Fatal("event changed")
		}
		return inventoryReply(200, `{"nextSequenceToken":"synthetic-private-next","rejectedLogEventsInfo":{"tooOldLogEventEndIndex":0},"rejectedEntityInfo":{"errorType":"synthetic-private-entity"}}`), nil
	}, 0, 0)
	// Existing explicitly scoped name is a locator, never ownership proof.
	scope := regionalView(t, c, "ru-1").Scope()
	group := types.ResourceRef{Scope: scope, Service: "logs", APIGeneration: "cloudwatch", Kind: "log_group", ID: "group", EndpointID: "selectel-logs.cloudwatch:ru-1", EndpointURL: "https://ru-1.logs.selcloud.ru"}
	stream := group
	stream.Kind = "log_stream"
	stream.ID = "stream"
	stream.Parent = &group
	timestamp, _ := types.ParseInteger("1234567890123")
	message := types.Protect([]byte("synthetic-private-message"))
	events := []logs.InputLogEvent{{Timestamp: &timestamp, Message: &message}}
	out, e := regionalView(t, c, "ru-1").Logs().PutEvents(context.Background(), stream, events, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Data.RejectedEntityInfo == nil || out.Data.RejectedEntityInfo.ErrorType == nil || out.Data.RejectedLogEventsInfo == nil || out.Data.RejectedLogEventsInfo.TooOldLogEventEndIndex.String() != "0" {
		t.Fatal("partial rejection/receipt lost", e)
	}
	for _, value := range []any{events, out} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "synthetic-private") || strings.Contains(fmt.Sprintf("%#v", value), "synthetic-private") {
			t.Fatal("protected telemetry leaked")
		}
	}
}

func TestLogsUnknownWriteNeverReplays(t *testing.T) {
	calls := 0
	c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF }, 0, 0)
	scope := regionalView(t, c, "ru-1").Scope()
	group := types.ResourceRef{Scope: scope, Service: "logs", APIGeneration: "cloudwatch", Kind: "log_group", ID: "group", EndpointID: "selectel-logs.cloudwatch:ru-1", EndpointURL: "https://ru-1.logs.selcloud.ru"}
	stream := group
	stream.Kind = "log_stream"
	stream.ID = "stream"
	stream.Parent = &group
	timestamp, _ := types.ParseInteger("1")
	message := types.Protect([]byte("test"))
	out, e := regionalView(t, c, "ru-1").Logs().PutEvents(context.Background(), stream, []logs.InputLogEvent{{Timestamp: &timestamp, Message: &message}}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || out.Receipt.ResourceRefs[0].Parent.ID != "group" {
		t.Fatal("unknown write replay/lost locator", e)
	}
}

func TestLogsEmptyAcknowledgementsAndRejectedProviderErrors(t *testing.T) {
	for _, tc := range []struct {
		body    string
		status  int
		code    types.Code
		outcome types.Outcome
	}{
		{"", 200, "", types.Accepted},
		{`{`, 200, types.ProtocolError, types.Accepted},
		{`{"__type":"provider#ResourceAlreadyExistsException","message":"synthetic-private-provider","expectedSequenceToken":"synthetic-private-token"}`, 400, types.Conflict, types.Rejected},
		{`{"__type":"OperationAbortedException"}`, 400, types.Conflict, types.Rejected},
		{`{"__type":"LimitExceededException"}`, 400, types.RateLimited, types.Rejected},
		{`{"__type":"ResourceNotFoundException"}`, 400, types.NotFound, types.Rejected},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.status, tc.code), func(t *testing.T) {
			calls := 0
			c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) { calls++; return inventoryReply(tc.status, tc.body), nil }, 0, 0)
			out, e := regionalView(t, c, "ru-1").Logs().CreateGroup(context.Background(), "group", types.CallOptions{})
			if types.ErrorCode(e) != tc.code || out.Receipt == nil || out.Receipt.Outcome != tc.outcome || calls != 1 {
				t.Fatal("receipt/error semantics", e)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "synthetic-private") || strings.Contains(fmt.Sprint(e), "synthetic-private") {
				t.Fatal("provider data leaked")
			}
		})
	}
}

func TestLogsWriteBoundsAndScopeFailBeforeCredentials(t *testing.T) {
	for _, name := range []string{"empty", "count", "bytes", "utf8", "unordered", "span", "timestamp", "foreign-project", "foreign-generation", "foreign-parent", "foreign-endpoint", "missing-parent"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			src := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
				calls++
				return credentials.Material{}, nil
			})
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: src, Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected HTTP"); return nil, nil })})
			if e != nil {
				t.Fatal(e)
			}
			view := regionalView(t, c, "ru-1")
			group := types.ResourceRef{Scope: view.Scope(), Service: "logs", APIGeneration: "cloudwatch", Kind: "log_group", ID: "group", EndpointID: "selectel-logs.cloudwatch:ru-1", EndpointURL: "https://ru-1.logs.selcloud.ru"}
			stream := group
			stream.Kind = "log_stream"
			stream.ID = "stream"
			stream.Parent = &group
			n, _ := types.ParseInteger("2")
			m := types.Protect([]byte("test"))
			events := []logs.InputLogEvent{{Timestamp: &n, Message: &m}}
			switch name {
			case "empty":
				events = nil
			case "count":
				events = make([]logs.InputLogEvent, 10001)
			case "bytes":
				m = types.Protect([]byte(strings.Repeat("x", 1048576)))
			case "utf8":
				m = types.Protect([]byte{255})
			case "unordered":
				n2, _ := types.ParseInteger("1")
				events = append(events, logs.InputLogEvent{Timestamp: &n2, Message: &m})
			case "span":
				n2, _ := types.ParseInteger("86400003")
				events = append(events, logs.InputLogEvent{Timestamp: &n2, Message: &m})
			case "timestamp":
				n, _ = types.ParseInteger("9223372036854775808")
			case "foreign-project":
				stream.Scope.ProjectID = "foreign"
			case "foreign-generation":
				stream.APIGeneration = "other"
			case "foreign-parent":
				group.Service = "compute"
			case "foreign-endpoint":
				stream.EndpointURL = "https://unconfirmed.example"
				group.EndpointURL = stream.EndpointURL
			case "missing-parent":
				stream.Parent = nil
			}
			out, e := view.Logs().PutEvents(context.Background(), stream, events, types.CallOptions{})
			if e == nil || calls != 0 || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.Attempts != 0 {
				t.Fatal("invalid input reached credentials or lost not_sent receipt", e)
			}
		})
	}
}

func TestLogsRejectedEventIndexesMustReferToSubmittedBatch(t *testing.T) {
	c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"rejectedLogEventsInfo":{"tooNewLogEventStartIndex":1}}`), nil
	}, 0, 0)
	scope := regionalView(t, c, "ru-1").Scope()
	group := types.ResourceRef{Scope: scope, Service: "logs", APIGeneration: "cloudwatch", Kind: "log_group", ID: "group", EndpointID: "selectel-logs.cloudwatch:ru-1", EndpointURL: "https://ru-1.logs.selcloud.ru"}
	stream := group
	stream.Kind = "log_stream"
	stream.ID = "stream"
	stream.Parent = &group
	n, _ := types.ParseInteger("1")
	m := types.Protect([]byte("test"))
	out, e := regionalView(t, c, "ru-1").Logs().PutEvents(context.Background(), stream, []logs.InputLogEvent{{Timestamp: &n, Message: &m}}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Receipt.ResourceRefs[0].ID != "stream" {
		t.Fatal("impossible rejection index accepted", e)
	}
}

func TestLogsSequenceHintsNeverTriggerWriteRepair(t *testing.T) {
	for _, code := range []string{"InvalidSequenceTokenException", "DataAlreadyAcceptedException"} {
		t.Run(code, func(t *testing.T) {
			calls := 0
			c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) {
				calls++
				verifyLogSignature(t, r)
				return inventoryReply(400, `{"__type":"`+code+`","expectedSequenceToken":"synthetic-private-token","message":"synthetic-private-message"}`), nil
			}, 0, 0)
			scope := regionalView(t, c, "ru-1").Scope()
			group := types.ResourceRef{Scope: scope, Service: "logs", APIGeneration: "cloudwatch", Kind: "log_group", ID: "group", EndpointID: "selectel-logs.cloudwatch:ru-1", EndpointURL: "https://ru-1.logs.selcloud.ru"}
			stream := group
			stream.Kind = "log_stream"
			stream.ID = "stream"
			stream.Parent = &group
			n, _ := types.ParseInteger("1")
			m := types.Protect([]byte("test"))
			out, e := regionalView(t, c, "ru-1").Logs().PutEvents(context.Background(), stream, []logs.InputLogEvent{{Timestamp: &n, Message: &m}}, types.CallOptions{})
			if types.ErrorCode(e) != types.Conflict || calls != 1 || out.Receipt.Outcome != types.Rejected || out.Receipt.ResourceRefs[0].ID != "stream" {
				t.Fatal("sequence repair/replay", e)
			}
			var diagnostic any = e
			raw, _ := json.Marshal(diagnostic)
			if strings.Contains(string(raw), "synthetic-private") {
				t.Fatal("sequence hint leaked")
			}
		})
	}
}

func TestLogsAcceptedResponseByteLimitPreservesReceipt(t *testing.T) {
	c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"unexpected":"synthetic-private-response"}`), nil
	}, 0, 8)
	out, e := regionalView(t, c, "ru-1").Logs().CreateGroup(context.Background(), "group", types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || out.Receipt == nil || out.Receipt.Outcome != types.Accepted {
		t.Fatal("accepted oversized response lost receipt", e)
	}
}

func TestLogsMutationBindingFailuresRetainNotSentReceipt(t *testing.T) {
	for _, pool := range []string{"", "gis-2"} {
		for _, action := range []string{"CreateGroup", "CreateStream", "DeleteGroup", "DeleteStream", "PutEvents"} {
			t.Run(action+"/"+pool, func(t *testing.T) {
				credentialCalls, httpCalls := 0, 0
				c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
					credentialCalls++
					return credentials.Material{}, nil
				}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { httpCalls++; return nil, io.ErrUnexpectedEOF })})
				if e != nil {
					t.Fatal(e)
				}
				project, e := c.Project("p")
				if e != nil {
					t.Fatal(e)
				}
				view, e := project.Location("spb", pool)
				if e != nil {
					t.Fatal(e)
				}
				group := types.ResourceRef{Scope: view.Scope(), Service: "logs", APIGeneration: "cloudwatch", Kind: "log_group", ID: "group", EndpointID: "explicit-log-endpoint", EndpointURL: "https://explicit.example"}
				stream := group
				stream.Kind = "log_stream"
				stream.ID = "stream"
				stream.Parent = &group
				var receipt *types.Receipt
				switch action {
				case "CreateGroup":
					out, err := view.Logs().CreateGroup(context.Background(), "group", types.CallOptions{})
					receipt, e = out.Receipt, err
				case "CreateStream":
					out, err := view.Logs().CreateStream(context.Background(), group, "stream", types.CallOptions{})
					receipt, e = out.Receipt, err
				case "DeleteGroup":
					out, err := view.Logs().DeleteGroup(context.Background(), group, types.CallOptions{})
					receipt, e = out.Receipt, err
				case "DeleteStream":
					out, err := view.Logs().DeleteStream(context.Background(), stream, types.CallOptions{})
					receipt, e = out.Receipt, err
				case "PutEvents":
					n, _ := types.ParseInteger("1")
					m := types.Protect([]byte("test"))
					out, err := view.Logs().PutEvents(context.Background(), stream, []logs.InputLogEvent{{Timestamp: &n, Message: &m}}, types.CallOptions{})
					receipt, e = out.Receipt, err
				}
				code := types.EndpointUnavailable
				if pool == "" {
					code = types.InvalidArgument
				}
				if types.ErrorCode(e) != code || receipt == nil || receipt.Outcome != types.NotSent || receipt.Attempts != 0 || receipt.Scope != view.Scope() || receipt.CallID == "" || credentialCalls != 0 || httpCalls != 0 {
					t.Fatalf("binding error lost not_sent receipt: code=%s receipt=%v credentials=%d HTTP=%d", types.ErrorCode(e), receipt, credentialCalls, httpCalls)
				}
				sdkError, ok := e.(*types.SDKError)
				if !ok || sdkError.Receipt == nil || sdkError.Receipt.CallID != receipt.CallID {
					t.Fatal("error/result receipt mismatch")
				}
			})
		}
	}
}
