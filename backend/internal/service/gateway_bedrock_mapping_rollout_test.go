package service

import (
	"fmt"
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

func TestBedrockCCCompatRespectsModelMappingPercent(t *testing.T) {
	groupID := int64(55)
	channels := &ChannelService{}
	channels.cache.Store(&channelCache{
		loadedAt: time.Now(),
		channelByGroupID: map[int64]*Channel{
			groupID: {Status: StatusActive, FeaturesConfig: map[string]any{featureKeyBedrockCCCompat: true}},
		},
	})
	for _, tc := range []struct {
		name         string
		source       string
		target       string
		percent      int
		wantSonnet55 bool
	}{
		{"disabled mapping to sonnet", "claude-haiku-4-5", "claude-sonnet-5-5", 0, false},
		{"enabled mapping to sonnet", "claude-haiku-4-5", "claude-sonnet-5-5", 100, true},
		{"disabled mapping from sonnet", "claude-sonnet-5-5", "claude-haiku-4-5", 0, true},
		{"enabled mapping from sonnet", "claude-sonnet-5-5", "claude-haiku-4-5", 100, false},
	} {
		for _, thinkingType := range []string{"enabled", "disabled"} {
			t.Run(tc.name+"/"+thinkingType, func(t *testing.T) {
				ctx := modelMappingRolloutTestContext("bedrock-cc-rollout")
				account := &Account{ID: 55, Platform: PlatformAnthropic, Type: AccountTypeBedrock,
					Credentials: map[string]any{
						"aws_region":                     "eu-west-1",
						"auth_mode":                      "apikey",
						"api_key":                        "test-bedrock-key",
						"model_mapping":                  map[string]any{tc.source: tc.target},
						ModelMappingPercentCredentialKey: tc.percent,
					}}
				upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"type":"message","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)),
				}}
				svc := &GatewayService{cfg: &config.Config{}, channelService: channels, httpUpstream: upstream}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
				c.Request.Header.Set("anthropic-beta", "unsupported-beta")
				body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":16384,"messages":[{"role":"user","content":"hello"}],"thinking":{"type":%q},"tools":[{"type":"tool_search_tool_regex_20251119","name":"search"}],"anthropic_beta":["unsupported-beta"],"output_config":{"effort":"low"}}`, tc.source, thinkingType))
				converted := svc.ApplyBedrockCCCompat(c, body, tc.source, account, &groupID)

				wantThinking := thinkingType
				if tc.wantSonnet55 {
					if thinkingType == "enabled" {
						wantThinking = "adaptive"
					} else {
						wantThinking = "between_tools"
					}
				}
				require.Equal(t, wantThinking, gjson.GetBytes(converted, "thinking.type").String())
				require.Equal(t, !tc.wantSonnet55 && thinkingType == "enabled", gjson.GetBytes(converted, "thinking.budget_tokens").Exists())
				// Haiku does not support tool search; Sonnet does. Both body and
				// header beta injection must follow the selected upstream model.
				require.Equal(t, tc.wantSonnet55, containsBetaToken(c.GetHeader("anthropic-beta"), "tool-search-tool-2025-10-19"))
				require.Equal(t, tc.wantSonnet55, containsStringInJSONArray(gjson.GetBytes(converted, "anthropic_beta"), "tool-search-tool-2025-10-19"))
				require.False(t, containsBetaToken(c.GetHeader("anthropic-beta"), "unsupported-beta"))

				result, err := svc.Forward(ctx, c, account, &ParsedRequest{Model: tc.source, Body: NewRequestBodyRef(converted)})
				require.NoError(t, err, "validation and CC conversion must use the same mapping branch")
				require.Equal(t, http.StatusOK, rec.Code)
				wantModel := "eu.anthropic.claude-haiku-4-5-20251001-v1:0"
				if tc.wantSonnet55 {
					wantModel = "global.anthropic.claude-sonnet-5-5"
				}
				require.Equal(t, wantModel, result.UpstreamModel)
				require.Contains(t, upstream.lastReq.URL.Path, wantModel)
				require.Equal(t, wantThinking, gjson.GetBytes(upstream.lastBody, "thinking.type").String())
				require.Equal(t, tc.wantSonnet55, containsStringInJSONArray(gjson.GetBytes(upstream.lastBody, "anthropic_beta"), "tool-search-tool-2025-10-19"))
				require.Equal(t, tc.wantSonnet55, gjson.GetBytes(upstream.lastBody, "output_config.effort").Exists())
			})
		}
	}
}

func TestBedrockCountTokensValidationRespectsModelMappingPercent(t *testing.T) {
	for _, tc := range []struct {
		source       string
		target       string
		percent      int
		wantRejected bool
	}{
		{"claude-haiku-4-5", "claude-sonnet-5-5", 0, false},
		{"claude-haiku-4-5", "claude-sonnet-5-5", 100, true},
		{"claude-sonnet-5-5", "claude-haiku-4-5", 0, true},
		{"claude-sonnet-5-5", "claude-haiku-4-5", 100, false},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.source, tc.percent), func(t *testing.T) {
			ctx := modelMappingRolloutTestContext("bedrock-count-rollout")
			account := &Account{ID: 55, Platform: PlatformAnthropic, Type: AccountTypeBedrock,
				Credentials: map[string]any{
					"aws_region":                     "eu-west-1",
					"model_mapping":                  map[string]any{tc.source: tc.target},
					ModelMappingPercentCredentialKey: tc.percent,
				}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil).WithContext(ctx)
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}],"thinking":{"type":"disabled"}}`, tc.source))
			err := (&GatewayService{}).ForwardCountTokens(ctx, c, account, &ParsedRequest{Model: tc.source, Body: NewRequestBodyRef(body)})
			if tc.wantRejected {
				require.Error(t, err)
				require.Equal(t, http.StatusBadRequest, rec.Code)
			} else {
				require.NoError(t, err)
				require.Equal(t, http.StatusNotFound, rec.Code, "Bedrock count_tokens must preserve its unsupported endpoint response")
			}
		})
	}
}
