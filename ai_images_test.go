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
	"github.com/festy23/selectel-sdk/ai"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func TestAIRouterImagesPreserveProtectedWireResultWithoutFetchingURL(t *testing.T) {
	for _, format := range []string{"url", "b64_json"} {
		t.Run(format, func(t *testing.T) {
			calls := 0
			c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "key", Revision: "one", Binding: r.Scope, Audience: r.Audience, Scheme: credentials.BearerToken, Token: types.Protect([]byte("synthetic-key"))}, nil
			}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				raw, _ := io.ReadAll(r.Body)
				expected := `{"model":"m","prompt":"synthetic-image-prompt","n":1,"response_format":"` + format + `"}`
				if r.URL.String() != "https://api.selectel.ru/aig/v1/images/generations" || string(raw) != expected {
					t.Fatalf("wire URL=%s body=%s", r.URL, raw)
				}
				body := `{"created":1,"data":[{"url":"https://private-image.invalid/?secret=synthetic-image-output","revised_prompt":"synthetic-revised-prompt"}]}`
				if format == "b64_json" {
					body = `{"created":1,"data":[{"b64_json":"c3ludGhldGljLWltYWdlLW91dHB1dA=="}],"usage":{"input_tokens":1,"input_tokens_details":{"image_tokens":0,"text_tokens":1},"output_tokens":2,"total_tokens":3}}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			n, _ := types.ParseInteger("1")
			result, err := c.Account().AIRouter().Images(context.Background(), ai.ImageRequest{Model: "m", Prompt: types.Protect([]byte("synthetic-image-prompt")), N: n, ResponseFormat: format})
			if err != nil || calls != 1 || !result.Data.Complete || len(result.Data.Response.Data) != 1 || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || len(result.Receipt.ResourceRefs) != 0 {
				t.Fatalf("error=%v result=%+v calls=%d", err, result, calls)
			}
			image := result.Data.Response.Data[0]
			if format == "url" {
				if image.URL == nil || !strings.Contains(string(image.URL.Reveal()), "synthetic-image-output") {
					t.Fatal("URL lost")
				}
			} else {
				if image.B64JSON == nil || string(image.B64JSON.Reveal()) != "c3ludGhldGljLWltYWdlLW91dHB1dA==" || result.Data.Response.Usage == nil || result.Data.Response.Usage.TotalTokens.String() != "3" {
					t.Fatal("base64/usage lost")
				}
			}
			raw, _ := json.Marshal(result)
			for _, s := range []string{string(raw), fmt.Sprintf("%+v", result), fmt.Sprintf("%#v", result)} {
				if strings.Contains(s, "synthetic-image-output") || strings.Contains(s, "synthetic-revised-prompt") || strings.Contains(s, "c3ludGhldGljLWltYWdlLW91dHB1dA==") {
					t.Fatal("image output leaked")
				}
			}
		})
	}
}
