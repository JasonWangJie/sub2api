package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMapperExposesMaintenanceToUsersAndAdmins(t *testing.T) {
	group := &service.Group{ID: 1, Name: "Standard", IsMaintenance: true, Status: service.StatusActive}
	for _, mapped := range []any{GroupFromService(group), GroupFromServiceAdmin(group)} {
		encoded, err := json.Marshal(mapped)
		require.NoError(t, err)
		require.Contains(t, string(encoded), `"is_maintenance":true`)
		require.Contains(t, string(encoded), `"status":"active"`)
	}
	group.IsMaintenance = false
	encoded, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"is_maintenance":false`)
}
