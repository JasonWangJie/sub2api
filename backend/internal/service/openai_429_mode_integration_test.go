package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/testutil/redisstatetest"
	"github.com/alicebob/miniredis/v2"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// All upstream traffic terminates in this in-process simulator. No credentials
// or external account are used and the billing service is deliberately absent.
type mode429Repo struct {
	AccountRepository
	mu    sync.Mutex
	a     *Account
	redis *miniredis.Miniredis
}

func (r *mode429Repo) GetByID(context.Context, int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := *r.a
	a.Extra = cloneOpenAIAutoResetExtra(r.a.Extra)
	return &a, nil
}

func (r *mode429Repo) StoreOpenAI429ModeState(_ context.Context, _ int64, state *OpenAI429State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !OpenAI429ModeEnabled(r.a) || state.Generation != r.a.GetExtraString(OpenAI429ModeGenerationKey) {
		return nil
	}
	old := openAI429StateFromExtra(r.a)
	if old != nil && old.Version >= state.Version {
		return nil
	}
	copy := *state
	r.a.Extra[OpenAI429ModeStateKey] = &copy
	return nil
}

func (r *mode429Repo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range updates {
		r.a.Extra[k] = v
	}
	return nil
}

type mode429Upstream struct {
	HTTPUpstream
	mu       sync.Mutex
	requests [][]byte
	respond  func(int) (int, string)
}

func (u *mode429Upstream) Do(req *http.Request, _ string, _ int64, concurrency int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	body, _ := io.ReadAll(req.Body)
	u.requests = append(u.requests, body)
	if concurrency != 8 {
		return nil, fmt.Errorf("concurrency changed: %d", concurrency)
	}
	status, data := u.respond(len(u.requests))
	header := http.Header{"Content-Type": []string{"application/json"}}
	if strings.HasPrefix(data, "data:") {
		header.Set("Content-Type", "text/event-stream")
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(data))}, nil
}

func newMode429Integration(t *testing.T, respond func(int) (int, string)) (*OpenAI429ModeService, *OpenAIGatewayService, *mode429Repo, *mode429Upstream) {
	t.Helper()
	cache, server := redisstatetest.New(t)
	a := mode429Account()
	a.Credentials = map[string]any{"access_token": "simulated-token"}
	repo := &mode429Repo{a: a, redis: server}
	upstream := &mode429Upstream{respond: respond}
	gateway := &OpenAIGatewayService{accountRepo: repo, httpUpstream: upstream}
	svc := NewOpenAI429ModeService(cache, repo, nil, gateway)
	gateway.openAI429Mode = svc
	t.Cleanup(svc.Stop)
	return svc, gateway, repo, upstream
}

const mode429QuotaError = `{"error":{"type":"usage_limit_reached","message":"simulated exhausted quota"}}`
const mode429Complete = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"simulated-response\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n"

func mode429RealCall(t *testing.T, gateway *OpenAIGatewayService, repo *mode429Repo) error {
	t.Helper()
	a, _ := repo.GetByID(context.Background(), 1)
	req, _ := http.NewRequest(http.MethodPost, "https://simulator.invalid/backend-api/codex/responses", bytes.NewBufferString(`{"model":"already-mapped-model","input":"PRIVATE USER TEXT","tools":[{"type":"function","name":"secret-tool"}]}`))
	resp, err := gateway.doOpenAIUpstream(req, "", a)
	if err != nil {
		return err
	}
	_, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return err
}

func TestOpenAI429ModeSimulatedUpstreamCycle(t *testing.T) {
	for _, realSuccess := range []bool{false, true} {
		t.Run(fmt.Sprint(realSuccess), func(t *testing.T) {
			svc, gateway, repo, upstream := newMode429Integration(t, func(n int) (int, string) {
				if n == 10 && realSuccess {
					return 200, mode429Complete
				}
				return 429, mode429QuotaError
			})
			require.NoError(t, mode429RealCall(t, gateway, repo))
			svc.drive(1)
			a, _ := repo.GetByID(context.Background(), 1)
			state := openAI429StateFromExtra(a)
			require.Equal(t, OpenAI429WaitingReal, state.Phase)
			require.Equal(t, 9, state.Consecutive429)
			require.EqualValues(t, 8, state.ProbeTotal)
			require.Len(t, upstream.requests, 9)
			for _, request := range upstream.requests[1:] {
				require.Equal(t, "already-mapped-model", gjson.GetBytes(request, "model").String())
				require.NotContains(t, string(request), "PRIVATE")
				require.False(t, gjson.GetBytes(request, "tools").Exists())
				require.False(t, gjson.GetBytes(request, "previous_response_id").Exists())
			}
			require.NoError(t, mode429RealCall(t, gateway, repo))
			a, _ = repo.GetByID(context.Background(), 1)
			state = openAI429StateFromExtra(a)
			if realSuccess {
				require.Zero(t, state.Consecutive429)
				require.Equal(t, OpenAI429Draining, state.Phase)
			} else {
				require.Equal(t, 10, state.Consecutive429)
				require.Equal(t, OpenAI429Stopped, state.Phase)
				svc.drive(1)
				require.Error(t, mode429RealCall(t, gateway, repo))
				require.Len(t, upstream.requests, 10)
				// A service restart reads the same Redis stop; rebuilding Redis
				// from the durable projection also retains the stop.
				repo.redis.FlushAll()
				restarted := NewOpenAI429ModeService(svc.store.cache, repo, nil, gateway)
				t.Cleanup(restarted.Stop)
				require.True(t, restarted.blocked(context.Background(), a))
			}
		})
	}
}

func TestOpenAI429ModeSimulatedProbeSuccessBudget(t *testing.T) {
	svc, gateway, repo, upstream := newMode429Integration(t, func(n int) (int, string) {
		if n == 1 || n%3 == 0 {
			return 429, mode429QuotaError
		}
		return 200, mode429Complete
	})
	require.NoError(t, mode429RealCall(t, gateway, repo))
	svc.drive(1)
	a, _ := repo.GetByID(context.Background(), 1)
	state := openAI429StateFromExtra(a)
	require.Equal(t, OpenAI429WaitingReal, state.Phase)
	require.Len(t, upstream.requests, 31)
	require.Equal(t, 30, state.RoundCalls)
	require.EqualValues(t, 30, state.ProbeTotal)
	require.Positive(t, state.ProbeInputTokens)
}

func TestOpenAI429ModeDisableManualResetAndLateResults(t *testing.T) {
	svc, _, repo, _ := newMode429Integration(t, func(int) (int, string) { return 429, mode429QuotaError })
	a, _ := repo.GetByID(context.Background(), 1)
	ticket, err := svc.begin(context.Background(), a, "model")
	require.NoError(t, err)
	require.NoError(t, svc.Reset(context.Background(), 1))
	ticket.finish(openAI429Outcome{QuotaExhausted: true, RateLimited: true})
	a, _ = repo.GetByID(context.Background(), 1)
	require.Zero(t, openAI429StateFromExtra(a).Consecutive429)
	ticket, err = svc.begin(context.Background(), a, "model")
	require.NoError(t, err)
	extra, err := normalizeOpenAI429ModeExtra(a, map[string]any{OpenAI429ModeEnabledKey: false})
	require.NoError(t, err)
	require.NoError(t, repo.UpdateExtra(context.Background(), 1, extra))
	svc.reconfigure(context.Background(), 1)
	ticket.finish(openAI429Outcome{QuotaExhausted: true, RateLimited: true})
	a, _ = repo.GetByID(context.Background(), 1)
	newTicket, err := svc.begin(context.Background(), a, "model")
	require.NoError(t, err)
	require.Nil(t, newTicket)
	require.False(t, svc.blocked(context.Background(), a))
}

type mode429FrameStub struct {
	writes   int
	response []byte
}

func (f *mode429FrameStub) ReadFrame(context.Context) (coderws.MessageType, []byte, error) {
	return coderws.MessageText, f.response, nil
}
func (f *mode429FrameStub) WriteFrame(context.Context, coderws.MessageType, []byte) error {
	f.writes++
	return nil
}
func (f *mode429FrameStub) Close() error { return nil }

func TestOpenAI429ModeWebSocketMultipleTurns(t *testing.T) {
	svc, _, repo, _ := newMode429Integration(t, func(int) (int, string) { return 429, mode429QuotaError })
	a, _ := repo.GetByID(context.Background(), 1)
	_, err := svc.change(context.Background(), a, func(st *OpenAI429State) { st.Active = true; st.Consecutive429 = 9; st.Phase = OpenAI429WaitingReal })
	require.NoError(t, err)
	inner := &mode429FrameStub{response: []byte(`{"type":"error","error":{"code":"usage_limit_reached"}}`)}
	conn := &openAI429FrameConn{inner: inner, service: svc, account: a, model: "model"}
	defer conn.Close()
	payload := []byte(`{"type":"response.create","model":"model","input":"hello"}`)
	require.NoError(t, conn.WriteFrame(context.Background(), coderws.MessageText, payload))
	_, _, err = conn.ReadFrame(context.Background())
	require.NoError(t, err)
	require.Error(t, conn.WriteFrame(context.Background(), coderws.MessageText, payload))
	require.Equal(t, 1, inner.writes)
	require.NoError(t, svc.Reset(context.Background(), 1))
	require.NoError(t, conn.WriteFrame(context.Background(), coderws.MessageText, payload))
	require.Equal(t, 2, inner.writes)
}

func TestOpenAI429ModeKnownRecoveryAndImageIsolation(t *testing.T) {
	svc, gateway, repo, upstream := newMode429Integration(t, func(int) (int, string) { return 200, mode429Complete })
	a, _ := repo.GetByID(context.Background(), 1)
	_, err := svc.change(context.Background(), a, func(st *OpenAI429State) {
		st.Active = true
		st.Phase = OpenAI429Stopped
		st.Consecutive429 = 10
		st.RecoverAt = time.Now().Add(-time.Second).UnixMilli()
	})
	require.NoError(t, err)
	svc.drive(1)
	a, _ = repo.GetByID(context.Background(), 1)
	require.Equal(t, OpenAI429Normal, openAI429StateFromExtra(a).Phase)
	_, err = svc.change(context.Background(), a, func(st *OpenAI429State) { st.Active = true; st.Phase = OpenAI429Stopped; st.Consecutive429 = 10 })
	require.NoError(t, err)
	imageBody, _ := json.Marshal(map[string]any{"model": "gpt-image-1", "input": "draw"})
	req, _ := http.NewRequest(http.MethodPost, "https://simulator.invalid/responses", bytes.NewReader(imageBody))
	resp, err := gateway.doOpenAIUpstream(req, "", a)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	a, _ = repo.GetByID(context.Background(), 1)
	require.Equal(t, 10, openAI429StateFromExtra(a).Consecutive429)
	require.Len(t, upstream.requests, 1)
}

type mode429QuotaStub struct {
	calls int
	usage *OpenAIQuotaUsage
}

func (s *mode429QuotaStub) QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error) {
	s.calls++
	return s.usage, nil
}

func TestOpenAI429ModeUnknownRecoveryWaitsForBothWindows(t *testing.T) {
	svc, _, repo, _ := newMode429Integration(t, func(int) (int, string) { return 429, mode429QuotaError })
	query := &mode429QuotaStub{usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{Allowed: true,
		PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 0, LimitWindowSeconds: 18000, ResetAfterSeconds: 3600},
		SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 100, LimitWindowSeconds: 604800, ResetAfterSeconds: 3600}}}}
	svc.quota = query
	a, _ := repo.GetByID(context.Background(), 1)
	_, err := svc.change(context.Background(), a, func(st *OpenAI429State) {
		st.Active = true
		st.Phase = OpenAI429Stopped
		st.Consecutive429 = 10
		st.UnknownReset = true
		st.ExhaustedWindows = 3
	})
	require.NoError(t, err)
	svc.drive(1)
	require.Equal(t, 1, query.calls)
	a, _ = repo.GetByID(context.Background(), 1)
	require.Equal(t, OpenAI429Stopped, openAI429StateFromExtra(a).Phase)
	svc.drive(1)
	require.Equal(t, 1, query.calls, "must not query more than once per five minutes")
	query.usage.RateLimit.SecondaryWindow.UsedPercent = 0
	_, err = svc.change(context.Background(), a, func(st *OpenAI429State) { st.NextQuotaQueryAt = 0 })
	require.NoError(t, err)
	svc.drive(1)
	require.Equal(t, 2, query.calls)
	a, _ = repo.GetByID(context.Background(), 1)
	require.Equal(t, OpenAI429Normal, openAI429StateFromExtra(a).Phase)
}

type mode429BlockingUpstream struct {
	HTTPUpstream
	started chan struct{}
}

func (u *mode429BlockingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	close(u.started)
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestOpenAI429ModeDisableCancelsRunningProbe(t *testing.T) {
	svc, gateway, repo, _ := newMode429Integration(t, func(int) (int, string) { return 429, mode429QuotaError })
	require.NoError(t, mode429RealCall(t, gateway, repo))
	blocking := &mode429BlockingUpstream{started: make(chan struct{})}
	gateway.httpUpstream = blocking
	done := make(chan struct{})
	go func() { svc.drive(1); close(done) }()
	select {
	case <-blocking.started:
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not start")
	}
	require.NoError(t, repo.UpdateExtra(context.Background(), 1, map[string]any{OpenAI429ModeEnabledKey: false, OpenAI429ModeGenerationKey: newOpenAI429Generation(), OpenAI429ModeStateKey: nil}))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probe was not cancelled on disable")
	}
	a, _ := repo.GetByID(context.Background(), 1)
	require.Nil(t, openAI429StateFromExtra(a))
}

func TestOpenAI429ModeRealTimeBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("30 second probe deadline")
	}
	svc, gateway, repo, _ := newMode429Integration(t, func(int) (int, string) { return 429, mode429QuotaError })
	require.NoError(t, mode429RealCall(t, gateway, repo))
	gateway.httpUpstream = &mode429BlockingUpstream{started: make(chan struct{})}
	start := time.Now()
	svc.drive(1)
	require.GreaterOrEqual(t, time.Since(start), 29*time.Second)
	require.Less(t, time.Since(start), 35*time.Second)
	a, _ := repo.GetByID(context.Background(), 1)
	state := openAI429StateFromExtra(a)
	require.Equal(t, OpenAI429WaitingReal, state.Phase)
	require.Equal(t, 1, state.Consecutive429, "budget expiration preserves the accumulated 429 count")
	require.EqualValues(t, 1, state.ProbeTotal)
}

type mode429DrainConcurrency struct {
	ConcurrencyCache
	active     atomic.Int32
	probeSlots atomic.Int32
	queried    chan struct{}
	once       sync.Once
}

func (c *mode429DrainConcurrency) GetAccountConcurrencyBatch(context.Context, []int64) (map[int64]int, error) {
	c.once.Do(func() { close(c.queried) })
	return map[int64]int{1: int(c.active.Load())}, nil
}

func (c *mode429DrainConcurrency) AcquireAccountSlot(_ context.Context, _ int64, cap int, _ string) (bool, error) {
	for {
		active := c.active.Load()
		if int(active) >= cap {
			return false, nil
		}
		if c.active.CompareAndSwap(active, active+1) {
			c.probeSlots.Add(1)
			return true, nil
		}
	}
}

func (c *mode429DrainConcurrency) ReleaseAccountSlot(context.Context, int64, string) error {
	c.active.Add(-1)
	return nil
}

func TestOpenAI429ModeWaitsForOriginalConcurrency(t *testing.T) {
	svc, gateway, repo, upstream := newMode429Integration(t, func(int) (int, string) { return 429, mode429QuotaError })
	require.NoError(t, mode429RealCall(t, gateway, repo))
	concurrency := &mode429DrainConcurrency{queried: make(chan struct{})}
	concurrency.active.Store(1)
	gateway.concurrencyService = NewConcurrencyService(concurrency)
	done := make(chan struct{})
	go func() { defer close(done); svc.drive(1) }()
	select {
	case <-concurrency.queried:
	case <-time.After(time.Second):
		t.Fatal("original account concurrency was not checked")
	}
	upstream.mu.Lock()
	calls := len(upstream.requests)
	upstream.mu.Unlock()
	require.Equal(t, 1, calls, "automatic calls wait for the original handler slot")
	concurrency.active.Store(0)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("probe round did not resume after drain")
	}
	a, _ := repo.GetByID(context.Background(), 1)
	require.Equal(t, 9, openAI429StateFromExtra(a).Consecutive429)
	require.EqualValues(t, 8, concurrency.probeSlots.Load())
	require.Zero(t, concurrency.active.Load())
}

func TestOpenAI429ModeWebSocketLateTerminalDoesNotFinishNextTurn(t *testing.T) {
	svc, _, repo, _ := newMode429Integration(t, func(int) (int, string) { return 429, mode429QuotaError })
	a, _ := repo.GetByID(context.Background(), 1)
	inner := &mode429FrameStub{response: []byte(`{"type":"response.completed","response":{"id":"old-response","status":"completed","output":[]}}`)}
	conn := &openAI429FrameConn{inner: inner, service: svc, account: a, model: "model"}
	t.Cleanup(func() { _ = conn.Close() })
	ctx := context.Background()
	payload := []byte(`{"type":"response.create","model":"model","input":"hello"}`)
	require.NoError(t, conn.WriteFrame(ctx, coderws.MessageText, payload))
	_, _, err := conn.ReadFrame(ctx)
	require.NoError(t, err)
	_, err = svc.change(ctx, a, func(st *OpenAI429State) { st.Active = true; st.Consecutive429 = 9; st.Phase = OpenAI429WaitingReal })
	require.NoError(t, err)
	require.NoError(t, conn.WriteFrame(ctx, coderws.MessageText, payload))
	_, _, err = conn.ReadFrame(ctx) // duplicated terminal from the preceding response
	require.NoError(t, err)
	fresh, _ := repo.GetByID(ctx, 1)
	require.Equal(t, OpenAI429RealInFlight, openAI429StateFromExtra(fresh).Phase)
	require.Equal(t, 9, openAI429StateFromExtra(fresh).Consecutive429)
	inner.response = []byte(`{"type":"response.failed","response":{"id":"new-response","error":{"code":"usage_limit_reached"}}}`)
	_, _, err = conn.ReadFrame(ctx)
	require.NoError(t, err)
	fresh, _ = repo.GetByID(ctx, 1)
	require.Equal(t, OpenAI429Stopped, openAI429StateFromExtra(fresh).Phase)
}

func TestOpenAI429ModeSetupTokenQuotaQuery(t *testing.T) {
	account := mode429Account()
	account.Type = AccountTypeSetupToken
	account.Credentials = map[string]any{"access_token": "setup-token", "chatgpt_account_id": "quota-account"}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{1: account}}
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		authorization = req.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":0,"limit_window_seconds":18000}}}`))
	}))
	t.Cleanup(server.Close)
	quota := NewOpenAIQuotaService(repo, nil, nil, newQuotaRedirectingFactory(server))
	usage, err := quota.QueryUsage(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, usage.RateLimit.Allowed)
	require.Equal(t, "Bearer setup-token", authorization)
	_, err = quota.ResetCreditTargeted(context.Background(), 1, "credit", "request")
	require.Error(t, err, "read-only setup token quota support does not enable credit consumption")
}

func TestOpenAI429ModeQueuedAutoResetRechecksSwitch(t *testing.T) {
	account := mode429Account()
	account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
	account.Extra[OpenAIAutoResetCredit5hThresholdExtraKey] = 0.9
	account.Extra[OpenAIAutoResetCredit7dThresholdExtraKey] = 0.95
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{}
	service := &OpenAIQuotaAutoResetService{accountRepo: repo, quota: quota}
	_, err := service.resetCreditIfEnabled(context.Background(), 1, "credit", "request")
	require.ErrorIs(t, err, errOpenAIAutoResetDisabled)
	require.Zero(t, quota.resetCalls.Load())
	require.Equal(t, true, account.Extra[OpenAIAutoResetCreditEnabledExtraKey])
	require.NoError(t, repo.UpdateExtra(context.Background(), 1, map[string]any{OpenAI429ModeEnabledKey: false}))
	_, err = service.resetCreditIfEnabled(context.Background(), 1, "credit", "request")
	require.NoError(t, err)
	require.EqualValues(t, 1, quota.resetCalls.Load())
	require.Equal(t, 0.9, ResolveOpenAIAutoResetCreditConfig(account).Threshold5h)
}

func TestOpenAI429ModeRedisFailureReturnsAdmissionFailure(t *testing.T) {
	_, gateway, repo, upstream := newMode429Integration(t, func(int) (int, string) { return 200, mode429Complete })
	repo.redis.Close()
	require.ErrorIs(t, mode429RealCall(t, gateway, repo), errOpenAI429Admission)
	require.Empty(t, upstream.requests)
}

func TestOpenAI429ModeDisabledHTTPDoesNotTouchCoordinatorDependencies(t *testing.T) {
	cache, server := redisstatetest.New(t)
	account := mode429Account()
	account.Extra = map[string]any{"unrelated": "preserved"}
	repo := &mode429Repo{a: account}
	upstream := &mode429Upstream{respond: func(int) (int, string) { return 200, mode429Complete }}
	gateway := &OpenAIGatewayService{accountRepo: repo, httpUpstream: upstream}
	gateway.openAI429Mode = NewOpenAI429ModeService(cache, repo, nil, gateway)
	server.Close() // Any accidental coordinator access now fails the request.
	req, _ := http.NewRequest(http.MethodPost, "https://simulator.invalid/backend-api/codex/responses", bytes.NewBufferString(`{"model":"gpt-5.1","input":"hello"}`))
	resp, err := gateway.doOpenAIUpstream(req, "", account)
	require.NoError(t, err)
	require.NotNil(t, resp)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Len(t, upstream.requests, 1)
}

func TestOpenAI429ModeDisabledSkipsConcentrationAndManualReset(t *testing.T) {
	cache, server := redisstatetest.New(t)
	account := mode429Account()
	account.Extra = map[string]any{}
	repo := &mode429Repo{a: account}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	svc := NewOpenAI429ModeService(cache, repo, nil, gateway)
	gateway.openAI429Mode = svc
	server.Close() // Both operations must return before Redis or repository use.
	selection, err := gateway.selectOpenAI429Concentrated(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-5.1"})
	require.NoError(t, err)
	require.Nil(t, selection)
	require.NoError(t, svc.Reset(context.Background(), account.ID))
	require.False(t, svc.handles429(context.Background(), account, nil, []byte(mode429QuotaError)))
}
