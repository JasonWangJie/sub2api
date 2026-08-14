package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageLibraryAdminHandlerRepo struct {
	service.ImageLibraryRepository
	listParams  service.ImagePublicationListParams
	listItems   []service.AdminImagePlazaPublication
	transitions []string
}

func (r *imageLibraryAdminHandlerRepo) ListPublicationsAdmin(_ context.Context, in service.ImagePublicationListParams) ([]service.AdminImagePlazaPublication, error) {
	r.listParams = in
	return r.listItems, nil
}

func (r *imageLibraryAdminHandlerRepo) GetPublicationObjectAdmin(context.Context, string) (*service.ObjectRef, error) {
	return nil, service.ErrImagePublicationNotFound
}

func (r *imageLibraryAdminHandlerRepo) TransitionPublication(_ context.Context, adminID int64, publicationID, action, reason string, _ time.Time) (*service.ImagePublication, error) {
	r.transitions = append(r.transitions, publicationID+":"+action+":"+reason)
	return &service.ImagePublication{PublicID: publicationID, Status: service.ImagePublicationPublished, UserID: adminID}, nil
}

func imageLibraryHandlerService(repo service.ImageLibraryRepository) *service.ImageLibraryService {
	settings := service.NewImageStorageSettingService(nil, nil, nil, nil, config.ImageStorageConfig{})
	durable := service.NewImageDurableStorageService(
		config.ImageDurableStorageConfig{Backend: config.ImageDurableBackendLocal},
		"./data",
		nil,
		settings,
	)
	return service.NewImageLibraryService(repo, settings, durable)
}

func TestWriteImageObjectAccessNegotiatesJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/image-library/img_test/view", nil)
	ctx.Request.Header.Set("Accept", "application/json")
	expiresAt := time.Date(2026, time.July, 21, 12, 30, 0, 0, time.UTC)

	writeImageObjectAccess(ctx, service.ObjectAccess{
		URL:       "https://storage.example.test/library/result.webp?signature=redacted",
		ExpiresAt: expiresAt,
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	var payload struct {
		Code int `json:"code"`
		Data struct {
			URL       string    `json:"url"`
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, 0, payload.Code)
	require.Equal(t, "https://storage.example.test/library/result.webp?signature=redacted", payload.Data.URL)
	require.True(t, expiresAt.Equal(payload.Data.ExpiresAt))
}

func TestWriteImageObjectAccessRedirectsByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/image-library/img_test/view", nil)

	writeImageObjectAccess(ctx, service.ObjectAccess{URL: "https://storage.example.test/library/result.png"})

	require.Equal(t, http.StatusTemporaryRedirect, recorder.Code)
	require.Equal(t, "https://storage.example.test/library/result.png", recorder.Header().Get("Location"))
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "no-referrer", recorder.Header().Get("Referrer-Policy"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
}

func TestPublishedPlazaObjectAllowsBrowserCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("stream", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/image-plaza/imgpub_test/content", nil)

		writePublishedPlazaObjectStream(ctx, bytes.NewReader([]byte("webp")), "image/webp")

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "image/webp", recorder.Header().Get("Content-Type"))
		require.Equal(t, "private, max-age=3600", recorder.Header().Get("Cache-Control"))
		require.Equal(t, "webp", recorder.Body.String())
	})

	t.Run("redirect within signed url lifetime", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/image-plaza/imgpub_test/content", nil)
		expiresAt := time.Now().Add(30 * time.Minute)

		redirectToPublishedPlazaObject(ctx, service.ObjectAccess{
			URL:       "https://storage.example.test/plaza/result.png?sig=1",
			ExpiresAt: expiresAt,
		})

		require.Equal(t, http.StatusTemporaryRedirect, recorder.Code)
		require.Equal(t, "https://storage.example.test/plaza/result.png?sig=1", recorder.Header().Get("Location"))
		cacheControl := recorder.Header().Get("Cache-Control")
		require.Regexp(t, `^private, max-age=\d+$`, cacheControl)
		var maxAge int
		_, err := fmt.Sscanf(cacheControl, "private, max-age=%d", &maxAge)
		require.NoError(t, err)
		require.Greater(t, maxAge, 1700)
		require.LessOrEqual(t, maxAge, 1740)
	})

	t.Run("redirect near expiry stays no-store", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/image-plaza/imgpub_test/content", nil)

		redirectToPublishedPlazaObject(ctx, service.ObjectAccess{
			URL:       "https://storage.example.test/plaza/result.png?sig=2",
			ExpiresAt: time.Now().Add(30 * time.Second),
		})

		require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	})
}

func TestAdminListPublicationsUsesFilterAndCreatedAtCursorContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	firstCreated := time.Date(2026, time.July, 20, 10, 0, 0, 0, time.UTC)
	lastCreated := firstCreated.Add(time.Hour)
	repo := &imageLibraryAdminHandlerRepo{listItems: []service.AdminImagePlazaPublication{
		{PublicImagePlazaItem: service.PublicImagePlazaItem{PublicationPK: 11, PublicationID: "imgpub_first", PublishedAt: firstCreated.Add(24 * time.Hour)}, CreatedAt: firstCreated},
		{PublicImagePlazaItem: service.PublicImagePlazaItem{PublicationPK: 12, PublicationID: "imgpub_last", PublishedAt: lastCreated.Add(24 * time.Hour)}, CreatedAt: lastCreated},
	}}
	h := NewImageLibraryHandler(imageLibraryHandlerService(repo), nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/image-plaza/publications?limit=2&platform=gemini&model=gemini-image&aspect_ratio=16%3A9&sort=oldest&q=city", nil)

	h.AdminListPublications(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "gemini", repo.listParams.Platform)
	require.Equal(t, "gemini-image", repo.listParams.Model)
	require.Equal(t, "16:9", repo.listParams.AspectRatio)
	require.Equal(t, "oldest", repo.listParams.Sort)
	require.Equal(t, "city", repo.listParams.Query)
	var payload struct {
		Data struct {
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, encodeImageCursor(lastCreated, 12), payload.Data.NextCursor)
	require.NotEqual(t, encodeImageCursor(lastCreated.Add(24*time.Hour), 12), payload.Data.NextCursor)
}

func TestAdminBatchTransitionPublicationsReturnsPerItemResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &imageLibraryAdminHandlerRepo{}
	h := NewImageLibraryHandler(imageLibraryHandlerService(repo), nil)
	body := []byte(`{"publication_ids":["imgpub_valid","invalid"],"action":"approve","reason":"reviewed"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/image-plaza/publications/batch", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})

	h.AdminBatchTransitionPublications(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []string{"imgpub_valid:approve:reviewed"}, repo.transitions)
	var payload struct {
		Data struct {
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
			Items     []struct {
				PublicationID string `json:"publication_id"`
				Success       bool   `json:"success"`
				Error         *struct {
					Reason string `json:"reason"`
				} `json:"error"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, 1, payload.Data.Succeeded)
	require.Equal(t, 1, payload.Data.Failed)
	require.Len(t, payload.Data.Items, 2)
	require.True(t, payload.Data.Items[0].Success)
	require.False(t, payload.Data.Items[1].Success)
	require.Equal(t, "INVALID_PUBLICATION_ID", payload.Data.Items[1].Error.Reason)
}
