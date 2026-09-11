package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/testutil/redisstatetest"
	"github.com/stretchr/testify/require"
)

func mode429Account() *Account {
	return &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 8, Extra: map[string]any{OpenAI429ModeEnabledKey: true, OpenAI429ModeGenerationKey: "0001"}}
}

func TestOpenAI429ModeConfiguration(t *testing.T) {
	a := mode429Account()
	a.Extra = nil
	require.False(t, OpenAI429ModeEnabled(a))
	for _, bad := range []any{nil, "true", 1, 0.0} {
		_, err := normalizeOpenAI429ModeExtra(a, map[string]any{OpenAI429ModeEnabledKey: bad})
		require.Error(t, err)
	}
	a.Extra = map[string]any{"other": "kept", OpenAI429ModeEnabledKey: true, OpenAI429ModeGenerationKey: "0001", OpenAI429ModeStateKey: map[string]any{"phase": "stopped"}}
	extra, err := normalizeOpenAI429ModeExtra(a, map[string]any{"other": "kept", OpenAI429ModeEnabledKey: false, OpenAI429ModeStateKey: "forged"})
	require.NoError(t, err)
	require.Equal(t, false, extra[OpenAI429ModeEnabledKey])
	require.Equal(t, "kept", extra["other"])
	require.NotContains(t, extra, OpenAI429ModeStateKey)
	require.NotEqual(t, "0001", extra[OpenAI429ModeGenerationKey])
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		a.Type = typ
		require.True(t, SupportsOpenAI429Mode(a))
	}
	a.Type = AccountTypeAPIKey
	_, err = normalizeOpenAI429ModeExtra(a, map[string]any{OpenAI429ModeEnabledKey: false})
	require.ErrorContains(t, err, "account 1")
	a.Type = AccountTypeOAuth
	parent := int64(2)
	a.ParentAccountID = &parent
	require.False(t, SupportsOpenAI429Mode(a))
}

func TestOpenAI429ModeThresholds(t *testing.T) {
	now := time.Now()
	for _, window := range []string{"5h", "7d"} {
		for _, percent := range []float64{89.9, 90, 100} {
			t.Run(fmt.Sprintf("%s/%g", window, percent), func(t *testing.T) {
				a := mode429Account()
				a.Extra["codex_usage_updated_at"] = now.Format(time.RFC3339)
				a.Extra["codex_"+window+"_used_percent"] = percent
				a.Extra["codex_"+window+"_reset_at"] = now.Add(time.Hour).Format(time.RFC3339)
				st := OpenAI429State{Model: "mapped-model"}
				st.observeQuota(a, now)
				expected := OpenAI429Normal
				if percent >= 90 {
					expected = OpenAI429Concentrating
				}
				if percent >= 100 {
					expected = OpenAI429Draining
				}
				require.Equal(t, expected, st.Phase)
				a.Extra["codex_usage_updated_at"] = now.Add(-openAI429SnapshotTTL - time.Second).Format(time.RFC3339)
				st = OpenAI429State{}
				st.observeQuota(a, now)
				require.Equal(t, OpenAI429Normal, st.Phase)
			})
		}
	}
	a := mode429Account()
	a.Extra["codex_usage_updated_at"] = now.Format(time.RFC3339)
	for _, w := range []string{"5h", "7d"} {
		a.Extra["codex_"+w+"_used_percent"] = 100.0
	}
	a.Extra["codex_5h_reset_at"] = now.Add(time.Hour).Format(time.RFC3339)
	a.Extra["codex_7d_reset_at"] = now.Add(7 * time.Hour).Format(time.RFC3339)
	_, exhausted, reset, unknown := openAI429Quota(a, now)
	require.True(t, exhausted)
	require.False(t, unknown)
	require.WithinDuration(t, now.Add(7*time.Hour), time.UnixMilli(reset), time.Second)
	a.Extra["codex_7d_reset_at"] = now.Add(-time.Second).Format(time.RFC3339)
	_, _, reset, _ = openAI429Quota(a, now)
	require.WithinDuration(t, now.Add(time.Hour), time.UnixMilli(reset), time.Second)
}

func countToNine(t *testing.T, state *OpenAI429State, now int64) {
	t.Helper()
	require.True(t, state.admit("initial", "mapped-model", now))
	require.True(t, state.finish("initial", openAI429Outcome{RateLimited: true, QuotaExhausted: true}, now))
	require.Equal(t, 1, state.Consecutive429)
	require.True(t, state.claimProbe("worker", now))
	for i := 0; i < 8; i++ {
		id := fmt.Sprint(i)
		require.True(t, state.nextProbe("worker", id, now+int64(i)))
		require.True(t, state.finish(id, openAI429Outcome{RateLimited: true}, now+int64(i)))
	}
	require.Equal(t, 9, state.Consecutive429)
	require.Equal(t, OpenAI429WaitingReal, state.Phase)
	require.False(t, state.nextProbe("worker", "extra", now+20))
}

func TestOpenAI429ModeNineThenReal(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			now := time.Now().UnixMilli()
			st := OpenAI429State{Phase: OpenAI429Normal}
			countToNine(t, &st, now)
			require.True(t, st.admit("real", "mapped-model", now+30))
			require.False(t, st.admit("second", "mapped-model", now+30))
			require.False(t, st.claimProbe("competitor", now+30))
			require.True(t, st.finish("real", openAI429Outcome{Success: success, RateLimited: !success}, now+40))
			if success {
				require.Zero(t, st.Consecutive429)
				require.True(t, st.claimProbe("new-worker", now+50))
			} else {
				require.Equal(t, 10, st.Consecutive429)
				require.Equal(t, OpenAI429Stopped, st.Phase)
				require.False(t, st.admit("late", "mapped-model", now+50))
			}
		})
	}
}

func TestOpenAI429ModeDrainOrderAndDedup(t *testing.T) {
	now := time.Now().UnixMilli()
	st := OpenAI429State{Phase: OpenAI429Normal}
	require.True(t, st.admit("a", "model", now))
	require.True(t, st.admit("b", "model", now))
	st.finish("a", openAI429Outcome{RateLimited: true, QuotaExhausted: true}, now)
	require.False(t, st.admit("new", "model", now))
	require.False(t, st.claimProbe("worker", now))
	require.False(t, st.finish("a", openAI429Outcome{RateLimited: true}, now))
	require.Equal(t, 1, st.Consecutive429)
	st.finish("b", openAI429Outcome{Success: true}, now)
	require.Zero(t, st.Consecutive429)
	require.True(t, st.claimProbe("worker", now))
}

func TestOpenAI429ModeBudgetsAndInterruption(t *testing.T) {
	now := time.Now().UnixMilli()
	for _, timeout := range []bool{false, true} {
		st := OpenAI429State{Phase: OpenAI429Draining, Active: true, Model: "model", Consecutive429: 3}
		require.True(t, st.claimProbe("worker", now))
		if timeout {
			require.False(t, st.nextProbe("worker", "late", now+30000))
			require.Equal(t, 3, st.Consecutive429)
		} else {
			for i := 0; i < 30; i++ {
				id := fmt.Sprint(i)
				require.True(t, st.nextProbe("worker", id, now+int64(i)))
				st.finish(id, openAI429Outcome{Success: true}, now+int64(i))
			}
			require.False(t, st.nextProbe("worker", "31", now+31))
			require.Equal(t, 30, st.RoundCalls)
		}
		require.Equal(t, OpenAI429WaitingReal, st.Phase)
	}
	st := OpenAI429State{Phase: OpenAI429Draining, Active: true, Model: "model", Consecutive429: 7}
	require.True(t, st.claimProbe("worker", now))
	require.True(t, st.nextProbe("worker", "error", now))
	st.finish("error", openAI429Outcome{}, now)
	require.Zero(t, st.Consecutive429)
	require.Equal(t, OpenAI429WaitingReal, st.Phase)
}

func TestOpenAI429ModeRedisCompetitionAndFencing(t *testing.T) {
	cache, server := redisstatetest.New(t)
	store := &openAI429StateStore{cache: cache}
	a := mode429Account()
	ctx := context.Background()
	_, err := store.mutate(ctx, a, func(st *OpenAI429State) { st.Active = true; st.Phase = OpenAI429WaitingReal; st.Consecutive429 = 9 })
	require.NoError(t, err)
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			allowed := false
			_, err := store.mutate(ctx, a, func(st *OpenAI429State) { allowed = st.admit(fmt.Sprint(i), "model", time.Now().UnixMilli()) })
			if err == nil && allowed {
				admitted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 1, admitted.Load())
	old := *a
	a.Extra = cloneOpenAIAutoResetExtra(a.Extra)
	a.Extra[OpenAI429ModeGenerationKey] = "0002"
	_, err = store.mutate(ctx, a, func(st *OpenAI429State) { st.reset() })
	require.NoError(t, err)
	_, err = store.mutate(ctx, &old, func(st *OpenAI429State) { st.Consecutive429 = 10 })
	require.Error(t, err)
	state, err := store.mutate(ctx, a, func(*OpenAI429State) {})
	require.NoError(t, err)
	require.Zero(t, state.Consecutive429)
	server.Close()
	_, err = store.mutate(ctx, a, func(*OpenAI429State) {})
	require.Error(t, err)
}

type mode429BlockingScanRepo struct {
	AccountRepository
	started chan struct{}
	release chan struct{}
}

func (r *mode429BlockingScanRepo) FindByExtraField(context.Context, string, any) ([]Account, error) {
	close(r.started)
	<-r.release
	return nil, nil
}

func TestOpenAI429ModeConfigScanDoesNotBlockRequestFastPath(t *testing.T) {
	cache, _ := redisstatetest.New(t)
	repo := &mode429BlockingScanRepo{started: make(chan struct{}), release: make(chan struct{})}
	svc := NewOpenAI429ModeService(cache, repo, nil, nil)
	started := make(chan struct{})
	go func() {
		svc.Start()
		close(started)
	}()
	<-repo.started

	updated := make(chan struct{})
	go func() {
		svc.setConfiguredEnabled(1, true)
		close(updated)
	}()
	select {
	case <-updated:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("configuration scan held the request-side lock")
	}

	close(repo.release)
	<-started
	require.True(t, svc.configuredEnabled(1), "a concurrent admin update must survive an older scan")
	svc.Stop()
}

type mode429PendingRepo struct {
	AccountRepository
	account *Account
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (r *mode429PendingRepo) GetByID(context.Context, int64) (*Account, error) {
	if r.calls.Add(1) == 1 {
		close(r.started)
		<-r.release
	}
	return r.account, nil
}

func TestOpenAI429ModeDriverKeepsNotificationDuringRun(t *testing.T) {
	account := mode429Account()
	account.Extra[OpenAI429ModeEnabledKey] = false
	repo := &mode429PendingRepo{account: account, started: make(chan struct{}), release: make(chan struct{})}
	svc := NewOpenAI429ModeService(nil, repo, nil, nil)

	svc.startDrive(account.ID)
	<-repo.started
	svc.startDrive(account.ID)
	close(repo.release)
	require.Eventually(t, func() bool { return repo.calls.Load() == 2 }, time.Second, time.Millisecond)
	svc.Stop()
}

func TestOpenAI429ModeResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		name, data                        string
		stream, success, limited, ignored bool
	}{
		{"empty200", `{}`, false, false, false, false},
		{"incomplete", `{"object":"response","id":"r","status":"incomplete","output":[]}`, false, false, false, false},
		{"complete", `{"object":"response","id":"r","status":"completed","output":[]}`, false, true, false, false},
		{"sse", "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"output\":[]}}\n\n", true, true, false, false},
		{"truncated", "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n", true, false, false, false},
		{"doneOnly", "data: [DONE]\n\n", true, false, false, false},
		{"semantic429", "data: {\"type\":\"error\",\"error\":{\"code\":\"usage_limit_reached\"}}\n\n", true, false, true, false},
		{"image", `{"object":"response","id":"r","status":"completed","output":[{"type":"image_generation_call"}]}`, false, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := openAI429Observer{status: 200, stream: tc.stream}
			for _, b := range []byte(tc.data) {
				o.feed([]byte{b})
			}
			out := o.result(true)
			require.Equal(t, tc.success, out.Success)
			require.Equal(t, tc.limited, out.RateLimited)
			require.Equal(t, tc.ignored, out.Ignore)
		})
	}
}

func TestOpenAI429ModeDrainsAllResultsBeforeStop(t *testing.T) {
	for _, lastSuccess := range []bool{false, true} {
		st := OpenAI429State{Phase: OpenAI429Normal}
		now := time.Now().UnixMilli()
		for i := 0; i < 11; i++ {
			require.True(t, st.admit(fmt.Sprint(i), "model", now))
		}
		for i := 0; i < 10; i++ {
			st.finish(fmt.Sprint(i), openAI429Outcome{QuotaExhausted: true, RateLimited: true}, now)
			require.Equal(t, OpenAI429Draining, st.Phase)
			require.False(t, st.claimProbe("worker", now))
		}
		st.finish("10", openAI429Outcome{Success: lastSuccess, RateLimited: !lastSuccess}, now)
		if lastSuccess {
			require.Zero(t, st.Consecutive429)
			require.True(t, st.claimProbe("worker", now))
		} else {
			require.Equal(t, OpenAI429Stopped, st.Phase)
		}
	}
}

func TestOpenAI429ModeUnknownWindowDoesNotExpireWithKnownWindow(t *testing.T) {
	now := time.Now()
	a := mode429Account()
	a.Extra["codex_usage_updated_at"] = now.Format(time.RFC3339)
	a.Extra["codex_5h_used_percent"], a.Extra["codex_7d_used_percent"] = 100.0, 100.0
	a.Extra["codex_5h_reset_at"] = now.Add(time.Hour).Format(time.RFC3339)
	delete(a.Extra, "codex_7d_reset_at")
	st := OpenAI429State{Model: "model"}
	st.observeQuota(a, now)
	require.True(t, st.UnknownReset)
	st.Consecutive429 = 9
	st.expire(now.Add(2 * time.Hour).UnixMilli())
	require.Equal(t, 9, st.Consecutive429)
}

func TestOpenAI429ModeCompactResponseValidation(t *testing.T) {
	require.True(t, openAI429TextRequest("/v1/responses/compact", "gpt-5.1", nil))
	for _, valid := range []bool{false, true} {
		body := `{"id":"compact","output":[]}`
		if valid {
			body = `{"id":"compact","object":"response.compaction","output":[{"type":"compaction","encrypted_content":"encrypted-result"}]}`
		}
		observer := openAI429Observer{status: 200, compact: true}
		observer.feed([]byte(body))
		require.Equal(t, valid, observer.result(true).Success)
	}
}
