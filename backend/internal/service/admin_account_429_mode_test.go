//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAdminOpenAI429ModeBulkPreflight(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		for _, unsupported := range []bool{false, true} {
			a, b := mode429Account(), mode429Account()
			b.ID = 2
			if unsupported {
				b.Type = AccountTypeAPIKey
				b.Name = "unsupported-key"
			}
			repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{a, b}, listData: []Account{*a, *b}}
			svc := &adminServiceImpl{accountRepo: repo}
			input := &BulkUpdateAccountsInput{AccountIDs: []int64{1, 2}, Extra: map[string]any{OpenAI429ModeEnabledKey: false, "other": "preserved", OpenAI429ModeStateKey: "forged"}}
			if filtered {
				input.AccountIDs = nil
				input.Filters = &BulkUpdateAccountFilters{Platform: PlatformOpenAI}
			}
			_, err := svc.BulkUpdateAccounts(context.Background(), input)
			if unsupported {
				require.ErrorContains(t, err, "account 2 (unsupported-key)")
				require.Zero(t, repo.bulkUpdateCalls)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, repo.bulkUpdateCalls)
				require.Equal(t, false, repo.lastBulkUpdate.Extra[OpenAI429ModeEnabledKey])
				require.Equal(t, "preserved", repo.lastBulkUpdate.Extra["other"])
				require.NotContains(t, repo.lastBulkUpdate.Extra, OpenAI429ModeStateKey)
			}
		}
	}
}

func TestAdminOpenAI429ModeBulkInvalidBoolean(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Extra: map[string]any{OpenAI429ModeEnabledKey: "true"}})
	require.Error(t, err)
	require.Zero(t, repo.bulkUpdateCalls)
}

func TestAdminOpenAI429ModeSingleTogglePreservesOtherExtra(t *testing.T) {
	a := mode429Account()
	a.Extra["custom_config"] = "keep"
	a.Extra[OpenAI429ModeStateKey] = &OpenAI429State{Generation: "0001", Phase: OpenAI429Stopped, Consecutive429: 10}
	repo := &accountRepoStubForBulkUpdate{getByIDAccounts: map[int64]*Account{1: a}}
	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{OpenAI429ModeEnabledKey: false}})
	require.NoError(t, err)
	require.Equal(t, "keep", updated.Extra["custom_config"])
	require.Equal(t, false, updated.Extra[OpenAI429ModeEnabledKey])
	require.NotContains(t, updated.Extra, OpenAI429ModeStateKey)
}

func TestOpenAI429ModeSparkStillUsesIndependentRateLimit(t *testing.T) {
	repo := &oauth429RateLimitRepo{}
	rateLimits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	gateway := &OpenAIGatewayService{rateLimitService: rateLimits}
	account := mode429Account()
	require.False(t, gateway.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(mode429QuotaError), "gpt-5.3-codex-spark"))
	require.Equal(t, 1, repo.setModelRateLimitCalls)
	require.Zero(t, repo.setRateLimitedCalls)
}

type mode429ExtraUpdateRepo struct {
	accountRepoStubForBulkUpdate
	updates map[string]any
}

func (r *mode429ExtraUpdateRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return nil
}

func TestAdminOpenAI429ModeExtraMergeNeverWritesRuntimeSnapshot(t *testing.T) {
	account := mode429Account()
	account.Extra[OpenAI429ModeStateKey] = &OpenAI429State{Phase: OpenAI429Stopped, Consecutive429: 10}
	repo := &mode429ExtraUpdateRepo{accountRepoStubForBulkUpdate: accountRepoStubForBulkUpdate{getByIDAccounts: map[int64]*Account{1: account}}}
	svc := &adminServiceImpl{accountRepo: repo}
	require.NoError(t, svc.UpdateAccountExtra(context.Background(), 1, map[string]any{OpenAI429ModeEnabledKey: true, "other": "keep", OpenAI429ModeStateKey: "forged"}))
	require.NotContains(t, repo.updates, OpenAI429ModeStateKey)
	require.Equal(t, "keep", repo.updates["other"])
	require.Equal(t, true, repo.updates[OpenAI429ModeEnabledKey])
}
