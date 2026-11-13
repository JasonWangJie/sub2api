package service

import (
	"bytes"
	"testing"
)

// Exercises the metadata/accounting consumers for an approximately 1 MiB
// text stream. This is a component benchmark, not an end-to-end HTTP result.
func BenchmarkOpenAISSEPayloadLargeTextStream(b *testing.B) {
	delta := append([]byte(`{"type":"response.output_text.delta","delta":"`), bytes.Repeat([]byte("x"), 1024)...)
	delta = append(delta, []byte(`"}`)...)
	terminal := []byte(`{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"finished"}]}]}}`)
	b.ReportAllocs()
	b.SetBytes(int64(1024*len(delta) + len(terminal)))
	for i := 0; i < b.N; i++ {
		images := newOpenAIImageOutputCounter()
		doneItems := newResponsesStreamOutputItems()
		searchSeen := make(map[string]struct{})
		imageSeen := make(map[string]struct{})
		for frame := 0; frame <= 1024; frame++ {
			data := delta
			if frame == 1024 {
				data = terminal
			}
			payload := newOpenAISSEPayload(data)
			if normalized, changed := normalizeCompletedImageGenerationStatusPayload(&payload); changed {
				payload = newOpenAISSEPayload(normalized)
			}
			images.addSSEPayload(&payload)
			countGrokNativeSearchCallsInSSEPayloadDedup(&payload, searchSeen)
			extractImageGenerationOutputFromSSEPayload(&payload, imageSeen)
			doneItems.observePayload(&payload)
		}
	}
}
