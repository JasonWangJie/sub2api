package admin

import (
	"context"
	"encoding/csv"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userExportServiceStub struct {
	service.AdminService
	users     []service.User
	filters   []service.UserListFilters
	pages     []int
	sortBy    string
	sortOrder string
	failPage  int
	duplicate bool
}

func (s *userExportServiceStub) ListUsers(_ context.Context, page, pageSize int, filters service.UserListFilters, sortBy, sortOrder string) ([]service.User, int64, error) {
	s.pages = append(s.pages, page)
	s.filters = append(s.filters, filters)
	s.sortBy, s.sortOrder = sortBy, sortOrder
	if page == s.failPage {
		return nil, 0, errors.New("database unavailable")
	}
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(s.users))
	if s.duplicate && page > 1 {
		return s.users[:1], int64(len(s.users)), nil
	}
	return s.users[start:end], int64(len(s.users)), nil
}

func (s *userExportServiceStub) GetAllGroupsIncludingInactive(context.Context) ([]service.Group, error) {
	return []service.Group{{ID: 42, Name: "专属,分组"}}, nil
}

type userExportAttributesStub struct {
	ids [][]int64
	err error
}

func (s *userExportAttributesStub) ListDefinitions(context.Context, bool) ([]service.UserAttributeDefinition, error) {
	return []service.UserAttributeDefinition{{ID: 7, Key: "company", Name: "=公司"}}, nil
}

func (s *userExportAttributesStub) GetBatchUserAttributes(_ context.Context, ids []int64) (map[int64]map[int64]string, error) {
	s.ids = append(s.ids, ids)
	return map[int64]map[int64]string{1: {7: "@SUM(1+1)"}}, s.err
}

func requestUserExport(stub *userExportServiceStub, attributes userExportAttributeReader, query string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	h := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	h.exportAttributes = attributes
	router := gin.New()
	router.GET("/admin/users/export", h.Export)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/users/export"+query, nil))
	return w
}

func TestUserHandlerExportAllMatchingUsers(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 34, 56, 0, time.UTC)
	users := make([]service.User, userExportPageSize+1)
	for i := range users {
		users[i] = service.User{ID: int64(i + 1), Email: "user" + strconv.Itoa(i+1) + "@example.com"}
	}
	users[0] = service.User{
		ID: 1, Email: "user1@example.com", Username: "=HYPERLINK(\"unsafe\")", Notes: "备注,含引号\"\n第二行",
		Balance: -12.3456789, FrozenBalance: 0.12345678, TotalRecharged: 99.87654321,
		Role: "user", Status: "active", Concurrency: 5, RPMLimit: 60, AllowedGroups: []int64{42},
		CreatedAt: now, UpdatedAt: now, LastLoginAt: &now,
		PasswordHash: "must-not-export-password", TotpSecretEncrypted: ptrExportSecret("must-not-export-totp"),
		Subscriptions: []service.UserSubscription{{ID: 3, GroupID: 42, Status: "active", StartsAt: now, ExpiresAt: now.Add(time.Hour)}},
	}
	stub := &userExportServiceStub{users: users}
	attributes := &userExportAttributesStub{}
	query := url.Values{
		"page": {"999"}, "page_size": {"1"}, "search": {"  示例  "}, "status": {"active"}, "role": {"user"},
		"group_name": {" 专属 "}, "has_exclusive_groups": {"true"}, "api_key_group_id": {"42"}, "attr[7]": {"公司"},
		"sort_by": {"balance"}, "sort_order": {"asc"}, "include_subscriptions": {"false"},
	}
	w := requestUserExport(stub, attributes, "?"+query.Encode())
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "text/csv; charset=utf-8", w.Header().Get("Content-Type"))
	require.Contains(t, w.Header().Get("Content-Disposition"), "attachment; filename=\"users_")
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.True(t, strings.HasPrefix(w.Body.String(), "\xef\xbb\xbf"))
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, len(users)+1)
	require.Equal(t, []int{1, 2}, stub.pages)
	require.Len(t, attributes.ids, 2)
	require.Len(t, attributes.ids[0], userExportPageSize)
	require.Equal(t, []int64{1001}, attributes.ids[1])
	for _, filters := range stub.filters {
		require.Equal(t, "示例", filters.Search)
		require.Equal(t, "active", filters.Status)
		require.Equal(t, "user", filters.Role)
		require.Equal(t, "专属", filters.GroupName)
		require.True(t, filters.HasExclusiveGroups)
		require.Equal(t, int64(42), filters.APIKeyGroupID)
		require.Equal(t, map[int64]string{7: "公司"}, filters.Attributes)
		require.NotNil(t, filters.IncludeSubscriptions)
		require.True(t, *filters.IncludeSubscriptions)
	}
	require.Equal(t, "balance", stub.sortBy)
	require.Equal(t, "asc", stub.sortOrder)
	require.Equal(t, "'=公司 (company)", rows[0][27])
	require.Equal(t, "'=HYPERLINK(\"unsafe\")", rows[1][2])
	require.Equal(t, "-12.3456789", rows[1][3])
	require.Equal(t, "0.12345678", rows[1][4])
	require.Equal(t, "99.87654321", rows[1][5])
	require.Equal(t, "专属,分组", rows[1][11])
	require.Contains(t, rows[1][14], `"group_id":42`)
	// encoding/csv normalizes CRLF embedded inside quoted fields to LF.
	require.Equal(t, users[0].Notes, rows[1][15])
	require.Equal(t, "'@SUM(1+1)", rows[1][27])
	require.Equal(t, "1001", rows[len(rows)-1][0])
	require.NotContains(t, w.Body.String(), "must-not-export")
}

func ptrExportSecret(value string) *string { return &value }

func TestUserHandlerExportEmptyStillWritesHeader(t *testing.T) {
	w := requestUserExport(&userExportServiceStub{}, nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "账号（邮箱）", rows[0][1])
}

func TestUserHandlerExportFailsWithoutPartialDownload(t *testing.T) {
	users := make([]service.User, userExportPageSize+1)
	for i := range users {
		users[i].ID = int64(i + 1)
	}
	for _, tc := range []struct {
		name       string
		stub       *userExportServiceStub
		attributes userExportAttributeReader
		status     int
	}{
		{name: "later page error", stub: &userExportServiceStub{users: users, failPage: 2}, status: http.StatusInternalServerError},
		{name: "duplicate after sort change", stub: &userExportServiceStub{users: users, duplicate: true}, status: http.StatusConflict},
		{name: "attribute error", stub: &userExportServiceStub{users: users}, attributes: &userExportAttributesStub{err: errors.New("attributes failed")}, status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := requestUserExport(tc.stub, tc.attributes, "")
			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Header().Get("Content-Type"), "application/json")
			require.Empty(t, w.Header().Get("Content-Disposition"))
			require.NotContains(t, w.Body.String(), "\xef\xbb\xbf")
		})
	}
}

func TestSafeUserCSVText(t *testing.T) {
	for _, value := range []string{"=1+1", "+SUM(1,2)", "-1+2", "@SUM(1)", " \t=1", "\r=1", "\n=1", "\ttext"} {
		require.Equal(t, "'"+value, safeUserCSVText(value))
	}
	for _, value := range []string{"", "用户", "hello,\"world\"", " space", "a@b.com", "12.5"} {
		require.Equal(t, value, safeUserCSVText(value))
	}
}
