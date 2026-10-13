package metrics

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

type ScrapeRequest struct{ Namespace string }
type TextSample struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Value     string            `json:"value"`
	Timestamp *types.Integer    `json:"timestamp_ms,omitempty"`
}
type Definition struct {
	Name string                `json:"name"`
	Kind string                `json:"kind,omitempty"`
	Help *types.ProtectedValue `json:"help,omitempty"`
}
type ScrapeData struct {
	Samples     []TextSample `json:"samples"`
	Definitions []Definition `json:"definitions,omitzero"`
	Complete    bool         `json:"complete"`
	Namespace   string       `json:"namespace"`
}

var metricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
var labelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var metricValue = regexp.MustCompile(`^([+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?|NaN|[+-]?Inf)$`)

func space(value string) string { return strings.TrimLeft(value, " \t") }
func word(value string) (string, string) {
	for i, c := range value {
		if c == ' ' || c == '\t' {
			return value[:i], space(value[i:])
		}
	}
	return value, ""
}
func quoted(value string) (string, string, error) {
	if value == "" || value[0] != '"' {
		return "", "", types.NewError(types.ProtocolError)
	}
	var out strings.Builder
	for i := 1; i < len(value); i++ {
		c := value[i]
		if c == '"' {
			return out.String(), value[i+1:], nil
		}
		if c == '\n' || c == '\r' {
			return "", "", types.NewError(types.ProtocolError)
		}
		if c == '\\' {
			i++
			if i >= len(value) {
				return "", "", types.NewError(types.ProtocolError)
			}
			switch value[i] {
			case '\\', '"':
				out.WriteByte(value[i])
			case 'n':
				out.WriteByte('\n')
			default:
				return "", "", types.NewError(types.ProtocolError)
			}
		} else {
			out.WriteByte(c)
		}
	}
	return "", "", types.NewError(types.ProtocolError)
}
func labels(value string) (map[string]string, string, error) {
	out := map[string]string{}
	value = value[1:]
	for {
		value = space(value)
		if value == "" {
			return nil, "", types.NewError(types.ProtocolError)
		}
		if value[0] == '}' {
			return out, value[1:], nil
		}
		end := strings.IndexAny(value, "= \t}")
		if end < 1 {
			return nil, "", types.NewError(types.ProtocolError)
		}
		key := value[:end]
		if !labelName.MatchString(key) {
			return nil, "", types.NewError(types.ProtocolError)
		}
		value = space(value[end:])
		if value == "" || value[0] != '=' {
			return nil, "", types.NewError(types.ProtocolError)
		}
		var text string
		var e error
		text, value, e = quoted(space(value[1:]))
		if e != nil {
			return nil, "", e
		}
		if _, exists := out[key]; exists {
			return nil, "", types.NewError(types.ProtocolError)
		}
		out[key] = text
		value = space(value)
		if value == "" {
			return nil, "", types.NewError(types.ProtocolError)
		}
		if value[0] == '}' {
			return out, value[1:], nil
		}
		if value[0] != ',' {
			return nil, "", types.NewError(types.ProtocolError)
		}
		value = value[1:]
	}
}
func textSample(line string) (TextSample, error) {
	var out TextSample
	end := strings.IndexAny(line, "{ \t")
	if end < 0 {
		return out, types.NewError(types.ProtocolError)
	}
	out.Name = line[:end]
	if !metricName.MatchString(out.Name) {
		return out, types.NewError(types.ProtocolError)
	}
	remaining := line[end:]
	out.Labels = map[string]string{}
	if remaining[0] == '{' {
		var e error
		out.Labels, remaining, e = labels(remaining)
		if e != nil {
			return out, e
		}
	}
	if remaining == "" || (remaining[0] != ' ' && remaining[0] != '\t') {
		return out, types.NewError(types.ProtocolError)
	}
	remaining = space(remaining)
	out.Value, remaining = word(remaining)
	if !metricValue.MatchString(out.Value) {
		return out, types.NewError(types.ProtocolError)
	}
	if remaining != "" {
		timestamp, rest := word(remaining)
		if rest != "" {
			return out, types.NewError(types.ProtocolError)
		}
		v, e := types.ParseInteger(timestamp)
		if e != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Timestamp = &v
	}
	return out, nil
}
func (c *Client) Scrape(ctx context.Context, r ScrapeRequest) (types.Result[ScrapeData], error) {
	op, e := c.operation("metrics.prometheus.samples.scrape")
	if e != nil {
		return types.Result[ScrapeData]{}, e
	}
	op.Headers = http.Header{"Accept": {"text/plain; version=0.0.4"}}
	path, e := c.base(r.Namespace)
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[ScrapeData], error) {
		out := ScrapeData{Samples: []TextSample{}, Complete: true, Namespace: r.Namespace}
		if !utf8.Valid(raw) {
			return core.Decoded[ScrapeData]{}, types.NewError(types.ProtocolError)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "#") {
				text := space(strings.TrimPrefix(line, "#"))
				kind, rest := word(text)
				if kind != "HELP" && kind != "TYPE" {
					continue
				}
				name, value := word(rest)
				if !metricName.MatchString(name) {
					return core.Decoded[ScrapeData]{}, types.NewError(types.ProtocolError)
				}
				d := Definition{Name: name}
				if kind == "HELP" {
					v := types.Protect([]byte(value))
					d.Help = &v
				} else {
					if value != "counter" && value != "gauge" && value != "histogram" && value != "summary" && value != "untyped" {
						return core.Decoded[ScrapeData]{}, types.NewError(types.ProtocolError)
					}
					d.Kind = value
				}
				out.Definitions = append(out.Definitions, d)
				continue
			}
			if uint(len(out.Samples)) >= c.runtime.MetricSampleLimit() {
				out.Complete = false
				return core.Decoded[ScrapeData]{Data: out, PartialRead: true}, types.NewError(types.ResponseLimit)
			}
			sample, e := textSample(line)
			if e != nil {
				return core.Decoded[ScrapeData]{}, e
			}
			out.Samples = append(out.Samples, sample)
		}
		return core.Decoded[ScrapeData]{Data: out}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/prometheus/metrics", Validate: func() error { return e }}, decode)
}
