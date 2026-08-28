package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type InvoiceHandler struct {
	service *service.InvoiceService
}

func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{service: invoiceService}
}

type invoiceApplicationRequest struct {
	Email       string                     `json:"email" binding:"required"`
	TaxNumber   string                     `json:"tax_number" binding:"required"`
	CompanyName string                     `json:"company_name" binding:"required"`
	Sources     []service.InvoiceSourceRef `json:"sources" binding:"required"`
}

func (h *InvoiceHandler) GetProfile(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	profile, err := h.service.GetProfile(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

func (h *InvoiceHandler) ListRecords(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	records, total, err := h.service.ListUserRecords(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, records, total, page, pageSize)
}

func (h *InvoiceHandler) CreateApplication(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	var req invoiceApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	payload := service.CreateInvoiceApplicationInput{
		InvoiceProfileData: service.InvoiceProfileData{Email: req.Email, TaxNumber: req.TaxNumber, CompanyName: req.CompanyName},
		Sources:            req.Sources,
	}
	executeUserIdempotentJSON(c, "user.invoices.create", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.service.CreateApplication(ctx, subject.UserID, payload)
	})
}

func (h *InvoiceHandler) ListApplications(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	applications, total, err := h.service.ListApplicationsForUser(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, applications, total, page, pageSize)
}
