package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// openAISSEPayload shares dispatch and validation across consumers of one
// immutable SSE JSON payload. Recreate it whenever a transform replaces data.
// The payload type is deliberately separate from the SSE event header: legacy
// accounting helpers inspect only the JSON type, whereas stream control also
// accepts an event-header fallback.
type openAISSEPayload struct {
	data      []byte
	rawType   string
	eventType string
	validated bool
	valid     bool
}

func newOpenAISSEPayload(data []byte) openAISSEPayload {
	rawType := gjson.GetBytes(data, "type").String()
	return openAISSEPayload{data: data, rawType: rawType, eventType: strings.TrimSpace(rawType)}
}

func (p *openAISSEPayload) effectiveEventType(headerType string) string {
	if p.eventType != "" {
		return p.eventType
	}
	return strings.TrimSpace(headerType)
}

func (p *openAISSEPayload) validJSON() bool {
	if !p.validated {
		p.valid = len(p.data) > 0 && gjson.ValidBytes(p.data)
		p.validated = true
	}
	return p.valid
}
