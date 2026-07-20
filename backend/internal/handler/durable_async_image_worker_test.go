package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestApplyCapturedGeminiImageDimensionsUsesActualOutputForBilling(t *testing.T) {
	requested := "0.5K"
	result := &service.ForwardResult{ImageCount: 1, ImageSize: service.ImageBillingSize2K}

	applyCapturedGeminiImageDimensions(result, []asyncImageCapturedOutput{{Width: 512, Height: 512}}, &requested)

	require.Equal(t, service.ImageBillingSize1K, result.ImageSize)
	require.Equal(t, "0.5K", result.ImageInputSize)
	require.Equal(t, "512x512", result.ImageOutputSize)
	require.Equal(t, service.ImageSizeSourceOutput, result.ImageSizeSource)
	require.Equal(t, map[string]int{service.ImageBillingSize1K: 1}, result.ImageSizeBreakdown)
}

func TestApplyCapturedOpenAIImageDimensionsUsesLargestActualTier(t *testing.T) {
	requested := "1024x1024"
	result := &service.OpenAIForwardResult{ImageCount: 2, ImageSize: service.ImageBillingSize1K}

	applyCapturedOpenAIImageDimensions(result, []asyncImageCapturedOutput{
		{Width: 1024, Height: 1024},
		{Width: 3840, Height: 2160},
	}, &requested)

	require.Equal(t, service.ImageBillingSize4K, result.ImageSize)
	require.Equal(t, "1024x1024", result.ImageInputSize)
	require.Equal(t, "1024x1024", result.ImageOutputSize)
	require.Equal(t, service.ImageSizeSourceOutput, result.ImageSizeSource)
	require.Equal(t, map[string]int{
		service.ImageBillingSize1K: 1,
		service.ImageBillingSize4K: 1,
	}, result.ImageSizeBreakdown)
}
