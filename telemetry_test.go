package selectelsdk_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/logs"
	"github.com/festy23/selectel-sdk/metrics"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func telemetryClient(t *testing.T, scheme credentials.Scheme, wire transportFunc, limit uint, maxBytes uint64) *sdk.Client {
	t.Helper()
	src := credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "telemetry", Revision: "revision", Binding: r.Scope, Audience: "selectel", Scheme: scheme, Username: types.Protect([]byte("synthetic-user-id")), Password: types.Protect([]byte("synthetic-private-password")), AccessKey: types.Protect([]byte("synthetic-access")), SecretKey: types.Protect([]byte("synthetic-private-key"))}, nil
	})
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: src, Transport: wire, MaxMetricSamples: limit, MaxJSONBytes: maxBytes})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func mac(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

// Verify the complete signed request independently of the SDK's signer.
func verifyLogSignature(t *testing.T, r *http.Request) []byte {
	t.Helper()
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		t.Fatal("body read failed")
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		t.Fatal("SigV4 missing")
	}
	fields := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(auth, "AWS4-HMAC-SHA256 "), ",") {
		pieces := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pieces) != 2 {
			t.Fatal("bad signature format")
		}
		fields[pieces[0]] = pieces[1]
	}
	credential := strings.Split(fields["Credential"], "/")
	if len(credential) != 5 || credential[0] != "synthetic-access" || credential[2] != "ru-1" || credential[3] != "logs" || credential[4] != "aws4_request" {
		t.Fatal("signature bound to wrong service or pool")
	}
	canonicalHeaders := ""
	for _, key := range strings.Split(fields["SignedHeaders"], ";") {
		value := r.Header.Get(key)
		if key == "host" {
			value = r.URL.Host
		}
		if key == "content-length" {
			value = fmt.Sprint(r.ContentLength)
		}
		canonicalHeaders += key + ":" + strings.Join(strings.Fields(value), " ") + "\n"
	}
	digest := sha256.Sum256(raw)
	canonical := r.Method + "\n" + r.URL.EscapedPath() + "\n" + r.URL.RawQuery + "\n" + canonicalHeaders + "\n" + fields["SignedHeaders"] + "\n" + hex.EncodeToString(digest[:])
	requestHash := sha256.Sum256([]byte(canonical))
	scope := strings.Join(credential[1:], "/")
	toSign := "AWS4-HMAC-SHA256\n" + r.Header.Get("X-Amz-Date") + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	key := mac([]byte("AWS4synthetic-private-key"), credential[1])
	key = mac(key, credential[2])
	key = mac(key, credential[3])
	key = mac(key, "aws4_request")
	if hex.EncodeToString(mac(key, toSign)) != fields["Signature"] {
		t.Fatal("request payload or headers not covered by signature")
	}
	return raw
}
func TestCloudwatchGroupsStreamsAndSignature(t *testing.T) {
	c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) {
		verifyLogSignature(t, r)
		if r.URL.Host != "ru-1.logs.selcloud.ru" || r.Header.Get("Content-Type") != "application/x-amz-json-1.1" {
			t.Error("CloudWatch endpoint/protocol wrong")
		}
		switch r.Header.Get("X-Amz-Target") {
		case "Logs_20140328.DescribeLogGroups":
			return inventoryReply(200, `{"logGroups":[{"logGroupName":"group","storedBytes":900719925474099312345}],"nextToken":null}`), nil
		case "Logs_20140328.DescribeLogStreams":
			return inventoryReply(200, `{"logStreams":[{"logStreamName":"stream","uploadSequenceToken":"synthetic-private-sequence"}]}`), nil
		}
		t.Fatal("unexpected action")
		return nil, nil
	}, 0, 0)
	l := regionalView(t, c, "ru-1").Logs()
	groups, e := l.Groups(context.Background(), logs.GroupsRequest{}, pagination.Options{})
	if e != nil || !groups.Complete || groups.Items[0].Bytes.String() != "900719925474099312345" {
		t.Fatal("groups decode failed", e)
	}
	streams, e := l.Streams(context.Background(), logs.StreamsRequest{Group: "group"}, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(streams)
	if strings.Contains(string(raw), "synthetic-private-sequence") {
		t.Fatal("sequence token leaked")
	}
}
func TestLogEventsEmptyPageAndRepeatedForwardTokenEnd(t *testing.T) {
	calls := 0
	c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) {
		raw := verifyLogSignature(t, r)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body["startFromHead"] != true || body["startTime"] != float64(1000) || body["endTime"] != float64(3000) {
			t.Error("event bounds/direction changed")
		}
		calls++
		switch calls {
		case 1:
			return inventoryReply(200, `{"events":[],"nextForwardToken":"private-a","nextBackwardToken":"private-backward"}`), nil
		case 2:
			if body["nextToken"] != "private-a" {
				t.Error("forward cursor missing")
			}
			return inventoryReply(200, `{"events":[{"timestamp":2000,"message":"synthetic-private-log","ingestionTime":2001}],"nextForwardToken":"private-b","nextBackwardToken":"private-backward"}`), nil
		case 3:
			return inventoryReply(200, `{"events":[],"nextForwardToken":"private-b","nextBackwardToken":"private-backward"}`), nil
		}
		t.Fatal("nonterminating event traversal")
		return nil, nil
	}, 0, 0)
	r, e := regionalView(t, c, "ru-1").Logs().Events(context.Background(), logs.EventsRequest{Group: "group", Stream: "stream", Start: time.Unix(1, 0), End: time.Unix(3, 0)}, pagination.Options{})
	if e != nil || !r.Complete || len(r.Items) != 1 || calls != 3 || len(r.ProviderPages) != 3 {
		t.Fatal("event traversal wrong", e)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "private-") || strings.Contains(fmt.Sprintf("%+v", r), "private-") {
		t.Fatal("event/cursor disclosed")
	}
	if string(r.Items[0].Message.Reveal()) != "synthetic-private-log" {
		t.Fatal("explicit log read changed value")
	}
}
func TestLogFilterRetainsSearchStatusAndExplicitEmptyPattern(t *testing.T) {
	pattern := ""
	c := telemetryClient(t, credentials.S3, func(r *http.Request) (*http.Response, error) {
		raw := verifyLogSignature(t, r)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		if v, present := b["filterPattern"]; !present || v != "" {
			t.Error("explicit empty pattern omitted")
		}
		return inventoryReply(200, `{"events":[{"eventId":"event","logStreamName":"stream","timestamp":1000,"message":"value"}],"searchedLogStreams":[{"logStreamName":"stream","searchedCompletely":false}]}`), nil
	}, 0, 0)
	r, e := regionalView(t, c, "ru-1").Logs().Filter(context.Background(), logs.FilterRequest{Group: "group", Pattern: &pattern, Start: time.Unix(1, 0), End: time.Unix(3, 0)}, pagination.Options{})
	if e != nil || !r.Complete || r.ProviderPages[0].SearchedStreams[0].Complete == nil || *r.ProviderPages[0].SearchedStreams[0].Complete {
		t.Fatal("filter metadata lost", e)
	}
}
func TestCloudwatchErrorCodeAndEventPrecisionValidation(t *testing.T) {
	calls := 0
	c := telemetryClient(t, credentials.S3, func(*http.Request) (*http.Response, error) {
		calls++
		return inventoryReply(400, `{"__type":"ResourceNotFoundException","message":"synthetic-private-error"}`), nil
	}, 0, 0)
	l := regionalView(t, c, "ru-1").Logs()
	_, e := l.Events(context.Background(), logs.EventsRequest{Group: "group", Stream: "stream", Start: time.Unix(1, 0), End: time.Unix(2, 0)}, pagination.Options{})
	if types.ErrorCode(e) != types.NotFound {
		t.Fatal("CloudWatch 400 not_found misclassified", e)
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "private-error") {
		t.Fatal("provider message leaked")
	}
	_, e = l.Events(context.Background(), logs.EventsRequest{Group: "group", Stream: "stream", Start: time.Unix(1, 1), End: time.Unix(1, 2)}, pagination.Options{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 1 {
		t.Fatal("zero-width millisecond interval sent")
	}
}
func verifyBasic(t *testing.T, r *http.Request) {
	t.Helper()
	u, p, ok := r.BasicAuth()
	if !ok || u != "synthetic-user-id" || p != "synthetic-private-password" || r.URL.Host != "ru-1.metrics.selcloud.ru" {
		t.Error("metrics auth/endpoint wrong")
	}
}
func TestScrapeExactValuesLabelsAndBoundedPrefix(t *testing.T) {
	c := telemetryClient(t, credentials.Basic, func(r *http.Request) (*http.Response, error) {
		verifyBasic(t, r)
		if r.URL.Path != "/projects/test-project/namespaces/compute/prometheus/metrics" {
			t.Error("scrape path wrong")
		}
		return inventoryReply(200, "# HELP metric synthetic-private-help\n# TYPE metric gauge\nmetric{label=\"a\\n\\\"b\"} 0.12345678901234567890 900719925474099312345\nmetric{label=\"second\"} +Inf\nmetric{label=\"third\"} NaN\n"), nil
	}, 2, 0)
	r, e := regionalView(t, c, "ru-1").Metrics().Scrape(context.Background(), metrics.ScrapeRequest{Namespace: "compute"})
	if types.ErrorCode(e) != types.ResponseLimit || r.Data.Complete || len(r.Data.Samples) != 2 || r.Data.Samples[0].Value != "0.12345678901234567890" || r.Data.Samples[0].Timestamp.String() != "900719925474099312345" || r.Data.Samples[0].Labels["label"] != "a\n\"b" || r.Data.Samples[1].Value != "+Inf" {
		t.Fatal("scrape lost precision/limits", e)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "synthetic-private-help") {
		t.Fatal("HELP annotation leaked")
	}
}
func TestPrometheusRangeExactStepTimestampHistogramAndLimits(t *testing.T) {
	c := telemetryClient(t, credentials.Basic, func(r *http.Request) (*http.Response, error) {
		verifyBasic(t, r)
		if r.URL.Path != "/projects/test-project/namespaces/compute/api/v1/query_range" || r.URL.Query().Get("step") != "0.000000001" {
			t.Error("range step rounded")
		}
		return inventoryReply(200, `{"status":"success","warnings":["synthetic-private-query"],"data":{"resultType":"matrix","result":[{"metric":{"__name__":"metric"},"values":[[1.12345678901234567890,"NaN"]],"histograms":[[2.000000000001,{"count":"900719925474099312345","sum":"+Inf","buckets":[[0,"-Inf","1.5","2"]]}],[3,{"count":"1","sum":"1"}]]}]}}`), nil
	}, 2, 0)
	r, e := regionalView(t, c, "ru-1").Metrics().QueryRange(context.Background(), metrics.RangeRequest{Namespace: "compute", PromQL: "metric", Start: time.Unix(1, 0), End: time.Unix(3, 0), Step: time.Nanosecond})
	if types.ErrorCode(e) != types.ResponseLimit || r.Data.Complete || r.Data.Series[0].Samples[0].Timestamp.String() != "1.12345678901234567890" || r.Data.Series[0].Histograms[0].Histogram.Count != "900719925474099312345" {
		t.Fatal("range values or prefix lost", e)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "synthetic-private-query") {
		t.Fatal("provider warning leaked")
	}
}
func TestPrometheusInstantResultKindsAndOptionalTime(t *testing.T) {
	for _, tc := range []struct{ kind, result string }{{"scalar", `[1.00000000001,"+Inf"]`}, {"string", `[1,"arbitrary string"]`}, {"vector", `[{"metric":{"name":"metric"},"value":[1,"1"]}]`}, {"matrix", `[]`}} {
		t.Run(tc.kind, func(t *testing.T) {
			c := telemetryClient(t, credentials.Basic, func(r *http.Request) (*http.Response, error) {
				verifyBasic(t, r)
				if r.URL.Query().Has("time") || r.URL.Query().Has("start") || r.URL.Query().Has("end") {
					t.Error("implicit instant time/bounds sent")
				}
				return inventoryReply(200, `{"status":"success","data":{"resultType":"`+tc.kind+`","result":`+tc.result+`}}`), nil
			}, 0, 0)
			r, e := regionalView(t, c, "ru-1").Metrics().Query(context.Background(), metrics.QueryRequest{Namespace: "compute", PromQL: "metric"})
			if e != nil || !r.Data.Complete || r.Data.Type != tc.kind {
				t.Fatal("instant result kind lost", e)
			}
		})
	}
}
func TestMetricsBodyLimitAndBadNamespaceNeverLookSuccessful(t *testing.T) {
	calls := 0
	c := telemetryClient(t, credentials.Basic, func(*http.Request) (*http.Response, error) {
		calls++
		return inventoryReply(200, strings.Repeat("x", 300)), nil
	}, 0, 32)
	m := regionalView(t, c, "ru-1").Metrics()
	r, e := m.Scrape(context.Background(), metrics.ScrapeRequest{Namespace: "compute"})
	if types.ErrorCode(e) != types.ResponseLimit || r.Data.Complete || len(r.Data.Samples) != 0 {
		t.Fatal("oversize scrape successful")
	}
	_, e = m.Query(context.Background(), metrics.QueryRequest{Namespace: "unknown", PromQL: "metric"})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 1 {
		t.Fatal("unknown namespace sent")
	}
}
