package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolSchemaInputProbePreservesMessageAndToolSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		item string
		want string
	}{
		{
			name: "message with schema-shaped user data",
			item: `{"type":"message","content":{"parameters":{"type":null},"pattern":"(?=example)"}}`,
		},
		{
			name: "escaped message type at end",
			item: `{"parameters":{"type":null},"ty\u0070e":" message "}`,
		},
		{
			name: "last string type selects message",
			item: `{"type":"function","parameters":{"type":null},"type":"message"}`,
		},
		{
			name: "last string type selects tool",
			item: `{"type":"message","parameters":{"type":null},"type":"function"}`,
			want: `{"type":"message","parameters":{"type":"object"},"type":"function"}`,
		},
		{
			name: "non-string type preserves prior probe behavior",
			item: `{"type":"function","parameters":{"type":null},"type":null}`,
			want: `{"type":"function","parameters":{"type":"object"},"type":null}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"input":[` + tc.item + `],"tools":[{"parameters":{"type":null}}]}`)
			got, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)
			require.NoError(t, err)
			require.True(t, changed)
			wantItem := tc.want
			if wantItem == "" {
				wantItem = tc.item
			}
			require.Equal(t, `{"input":[`+wantItem+`],"tools":[{"parameters":{"type":"object"}}]}`, string(got))
		})
	}
}

func TestToolSchemaInputProbeStillRejectsMalformedMessage(t *testing.T) {
	for _, item := range []string{
		`{"type":"message","content":"bad\q"}`,
		`{"type":"message","content":[1,]}`,
		`{"type":"message","metadata":{"x":01}}`,
	} {
		_, _, err := sanitizeOpenAIResponsesToolParameterTypes([]byte(`{"input":[` + item + `]}`))
		require.Error(t, err, item)
	}
}

func BenchmarkToolSchemaLargeMessage(b *testing.B) {
	for _, fixture := range []struct {
		name string
		size int
	}{
		{"1MiB", 1 << 20},
		{"8MiB", 8 << 20},
	} {
		body := []byte(`{"model":"test","input":[{"type":"message","role":"user","content":"` + strings.Repeat("x", fixture.size) + `"}],"tools":[{"type":"function","name":"test","parameters":{"type":"object"}}]}`)
		b.Run(fixture.name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				got, changed, err := sanitizeOpenAIResponsesToolSchemasForPlatform(body, PlatformOpenAI)
				if err != nil || changed || len(got) != len(body) {
					b.Fatalf("valid message changed: changed=%v err=%v", changed, err)
				}
			}
		})
	}
}
