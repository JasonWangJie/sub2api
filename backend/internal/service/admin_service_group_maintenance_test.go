//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAdminServiceUpdateGroupMaintenance(t *testing.T) {
	for _, mode := range []string{"", config.RunModeSimple} {
		t.Run("mode="+mode, func(t *testing.T) {
			group := &Group{ID: 1, Name: "Standard", Platform: PlatformAnthropic, Status: StatusActive, RateMultiplier: 1}
			repo := &groupRepoStubForAdmin{getByID: group}
			svc := &adminServiceImpl{cfg: &config.Config{RunMode: mode}, groupRepo: repo}
			maintenance := true
			_, err := svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{IsMaintenance: &maintenance})
			require.NoError(t, err)
			require.True(t, repo.updated.IsMaintenance)
			require.True(t, repo.updated.IsActive(), "maintenance must remain a display marker")

			_, err = svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{Name: "Renamed"})
			require.NoError(t, err)
			require.True(t, repo.updated.IsMaintenance, "omitting the flag must preserve it")

			maintenance = false
			_, err = svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{IsMaintenance: &maintenance})
			require.NoError(t, err)
			require.False(t, repo.updated.IsMaintenance)
			require.True(t, repo.updated.IsActive())
		})
	}
}

func TestGroupMaintenanceSurvivesAuthSnapshot(t *testing.T) {
	svc := &APIKeyService{}
	key := &APIKey{User: &User{ID: 2}, Group: &Group{ID: 1, IsMaintenance: true, Status: StatusActive}}
	snapshot := svc.snapshotFromAPIKey(context.Background(), key)
	restored := svc.snapshotToAPIKey("key", snapshot)
	require.NotNil(t, restored.Group)
	require.True(t, restored.Group.IsMaintenance)
	require.True(t, restored.Group.IsActive())

	copy := cloneGroupForDuplicate(key.Group, "operation")
	require.True(t, copy.IsMaintenance)
}
