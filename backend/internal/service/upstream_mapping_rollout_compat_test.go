//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type rolloutWSConn struct{ *stagedPassthroughConn }

func (c *rolloutWSConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

func TestWebSocketReasoningModeRespectsLocalMappingAcrossTurns(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, source := range []string{"gpt-6-sol", "gpt-5.6-sol"} {
			for _, percent := range []int{0, 100} {
				t.Run(fmt.Sprintf("passthrough=%t/source=%s/percent=%d", passthrough, source, percent), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(modelMappingRolloutTestContext("ws-rollout-smoke"), 10*time.Second)
					defer cancel()
					cfg := passthroughLifecycleConfig()
					cfg.Gateway.OpenAIWS.OAuthEnabled = true
					cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
					cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
					account := newAstraOAuthSetup(t, passthrough).account
					account.Credentials["model_mapping"] = map[string]any{
						"gpt-6-sol": "gpt-5.6-sol", "gpt-5.6-sol": "gpt-6-sol",
					}
					account.Credentials[ModelMappingPercentCredentialKey] = percent
					mode := OpenAIWSIngressModeCtxPool
					if passthrough {
						mode = OpenAIWSIngressModePassthrough
					}
					account.Extra["openai_oauth_responses_websockets_v2_mode"] = mode
					upstreamModel := account.GetMappedModelForRequest(ctx, source)
					if passthrough {
						upstreamModel = source
					}
					upstream := &rolloutWSConn{newStagedPassthroughConn()}
					svc := newPassthroughLifecycleService(cfg, upstream.stagedPassthroughConn)
					svc.openaiWSPassthroughDialer = &stagedPassthroughDialer{conn: upstream}
					pool := newOpenAIWSConnPool(cfg)
					pool.setClientDialerForTest(&stagedPassthroughDialer{conn: upstream})
					svc.openaiWSPool = pool
					defer pool.Close()
					server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
					defer server.Close()
					firstBody := fmt.Sprintf(`{"type":"response.create","model":%q,"instructions":"test","input":[],"stream":false,"reasoning":{"mode":"pro","effort":"max"}}`, source)
					client := dialPassthroughLifecycleClientWithPayload(t, server, firstBody)
					defer client.CloseNow()
					for turn := 1; turn <= 2; turn++ {
						if turn == 2 {
							// 后续帧省略模型，兼容处理仍须使用会话实际出站模型。
							err := client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","instructions":"test","input":[],"stream":false,"reasoning":{"mode":"pro","effort":"max"}}`))
							require.NoError(t, err)
						}
						payload := requirePassthroughUpstreamWrite(t, upstream.stagedPassthroughConn, 3*time.Second)
						require.Equal(t, upstreamModel, gjson.GetBytes(payload, "model").String(), "turn %d", turn)
						require.Equal(t, isOpenAIGPT6Model(upstreamModel), gjson.GetBytes(payload, "reasoning.mode").Exists(), "turn %d", turn)
						upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_rollout_%d","model":%q,"output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`, turn, upstreamModel))
						completed, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
						require.NoError(t, err)
						require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
					}
					require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
					select {
					case err := <-serverErr:
						require.NoError(t, err)
					case <-ctx.Done():
						t.Fatal("websocket smoke did not finish")
					}
				})
			}
		}
	}
}

func TestForwardReasoningModeRespectsModelMappingPercent(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, source := range []string{"gpt-6-sol", "gpt-5.6-sol"} {
			for _, percent := range []int{0, 50, 100} {
				t.Run(fmt.Sprintf("passthrough=%t/source=%s/percent=%d", passthrough, source, percent), func(t *testing.T) {
					s := newAstraOAuthSetup(t, passthrough)
					s.account.Credentials["model_mapping"] = map[string]any{
						"gpt-6-sol": "gpt-5.6-sol", "gpt-5.6-sol": "gpt-6-sol",
					}
					s.account.Credentials[ModelMappingPercentCredentialKey] = percent
					ctx := modelMappingRolloutTestContext("reasoning-rollout-smoke")
					upstreamModel := s.account.GetMappedModelForRequest(ctx, source)
					if passthrough {
						upstreamModel = source // 本地透传模式忽略普通账号映射。
					}
					inner := `{"id":"resp_rollout","model":"` + upstreamModel + `","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
					s.upstream.resp = &http.Response{StatusCode: http.StatusOK,
						Header: http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:   io.NopCloser(strings.NewReader(codexCompletedSSE(inner)))}
					_, err := s.svc.Forward(ctx, s.c, s.account, astraRequestBody(source, true, "pro", "max"))
					require.NoError(t, err)
					require.Equal(t, upstreamModel, gjson.GetBytes(s.upstream.lastBody, "model").String())
					require.Equal(t, isOpenAIGPT6Model(upstreamModel),
						gjson.GetBytes(s.upstream.lastBody, "reasoning.mode").Exists(),
						"reasoning compatibility must follow the selected upstream model, including 0%% and reverse mappings")
				})
			}
		}
	}
}

func TestSeedanceForwardRespectsModelMappingPercent(t *testing.T) {
	for _, percent := range []int{0, 100} {
		t.Run(fmt.Sprint(percent), func(t *testing.T) {
			account := seedanceTestAccount()
			account.Credentials[ModelMappingPercentCredentialKey] = percent
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"local-task"}`)}
			service := &OpenAIGatewayService{httpUpstream: upstream}
			c, _ := grokMediaContentTestContext(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
			ctx := modelMappingRolloutTestContext("seedance-rollout-smoke")
			result, err := service.ForwardSeedance(ctx, c, account, SeedanceEndpointCreate, "",
				[]byte(`{"model":"video","content":[{"type":"text","text":"waves"}]}`))
			require.NoError(t, err)
			forwarded, err := io.ReadAll(upstream.request.Body)
			require.NoError(t, err)
			wantModel := account.GetMappedModelForRequest(ctx, "video")
			require.Equal(t, wantModel, gjson.GetBytes(forwarded, "model").String())
			require.Equal(t, wantModel, result.UpstreamModel)
		})
	}
}

func TestAnthropicValidationRespectsModelMappingPercent(t *testing.T) {
	for _, source := range []string{"claude-sonnet-4-6", "claude-opus-5-5"} {
		for _, percent := range []int{0, 100} {
			for _, countTokens := range []bool{false, true} {
				t.Run(fmt.Sprintf("source=%s/percent=%d/count_tokens=%t", source, percent, countTokens), func(t *testing.T) {
					account := newAnthropicAPIKeyAccountForTest()
					account.Credentials["model_mapping"] = map[string]any{
						"claude-sonnet-4-6": "claude-opus-5-5", "claude-opus-5-5": "claude-sonnet-4-6",
					}
					account.Credentials[ModelMappingPercentCredentialKey] = percent
					ctx := modelMappingRolloutTestContext("anthropic-validation-rollout")
					upstreamModel := account.GetMappedModelForRequest(ctx, source)
					response := fmt.Sprintf(`{"id":"msg_smoke","type":"message","model":%q,"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`, upstreamModel)
					if countTokens {
						response = `{"input_tokens":1}`
					}
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
						Body: io.NopCloser(strings.NewReader(response)),
					}}
					svc := &GatewayService{cfg: passthroughLifecycleConfig(), httpUpstream: upstream,
						rateLimitService: &RateLimitService{}, deferredService: &DeferredService{}}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
					body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":128,"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"disabled"}}`, source))
					parsed := &ParsedRequest{Model: source, Body: NewRequestBodyRef(body)}
					var err error
					if countTokens {
						err = svc.ForwardCountTokens(ctx, c, account, parsed)
					} else {
						_, err = svc.Forward(ctx, c, account, parsed)
					}
					if upstreamModel == "claude-opus-5-5" {
						require.Error(t, err)
						require.Equal(t, http.StatusBadRequest, rec.Code)
						require.Nil(t, upstream.lastReq, "invalid Opus 5.5 parameters must be rejected before upstream")
					} else {
						require.NoError(t, err, "validation must not classify a disabled mapping target as the upstream model")
						require.Equal(t, http.StatusOK, rec.Code)
						require.Equal(t, upstreamModel, gjson.GetBytes(upstream.lastBody, "model").String())
					}
				})
			}
		}
	}
}
