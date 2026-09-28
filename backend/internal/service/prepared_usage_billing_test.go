package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type preparedUsageLogRepoStub struct {
	UsageLogRepository
	err   error
	calls int
}

func (s *preparedUsageLogRepoStub) Create(context.Context, *UsageLog) (bool, error) {
	s.calls++
	return s.err == nil, s.err
}

func TestCapturePreparedUsageBillingFreezesCommandAndCost(t *testing.T) {
	sink := &usageBillingPreparationSink{}
	ctx := withUsageBillingPreparation(context.Background(), sink)
	command := &UsageBillingCommand{
		RequestID:          "client:async-image:imgtask_1",
		APIKeyID:           7,
		UserID:             9,
		AccountID:          11,
		Model:              "gemini-image",
		ImageCount:         1,
		BalanceCost:        0.04,
		RequestPayloadHash: "payload-hash",
	}
	usageLog := &UsageLog{RequestID: command.RequestID, APIKeyID: 7, ActualCost: 0.04, ImageCount: 1}
	params := &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 0.02, ActualCost: 0.04},
		IsSubscriptionBill:    false,
		AccountRateMultiplier: 1.25,
		Platform:              PlatformGemini,
	}

	require.True(t, capturePreparedUsageBilling(ctx, command, usageLog, params))
	prepared := sink.get()
	require.NotNil(t, prepared)
	require.NotEmpty(t, prepared.Command.RequestFingerprint)
	require.Equal(t, 0.04, prepared.ActualCost())
	require.Equal(t, PlatformGemini, prepared.Platform)

	command.BalanceCost = 9
	params.Cost.ActualCost = 9
	require.Equal(t, 0.04, prepared.Command.BalanceCost)
	require.Equal(t, 0.04, prepared.Cost.ActualCost)
}

func TestValidatePreparedUsageBilling(t *testing.T) {
	prepared := &PreparedUsageBilling{Command: UsageBillingCommand{
		RequestID: "client:async-image:imgtask_1",
		APIKeyID:  7,
	}}
	require.NoError(t, ValidatePreparedUsageBilling(prepared, "imgtask_1", 7))
	require.Error(t, ValidatePreparedUsageBilling(prepared, "imgtask_2", 7))
	require.Error(t, ValidatePreparedUsageBilling(prepared, "imgtask_1", 8))
}

func TestPrepareUsageBillingRequiresCapturedCommand(t *testing.T) {
	_, err := prepareUsageBilling(context.Background(), func(context.Context) error { return nil })
	require.ErrorContains(t, err, "produced no billing command")
}

func TestCapturePreparedNotBillableUsageBillingCreatesZeroChargeCommand(t *testing.T) {
	sink := &usageBillingPreparationSink{}
	ctx := withUsageBillingPreparation(context.Background(), sink)
	usageLog := &UsageLog{RequestID: "client:async-image:simple_1", APIKeyID: 7, UserID: 9, AccountID: 11, ActualCost: 4}
	params := &postUsageBillingParams{
		Cost: &CostBreakdown{TotalCost: 2, ActualCost: 4},
		User: &User{ID: 9}, APIKey: &APIKey{ID: 7}, Account: &Account{ID: 11},
		Platform: PlatformGemini,
	}

	require.True(t, capturePreparedNotBillableUsageBilling(ctx, usageLog.RequestID, usageLog, params))
	prepared := sink.get()
	require.NotNil(t, prepared)
	require.True(t, prepared.NotBillable)
	require.Zero(t, prepared.ActualCost())
	require.Zero(t, prepared.Command.BalanceCost)
	require.Equal(t, usageLog.RequestID, prepared.Command.RequestID)
}

func TestApplyPreparedNotBillableUsageRetriesWhenUsageLogFails(t *testing.T) {
	logRepo := &preparedUsageLogRepoStub{err: errors.New("database unavailable")}
	prepared := &PreparedUsageBilling{
		NotBillable: true,
		Command:     UsageBillingCommand{RequestID: "client:async-image:simple_2", APIKeyID: 7},
		UsageLog:    UsageLog{RequestID: "client:async-image:simple_2", APIKeyID: 7},
	}

	err := applyPreparedUsageBilling(context.Background(), prepared, nil, nil, nil, logRepo, "test")
	require.ErrorContains(t, err, "persist prepared usage log")
	require.Equal(t, 1, logRepo.calls)

	logRepo.err = nil
	require.NoError(t, applyPreparedUsageBilling(context.Background(), prepared, nil, nil, nil, logRepo, "test"))
	require.Equal(t, 2, logRepo.calls)
}

func TestPreparedUsageBillingPreservesSimpleModeAcrossRestore(t *testing.T) {
	for _, gateway := range []string{"gateway", "openai"} {
		t.Run(gateway, func(t *testing.T) {
			user := &User{ID: 7}
			apiKey := &APIKey{ID: 13, Quota: 100, RateLimit5h: 10}
			account := &Account{ID: 9, Type: AccountTypeAPIKey}
			keyService := &apiKeyQuotaUpdaterStub{}
			requestID := "client:async-image:simple-window"
			sink := &usageBillingPreparationSink{}
			_, err := applyUsageBilling(withUsageBillingPreparation(context.Background(), sink), requestID,
				&UsageLog{RequestID: requestID, UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID},
				&postUsageBillingParams{
					Cost: &CostBreakdown{ActualCost: 3.25, TotalCost: 2.5},
					User: user, APIKey: apiKey, Account: account, APIKeyService: keyService,
					SimpleModeKeyRateLimitOnly: true,
				}, &billingDeps{}, nil)
			require.NoError(t, err)
			require.NotNil(t, sink.get())
			stored, err := json.Marshal(sink.get())
			require.NoError(t, err)
			var restored PreparedUsageBilling
			require.NoError(t, json.Unmarshal(stored, &restored))
			require.True(t, restored.SimpleModeKeyRateLimitOnly)

			repo := &simpleModeUsageBillingRepoStub{}
			logRepo := &preparedUsageLogRepoStub{}
			cache := &simpleModeInvalidationCache{}
			writes := make(chan cacheWriteTask, 4)
			billingCache := &BillingCacheService{cache: cache, cacheWriteChan: writes}
			deferred := &DeferredService{}
			if gateway == "gateway" {
				service := &GatewayService{usageBillingRepo: repo, usageLogRepo: logRepo,
					billingCacheService: billingCache, deferredService: deferred}
				err = service.ApplyPreparedRecordUsage(context.Background(), &restored, &RecordUsageInput{
					User: user, APIKey: apiKey, Account: account, APIKeyService: keyService,
				})
			} else {
				service := &OpenAIGatewayService{usageBillingRepo: repo, usageLogRepo: logRepo,
					billingCacheService: billingCache, deferredService: deferred}
				err = service.ApplyPreparedRecordUsage(context.Background(), &restored, &OpenAIRecordUsageInput{
					User: user, APIKey: apiKey, Account: account, APIKeyService: keyService,
				})
			}
			require.NoError(t, err)
			require.Len(t, repo.cmds, 1)
			require.Equal(t, 3.25, repo.cmds[0].APIKeyRateLimitCost)
			require.Zero(t, repo.cmds[0].BalanceCost)
			require.Zero(t, repo.cmds[0].SubscriptionCost)
			require.Zero(t, repo.cmds[0].APIKeyQuotaCost)
			require.Zero(t, repo.cmds[0].AccountQuotaCost)
			require.Equal(t, []int64{13}, cache.invalidated)
			require.Empty(t, writes, "restored simple-mode tasks must not enqueue financial cache updates")
			require.Equal(t, 1, logRepo.calls)
			_, scheduled := deferred.lastUsedUpdates.Load(account.ID)
			require.True(t, scheduled)
		})
	}
}
