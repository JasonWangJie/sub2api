package admin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const userExportPageSize = 1000 // Matches pagination.PaginationParams.Limit.

type userExportAttributeReader interface {
	ListDefinitions(context.Context, bool) ([]service.UserAttributeDefinition, error)
	GetBatchUserAttributes(context.Context, []int64) (map[int64]map[int64]string, error)
}

// NewUserHandlerWithAttributes wires custom attributes into the user CSV export.
func NewUserHandlerWithAttributes(
	adminService service.AdminService,
	concurrencyService *service.ConcurrencyService,
	userPlatformQuotaRepo service.UserPlatformQuotaRepository,
	billingCache service.BillingCache,
	totpService *service.TotpService,
	userService *service.UserService,
	settingService *service.SettingService,
	attributeService *service.UserAttributeService,
) *UserHandler {
	h := NewUserHandler(adminService, concurrencyService, userPlatformQuotaRepo, billingCache, totpService, userService, settingService)
	h.exportAttributes = attributeService
	return h
}

// Export downloads all users matching the list filters, independent of pagination.
// GET /api/v1/admin/users/export
func (h *UserHandler) Export(c *gin.Context) {
	ctx := c.Request.Context()
	filters, sortBy, sortOrder := parseUserListQuery(c)
	// Subscriptions are exported even when the corresponding list column is hidden.
	includeSubscriptions := true
	filters.IncludeSubscriptions = &includeSubscriptions

	groups, err := h.adminService.GetAllGroupsIncludingInactive(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groupNames := make(map[int64]string, len(groups))
	for _, group := range groups {
		groupNames[group.ID] = group.Name
	}
	var definitions []service.UserAttributeDefinition
	if h.exportAttributes != nil {
		definitions, err = h.exportAttributes.ListDefinitions(ctx, false)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	// Finish writing before sending HTTP headers. A later-page database failure
	// must return an error, rather than a successful but incomplete download.
	file, err := os.CreateTemp("", "sub2api-users-*.csv")
	if err != nil {
		response.InternalError(c, "Failed to prepare user export")
		return
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()
	if _, err = file.WriteString("\xef\xbb\xbf"); err != nil {
		response.InternalError(c, "Failed to write user export")
		return
	}
	writer := csv.NewWriter(file)
	writer.UseCRLF = true
	header := []string{
		"用户ID", "账号（邮箱）", "用户名", "余额（USD）", "冻结余额（USD）", "累计充值（USD）",
		"角色", "状态", "并发数", "RPM限制", "授权分组ID", "授权分组名称", "限制公开分组",
		"专属分组倍率", "订阅信息", "备注", "注册来源", "最后登录时间", "最后活跃时间", "最后调用时间",
		"创建时间", "更新时间", "双因素认证", "余额通知启用", "余额通知阈值类型", "余额通知阈值", "余额通知附加邮箱",
	}
	for _, definition := range definitions {
		header = append(header, safeUserCSVText(definition.Name+" ("+definition.Key+")"))
	}
	if err = writer.Write(header); err != nil {
		response.InternalError(c, "Failed to write user export")
		return
	}

	seen := make(map[int64]struct{})
	var expectedTotal int64
	for page := 1; ; page++ {
		if err = ctx.Err(); err != nil {
			response.ErrorFrom(c, err)
			return
		}
		users, total, listErr := h.adminService.ListUsers(ctx, page, userExportPageSize, filters, sortBy, sortOrder)
		if listErr != nil {
			response.ErrorFrom(c, listErr)
			return
		}
		if page == 1 {
			expectedTotal = total
		}
		if total != expectedTotal || (len(users) == 0 && int64(len(seen)) < expectedTotal) {
			response.Error(c, http.StatusConflict, "User list changed during export; please retry")
			return
		}
		ids := make([]int64, len(users))
		for i := range users {
			ids[i] = users[i].ID
		}
		var attributes map[int64]map[int64]string
		if len(definitions) > 0 && len(ids) > 0 {
			attributes, err = h.exportAttributes.GetBatchUserAttributes(ctx, ids)
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
		}
		for i := range users {
			user := &users[i]
			if _, exists := seen[user.ID]; exists {
				response.Error(c, http.StatusConflict, "User list changed during export; please retry")
				return
			}
			seen[user.ID] = struct{}{}
			row := userCSVRow(user, groupNames)
			for _, definition := range definitions {
				row = append(row, safeUserCSVText(attributes[user.ID][definition.ID]))
			}
			if err = writer.Write(row); err != nil {
				response.InternalError(c, "Failed to write user export")
				return
			}
		}
		if int64(len(seen)) == expectedTotal {
			break
		}
		if int64(len(seen)) > expectedTotal || len(users) < userExportPageSize {
			response.Error(c, http.StatusConflict, "User list changed during export; please retry")
			return
		}
	}
	writer.Flush()
	if writer.Error() != nil {
		response.InternalError(c, "Failed to write user export")
		return
	}
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		response.InternalError(c, "Failed to read user export")
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		response.InternalError(c, "Failed to read user export")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="users_`+time.Now().UTC().Format("20060102_150405")+`.csv"`)
	c.Header("Cache-Control", "no-store")
	c.DataFromReader(http.StatusOK, size, "text/csv; charset=utf-8", file, nil)
}

// Prefix spreadsheet formula candidates, including leading whitespace/control
// characters, without changing numeric columns such as negative balances.
func safeUserCSVText(value string) string {
	trimmed := strings.TrimLeftFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") {
		return "'" + value
	}
	return value
}

func userCSVTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func userCSVJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func userCSVRow(user *service.User, groupNames map[int64]string) []string {
	ids := make([]string, len(user.AllowedGroups))
	names := make([]string, len(user.AllowedGroups))
	for i, id := range user.AllowedGroups {
		ids[i] = strconv.FormatInt(id, 10)
		names[i] = groupNames[id]
	}
	type subscriptionSummary struct {
		ID              int64   `json:"id"`
		GroupID         int64   `json:"group_id"`
		Group           string  `json:"group"`
		Status          string  `json:"status"`
		StartsAt        string  `json:"starts_at"`
		ExpiresAt       string  `json:"expires_at"`
		DailyUsageUSD   float64 `json:"daily_usage_usd"`
		WeeklyUsageUSD  float64 `json:"weekly_usage_usd"`
		MonthlyUsageUSD float64 `json:"monthly_usage_usd"`
	}
	subscriptions := make([]subscriptionSummary, 0, len(user.Subscriptions))
	for _, subscription := range user.Subscriptions {
		name := groupNames[subscription.GroupID]
		if subscription.Group != nil {
			name = subscription.Group.Name
		}
		subscriptions = append(subscriptions, subscriptionSummary{
			ID: subscription.ID, GroupID: subscription.GroupID, Group: name, Status: subscription.Status,
			StartsAt: userCSVTime(&subscription.StartsAt), ExpiresAt: userCSVTime(&subscription.ExpiresAt),
			DailyUsageUSD: subscription.DailyUsageUSD, WeeklyUsageUSD: subscription.WeeklyUsageUSD, MonthlyUsageUSD: subscription.MonthlyUsageUSD,
		})
	}
	threshold := ""
	if user.BalanceNotifyThreshold != nil {
		threshold = strconv.FormatFloat(*user.BalanceNotifyThreshold, 'f', -1, 64)
	}
	return []string{
		strconv.FormatInt(user.ID, 10), safeUserCSVText(user.Email), safeUserCSVText(user.Username),
		strconv.FormatFloat(user.Balance, 'f', -1, 64), strconv.FormatFloat(user.FrozenBalance, 'f', -1, 64), strconv.FormatFloat(user.TotalRecharged, 'f', -1, 64),
		safeUserCSVText(user.Role), safeUserCSVText(user.Status), strconv.Itoa(user.Concurrency), strconv.Itoa(user.RPMLimit),
		strings.Join(ids, "; "), safeUserCSVText(strings.Join(names, "; ")), strconv.FormatBool(user.RestrictPublicGroups),
		userCSVJSON(user.GroupRates), userCSVJSON(subscriptions), safeUserCSVText(user.Notes), safeUserCSVText(user.SignupSource),
		userCSVTime(user.LastLoginAt), userCSVTime(user.LastActiveAt), userCSVTime(user.LastUsedAt),
		userCSVTime(&user.CreatedAt), userCSVTime(&user.UpdatedAt), strconv.FormatBool(user.TotpEnabled), strconv.FormatBool(user.BalanceNotifyEnabled),
		safeUserCSVText(user.BalanceNotifyThresholdType), threshold, userCSVJSON(user.BalanceNotifyExtraEmails),
	}
}
