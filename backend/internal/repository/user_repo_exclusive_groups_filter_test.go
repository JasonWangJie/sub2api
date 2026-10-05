package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserRepositoryListWithFiltersExclusiveGroups(t *testing.T) {
	repo, client := newUserEntRepo(t)
	ctx := context.Background()
	groups := make(map[string]*dbent.Group)
	for _, spec := range []struct {
		name      string
		exclusive bool
		status    string
		typeName  string
		deleted   bool
	}{
		{"exclusive", true, service.StatusActive, service.SubscriptionTypeStandard, false},
		{"second", true, service.StatusActive, service.SubscriptionTypeStandard, false},
		{"public", false, service.StatusActive, service.SubscriptionTypeStandard, false},
		{"disabled", true, service.StatusDisabled, service.SubscriptionTypeStandard, false},
		{"subscription", true, service.StatusActive, service.SubscriptionTypeSubscription, false},
		{"deleted", true, service.StatusActive, service.SubscriptionTypeStandard, true},
	} {
		create := client.Group.Create().SetName(spec.name).SetIsExclusive(spec.exclusive).
			SetStatus(spec.status).SetSubscriptionType(spec.typeName)
		if spec.deleted {
			create.SetDeletedAt(time.Now())
		}
		g, err := create.Save(ctx)
		require.NoError(t, err)
		groups[spec.name] = g
	}

	users := make(map[string]*dbent.User)
	for _, spec := range []struct {
		name    string
		groups  []string
		deleted bool
	}{
		{"first", []string{"exclusive", "second"}, false},
		{"second", []string{"exclusive"}, false},
		{"public-only", []string{"public"}, false},
		{"disabled-only", []string{"disabled"}, false},
		{"subscription-only", []string{"subscription"}, false},
		{"deleted-group-only", []string{"deleted"}, false},
		{"unassigned", nil, false},
		{"deleted-user", []string{"exclusive"}, true},
	} {
		create := client.User.Create().SetEmail(spec.name + "@example.com").SetPasswordHash("hash").
			SetRestrictPublicGroups(spec.name == "public-only")
		for _, name := range spec.groups {
			create.AddAllowedGroupIDs(groups[name].ID)
		}
		if spec.deleted {
			create.SetDeletedAt(time.Now())
		}
		u, err := create.Save(ctx)
		require.NoError(t, err)
		users[spec.name] = u
	}

	// Multiple exclusive assignments must not duplicate a user or inflate totals.
	filters := service.UserListFilters{HasExclusiveGroups: true}
	params := pagination.PaginationParams{Page: 1, PageSize: 1, SortBy: "id", SortOrder: "asc"}
	for i, name := range []string{"first", "second"} {
		params.Page = i + 1
		items, page, err := repo.ListWithFilters(ctx, params, filters)
		require.NoError(t, err)
		require.Equal(t, int64(2), page.Total)
		require.Len(t, items, 1)
		require.Equal(t, users[name].ID, items[0].ID)
	}
	params.Page = 3
	items, page, err := repo.ListWithFilters(ctx, params, filters)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, int64(2), page.Total)

	// Turning the filter off restores users with public or unusable assignments.
	params.Page, params.PageSize = 1, 20
	items, page, err = repo.ListWithFilters(ctx, params, service.UserListFilters{})
	require.NoError(t, err)
	require.Len(t, items, 7)
	require.Equal(t, int64(7), page.Total)

	// Existing filters remain an intersection with the exclusive assignments.
	filters.Search = "second@example.com"
	filters.GroupName = "exclusive"
	filters.Status = service.StatusActive
	items, page, err = repo.ListWithFilters(ctx, params, filters)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, users["second"].ID, items[0].ID)
	require.Equal(t, int64(1), page.Total)

	filters.Status = service.StatusDisabled
	items, page, err = repo.ListWithFilters(ctx, params, filters)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, page.Total)
}
