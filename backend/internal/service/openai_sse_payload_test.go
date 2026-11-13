package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAISSEPayloadAccountingKeepsJSONTypeSeparateFromHeader(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		header     string
		wantType   string
		wantImages int
		wantSearch int
		wantItems  int
	}{
		{
			name:   "header only does not turn an image item into a done payload",
			body:   `{"output_index":0,"item":{"id":"img1","type":"image_generation_call","result":"abc"}}`,
			header: "response.output_item.done", wantType: "response.output_item.done",
		},
		{
			name:   "bare search item keeps its accounting with a header",
			body:   `{"item":{"id":"search1","type":"web_search_call"}}`,
			header: "response.output_text.delta", wantType: "response.output_text.delta", wantSearch: 1,
		},
		{
			name:   "JSON type wins over conflicting header",
			body:   `{"type":"response.output_item.done","output_index":0,"item":{"id":"img1","type":"image_generation_call","result":"abc"}}`,
			header: "response.output_text.delta", wantType: "response.output_item.done", wantImages: 1, wantItems: 1,
		},
		{
			name:   "delta can carry compatible image data",
			body:   `{"type":"response.output_text.delta","delta":"hello","data":[{"url":"https://example.invalid/image.png"}]}`,
			header: "response.completed", wantType: "response.output_text.delta", wantImages: 1,
		},
		{
			name:     "escaped data key still counts",
			body:     `{"type":"response.output_text.delta","da\u0074a":[{"b64_json":"abc"}]}`,
			wantType: "response.output_text.delta", wantImages: 1,
		},
		{
			name:     "malformed done payload cannot mutate accounting",
			body:     `{"type":"response.output_item.done","output_index":0,"item":{"id":"img1","type":"image_generation_call","result":"abc"}} trailing`,
			wantType: "response.output_item.done",
		},
		{
			name:     "malformed image data cannot mutate accounting",
			body:     `{"type":"response.output_text.delta","data":[{"b64_json":"abc"}]} trailing`,
			wantType: "response.output_text.delta",
		},
		{
			name:     "malformed search payload cannot mutate accounting",
			body:     `{"type":"response.output_item.done","item":{"type":"web_search_call","id":"search1"}} trailing`,
			wantType: "response.output_item.done",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := newOpenAISSEPayload([]byte(tc.body))
			require.Equal(t, tc.wantType, payload.effectiveEventType(tc.header))
			counter := newOpenAIImageOutputCounter()
			counter.addSSEPayload(&payload)
			require.Equal(t, tc.wantImages, counter.Count())
			require.Equal(t, tc.wantSearch, countGrokNativeSearchCallsInSSEPayloadDedup(&payload, make(map[string]struct{})))
			items := newResponsesStreamOutputItems()
			items.observePayload(&payload)
			require.Equal(t, tc.wantItems, items.Count())
		})
	}
}

func TestOpenAISSEPayloadRefreshAfterImageNormalization(t *testing.T) {
	original := []byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"img1","type":"image_generation_call","status":"in_progress","result":"abc"}}`)
	payload := newOpenAISSEPayload(original)
	normalized, changed := normalizeCompletedImageGenerationStatusPayload(&payload)
	require.True(t, changed)
	payload = newOpenAISSEPayload(normalized)
	items := newResponsesStreamOutputItems()
	items.observePayload(&payload)
	output, ok := items.BuildOutput()
	require.True(t, ok)
	require.Equal(t, "completed", gjson.GetBytes(output, "0.status").String())
	require.Equal(t, "in_progress", gjson.GetBytes(original, "item.status").String(), "input remains immutable")
	image, ok := extractImageGenerationOutputFromSSEPayload(&payload, make(map[string]struct{}))
	require.True(t, ok)
	require.Equal(t, "completed", gjson.GetBytes(image, "status").String())
}

func TestOpenAISSEPayloadWhitespaceTypeKeepsImageExtractionContract(t *testing.T) {
	data := []byte(`{"type":" response.output_item.done ","output_index":0,"item":{"id":"img1","type":"image_generation_call","result":"abc"}}`)
	payload := newOpenAISSEPayload(data)
	items := newResponsesStreamOutputItems()
	items.observePayload(&payload)
	require.Equal(t, 1, items.Count(), "done-item collection has always trimmed the type")
	_, ok := extractImageGenerationOutputFromSSEPayload(&payload, nil)
	require.False(t, ok, "image extraction historically uses the exact untrimmed JSON type")
}

func TestOpenAISSEPayloadStreamPreservesMixedOutputAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		toolCorrector: NewCodexToolCorrector(),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	frames := []string{
		`{"type":"response.created","response":{"id":"resp_mixed"}}`,
		`{"type":"response.output_text.delta","delta":"hello"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"msg1","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"id":"search1","type":"web_search_call","status":"completed"}}`,
		`{"type":"response.output_item.done","output_index":2,"item":{"id":"img1","type":"image_generation_call","status":"in_progress","result":"abc"}}`,
		`{"type":"response.completed","response":{"id":"resp_mixed","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":5}}}`,
	}
	var stream strings.Builder
	for _, frame := range frames {
		stream.WriteString("data: ")
		stream.WriteString(frame)
		stream.WriteString("\n\n")
	}
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(stream.String()))}
	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, time.Now(), "model", "model")
	require.NoError(t, err)
	require.Equal(t, 10, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
	require.Equal(t, 1, result.searchCount)
	require.Equal(t, 1, result.imageCount)
	require.Equal(t, "resp_mixed", result.responseID)
	var terminal []byte
	forEachOpenAISSEDataPayload(recorder.Body.String(), func(data []byte) {
		if gjson.GetBytes(data, "type").String() == "response.completed" {
			terminal = append([]byte(nil), data...)
		}
	})
	require.Equal(t, int64(3), gjson.GetBytes(terminal, "response.output.#").Int())
	require.Equal(t, "completed", gjson.GetBytes(terminal, "response.output.2.status").String())
	require.Contains(t, recorder.Body.String(), `"delta":"hello"`)
}
