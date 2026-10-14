package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/metrics"
	"github.com/festy23/selectel-sdk/types"
)

func TestMetricsArbitraryStringResultIsProtectedAndNullIsInvalid(t *testing.T) {
	for _, value := range []string{`"synthetic-private-result"`, `null`} {
		t.Run(value, func(t *testing.T) {
			c := telemetryClient(t, credentials.Basic, func(r *http.Request) (*http.Response, error) {
				verifyBasic(t, r)
				return inventoryReply(200, `{"status":"success","data":{"resultType":"string","result":[1.12345678901234567890,`+value+`]}}`), nil
			}, 0, 0)
			out, e := regionalView(t, c, "ru-1").Metrics().Query(context.Background(), metrics.QueryRequest{Namespace: "compute", PromQL: "metric"})
			if value == "null" {
				if types.ErrorCode(e) != types.ProtocolError || out.Data.Complete {
					t.Fatal("null string sample accepted", e)
				}
				return
			}
			if e != nil || out.Data.String.Timestamp.String() != "1.12345678901234567890" {
				t.Fatal("string timestamp changed", e)
			}
			if string(out.Data.String.Value.Reveal()) != "synthetic-private-result" {
				t.Fatal("explicit string reveal changed")
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "synthetic-private-result") || strings.Contains(fmt.Sprintf("%+v", out.Data.String), "synthetic-private-result") {
				t.Fatal("arbitrary raw string leaked")
			}
		})
	}
}

func TestMetricsConfirmedNamespacesAcrossReadInterfacesAndDedicatedAlias(t *testing.T) {
	for _, namespace := range []string{"compute", "dedicated", "inference"} {
		for _, method := range []string{"scrape", "query", "range"} {
			t.Run(namespace+"/"+method, func(t *testing.T) {
				c := telemetryClient(t, credentials.Basic, func(r *http.Request) (*http.Response, error) {
					verifyBasic(t, r)
					if r.URL.Host != "ru-1.metrics.selcloud.ru" || !strings.HasPrefix(r.URL.Path, "/projects/test-project/namespaces/"+namespace+"/") {
						t.Fatal("scope/namespace endpoint changed")
					}
					if method == "scrape" {
						return inventoryReply(200, "metric 9007199254740993\n"), nil
					}
					kind := "vector"
					if method == "range" {
						kind = "matrix"
					}
					return inventoryReply(200, `{"status":"success","data":{"resultType":"`+kind+`","result":[]}}`), nil
				}, 0, 0)
				project := projectView(t, c)
				view, e := project.Location("spb", "SPB-3")
				if e != nil {
					t.Fatal(e)
				}
				if method == "scrape" {
					out, err := view.Metrics().Scrape(context.Background(), metrics.ScrapeRequest{Namespace: namespace})
					if err != nil || !out.Data.Complete || out.Data.Samples[0].Value != "9007199254740993" {
						t.Fatal(err)
					}
				} else if method == "query" {
					out, err := view.Metrics().Query(context.Background(), metrics.QueryRequest{Namespace: namespace, PromQL: "metric"})
					if err != nil || !out.Data.Complete {
						t.Fatal(err)
					}
				} else {
					out, err := view.Metrics().QueryRange(context.Background(), metrics.RangeRequest{Namespace: namespace, PromQL: "metric", Start: time.Unix(1, 0), End: time.Unix(2, 0), Step: time.Second})
					if err != nil || !out.Data.Complete {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestMetricsPartialRangeRetainsBoundsAndProtectedProviderAnnotations(t *testing.T) {
	c := telemetryClient(t, credentials.Basic, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"status":"success","warnings":["synthetic-private-warning"],"infos":["synthetic-private-info"],"data":{"resultType":"matrix","result":[{"metric":{"__name__":"metric"},"values":[[1.000000000000000001,"9007199254740993"],[2,"2"]]}]}}`), nil
	}, 1, 0)
	start, end := time.Unix(1, 0), time.Unix(3, 0)
	out, e := regionalView(t, c, "ru-1").Metrics().QueryRange(context.Background(), metrics.RangeRequest{Namespace: "inference", PromQL: "metric", Start: start, End: end, Step: time.Nanosecond})
	if types.ErrorCode(e) != types.ResponseLimit || out.Data.Complete || len(out.Data.Series) != 1 || len(out.Data.Series[0].Samples) != 1 || out.Data.Series[0].Samples[0].Timestamp.String() != "1.000000000000000001" || out.Data.Series[0].Samples[0].Value != "9007199254740993" || out.Data.Namespace != "inference" || !out.Data.Start.Equal(start) || !out.Data.End.Equal(end) || out.Data.StepSeconds != "0.000000001" || len(out.Data.Warnings) != 1 || len(out.Data.Infos) != 1 {
		t.Fatal("bounded prefix lost metadata/precision", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") || strings.Contains(fmt.Sprintf("%#v", out.Data.Warnings), "synthetic-private") || strings.Contains(fmt.Sprintf("%+v", out.Data.Infos), "synthetic-private") {
		t.Fatal("partial annotations leaked")
	}
}
