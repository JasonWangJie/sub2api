package handler

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ImagePlazaHandler struct {
	svc *service.ImagePlazaService
}

func NewImagePlazaHandler(svc *service.ImagePlazaService) *ImagePlazaHandler {
	return &ImagePlazaHandler{svc: svc}
}

type imagePlazaPublishRequest struct {
	Prompt     string `json:"prompt"`
	Title      string `json:"title"`
	Model      string `json:"model"`
	Size       string `json:"size"`
	Quality    string `json:"quality"`
	Format     string `json:"format"`
	Background string `json:"background"`
	Style      string `json:"style"`
	Image      string `json:"image"` // data URL or base64
}

// List GET /api/v1/image-plaza
func (h *ImagePlazaHandler) List(c *gin.Context) {
	if _, ok := middleware2.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "24"))
	result, err := h.svc.List(c.Request.Context(), service.ImagePlazaListParams{
		Query:    strings.TrimSpace(c.Query("q")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"items":     result.Items,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// Publish POST /api/v1/image-plaza
func (h *ImagePlazaHandler) Publish(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	var req imagePlazaPublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	data, mime, detectedFormat, err := service.DecodeImagePayload(req.Image)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = detectedFormat
	}

	item, err := h.svc.Publish(c.Request.Context(), subject.UserID, service.ImagePlazaPublishInput{
		Prompt:     req.Prompt,
		Title:      req.Title,
		Model:      req.Model,
		Size:       req.Size,
		Quality:    req.Quality,
		Format:     format,
		Background: req.Background,
		Style:      req.Style,
		ImageData:  data,
		MimeType:   mime,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

// Content GET /api/v1/image-plaza/:id/content (public for public items)
func (h *ImagePlazaHandler) Content(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	path, contentType, err := h.svc.OpenContent(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.File(path)
}

// Delete DELETE /api/v1/image-plaza/:id
func (h *ImagePlazaHandler) Delete(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}
