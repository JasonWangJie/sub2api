package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const openAITraceHandlerStream = "event: response.created\n" +
	`data: {"type":"response.created","response":{"id":"resp_trace","created_at":1700000000}}` + "\n\n" +
	`data: {"type":"response.output_text.delta","delta":"visible"}` + "\n\n" +
	`data: {"type":"response.completed","response":{"id":"resp_trace","created_at":1700000000,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]}],"usage":{"input_tokens":10,"output_tokens":9,"output_tokens_details":{"reasoning_tokens":6}}}}` + "\n\n"

func runOpenAITraceHandler(t *testing.T, passthrough bool, recorder *openAIResponseFlushRecorder, body io.ReadCloser, cfg config.GatewayConfig, account *Account, trace *openAIStreamTrace) (OpenAIUsage, error) {
	t.Helper()
	ctx := context.Background()
	if trace != nil {
		ctx = withOpenAIStreamTrace(ctx, trace)
	}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: cfg}, toolCorrector: NewCodexToolCorrector()}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: body}
	if passthrough {
		result, err := svc.handleStreamingResponsePassthrough(ctx, resp, c, account, time.Now(), "model", "model")
		if result != nil && result.usage != nil {
			return *result.usage, err
		}
		return OpenAIUsage{}, err
	}
	result, err := svc.handleStreamingResponse(ctx, resp, c, account, time.Now(), "model", "model")
	if result != nil && result.usage != nil {
		return *result.usage, err
	}
	return OpenAIUsage{}, err
}

func TestOpenAIStreamTraceHandlersPreserveStreamAndBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			cfg := config.GatewayConfig{OpenAIEarlyFlushCreated: true}
			baseline := newOpenAIResponseFlushRecorder()
			baselineUsage, err := runOpenAITraceHandler(t, passthrough, baseline, io.NopCloser(strings.NewReader(openAITraceHandlerStream)), cfg, account, nil)
			require.NoError(t, err)
			trace := newOpenAIStreamTrace(time.Now())
			recorder := newOpenAIResponseFlushRecorder()
			usage, err := runOpenAITraceHandler(t, passthrough, recorder, io.NopCloser(strings.NewReader(openAITraceHandlerStream)), cfg, account, trace)
			require.NoError(t, err)
			body, flushes := recorder.snapshot()
			baselineBody, baselineFlushes := baseline.snapshot()
			require.Equal(t, baselineBody, body)
			require.Equal(t, baselineFlushes, flushes)
			require.Equal(t, baselineUsage, usage)
			require.Equal(t, 10, usage.InputTokens)
			require.Equal(t, 9, usage.OutputTokens)
			snapshot := trace.Snapshot()
			require.Equal(t, int64(3), snapshot.EventCount)
			require.Equal(t, int64(1), snapshot.DeltaEventCount)
			require.Equal(t, "response.created", snapshot.FirstEventType)
			require.Equal(t, 200, snapshot.HTTPStatus)
			require.Equal(t, "text/event-stream", snapshot.ContentType)
			require.NotNil(t, snapshot.ResponseHeadersMs)
			require.NotNil(t, snapshot.FirstDataMs)
			require.NotNil(t, snapshot.FirstSemanticMs)
			require.NotNil(t, snapshot.FirstVisibleMs)
			require.NotNil(t, snapshot.FirstFlushMs)
			require.Equal(t, int64(9), *snapshot.OutputTokens)
			require.Equal(t, int64(6), *snapshot.ReasoningTokens)
			require.LessOrEqual(t, *snapshot.FirstDataMs, *snapshot.FirstSemanticMs)
			require.LessOrEqual(t, *snapshot.FirstDataMs, *snapshot.FirstFlushMs)
		})
	}
}

func TestOpenAIStreamTraceHandlersDoNotRecordFailedWritesAsFlush(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			recorder := newOpenAIResponseFlushRecorder()
			recorder.failAfterWrites = 0
			trace := newOpenAIStreamTrace(time.Now())
			usage, err := runOpenAITraceHandler(t, passthrough, recorder, io.NopCloser(strings.NewReader(openAITraceHandlerStream)), config.GatewayConfig{}, &Account{ID: 1, Platform: PlatformOpenAI}, trace)
			require.NoError(t, err)
			require.Equal(t, 9, usage.OutputTokens, "disconnected clients still drain upstream usage")
			snapshot := trace.Snapshot()
			require.Equal(t, int64(3), snapshot.EventCount)
			require.NotNil(t, snapshot.FirstVisibleMs)
			require.Nil(t, snapshot.FirstFlushMs)
		})
	}
}

func TestOpenAIStreamTraceNativeKeepaliveDoesNotFakeDataOrFlush(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{PlatformOpenAI, "other"} {
		t.Run(platform, func(t *testing.T) {
			reader, writer := io.Pipe()
			t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
			recorder := newOpenAIResponseFlushRecorder()
			trace := newOpenAIStreamTrace(time.Now())
			errCh := make(chan error, 1)
			go func() {
				_, err := runOpenAITraceHandler(t, false, recorder, reader, config.GatewayConfig{StreamKeepaliveInterval: 1}, &Account{ID: 1, Platform: platform}, trace)
				errCh <- err
			}()
			waitOpenAIResponseFlushCount(t, recorder, 1)
			snapshot := trace.Snapshot()
			require.Zero(t, snapshot.EventCount)
			require.Nil(t, snapshot.FirstDataMs)
			require.Nil(t, snapshot.FirstFlushMs)
			_, err := io.WriteString(writer, openAITraceHandlerStream)
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			select {
			case err := <-errCh:
				require.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("stream handler did not finish")
			}
			snapshot = trace.Snapshot()
			require.Equal(t, int64(3), snapshot.EventCount)
			require.NotNil(t, snapshot.FirstFlushMs)
			require.LessOrEqual(t, *snapshot.FirstDataMs, *snapshot.FirstFlushMs)
		})
	}
}

func TestOpenAIStreamTraceRecordsFlushAfterWriterReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseWriter := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(releaseWriter)
			recorder := newOpenAIResponseFlushRecorder()
			recorder.blockFlush = 1
			recorder.flushBlocked = make(chan struct{})
			recorder.releaseFlush = release
			trace := newOpenAIStreamTrace(time.Now())
			errCh := make(chan error, 1)
			go func() {
				_, err := runOpenAITraceHandler(t, passthrough, recorder, io.NopCloser(strings.NewReader(openAITraceHandlerStream)), config.GatewayConfig{OpenAIEarlyFlushCreated: true}, &Account{ID: 1, Platform: PlatformOpenAI}, trace)
				errCh <- err
			}()
			select {
			case <-recorder.flushBlocked:
				snapshot := trace.Snapshot()
				require.NotNil(t, snapshot.FirstDataMs)
				require.Nil(t, snapshot.FirstFlushMs)
			case <-time.After(3 * time.Second):
				releaseWriter()
				t.Fatal("writer flush was not reached")
			}
			releaseWriter()
			select {
			case err := <-errCh:
				require.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("stream handler did not finish")
			}
			require.NotNil(t, trace.Snapshot().FirstFlushMs)
		})
	}
}
