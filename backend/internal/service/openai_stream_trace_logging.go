package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

var openAIStreamTraceSequence atomic.Uint64

// A global cap bounds log volume even if operators sample every request.
var openAIStreamTraceLogBudget struct {
	sync.Mutex
	minute int64
	count  int
}

func allowOpenAIStreamTraceLog(now time.Time) bool {
	openAIStreamTraceLogBudget.Lock()
	defer openAIStreamTraceLogBudget.Unlock()
	minute := now.Unix() / 60
	if openAIStreamTraceLogBudget.minute != minute {
		openAIStreamTraceLogBudget.minute = minute
		openAIStreamTraceLogBudget.count = 0
	}
	if openAIStreamTraceLogBudget.count >= 60 {
		return false
	}
	openAIStreamTraceLogBudget.count++
	return true
}

func (s *OpenAIGatewayService) beginOpenAIStreamTrace(ctx context.Context, c *gin.Context, account *Account, body []byte, start time.Time) (context.Context, func()) {
	noop := func() {}
	if s == nil || s.cfg == nil || !s.cfg.Gateway.OpenAIStreamTraceEnabled || account == nil || account.Platform != PlatformOpenAI {
		return ctx, noop
	}
	every := s.cfg.Gateway.OpenAIStreamTraceSampleEvery
	if every < 1 {
		every = 20
	}
	if (openAIStreamTraceSequence.Add(1)-1)%uint64(every) != 0 || !gjson.GetBytes(body, "stream").Bool() {
		return ctx, noop
	}
	// A fresh trace per Forward attempt prevents account failovers from merging
	// unrelated timelines. An internal retry is detectable via transport_count.
	trace := newOpenAIStreamTrace(start)
	ctx = withOpenAIStreamTrace(ctx, trace)
	accountID, apiKeyID := account.ID, getAPIKeyIDFromContext(c)
	bodyBytes := len(body)
	return ctx, func() {
		snapshot := trace.Snapshot()
		if snapshot.TransportCount == 0 {
			return
		}
		if !allowOpenAIStreamTraceLog(time.Now()) {
			return
		}
		duration := time.Since(start).Milliseconds()
		// The compatibility Chat Completions scanner does not use the Responses
		// SSE hooks. Make that coverage explicit so absent stages are not mistaken
		// for buffered or missing upstream events. Only fixed endpoint names escape.
		endpoint, coverage := GetActualOpenAIUpstreamEndpoint(c), "http_only"
		switch endpoint {
		case "/v1/responses":
			coverage = "responses_sse"
		case "/v1/chat/completions":
		default:
			endpoint = "other"
		}
		logger.FromContext(ctx).Info("openai.stream_timing",
			zap.Int64("account_id", accountID), zap.Int64("api_key_id", apiKeyID),
			zap.String("upstream_endpoint", endpoint), zap.String("stage_coverage", coverage),
			zap.Int("body_bytes", bodyBytes), zap.Int("sample_every", every),
			zap.Int64("attempt_duration_ms", duration), zap.Bool("slow", duration >= 10000),
			zap.Any("stages", snapshot), zap.Bool(logger.OpsSystemLogSkipField, true),
		)
	}
}
