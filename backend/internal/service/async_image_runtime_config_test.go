package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAsyncImageGeminiModelSupportsHalfK(t *testing.T) {
	cfg := AsyncImageRuntimeConfig{GeminiHalfKModels: []string{"nano-banana-2", "gemini-image-*"}}
	normalizeAsyncImageRuntimeConfig(&cfg)
	require.True(t, AsyncImageGeminiModelSupportsHalfK(cfg, "nano-banana-2"))
	require.True(t, AsyncImageGeminiModelSupportsHalfK(cfg, "gemini-image-preview"))
	require.False(t, AsyncImageGeminiModelSupportsHalfK(cfg, "other-model"))
}
