package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamTraceSeparatesTransportSemanticAndVisibleOutput(t *testing.T) {
	trace := newOpenAIStreamTrace(time.Now().Add(-time.Second))
	trace.RecordWroteRequest(nil)
	trace.RecordHeaders(200, "Text/Event-Stream; charset=utf-8")
	trace.ObserveSSE([]byte(`{"type":"response.created","response":{"id":"not-retained"}}`), "")
	trace.RecordFlush()
	snapshot := trace.Snapshot()
	require.NotNil(t, snapshot.RequestWriteMs)
	require.NotNil(t, snapshot.ResponseHeadersMs)
	require.NotNil(t, snapshot.FirstDataMs)
	require.NotNil(t, snapshot.FirstFlushMs)
	require.GreaterOrEqual(t, *snapshot.FirstDataMs, int64(1000))
	require.Nil(t, snapshot.FirstSemanticMs)
	require.Nil(t, snapshot.FirstVisibleMs)
	require.Equal(t, "response.created", snapshot.FirstEventType)
	require.Equal(t, "text/event-stream", snapshot.ContentType)
	require.Equal(t, 200, snapshot.HTTPStatus)

	// An empty structural item starts historical semantic TTFT, but not visible
	// TTFT; an actual content delta then reaches the visible boundary.
	trace.ObserveSSE([]byte(`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[]}}`), "response.output_item.added")
	snapshot = trace.Snapshot()
	require.NotNil(t, snapshot.FirstSemanticMs)
	require.Nil(t, snapshot.FirstVisibleMs)
	trace.ObserveSSE([]byte(`{"type":"response.output_text.delta","delta":"not-retained"}`), "response.output_text.delta")
	trace.ObserveSSE([]byte(`{"type":"response.completed","response":{"usage":{"output_tokens":21,"output_tokens_details":{"reasoning_tokens":15}}}}`), "response.completed")
	trace.ObserveSSE([]byte(" [DONE] "), "")
	snapshot = trace.Snapshot()
	require.NotNil(t, snapshot.FirstVisibleMs)
	require.LessOrEqual(t, *snapshot.FirstDataMs, *snapshot.FirstSemanticMs)
	require.LessOrEqual(t, *snapshot.FirstSemanticMs, *snapshot.FirstVisibleMs)
	require.Equal(t, int64(5), snapshot.EventCount)
	require.Equal(t, int64(1), snapshot.DeltaEventCount)
	require.Equal(t, int64(21), *snapshot.OutputTokens)
	require.Equal(t, int64(15), *snapshot.ReasoningTokens)

	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "not-retained")
}

func TestOpenAIStreamTraceCountsOnlyTerminalUsage(t *testing.T) {
	trace := newOpenAIStreamTrace(time.Now())
	trace.ObserveSSE([]byte(`{"type":"response.output_text.delta","delta":"x","usage":{"output_tokens":999,"output_tokens_details":{"reasoning_tokens":888}}}`), "")
	require.Nil(t, trace.Snapshot().OutputTokens)
	require.Nil(t, trace.Snapshot().ReasoningTokens)
	trace.ObserveSSE([]byte(`{"type":"response.completed","response":{"usage":{"output_tokens":10,"output_tokens_details":{"reasoning_tokens":7}}}}`), "")
	trace.ObserveSSE([]byte(`{"type":"response.done","usage":{"completion_tokens":12,"completion_tokens_details":{"reasoning_tokens":8}}}`), "")
	snapshot := trace.Snapshot()
	require.Equal(t, int64(12), *snapshot.OutputTokens)
	require.Equal(t, int64(8), *snapshot.ReasoningTokens)

	// Missing or malformed later usage does not erase the last available value.
	trace.ObserveSSE([]byte(`{"type":"response.failed","response":{"usage":{"output_tokens":-1,"output_tokens_details":{"reasoning_tokens":"secret"}}}}`), "")
	trace.ObserveSSE([]byte(`{"type":"response.done","usage":{"output_tokens":999,"output_tokens_details":{"reasoning_tokens":888}}} trailing garbage`), "response.done")
	snapshot = trace.Snapshot()
	require.Equal(t, int64(12), *snapshot.OutputTokens)
	require.Equal(t, int64(8), *snapshot.ReasoningTokens)
	trace.ObserveSSE([]byte(`{"type":"response.completed","response":{"usage":{"output_tokens":0,"output_tokens_details":{"reasoning_tokens":0}}}}`), "")
	require.Zero(t, *trace.Snapshot().OutputTokens)
	require.Zero(t, *trace.Snapshot().ReasoningTokens)
}

func TestOpenAIStreamTraceUsageOnlyCompletionAndSentinel(t *testing.T) {
	trace := newOpenAIStreamTrace(time.Now())
	trace.ObserveSSE([]byte(" \n\t"), "keepalive")
	require.Zero(t, trace.Snapshot().EventCount)
	trace.ObserveSSE([]byte(`{"type":"response.completed","response":{"usage":{"output_tokens":6}}}`), "")
	require.NotNil(t, trace.Snapshot().FirstSemanticMs)
	require.Nil(t, trace.Snapshot().FirstVisibleMs)

	sentinel := newOpenAIStreamTrace(time.Now())
	sentinel.ObserveSSE([]byte("[DONE]"), "")
	snapshot := sentinel.Snapshot()
	require.NotNil(t, snapshot.FirstDataMs)
	require.Nil(t, snapshot.FirstSemanticMs)
	require.Nil(t, snapshot.FirstVisibleMs)
	require.Equal(t, "done_sentinel", snapshot.FirstEventType)
	require.Equal(t, int64(1), snapshot.EventCount)
}

func TestOpenAIStreamTraceSnapshotIsBoundedAndDetached(t *testing.T) {
	trace := newOpenAIStreamTrace(time.Now().Add(time.Hour))
	trace.RecordWroteRequest(errors.New("Bearer secret https://private.example/secret"))
	trace.RecordHeaders(999, "application/secret; token=secret")
	trace.ObserveSSE([]byte(`{"type":"private.secret","delta":"secret"}`), strings.Repeat("secret", 1024))
	snapshot := trace.Snapshot()
	require.True(t, snapshot.RequestWriteFailed)
	require.Equal(t, "other", snapshot.ContentType)
	require.Equal(t, "other", snapshot.FirstEventType)
	require.Zero(t, snapshot.HTTPStatus)
	require.NotNil(t, snapshot.RequestWriteMs)
	require.Zero(t, *snapshot.RequestWriteMs, "zero is an observed timing, not a missing stage")
	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret")
	require.NotContains(t, string(encoded), "private.example")
	require.Contains(t, string(encoded), `"request_write_ms":0`)

	*snapshot.RequestWriteMs = 123456
	*snapshot.FirstDataMs = 123456
	require.Zero(t, *trace.Snapshot().RequestWriteMs)
	require.Zero(t, *trace.Snapshot().FirstDataMs)
	// Repeated callbacks do not replace the attempt's first observations.
	trace.RecordWroteRequest(nil)
	trace.RecordHeaders(200, "text/event-stream")
	require.True(t, trace.Snapshot().RequestWriteFailed)
	require.Equal(t, "other", trace.Snapshot().ContentType)
}

func TestOpenAIStreamTraceConcurrentCallbacks(t *testing.T) {
	trace := newOpenAIStreamTrace(time.Now())
	const workers, events = 12, 25
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range events {
				trace.RecordTransport()
				trace.RecordWroteRequest(nil)
				trace.RecordHeaders(200, "text/event-stream")
				trace.ObserveSSE([]byte(`{"type":"response.output_text.delta","delta":"x"}`), "response.output_text.delta")
				trace.RecordFlush()
				_ = trace.Snapshot()
			}
		}()
	}
	wg.Wait()
	snapshot := trace.Snapshot()
	require.Equal(t, int64(workers*events), snapshot.TransportCount)
	require.Equal(t, int64(workers*events), snapshot.EventCount)
	require.Equal(t, int64(workers*events), snapshot.DeltaEventCount)
	require.NotNil(t, snapshot.FirstDataMs)
	require.NotNil(t, snapshot.FirstSemanticMs)
	require.NotNil(t, snapshot.FirstVisibleMs)
	require.NotNil(t, snapshot.FirstFlushMs)
}

func TestOpenAIStreamTraceCountsTransportsIndependentlyFromEvents(t *testing.T) {
	trace := newOpenAIStreamTrace(time.Now())
	trace.RecordTransport()
	trace.RecordHeaders(429, "application/json")
	trace.RecordTransport()
	trace.RecordHeaders(200, "text/event-stream")
	snapshot := trace.Snapshot()
	require.Equal(t, int64(2), snapshot.TransportCount)
	require.Zero(t, snapshot.EventCount)
	require.Equal(t, 429, snapshot.HTTPStatus, "headers still describe the first transport")
}

func TestOpenAIStreamTraceContextAndNilSafety(t *testing.T) {
	var nilTrace *openAIStreamTrace
	nilTrace.RecordTransport()
	nilTrace.RecordWroteRequest(nil)
	nilTrace.RecordHeaders(200, "text/event-stream")
	nilTrace.ObserveSSE([]byte(`{"type":"response.created"}`), "")
	nilTrace.RecordFlush()
	require.Equal(t, openAIStreamTraceSnapshot{}, nilTrace.Snapshot())
	require.Nil(t, openAIStreamTraceFromContext(nil))
	trace := newOpenAIStreamTrace(time.Time{})
	ctx := withOpenAIStreamTrace(context.Background(), trace)
	require.Same(t, trace, openAIStreamTraceFromContext(ctx))
	require.Nil(t, openAIStreamTraceFromContext(withOpenAIStreamTrace(ctx, nil)))
	require.Same(t, trace, openAIStreamTraceFromContext(withOpenAIStreamTrace(nil, trace)))
}
