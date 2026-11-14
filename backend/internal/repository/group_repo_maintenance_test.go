package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupRepositoryMaintenanceRoundTrip(t *testing.T) {
	users, client := newUserEntRepo(t)
	repo := newGroupRepositoryWithSQL(client, users.sql)
	ctx := context.Background()
	group := &service.Group{
		Name: "Maintenance", Platform: service.PlatformAnthropic, Status: service.StatusActive,
		RateMultiplier: 1, IsMaintenance: true, SubscriptionType: service.SubscriptionTypeStandard,
	}
	require.NoError(t, repo.Create(ctx, group))
	loaded, err := repo.GetByIDLite(ctx, group.ID)
	require.NoError(t, err)
	require.True(t, loaded.IsMaintenance)

	activeIDs, err := repo.ListActiveIDs(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{group.ID}, activeIDs, "maintenance groups must remain active")

	loaded.IsMaintenance = false
	require.NoError(t, repo.Update(ctx, loaded))
	loaded, err = repo.GetByIDLite(ctx, group.ID)
	require.NoError(t, err)
	require.False(t, loaded.IsMaintenance)
	require.True(t, loaded.IsActive())
}
