package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func openAI429Classify(status int, headers http.Header, body []byte) openAI429Outcome {
	out := openAI429Outcome{RateLimited: status == 429}
	if isOpenAIImageRateLimitError(status, body) {
		out.Ignore = true
		return out
	}
	if status != 429 {
		return out
	}
	for _, path := range []string{"error.type", "error.code", "response.error.type", "response.error.code"} {
		switch gjson.GetBytes(body, path).String() {
		case "usage_limit_reached", "GoUsageLimitError", "insufficient_quota":
			out.QuotaExhausted = true
		}
	}
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		limits := snapshot.Normalize()
		if limits != nil {
			for i, window := range []struct {
				used  *float64
				reset *int
			}{{limits.Used5hPercent, limits.Reset5hSeconds}, {limits.Used7dPercent, limits.Reset7dSeconds}} {
				if window.used == nil || *window.used < 100 {
					continue
				}
				out.QuotaExhausted = true
				out.ExhaustedWindows |= 1 << i
				if window.reset == nil || *window.reset <= 0 {
					out.UnknownReset = true
					continue
				}
				reset := time.Now().Add(time.Duration(*window.reset) * time.Second).UnixMilli()
				if reset > out.ResetAt {
					out.ResetAt = reset
				}
			}
		}
	}
	if reset := parseOpenAIRateLimitResetTime(body); reset != nil && *reset > time.Now().Unix() {
		out.QuotaExhausted = true
		if *reset*1000 > out.ResetAt {
			out.ResetAt = *reset * 1000
		}
	}
	if out.QuotaExhausted && out.ResetAt == 0 {
		for _, path := range []string{"error.resets_at", "error.reset_at", "response.error.resets_at"} {
			if reset := gjson.GetBytes(body, path).Int(); reset > time.Now().Unix() {
				out.ResetAt = reset * 1000
				break
			}
		}
		out.UnknownReset = out.ResetAt == 0
	}
	return out
}

// Bounded raw response observer: it never retains a conversation. SSE documents
// are inspected independently; HTTP 200, [DONE], and truncated JSON are not success.
type openAI429Observer struct {
	status        int
	headers       http.Header
	stream        bool
	compact       bool
	hasCompaction bool
	buffer        []byte
	invalid       bool
	terminal      bool
	outcome       openAI429Outcome
}

func (o *openAI429Observer) feed(data []byte) {
	if o.invalid {
		return
	}
	if len(o.buffer)+len(data) > 4<<20 {
		o.invalid = true
		o.buffer = nil
		return
	}
	o.buffer = append(o.buffer, data...)
	if !o.stream {
		return
	}
	for {
		i := bytes.IndexByte(o.buffer, '\n')
		if i < 0 {
			break
		}
		line := bytes.TrimSpace(o.buffer[:i])
		if bytes.HasPrefix(line, []byte("data:")) {
			o.document(bytes.TrimSpace(line[5:]))
		}
		o.buffer = o.buffer[i+1:]
	}
}

func (o *openAI429Observer) document(data []byte) {
	if o.terminal {
		return
	}
	if bytes.Equal(data, []byte("[DONE]")) || len(data) == 0 {
		return
	}
	if !gjson.ValidBytes(data) {
		o.invalid = true
		return
	}
	event := gjson.GetBytes(data, "type").String()
	response := data
	if nested := gjson.GetBytes(data, "response"); nested.IsObject() {
		response = []byte(nested.Raw)
	}
	if gjson.GetBytes(response, `output.#(type=="image_generation_call")`).Exists() || strings.Contains(event, "image_generation") {
		o.outcome.Ignore = true
	}
	status := gjson.GetBytes(response, "status").String()
	if item := gjson.GetBytes(data, "item"); item.Get("type").String() == "compaction" && item.Get("encrypted_content").String() != "" {
		o.hasCompaction = true
	}
	if item := gjson.GetBytes(response, `output.#(type=="compaction")`); item.Get("encrypted_content").String() != "" {
		o.hasCompaction = true
	}
	if event == "error" || event == "response.failed" || event == "response.incomplete" || event == "response.cancelled" || status == "failed" || status == "incomplete" || status == "cancelled" {
		o.terminal = true
		code := gjson.GetBytes(data, "error.code").String()
		if code == "" {
			code = gjson.GetBytes(response, "error.code").String()
		}
		typ := gjson.GetBytes(response, "error.type").String()
		errorStatus := int(gjson.GetBytes(data, "status").Int())
		if errorStatus == 0 {
			errorStatus = int(gjson.GetBytes(data, "error.status").Int())
		}
		if code == "rate_limit_exceeded" || code == "usage_limit_reached" || typ == "usage_limit_reached" || typ == "rate_limit_error" {
			errorStatus = 429
		}
		ignored := o.outcome.Ignore
		o.outcome = openAI429Classify(errorStatus, o.headers, data)
		o.outcome.Ignore = o.outcome.Ignore || ignored
		return
	}
	if event == "response.completed" || event == "response.done" || (!o.stream && (gjson.GetBytes(response, "object").String() == "response" || o.compact)) {
		o.terminal = true
		o.outcome.Success = (status == "completed" || status == "") && gjson.GetBytes(response, "id").String() != "" && gjson.GetBytes(response, "output").IsArray() && !gjson.GetBytes(response, "error").IsObject()
		if o.compact && !o.hasCompaction {
			o.outcome.Success = false
		}
	} else if choices := gjson.GetBytes(data, "choices"); choices.IsArray() {
		for _, choice := range choices.Array() {
			finish := choice.Get("finish_reason").String()
			if gjson.GetBytes(data, "id").String() != "" && (o.stream || choice.Get("message").IsObject()) && (finish == "stop" || finish == "tool_calls" || finish == "length") {
				o.terminal = true
				o.outcome.Success = true
			}
		}
	}
	if usage := gjson.GetBytes(response, "usage"); usage.IsObject() {
		o.outcome.Usage.InputTokens = int(usage.Get("input_tokens").Int())
		o.outcome.Usage.OutputTokens = int(usage.Get("output_tokens").Int())
		if usage.Get("prompt_tokens").Exists() {
			o.outcome.Usage.InputTokens = int(usage.Get("prompt_tokens").Int())
			o.outcome.Usage.OutputTokens = int(usage.Get("completion_tokens").Int())
		}
	}
}

func (o *openAI429Observer) result(eof bool) openAI429Outcome {
	if o.status >= 400 {
		return openAI429Classify(o.status, o.headers, o.buffer)
	}
	if eof && len(bytes.TrimSpace(o.buffer)) > 0 {
		if !o.stream {
			o.document(o.buffer)
		} else if line := bytes.TrimSpace(o.buffer); bytes.HasPrefix(line, []byte("data:")) {
			o.document(bytes.TrimSpace(line[5:]))
		}
		o.buffer = nil
	}
	result := o.outcome
	if o.invalid || !o.terminal {
		result.Success = false
	}
	return result
}

type openAI429ResponseBody struct {
	io.ReadCloser
	observer openAI429Observer
	ticket   *openAI429Ticket
}

func (b *openAI429ResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.observer.feed(p[:n])
	if err != nil {
		b.ticket.finish(b.observer.result(err == io.EOF))
	}
	return n, err
}

func (b *openAI429ResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.ticket.finish(b.observer.result(false))
	return err
}

func openAI429TextRequest(path, model string, payload []byte) bool {
	return (strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/responses/compact") || strings.HasSuffix(path, "/chat/completions")) && !isCodexSparkModel(model) && !IsImageGenerationIntent(path, model, payload)
}

func (s *OpenAIGatewayService) beginOpenAI429HTTP(req *http.Request, account *Account) (*openAI429Ticket, error) {
	if !SupportsOpenAI429Mode(account) {
		return nil, nil
	}
	if !OpenAI429ModeEnabled(account) && (s.openAI429Mode == nil || !s.openAI429Mode.configuredEnabled(account.ID)) {
		return nil, nil
	}
	if s.openAI429Mode == nil {
		return s.openAI429Mode.begin(req.Context(), account, "")
	}
	var copyBody io.ReadCloser
	var err error
	if req.GetBody != nil {
		copyBody, err = req.GetBody()
	} else {
		copyBody = req.Body
	}
	if err != nil || copyBody == nil {
		return nil, errOpenAI429Admission
	}
	payload, err := io.ReadAll(copyBody)
	_ = copyBody.Close()
	if req.GetBody == nil {
		req.Body = io.NopCloser(bytes.NewReader(payload))
	}
	if err != nil {
		return nil, err
	}
	model := gjson.GetBytes(payload, "model").String()
	if !openAI429TextRequest(req.URL.Path, model, payload) {
		return nil, nil
	}
	return s.openAI429Mode.begin(req.Context(), account, model)
}

func (s *OpenAIGatewayService) runOpenAI429Probe(ctx context.Context, account *Account, model string) openAI429Outcome {
	if model == "" {
		return openAI429Outcome{}
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return openAI429Outcome{}
	}
	// Model is already mapped by the triggering upstream call. Never map it again.
	body, _ := json.Marshal(map[string]any{"model": model, "input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Reply OK."}}}}, "instructions": "", "stream": true, "store": false})
	c := &gin.Context{}
	c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req, err := s.buildUpstreamRequest(ctx, c, account, body, token, true, "", true)
	if err != nil {
		return openAI429Outcome{}
	}
	latest, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil || !OpenAI429ModeEnabled(latest) || latest.GetExtraString(OpenAI429ModeGenerationKey) != account.GetExtraString(OpenAI429ModeGenerationKey) || !latest.IsSchedulable() {
		return openAI429Outcome{}
	}
	if latest.isRateLimitActiveForKey(model) || s.isOpenAIProxyStreamQuarantined(ctx, latest) || s.getOpenAIAccountModelTransientState().isBlocked(latest.ID, openAIAccountModelTransientModel(model), time.Now()) {
		return openAI429Outcome{}
	}
	// Independent image traffic may still use this account during a text round.
	// Automatic calls must occupy an original slot as well, keeping its cap intact.
	slot, err := s.tryAcquireAccountSlot(ctx, latest.ID, latest.Concurrency)
	if err != nil || slot == nil || !slot.Acquired {
		return openAI429Outcome{}
	}
	if slot.ReleaseFunc != nil {
		defer slot.ReleaseFunc()
	}
	resp, err := s.doOpenAIUpstreamRaw(req, resolveAccountProxyURL(account), account)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return openAI429Outcome{}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusTooManyRequests {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		s.handleFailoverSideEffects(ctx, resp, account, errorBody, model)
		return openAI429Outcome{}
	}
	observer := openAI429Observer{status: resp.StatusCode, headers: resp.Header, stream: resp.StatusCode < 400 && strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")}
	buffer := make([]byte, 16<<10)
	for {
		n, err := resp.Body.Read(buffer)
		observer.feed(buffer[:n])
		if err != nil {
			return observer.result(err == io.EOF)
		}
	}
}
