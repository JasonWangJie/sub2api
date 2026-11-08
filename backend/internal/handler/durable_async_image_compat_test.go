//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type durableAsyncSubmitCompatibilityRepo struct {
	service.AsyncImageTaskRepository
	created int
}

func (r *durableAsyncSubmitCompatibilityRepo) CreateAsyncImageTask(_ context.Context, p service.CreateAsyncImageTaskParams) (*service.AsyncImageTask, bool, error) {
	r.created++
	return &service.AsyncImageTask{TaskID: p.TaskID, Platform: p.Platform}, false, nil
}

func newDurableAsyncSubmitCompatibilityRouter(t *testing.T, maxBodyBytes, budgetBytes int64) (*gin.Engine, *durableAsyncSubmitCompatibilityRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &durableAsyncSubmitCompatibilityRepo{}
	storage := service.NewImageStorageSettingService(nil, nil, nil,
		func(context.Context, *config.ImageStorageConfig) (service.ImageStorage, error) {
			return &noopImageStorage{}, nil
		},
		config.ImageStorageConfig{
			Enabled: true, Provider: service.ImageStorageProviderCustomS3,
			Endpoint: "https://storage.example.test", Region: "auto", Bucket: "images",
			AccessKeyID: "ak", SecretAccessKey: "sk", Prefix: "images/",
		}, config.AsyncImageConfig{})
	h := &DurableAsyncImageHandler{
		tasks: service.NewAsyncImageTaskService(repo), storage: storage, encryptor: passthroughEncryptor{},
		openAI: &OpenAIGatewayHandler{gatewayService: &service.OpenAIGatewayService{}},
	}
	router := gin.New()
	router.Use(middleware.RequestBodyLimit(maxBodyBytes, pkghttputil.NewBodyMemoryBudget(budgetBytes)))
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
			ID: 9, UserID: 7, User: &service.User{ID: 7}, GroupID: &groupID,
			Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI,
				AllowImageGeneration: true, AllowAsyncImageGeneration: true},
		})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Next()
	})
	router.POST("/v1/images/generations_async", h.SubmitOpenAIBB)
	return router, repo
}

func TestDurableAsyncSubmitRejectsUnsupportedNativeOutputsBeforeTaskCreation(t *testing.T) {
	for _, fields := range []map[string]string{
		{"output_format": "gif"},
		{"output_format": " GIF "},
		{"output_format": "svg"},
		{"output_format": " SvG "},
		{"svg": `{}`},
		{"frame_count": "1"},
		{"frame_duration_ms": "0"},
	} {
		for _, multipartRequest := range []bool{false, true} {
			encodedFields, err := json.Marshal(fields)
			require.NoError(t, err)
			name := string(encodedFields)
			if multipartRequest {
				name += "/multipart"
			}
			t.Run(name, func(t *testing.T) {
				router, repo := newDurableAsyncSubmitCompatibilityRouter(t, 1<<20, 0)
				var body bytes.Buffer
				contentType := "application/json"
				if multipartRequest {
					writer := multipart.NewWriter(&body)
					require.NoError(t, writer.WriteField("model", "gpt-image-2.6"))
					require.NoError(t, writer.WriteField("prompt", "draw a cat"))
					for key, value := range fields {
						require.NoError(t, writer.WriteField(key, value))
					}
					require.NoError(t, writer.Close())
					contentType = writer.FormDataContentType()
				} else {
					payload := map[string]any{"model": "gpt-image-2.6", "prompt": "draw a cat"}
					for key, value := range fields {
						if key == "output_format" {
							payload[key] = value
						} else {
							payload[key] = json.RawMessage(value)
						}
					}
					require.NoError(t, json.NewEncoder(&body).Encode(payload))
				}
				req := httptest.NewRequest(http.MethodPost, "/v1/images/generations_async", &body)
				req.Header.Set("Content-Type", contentType)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), "supports only PNG, JPEG, and WebP outputs")
				require.Zero(t, repo.created, "unsupported output must not create a billable upstream task")
			})
		}
	}
}

func TestDurableAsyncSubmitKeepsSupportedRasterOutputs(t *testing.T) {
	for _, format := range []string{"", "png", "jpeg", "webp"} {
		t.Run("format_"+format, func(t *testing.T) {
			router, repo := newDurableAsyncSubmitCompatibilityRouter(t, 1<<20, 0)
			payload, err := json.Marshal(map[string]any{"model": "gpt-image-2.6", "prompt": "draw a cat", "output_format": format})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations_async", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
			require.Equal(t, 1, repo.created)
		})
	}
}

func TestDurableAsyncSubmitClassifiesBodyReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		budget     int64
		maxBody    int64
		readErr    error
		cancelled  bool
		wantStatus int
	}{
		{name: "memory_overload", budget: 1, maxBody: 1 << 20, wantStatus: http.StatusServiceUnavailable},
		{name: "body_limit", maxBody: 4, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "timeout", maxBody: 1 << 20, readErr: &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}, cancelled: true, wantStatus: http.StatusRequestTimeout},
		{name: "client_gone", maxBody: 1 << 20, readErr: context.Canceled, cancelled: true, wantStatus: pkghttputil.StatusClientClosedRequest},
		{name: "truncated", maxBody: 1 << 20, readErr: io.ErrUnexpectedEOF, wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, repo := newDurableAsyncSubmitCompatibilityRouter(t, tc.maxBody, tc.budget)
			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations_async", strings.NewReader(`{"model":"gpt-image-2.6","prompt":"draw a cat"}`))
			req.Header.Set("Content-Type", "application/json")
			if tc.readErr != nil {
				req.Body = io.NopCloser(iotest.ErrReader(tc.readErr))
			}
			if tc.cancelled {
				ctx, cancel := context.WithCancel(req.Context())
				defer cancel()
				req = req.WithContext(ctx)
				req.Body = io.NopCloser(cancelOnReadErr{err: tc.readErr, cancel: cancel})
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			require.Zero(t, repo.created)
			if tc.wantStatus == http.StatusServiceUnavailable {
				require.Equal(t, "1", recorder.Header().Get("Retry-After"))
			}
		})
	}
}
