package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminServiceWithTotalBalance struct {
	*stubAdminService
	totalBalance float64
	err          error
}

func (s *adminServiceWithTotalBalance) GetTotalUserBalance(context.Context) (float64, error) {
	return s.totalBalance, s.err
}

func TestUserHandlerListIncludesTotalNonAdminBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serviceStub := &adminServiceWithTotalBalance{
		stubAdminService: &stubAdminService{},
		totalBalance:     12.345,
	}
	handler := NewUserHandler(serviceStub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/users", handler.List)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Data struct {
			Items        []any   `json:"items"`
			Total        int64   `json:"total"`
			Page         int     `json:"page"`
			PageSize     int     `json:"page_size"`
			Pages        int     `json:"pages"`
			TotalBalance float64 `json:"total_balance"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Empty(t, body.Data.Items)
	require.Zero(t, body.Data.Total)
	require.Equal(t, 1, body.Data.Page)
	require.Equal(t, 20, body.Data.PageSize)
	require.Equal(t, 1, body.Data.Pages)
	require.InDelta(t, 12.345, body.Data.TotalBalance, 1e-9)
}

func TestUserHandlerListReturnsBalanceSummaryError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serviceStub := &adminServiceWithTotalBalance{
		stubAdminService: &stubAdminService{},
		err:              errors.New("balance summary failed"),
	}
	handler := NewUserHandler(serviceStub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/users", handler.List)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.NotEqual(t, "success", body.Message)
}
