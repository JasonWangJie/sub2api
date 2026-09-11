package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/testutil/redisstatetest"
	"github.com/stretchr/testify/require"
)

type mode429Concurrency struct {
	schedulerTestConcurrencyCache
	caps []int
}

func (c *mode429Concurrency) AcquireAccountSlot(ctx context.Context, id int64, cap int, requestID string) (bool, error) {
	c.caps = append(c.caps, cap)
	return c.schedulerTestConcurrencyCache.AcquireAccountSlot(ctx, id, cap, requestID)
}

func TestOpenAI429ModeConcentrationPreservesConcurrencyAndVetoes(t *testing.T) {
	cacheStore, _ := redisstatetest.New(t)
	accounts := []Account{*mode429Account(), *mode429Account(), *mode429Account(), *mode429Account()}
	for i := range accounts {
		accounts[i].ID = int64(i + 1)
		accounts[i].Extra["codex_usage_updated_at"] = time.Now().Format(time.RFC3339)
		accounts[i].Extra["codex_5h_reset_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
		accounts[i].Extra["codex_5h_used_percent"] = float64(95 - i)
	}
	accounts[3].Extra["codex_5h_used_percent"] = 99.0
	accounts[3].Schedulable = false // administrator pause always wins
	accounts[1].Extra[OpenAI429ModeEnabledKey] = false
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"sticky": 3}}
	concurrency := &mode429Concurrency{schedulerTestConcurrencyCache: schedulerTestConcurrencyCache{acquireResults: map[int64]bool{}}}
	repo := schedulerTestOpenAIAccountRepo{accounts: accounts}
	gateway := &OpenAIGatewayService{accountRepo: repo, cfg: &config.Config{}, cache: cache, concurrencyService: NewConcurrencyService(concurrency)}
	gateway.openAI429Mode = NewOpenAI429ModeService(cacheStore, repo, nil, gateway)
	gateway.openAI429Mode.setConfiguredEnabled(1, true)
	t.Cleanup(gateway.openAI429Mode.Stop)
	selection, _, err := gateway.SelectAccountWithScheduler(context.Background(), nil, "", "sticky", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 1, selection.Account.ID, "concentration wins over ordinary session stickiness")
	selection.ReleaseFunc()
	concurrency.acquireResults[1] = false
	selection, err = gateway.selectOpenAI429Concentrated(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-5.1"})
	require.NoError(t, err)
	require.EqualValues(t, 3, selection.Account.ID, "disabled accounts never enter concentration")
	selection.ReleaseFunc()
	for _, cap := range concurrency.caps {
		require.Equal(t, 8, cap)
	}
	selection, err = gateway.selectOpenAI429Concentrated(WithOpenAI429ModeExcluded(context.Background(), true), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-5.1"})
	require.NoError(t, err)
	require.Nil(t, selection, "image requests bypass text concentration")
	selection, err = gateway.selectOpenAI429Concentrated(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-5.1", PreviousResponseID: "bound-response"})
	require.NoError(t, err)
	require.Nil(t, selection, "mandatory context binding wins")
}
