package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupHandlerUpdateMaintenance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, simple := range []bool{false, true} {
		svc := newStubAdminService()
		var r *gin.Engine
		if simple {
			r = newSimpleModeGroupRouter(svc)
		} else {
			r = gin.New()
			h := NewGroupHandler(svc, nil, nil)
			r.PUT("/groups/:id", h.Update)
		}
		for _, payload := range []string{`{"is_maintenance":true}`, `{"is_maintenance":false}`, `{}`} {
			req := httptest.NewRequest(http.MethodPut, "/groups/1", bytes.NewBufferString(payload))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)
			require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		}
		require.Len(t, svc.updatedGroups, 3)
		require.NotNil(t, svc.updatedGroups[0].IsMaintenance)
		require.True(t, *svc.updatedGroups[0].IsMaintenance)
		require.NotNil(t, svc.updatedGroups[1].IsMaintenance)
		require.False(t, *svc.updatedGroups[1].IsMaintenance)
		require.Nil(t, svc.updatedGroups[2].IsMaintenance)
	}
}
