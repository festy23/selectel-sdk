package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/ai"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func TestInferenceCompletionExactParametersAndProtectedText(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v1/completions" || string(raw) != `{"model":"m","prompt":"synthetic-prompt","temperature":0.70,"max_tokens":16}` || r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Fatalf("request path=%s body=%s", r.URL.Path, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"c","object":"text_completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"length","text":"synthetic-completion-text"}],"usage":{"prompt_tokens":1,"completion_tokens":16,"total_tokens":17}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	temperature, _ := types.ParseNumber("0.70")
	maximum, _ := types.ParseInteger("16")
	p, _ := c.Project(scope.ProjectID)
	result, err := p.Inference().Complete(context.Background(), ai.CompletionRequest{Model: "m", Prompt: types.Protect([]byte("synthetic-prompt")), Temperature: &temperature, MaxTokens: &maximum})
	if err != nil || calls != 1 || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Data.Choices) != 1 || string(result.Data.Choices[0].Text.Reveal()) != "synthetic-completion-text" || result.Data.Usage == nil || result.Data.Usage.CompletionTokens.String() != "16" {
		t.Fatalf("error=%v result=%+v", err, result)
	}
}

func TestInferenceEmbeddingsProtectExactVectorValues(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
	c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v1/embeddings" || string(raw) != `{"model":"m","input":["synthetic-embedding-input"],"encoding_format":"float"}` {
			t.Fatalf("wire path=%s body=%s", r.URL.Path, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"object":"list","model":"m","data":[{"object":"embedding","index":0,"embedding":[0.12345678901234567890123456789,-0.5]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project(scope.ProjectID)
	result, err := p.Inference().Embed(context.Background(), ai.EmbeddingRequest{Model: "m", Input: []types.ProtectedValue{types.Protect([]byte("synthetic-embedding-input"))}, EncodingFormat: "float"})
	if err != nil || !result.Data.Complete || len(result.Data.Response.Data) != 1 || result.Data.Response.Data[0].Embedding == nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted {
		t.Fatalf("error=%v result=%+v", err, result)
	}
	vector := result.Data.Response.Data[0].Embedding.Reveal()
	if len(vector) != 2 || vector[0].String() != "0.12345678901234567890123456789" {
		t.Fatal("vector precision lost")
	}
	raw, _ := json.Marshal(result)
	for _, s := range []string{string(raw), fmt.Sprintf("%+v", result), fmt.Sprintf("%#v", result)} {
		if strings.Contains(s, "0.12345678901234567890123456789") {
			t.Fatal("vector output leaked")
		}
	}
	vector[0], _ = types.ParseNumber("999")
	if result.Data.Response.Data[0].Embedding.Reveal()[0].String() != "0.12345678901234567890123456789" {
		t.Fatal("Reveal aliases protected vector")
	}
}

func TestInferenceRerankPreservesOrdinalAndProtectedScore(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
	c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v1/rerank" || string(raw) != `{"model":"m","query":"synthetic-query","documents":["first-document","second-document"]}` {
			t.Fatalf("wire path=%s body=%s", r.URL.Path, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"model":"m","usage":{"total_tokens":10},"results":[{"index":1,"relevance_score":0.123456789012345678901,"document":{"text":"second-document"}},{"index":0,"relevance_score":0.1,"document":"first-document"}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project(scope.ProjectID)
	req := ai.RerankRequest{Model: "m", Query: types.Protect([]byte("synthetic-query")), Documents: []types.ProtectedValue{types.Protect([]byte("first-document")), types.Protect([]byte("second-document"))}}
	result, err := p.Inference().Rerank(context.Background(), req)
	if err != nil || len(result.Data.Results) != 2 || result.Data.Results[0].Index.String() != "1" || result.Data.Results[0].RelevanceScore.Reveal().String() != "0.123456789012345678901" || result.Data.Results[0].Document.Form != "object" || string(result.Data.Results[0].Document.Text.Reveal()) != "second-document" || result.Receipt == nil || result.Receipt.Outcome != types.Accepted {
		t.Fatalf("error=%v result=%+v", err, result)
	}
	raw, _ := json.Marshal(result)
	for _, s := range []string{string(raw), fmt.Sprintf("%+v", result), fmt.Sprintf("%#v", result)} {
		if strings.Contains(s, "0.123456789012345678901") || strings.Contains(s, "second-document") {
			t.Fatal("rerank output leaked")
		}
	}
}

func TestInferenceTranscriptionMultipartAndTypedFormats(t *testing.T) {
	for _, format := range []string{"json", "verbose_json"} {
		t.Run(format, func(t *testing.T) {
			language := "en"
			expectedFields := 4
			if format == "verbose_json" {
				language = ""
				expectedFields = 3
			}
			scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
			c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
			}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				media, params, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if e != nil || media != "multipart/form-data" || r.URL.Path != "/v1/audio/transcriptions" {
					t.Fatal("wrong multipart route/header")
				}
				reader := multipart.NewReader(r.Body, params["boundary"])
				fields := map[string]string{}
				for {
					part, e := reader.NextPart()
					if e == io.EOF {
						break
					}
					if e != nil {
						t.Fatal(e)
					}
					raw, _ := io.ReadAll(part)
					fields[part.FormName()] = string(raw)
					if part.FormName() == "file" && part.FileName() != "synthetic.wav" {
						t.Fatal("wrong upload filename")
					}
				}
				if fields["file"] != "synthetic-audio" || fields["model"] != "m" || fields["language"] != language || fields["response_format"] != format || len(fields) != expectedFields {
					t.Fatalf("multipart names/mapping wrong")
				}
				body := `{"text":"synthetic-transcript","usage":{"type":"duration","seconds":1.25}}`
				if format == "verbose_json" {
					body = `{"text":"synthetic-transcript","language":"english","duration":1.25,"segments":[{"id":0,"seek":0,"start":0,"end":1.25,"text":"synthetic-transcript","tokens":[42,9007199254740993],"temperature":0,"avg_logprob":-0.1,"compression_ratio":1.1,"no_speech_prob":0.01}],"words":[{"word":"synthetic-word","start":0,"end":1.25}]}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			p, _ := c.Project(scope.ProjectID)
			result, err := p.Inference().Transcribe(context.Background(), ai.TranscriptionRequest{Model: "m", Language: language, ResponseFormat: format, FileName: types.Protect([]byte("synthetic.wav")), Audio: types.Protect([]byte("synthetic-audio")), MaxAudioBytes: 1024})
			if err != nil || result.Data.ResponseFormat != format || result.Data.Response.Text == nil || string(result.Data.Response.Text.Reveal()) != "synthetic-transcript" || result.Receipt == nil || result.Receipt.Outcome != types.Accepted {
				t.Fatalf("error=%v result=%+v", err, result)
			}
			if format == "verbose_json" {
				if len(result.Data.Response.Segments) != 1 || result.Data.Response.Segments[0].Tokens.Reveal()[1].String() != "9007199254740993" {
					t.Fatal("verbose tokens lost")
				}
			}
			raw, _ := json.Marshal(result)
			for _, s := range []string{string(raw), fmt.Sprintf("%+v", result), fmt.Sprintf("%#v", result)} {
				if strings.Contains(s, "synthetic-transcript") || strings.Contains(s, "synthetic-word") || strings.Contains(s, "9007199254740993") {
					t.Fatal("transcription output leaked")
				}
			}
		})
	}
}

func TestAIAdditionalModelCallsPreserveMutationReceipts(t *testing.T) {
	type call func(*sdk.Client) (*types.Receipt, error)
	requestChat := func(c *sdk.Client) *ai.InferenceClient { p, _ := c.Project("project-one"); return p.Inference() }
	one, _ := types.ParseInteger("1")
	calls := map[string]call{
		"completion": func(c *sdk.Client) (*types.Receipt, error) {
			r, e := requestChat(c).Complete(context.Background(), ai.CompletionRequest{Model: "m", Prompt: types.Protect([]byte("p"))})
			return r.Receipt, e
		},
		"embedding": func(c *sdk.Client) (*types.Receipt, error) {
			r, e := requestChat(c).Embed(context.Background(), ai.EmbeddingRequest{Model: "m", Input: []types.ProtectedValue{types.Protect([]byte("p"))}, EncodingFormat: "float"})
			return r.Receipt, e
		},
		"rerank": func(c *sdk.Client) (*types.Receipt, error) {
			r, e := requestChat(c).Rerank(context.Background(), ai.RerankRequest{Model: "m", Query: types.Protect([]byte("p")), Documents: []types.ProtectedValue{types.Protect([]byte("d"))}})
			return r.Receipt, e
		},
		"transcription": func(c *sdk.Client) (*types.Receipt, error) {
			r, e := requestChat(c).Transcribe(context.Background(), ai.TranscriptionRequest{Model: "m", ResponseFormat: "json", FileName: types.Protect([]byte("a.wav")), Audio: types.Protect([]byte("audio")), MaxAudioBytes: 1024})
			return r.Receipt, e
		},
		"images": func(c *sdk.Client) (*types.Receipt, error) {
			r, e := c.Account().AIRouter().Images(context.Background(), ai.ImageRequest{Model: "m", Prompt: types.Protect([]byte("p")), N: one, ResponseFormat: "url"})
			return r.Receipt, e
		},
	}
	for name, invoke := range calls {
		for _, tc := range []struct {
			name       string
			configured bool
			status     int
			outcome    types.Outcome
			code       types.Code
		}{
			{"missing endpoint", false, 200, types.NotSent, types.EndpointUnavailable},
			{"unknown no replay", true, 0, types.Unknown, types.UnknownOutcome},
			{"accepted malformed", true, 200, types.Accepted, types.ProtocolError},
			{"rejected", true, 429, types.Rejected, types.RateLimited},
		} {
			if name == "images" && !tc.configured {
				continue
			}
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				lookups, sends := 0, 0
				scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
				cfg := sdk.Config{AccountID: scope.AccountID, ReadMaxAttempts: 5, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
					lookups++
					return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
				}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
					sends++
					if tc.status == 0 {
						return nil, io.ErrUnexpectedEOF
					}
					return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				})}
				if tc.configured {
					cfg.EndpointOverrides = map[types.EndpointKey]types.EndpointBinding{{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}
				}
				c, e := sdk.New(cfg)
				if e != nil {
					t.Fatal(e)
				}
				receipt, e := invoke(c)
				expected := 1
				if !tc.configured {
					expected = 0
				}
				if types.ErrorCode(e) != tc.code || lookups != expected || sends != expected || receipt == nil || receipt.Outcome != tc.outcome || receipt.Attempts != uint(expected) || receipt.ContractRevision == "" || receipt.Recovery != "unavailable" || len(receipt.ResourceRefs) != 0 {
					t.Fatalf("error=%v receipt=%+v lookups=%d sends=%d", e, receipt, lookups, sends)
				}
			})
		}
	}
}

func TestInferenceCompletionPreservesExplicitZeroTokenBound(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
	sends := 0
	c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "key", Revision: "one", Binding: scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("key"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		var wire map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&wire) != nil || string(wire["max_tokens"]) != "0" {
			t.Fatal("explicit zero bound lost")
		}
		return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	zero, _ := types.ParseInteger("0")
	p, _ := c.Project(scope.ProjectID)
	result, err := p.Inference().Complete(context.Background(), ai.CompletionRequest{Model: "m", Prompt: types.Protect([]byte("p")), MaxTokens: &zero})
	if types.ErrorCode(err) != types.RateLimited || sends != 1 || result.Receipt == nil || result.Receipt.Outcome != types.Rejected {
		t.Fatalf("error=%v receipt=%+v sends=%d", err, result.Receipt, sends)
	}
}

func TestInferenceTranscriptionRejectsUnboundedOrInvalidUploadBeforeIO(t *testing.T) {
	for _, tc := range []struct {
		name, filename, format string
		bound                  uint64
	}{{"oversized", "a.wav", "json", 1}, {"unbounded", "a.wav", "json", 0}, {"header injection", "a\r\nsecret.wav", "json", 1024}, {"unconfirmed format", "a.wav", "text", 1024}} {
		t.Run(tc.name, func(t *testing.T) {
			scope := types.Scope{AccountID: "test-account", ProjectID: "project-one"}
			ioCalls := 0
			c, err := sdk.New(sdk.Config{AccountID: scope.AccountID, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "dedicated_inference", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://inference.invalid/v1", Scope: scope, APIGeneration: "v1", Audience: credentials.SelectelAudience}}, Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
				ioCalls++
				return credentials.Material{}, nil
			}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { ioCalls++; return nil, context.Canceled })})
			if err != nil {
				t.Fatal(err)
			}
			p, _ := c.Project(scope.ProjectID)
			result, err := p.Inference().Transcribe(context.Background(), ai.TranscriptionRequest{Model: "m", ResponseFormat: tc.format, FileName: types.Protect([]byte(tc.filename)), Audio: types.Protect([]byte("synthetic-audio")), MaxAudioBytes: tc.bound})
			if types.ErrorCode(err) != types.InvalidArgument || ioCalls != 0 || result.Receipt == nil || result.Receipt.Outcome != types.NotSent || result.Receipt.Attempts != 0 || result.Receipt.ContractRevision == "" {
				t.Fatalf("error=%v receipt=%+v I/O=%d", err, result.Receipt, ioCalls)
			}
		})
	}
}
