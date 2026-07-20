package service

import (
	"context"
	"encoding/base64"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

const asyncImageOnePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func TestParseBBGeminiImageRequest(t *testing.T) {
	body := []byte(`{
        "model":"gemini-3-pro-image-preview",
        "stream":false,
        "messages":[{"role":"user","content":[
          {"type":"image_url","image_url":{"url":"https://images.example/ref.png"}},
          {"type":"text","text":"paint a quiet harbor"}
        ]}],
        "extra_body":{"google":{"image_config":{"image_size":"4k","aspect_ratio":"16:9"}}}
      }`)

	req, err := ParseBBGeminiImageRequest(body, "/v1/chat/completions_gm")
	require.NoError(t, err)
	require.Equal(t, PlatformGemini, req.Platform)
	require.Equal(t, AsyncImageKindEdit, req.Kind)
	require.Equal(t, "4K", req.ImageSize)
	require.Equal(t, "16:9", req.AspectRatio)
	require.Equal(t, "paint a quiet harbor", req.Prompt)
	require.Equal(t, 1, req.ReferenceCount())
}

func TestParseBBGeminiImageRequestRejectsStreamingAndUnsupportedRole(t *testing.T) {
	_, err := ParseBBGeminiImageRequest([]byte(`{"model":"gemini-image","stream":true,"messages":[{"role":"user","content":"x"}]}`), "")
	require.ErrorContains(t, err, "stream must be false")

	_, err = ParseBBGeminiImageRequest([]byte(`{"model":"gemini-image","messages":[{"role":"system","content":"x"}]}`), "")
	require.ErrorContains(t, err, "unsupported message role")
}

func TestParseSCGeminiImageRequestDimensions(t *testing.T) {
	req, err := ParseSCGeminiImageRequest([]byte(`{
        "model":"nano-banana-2","prompt":"modern living room",
        "image_urls":["https://images.example/ref.png"],
        "resolution":"2K","aspect_ratio":"auto"
      }`), "/v1/images/generations_sc")
	require.NoError(t, err)
	require.Equal(t, AsyncImageKindEdit, req.Kind)
	require.Equal(t, "2K", req.ImageSize)
	require.Empty(t, req.AspectRatio, "auto is represented by omitting the upstream ratio")

	halfK, err := ParseSCGeminiImageRequest([]byte(`{"model":"m","prompt":"p","resolution":"0.5K"}`), "")
	require.NoError(t, err)
	require.Equal(t, "0.5K", halfK.ImageSize)

	_, err = ParseSCGeminiImageRequest([]byte(`{"model":"m","prompt":"p","aspect_ratio":"auto"}`), "")
	require.ErrorContains(t, err, "requires at least one reference image")
}

func TestAsyncImageReferenceDownloaderDataURI(t *testing.T) {
	downloader := AsyncImageReferenceDownloader{MaxBytes: 1 << 20, MaxPixels: 100}
	ref, err := downloader.Download(context.Background(), "data:image/png;base64,"+asyncImageOnePixelPNG)
	require.NoError(t, err)
	require.Equal(t, "image/png", ref.MIMEType)
	require.Equal(t, 1, ref.Width)
	require.Equal(t, 1, ref.Height)
	require.NotEmpty(t, ref.SHA256)
	require.Equal(t, asyncImageOnePixelPNG, base64.StdEncoding.EncodeToString(ref.Data))
}

func TestAsyncImagePublicIPPolicy(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "::1", "fc00::1"}
	for _, raw := range blocked {
		require.False(t, isAsyncImagePublicIP(netip.MustParseAddr(raw)), raw)
	}
	require.True(t, isAsyncImagePublicIP(netip.MustParseAddr("1.1.1.1")))
	require.True(t, isAsyncImagePublicIP(netip.MustParseAddr("2606:4700:4700::1111")))
}

func TestBuildGeminiAsyncChatBodyWithDataURI(t *testing.T) {
	req := &AsyncImageNormalizedRequest{
		Model:       "gemini-image",
		ImageSize:   "2K",
		AspectRatio: "1:1",
		Parts: []AsyncImageInputPart{
			{Type: "image_url", URL: "data:image/png;base64," + asyncImageOnePixelPNG},
			{Type: "text", Text: "restyle"},
		},
	}
	body, err := BuildGeminiAsyncChatBody(context.Background(), req, AsyncImageReferenceDownloader{})
	require.NoError(t, err)
	require.JSONEq(t, `{
      "model":"gemini-image","stream":false,
      "messages":[{"role":"user","content":[
        {"type":"image_url","image_url":{"url":"data:image/png;base64,`+asyncImageOnePixelPNG+`"}},
        {"type":"text","text":"restyle"}
      ]}],
      "extra_body":{"google":{"image_config":{"image_size":"2K","aspect_ratio":"1:1"}}}
    }`, string(body))
}

func TestAsyncImageTaskRequestHashIncludesDialectAndEndpoint(t *testing.T) {
	body := []byte(`{"model":"m"}`)
	a := AsyncImageTaskRequestHash(PlatformGemini, AsyncImageDialectBB, "/a", body)
	b := AsyncImageTaskRequestHash(PlatformGemini, AsyncImageDialectSC, "/a", body)
	c := AsyncImageTaskRequestHash(PlatformGemini, AsyncImageDialectBB, "/b", body)
	require.NotEqual(t, a, b)
	require.NotEqual(t, a, c)
	require.Equal(t, a, AsyncImageTaskRequestHash(PlatformGemini, AsyncImageDialectBB, "/a", body))
}
