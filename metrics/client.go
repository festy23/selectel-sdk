// Package metrics reads the documented Prometheus interfaces of Selectel Metrics.
package metrics

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, scope types.Scope) *Client { return &Client{runtime: r, scope: scope} }

type Sample struct {
	Timestamp types.Number `json:"timestamp"`
	Value     string       `json:"value"`
}
type Histogram struct {
	Count   string   `json:"count"`
	Sum     string   `json:"sum"`
	Buckets []Bucket `json:"buckets,omitzero"`
}
type Bucket struct {
	Rule  types.Integer `json:"rule"`
	Lower string        `json:"lower"`
	Upper string        `json:"upper"`
	Count string        `json:"count"`
}
type HistogramSample struct {
	Timestamp types.Number `json:"timestamp"`
	Histogram Histogram    `json:"histogram"`
}
type Series struct {
	Labels     map[string]string `json:"metric"`
	Samples    []Sample          `json:"values,omitzero"`
	Histograms []HistogramSample `json:"histograms,omitzero"`
}
type QueryData struct {
	Type           string                 `json:"resultType"`
	Series         []Series               `json:"series,omitzero"`
	Scalar         *Sample                `json:"scalar,omitempty"`
	String         *Sample                `json:"string,omitempty"`
	Warnings       []types.ProtectedValue `json:"warnings,omitzero"`
	Infos          []types.ProtectedValue `json:"infos,omitzero"`
	Complete       bool                   `json:"complete"`
	Namespace      string                 `json:"namespace"`
	EvaluationTime *time.Time             `json:"evaluation_time,omitempty"`
	Start          *time.Time             `json:"start,omitempty"`
	End            *time.Time             `json:"end,omitempty"`
	Step           *time.Duration         `json:"-"`
	StepSeconds    string                 `json:"step_seconds,omitempty"`
}
type QueryRequest struct {
	Namespace, PromQL string
	EvaluationTime    *time.Time
}
type RangeRequest struct {
	Namespace, PromQL string
	Start, End        time.Time
	Step              time.Duration
}

func validNamespace(value string) bool {
	return value == "compute" || value == "dedicated" || value == "inference"
}
func (c *Client) operation(id string) (core.Operation, error) {
	return c.runtime.BindDocumented(core.Operation{ID: id, Service: "metrics", APIGeneration: "prometheus", Method: "GET", Effect: "read", Auth: types.Basic, RequireProject: true, RequireLocation: true, Success: []int{200}, Headers: http.Header{"Accept": {"application/json"}}}, c.scope)
}
func (c *Client) base(namespace string) (string, error) {
	if !validNamespace(namespace) {
		return "", types.NewError(types.InvalidArgument)
	}
	return "/projects/" + url.PathEscape(c.scope.ProjectID) + "/namespaces/" + url.PathEscape(namespace), nil
}
func (c *Client) Query(ctx context.Context, r QueryRequest) (types.Result[QueryData], error) {
	op, e := c.operation("metrics.prometheus.samples.query")
	if e != nil {
		return types.Result[QueryData]{}, e
	}
	path, e := c.base(r.Namespace)
	if strings.TrimSpace(r.PromQL) == "" {
		e = types.NewError(types.InvalidArgument)
	}
	q := url.Values{"query": {r.PromQL}}
	var at *time.Time
	if r.EvaluationTime != nil {
		v := r.EvaluationTime.UTC()
		at = &v
		q.Set("time", v.Format(time.RFC3339Nano))
	}
	decode := c.queryDecoder(false)
	result, err := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/api/v1/query", Query: q, Validate: func() error { return e }}, decode)
	if err == nil || types.ErrorCode(err) == types.ResponseLimit {
		result.Data.Namespace = r.Namespace
		result.Data.EvaluationTime = at
	}
	return result, err
}
func seconds(duration time.Duration) string {
	whole := int64(duration) / int64(time.Second)
	fraction := int64(duration) % int64(time.Second)
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return strconv.FormatInt(whole, 10) + "." + strings.TrimRight(strconv.FormatInt(fraction+1_000_000_000, 10)[1:], "0")
}
func (c *Client) QueryRange(ctx context.Context, r RangeRequest) (types.Result[QueryData], error) {
	op, e := c.operation("metrics.prometheus.samples.query_range")
	if e != nil {
		return types.Result[QueryData]{}, e
	}
	path, e := c.base(r.Namespace)
	if strings.TrimSpace(r.PromQL) == "" || r.Start.IsZero() || r.End.IsZero() || !r.Start.Before(r.End) || r.Step <= 0 {
		e = types.NewError(types.InvalidArgument)
	}
	start, end := r.Start.UTC(), r.End.UTC()
	step := r.Step
	q := url.Values{"query": {r.PromQL}, "start": {start.Format(time.RFC3339Nano)}, "end": {end.Format(time.RFC3339Nano)}, "step": {seconds(step)}}
	result, err := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/api/v1/query_range", Query: q, Validate: func() error { return e }}, c.queryDecoder(true))
	if err == nil || types.ErrorCode(err) == types.ResponseLimit {
		result.Data.Namespace = r.Namespace
		result.Data.Start = &start
		result.Data.End = &end
		result.Data.Step = &step
		result.Data.StepSeconds = seconds(step)
	}
	return result, err
}
func pair(raw json.RawMessage) (Sample, error) {
	var parts []json.RawMessage
	var out Sample
	if json.Unmarshal(raw, &parts) != nil || len(parts) != 2 || json.Unmarshal(parts[0], &out.Timestamp) != nil || json.Unmarshal(parts[1], &out.Value) != nil {
		return Sample{}, types.NewError(types.ProtocolError)
	}
	return out, nil
}
func histogram(raw json.RawMessage) (HistogramSample, error) {
	var parts []json.RawMessage
	var out HistogramSample
	if json.Unmarshal(raw, &parts) != nil || len(parts) != 2 || json.Unmarshal(parts[0], &out.Timestamp) != nil {
		return out, types.NewError(types.ProtocolError)
	}
	var body struct {
		Count, Sum *string
		Buckets    []json.RawMessage
	}
	if json.Unmarshal(parts[1], &body) != nil || body.Count == nil || body.Sum == nil {
		return out, types.NewError(types.ProtocolError)
	}
	out.Histogram.Count = *body.Count
	out.Histogram.Sum = *body.Sum
	if body.Buckets != nil {
		out.Histogram.Buckets = []Bucket{}
	}
	for _, data := range body.Buckets {
		var fields []json.RawMessage
		var b Bucket
		if json.Unmarshal(data, &fields) != nil || len(fields) != 4 || json.Unmarshal(fields[0], &b.Rule) != nil || json.Unmarshal(fields[1], &b.Lower) != nil || json.Unmarshal(fields[2], &b.Upper) != nil || json.Unmarshal(fields[3], &b.Count) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Histogram.Buckets = append(out.Histogram.Buckets, b)
	}
	return out, nil
}
func (c *Client) queryDecoder(rangeQuery bool) core.Decoder[QueryData] {
	return func(raw []byte, _ types.EndpointBinding) (core.Decoded[QueryData], error) {
		var envelope struct {
			Status          string
			Warnings, Infos []types.ProtectedValue
			Data            *struct {
				Type   string `json:"resultType"`
				Result json.RawMessage
			}
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Status != "success" || envelope.Data == nil {
			return core.Decoded[QueryData]{}, types.NewError(types.ProtocolError)
		}
		out := QueryData{Type: envelope.Data.Type, Warnings: envelope.Warnings, Infos: envelope.Infos, Complete: true}
		if rangeQuery && out.Type != "matrix" {
			return core.Decoded[QueryData]{}, types.NewError(types.ProtocolError)
		}
		if out.Type == "scalar" || out.Type == "string" {
			v, e := pair(envelope.Data.Result)
			if e != nil {
				return core.Decoded[QueryData]{}, e
			}
			if out.Type == "scalar" {
				if !metricValue.MatchString(v.Value) {
					return core.Decoded[QueryData]{}, types.NewError(types.ProtocolError)
				}
				out.Scalar = &v
			} else {
				out.String = &v
			}
			return core.Decoded[QueryData]{Data: out}, nil
		}
		if out.Type != "vector" && out.Type != "matrix" {
			return core.Decoded[QueryData]{}, types.NewError(types.ProtocolError)
		}
		var rows []struct {
			Labels     map[string]string `json:"metric"`
			Value      json.RawMessage   `json:"value"`
			Values     []json.RawMessage `json:"values"`
			Histogram  json.RawMessage   `json:"histogram"`
			Histograms []json.RawMessage `json:"histograms"`
		}
		if json.Unmarshal(envelope.Data.Result, &rows) != nil || rows == nil {
			return core.Decoded[QueryData]{}, types.NewError(types.ProtocolError)
		}
		out.Series = []Series{}
		count := uint(0)
		for _, row := range rows {
			if row.Labels == nil {
				return core.Decoded[QueryData]{}, types.NewError(types.ProtocolError)
			}
			series := Series{Labels: row.Labels}
			values := row.Values
			histograms := row.Histograms
			if out.Type == "vector" {
				if len(row.Value) > 0 {
					values = []json.RawMessage{row.Value}
				}
				if len(row.Histogram) > 0 {
					histograms = []json.RawMessage{row.Histogram}
				}
				if (len(values) == 0) == (len(histograms) == 0) {
					return core.Decoded[QueryData]{}, types.NewError(types.ProtocolError)
				}
			}
			for _, v := range values {
				if count >= c.runtime.MetricSampleLimit() {
					out.Complete = false
					if len(series.Samples) > 0 || len(series.Histograms) > 0 {
						out.Series = append(out.Series, series)
					}
					return core.Decoded[QueryData]{Data: out, PartialRead: true}, types.NewError(types.ResponseLimit)
				}
				sample, e := pair(v)
				if e != nil {
					return core.Decoded[QueryData]{}, e
				}
				if !metricValue.MatchString(sample.Value) {
					return core.Decoded[QueryData]{}, types.NewError(types.ProtocolError)
				}
				series.Samples = append(series.Samples, sample)
				count++
			}
			for _, v := range histograms {
				if count >= c.runtime.MetricSampleLimit() {
					out.Complete = false
					if len(series.Samples) > 0 || len(series.Histograms) > 0 {
						out.Series = append(out.Series, series)
					}
					return core.Decoded[QueryData]{Data: out, PartialRead: true}, types.NewError(types.ResponseLimit)
				}
				sample, e := histogram(v)
				if e != nil {
					return core.Decoded[QueryData]{}, e
				}
				series.Histograms = append(series.Histograms, sample)
				count++
			}
			out.Series = append(out.Series, series)
		}
		return core.Decoded[QueryData]{Data: out}, nil
	}
}
