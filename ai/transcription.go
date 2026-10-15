package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type TranscriptionRequest struct {
	Model, Language, ResponseFormat string
	FileName, Audio                 types.ProtectedValue
	MaxAudioBytes                   uint64
}
type TranscriptionResult struct {
	ResponseFormat string
	Response       Transcription
}

type audioBoundedWriter struct {
	writer    io.Writer
	remaining uint64
}

func (w *audioBoundedWriter) Write(p []byte) (int, error) {
	if uint64(len(p)) > w.remaining {
		return 0, types.NewError(types.InvalidArgument)
	}
	n, e := w.writer.Write(p)
	w.remaining -= uint64(n)
	return n, e
}

func (c *InferenceClient) Transcribe(ctx context.Context, request TranscriptionRequest) (types.Result[TranscriptionResult], error) {
	op, validation := c.operation("audio_transcriptions")
	file := string(request.FileName.Reveal())
	if strings.TrimSpace(request.Model) == "" || file == "" || len(file) > 255 || strings.ContainsAny(file, "\r\n\x00") || request.Audio.Empty() || request.MaxAudioBytes == 0 || request.MaxAudioBytes >= uint64(^uint(0)>>1) || (request.ResponseFormat != "json" && request.ResponseFormat != "verbose_json") {
		validation = types.NewError(types.InvalidArgument)
	}
	var body bytes.Buffer
	multipartBody := multipart.NewWriter(&body)
	if validation == nil {
		part, e := multipartBody.CreateFormFile("file", file)
		if e != nil {
			validation = types.NewError(types.InvalidArgument)
		} else if _, e = request.Audio.CopyTo(&audioBoundedWriter{writer: part, remaining: request.MaxAudioBytes}); e != nil {
			validation = types.NewError(types.InvalidArgument)
		}
		for _, field := range []struct{ name, value string }{{"model", request.Model}, {"language", request.Language}, {"response_format", request.ResponseFormat}} {
			if field.name == "language" && field.value == "" {
				continue
			}
			if e := multipartBody.WriteField(field.name, field.value); e != nil {
				validation = types.NewError(types.InvalidArgument)
			}
		}
	}
	if e := multipartBody.Close(); e != nil {
		validation = types.NewError(types.InvalidArgument)
	}
	op.Headers.Set("Content-Type", multipartBody.FormDataContentType())
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/audio/transcriptions", Body: body.Bytes(), Transfer: true, Validate: func() error { return validation }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[TranscriptionResult], error) {
		var out Transcription
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil || (request.ResponseFormat == "verbose_json" && (out.Language == nil || out.Duration == nil)) {
			return core.Decoded[TranscriptionResult]{}, types.NewError(types.ProtocolError)
		}
		if out.Usage != nil {
			switch *out.Usage.Type {
			case "tokens":
				if out.Usage.InputTokens == nil || out.Usage.OutputTokens == nil || out.Usage.TotalTokens == nil {
					return core.Decoded[TranscriptionResult]{}, types.NewError(types.ProtocolError)
				}
			case "duration":
				if out.Usage.Seconds == nil {
					return core.Decoded[TranscriptionResult]{}, types.NewError(types.ProtocolError)
				}
			default:
				return core.Decoded[TranscriptionResult]{}, types.NewError(types.ProtocolError)
			}
		}
		return core.Decoded[TranscriptionResult]{Data: TranscriptionResult{ResponseFormat: request.ResponseFormat, Response: out}}, nil
	})
}
