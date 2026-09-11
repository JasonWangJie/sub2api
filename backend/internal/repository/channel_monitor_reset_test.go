package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitordailyrollup"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitorhistory"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorResetDataClearsRecordedStateOnly(t *testing.T) {
	ctx := context.Background()
	client := newSecuritySecretTestClient(t)
	checkedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	monitor, err := client.ChannelMonitor.Create().
		SetName("primary").
		SetProvider(channelmonitor.ProviderOpenai).
		SetEndpoint("https://api.example.com").
		SetAPIKeyEncrypted("ciphertext").
		SetPrimaryModel("gpt-4o-mini").
		SetEnabled(true).
		SetIntervalSeconds(60).
		SetLastCheckedAt(checkedAt).
		SetCreatedBy(7).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ChannelMonitorHistory.Create().
		SetMonitorID(monitor.ID).
		SetModel("gpt-4o-mini").
		SetStatus(channelmonitorhistory.StatusOperational).
		SetCheckedAt(checkedAt).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ChannelMonitorDailyRollup.Create().
		SetMonitorID(monitor.ID).
		SetModel("gpt-4o-mini").
		SetBucketDate(checkedAt).
		SetTotalChecks(1).
		SetOkCount(1).
		Save(ctx)
	require.NoError(t, err)

	repo := &channelMonitorRepository{client: client}
	require.NoError(t, repo.ResetData(ctx, monitor.ID))

	got, err := client.ChannelMonitor.Get(ctx, monitor.ID)
	require.NoError(t, err)
	require.Nil(t, got.LastCheckedAt)
	require.Equal(t, "ciphertext", got.APIKeyEncrypted)
	require.True(t, got.Enabled)
	historyCount, err := client.ChannelMonitorHistory.Query().
		Where(channelmonitorhistory.MonitorIDEQ(monitor.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, historyCount)
	rollupCount, err := client.ChannelMonitorDailyRollup.Query().
		Where(channelmonitordailyrollup.MonitorIDEQ(monitor.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, rollupCount)
}

func TestChannelMonitorResetDataRejectsMissingMonitor(t *testing.T) {
	repo := &channelMonitorRepository{client: newSecuritySecretTestClient(t)}
	err := repo.ResetData(context.Background(), 999)
	require.ErrorIs(t, err, service.ErrChannelMonitorNotFound)
}
