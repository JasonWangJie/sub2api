package service

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

// openAIStreamTrace is an optional, attempt-local diagnostic. Callers decide
// which attempts to trace and whether to emit its bounded, payload-free snapshot.
// HTTP transport callbacks may race with SSE processing or final logging.
type openAIStreamTrace struct {
	mu       sync.Mutex
	start    time.Time
	snapshot openAIStreamTraceSnapshot
}

// Nil timestamps distinguish unseen stages from stages reached in the first
// millisecond. These are attempt-relative timings, not client-observed latency.
type openAIStreamTraceSnapshot struct {
	RequestWriteMs     *int64 `json:"request_write_ms,omitempty"`
	ResponseHeadersMs  *int64 `json:"response_headers_ms,omitempty"`
	FirstDataMs        *int64 `json:"first_data_ms,omitempty"`
	FirstSemanticMs    *int64 `json:"first_semantic_ms,omitempty"`
	FirstVisibleMs     *int64 `json:"first_visible_ms,omitempty"`
	FirstFlushMs       *int64 `json:"first_flush_ms,omitempty"`
	HTTPStatus         int    `json:"http_status,omitempty"`
	ContentType        string `json:"content_type,omitempty"`
	RequestWriteFailed bool   `json:"request_write_failed"`
	FirstEventType     string `json:"first_event_type,omitempty"`
	TransportCount     int64  `json:"transport_count"`
	EventCount         int64  `json:"event_count"`
	DeltaEventCount    int64  `json:"delta_event_count"`
	OutputTokens       *int64 `json:"output_tokens,omitempty"`
	ReasoningTokens    *int64 `json:"reasoning_tokens,omitempty"`
}

type openAIStreamTraceContextKey struct{}

func newOpenAIStreamTrace(start time.Time) *openAIStreamTrace {
	if start.IsZero() {
		start = time.Now()
	}
	return &openAIStreamTrace{start: start}
}

func withOpenAIStreamTrace(ctx context.Context, trace *openAIStreamTrace) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIStreamTraceContextKey{}, trace)
}

func openAIStreamTraceFromContext(ctx context.Context) *openAIStreamTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(openAIStreamTraceContextKey{}).(*openAIStreamTrace)
	return trace
}

func (t *openAIStreamTrace) elapsedMs() int64 {
	ms := time.Since(t.start).Milliseconds()
	if ms < 0 {
		return 0
	}
	return ms
}

// RecordTransport counts actual HTTP calls, including same-account retries
// sharing a Forward attempt. Calls rejected by admission are not transports.
func (t *openAIStreamTrace) RecordTransport() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.snapshot.TransportCount++
	t.mu.Unlock()
}

// RecordWroteRequest records the first transport write callback. Error text is
// deliberately discarded: transport errors may embed URLs or credentials.
func (t *openAIStreamTrace) RecordWroteRequest(err error) {
	if t == nil {
		return
	}
	ms := t.elapsedMs()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.snapshot.RequestWriteMs == nil {
		t.snapshot.RequestWriteMs = &ms
		t.snapshot.RequestWriteFailed = err != nil
	}
}

func (t *openAIStreamTrace) RecordHeaders(status int, contentType string) {
	if t == nil {
		return
	}
	ms := t.elapsedMs()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.snapshot.ResponseHeadersMs != nil {
		return
	}
	t.snapshot.ResponseHeadersMs = &ms
	if status >= 100 && status <= 599 {
		t.snapshot.HTTPStatus = status
	}
	t.snapshot.ContentType = openAIStreamTraceContentType(contentType)
}

// ObserveSSE counts nonempty SSE data payloads, including [DONE]. Comments and
// blank data lines are not events. It never retains the payload or any text from
// it: only whitelisted event names and numeric terminal usage are kept.
func (t *openAIStreamTrace) ObserveSSE(data []byte, eventType string) {
	if t == nil {
		return
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return
	}
	ms := t.elapsedMs()
	isDone := bytes.Equal(data, []byte("[DONE]"))
	eventType = strings.TrimSpace(eventType)
	if eventType == "" && !isDone {
		eventType = strings.TrimSpace(gjson.GetBytes(data, "type").String())
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot := &t.snapshot
	snapshot.EventCount++
	if strings.HasSuffix(eventType, ".delta") {
		snapshot.DeltaEventCount++
	}
	if snapshot.FirstDataMs == nil {
		snapshot.FirstDataMs = &ms
		if isDone {
			snapshot.FirstEventType = "done_sentinel"
		} else {
			snapshot.FirstEventType = openAIStreamTraceEventType(eventType)
		}
	}
	if isDone {
		return
	}
	// Classification is only needed until each boundary is first observed.
	// Steady-state token deltas require no body decoding or copying here.
	if snapshot.FirstSemanticMs == nil || snapshot.FirstVisibleMs == nil {
		text := string(data)
		if snapshot.FirstSemanticMs == nil && openAIStreamDataStartsSemanticTTFT(text, eventType) {
			snapshot.FirstSemanticMs = &ms
		}
		if snapshot.FirstVisibleMs == nil && openAIStreamDataStartsVisibleOutput(text, eventType) {
			snapshot.FirstVisibleMs = &ms
		}
	}
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete":
		if !gjson.ValidBytes(data) {
			return
		}
		usage := gjson.GetBytes(data, "response.usage")
		if !usage.IsObject() {
			usage = gjson.GetBytes(data, "usage")
		}
		if !usage.IsObject() {
			return
		}
		if tokens := openAIStreamTraceTokenCount(usage.Get("output_tokens")); tokens != nil {
			snapshot.OutputTokens = tokens
		} else if tokens := openAIStreamTraceTokenCount(usage.Get("completion_tokens")); tokens != nil {
			snapshot.OutputTokens = tokens
		}
		if tokens := openAIStreamTraceTokenCount(usage.Get("output_tokens_details.reasoning_tokens")); tokens != nil {
			snapshot.ReasoningTokens = tokens
		} else if tokens := openAIStreamTraceTokenCount(usage.Get("completion_tokens_details.reasoning_tokens")); tokens != nil {
			snapshot.ReasoningTokens = tokens
		}
	}
}

// RecordFlush is called only after a successful downstream flush. A transport
// flush (for example response.created) is deliberately separate from TTFT.
func (t *openAIStreamTrace) RecordFlush() {
	if t == nil {
		return
	}
	ms := t.elapsedMs()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.snapshot.FirstFlushMs == nil {
		t.snapshot.FirstFlushMs = &ms
	}
}

func (t *openAIStreamTrace) Snapshot() openAIStreamTraceSnapshot {
	if t == nil {
		return openAIStreamTraceSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot := t.snapshot
	// Do not expose mutable pointers shared with the live trace.
	for _, field := range []**int64{
		&snapshot.RequestWriteMs, &snapshot.ResponseHeadersMs, &snapshot.FirstDataMs,
		&snapshot.FirstSemanticMs, &snapshot.FirstVisibleMs, &snapshot.FirstFlushMs,
		&snapshot.OutputTokens, &snapshot.ReasoningTokens,
	} {
		if *field != nil {
			value := **field
			*field = &value
		}
	}
	return snapshot
}

func openAIStreamTraceTokenCount(value gjson.Result) *int64 {
	if value.Type != gjson.Number {
		return nil
	}
	count, err := strconv.ParseInt(value.Raw, 10, 64)
	if err != nil || count < 0 {
		return nil
	}
	return &count
}

func openAIStreamTraceContentType(value string) string {
	mediaType, _, _ := strings.Cut(value, ";")
	if len(mediaType) > 64 {
		return "other"
	}
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "":
		return ""
	case "text/event-stream":
		return "text/event-stream"
	case "application/json":
		return "application/json"
	case "application/problem+json":
		return "application/problem+json"
	case "text/plain":
		return "text/plain"
	case "text/html":
		return "text/html"
	case "application/octet-stream":
		return "application/octet-stream"
	default:
		return "other"
	}
}

func openAIStreamTraceEventType(eventType string) string {
	// Retain only exact known identifiers, detached from the upstream payload.
	switch eventType {
	case "response.created", "response.queued", "response.in_progress", "response.metadata",
		"codex.rate_limits", "codex.response.metadata", "keepalive",
		"response.completed", "response.done", "response.failed", "response.incomplete", "error",
		"response.output_item.added", "response.output_item.done",
		"response.content_part.added", "response.content_part.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done",
		"response.output_text.delta", "response.output_text.done",
		"response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.reasoning_text.done",
		"response.function_call_arguments.delta", "response.function_call_arguments.done",
		"response.custom_tool_call_input.delta", "response.custom_tool_call_input.done",
		"response.refusal.delta", "response.refusal.done",
		"response.audio.delta", "response.audio.done",
		"response.audio_transcript.delta", "response.audio_transcript.done",
		"response.image_generation_call.partial_image", "response.image_generation_call.completed",
		"response.web_search_call.in_progress", "response.web_search_call.searching", "response.web_search_call.completed":
		return strings.Clone(eventType)
	default:
		return "other"
	}
}
