package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type InvoiceHandler struct {
	service *service.InvoiceService
}

func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{service: invoiceService}
}

func (h *InvoiceHandler) ListApplications(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	applications, total, err := h.service.ListApplications(c.Request.Context(), service.InvoiceApplicationListParams{
		Page: page, PageSize: pageSize, Status: c.Query("status"), Keyword: c.Query("keyword"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, applications, total, page, pageSize)
}

func (h *InvoiceHandler) ListRecords(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	var userID int64
	if raw := c.Query("user_id"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "Invalid user ID")
			return
		}
		userID = parsed
	}
	records, total, err := h.service.ListHistoricalRecords(c.Request.Context(), service.InvoiceRecordListParams{
		Page: page, PageSize: pageSize, UserID: userID,
		Keyword: c.Query("keyword"), InvoiceStatus: c.Query("invoice_status"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, records, total, page, pageSize)
}

func (h *InvoiceHandler) MarkHistoricalRecords(c *gin.Context) {
	adminID, ok := adminActorID(c)
	if !ok {
		return
	}
	var req struct {
		Sources []service.InvoiceSourceRef `json:"sources" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	executeAdminIdempotentJSON(c, "admin.invoices.historical_marks", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		count, err := h.service.MarkHistoricalRecords(ctx, adminID, req.Sources)
		if err != nil {
			return nil, err
		}
		return gin.H{"marked_count": count, "status": service.InvoiceRecordStatusHistorical}, nil
	})
}

func (h *InvoiceHandler) GetApplication(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid invoice application ID")
		return
	}
	application, err := h.service.GetApplication(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, application)
}

func (h *InvoiceHandler) CompleteApplication(c *gin.Context) {
	adminID, ok := adminActorID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid invoice application ID")
		return
	}
	executeAdminIdempotentJSON(c, "admin.invoices.complete", map[string]any{"id": id}, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		if err := h.service.CompleteApplication(ctx, id, adminID); err != nil {
			return nil, err
		}
		return gin.H{"status": service.InvoiceApplicationStatusCompleted}, nil
	})
}

func (h *InvoiceHandler) RejectApplication(c *gin.Context) {
	adminID, ok := adminActorID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid invoice application ID")
		return
	}
	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	payload := map[string]any{"id": id, "reason": req.Reason}
	executeAdminIdempotentJSON(c, "admin.invoices.reject", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		if err := h.service.RejectApplication(ctx, id, adminID, req.Reason); err != nil {
			return nil, err
		}
		return gin.H{"status": service.InvoiceApplicationStatusRejected}, nil
	})
}

func adminActorID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Administrator not authenticated")
		return 0, false
	}
	return subject.UserID, true
}
