package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexBootstrapPrecheckScopesNamesToOutputItems(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "tool declarations and unrelated output",
			body: `{"tools":[{"type":"function","name":"automation_update"},{"type":"function","name":"create_thread"},{"type":"function","name":"codex_apps_open_page"}],"input":[{"type":"function_call_output","name":"read_file","output":"normal output"}]}`,
		},
		{
			name: "Unicode in normal text",
			body: `{"input":[{"type":"message","role":"user","content":"\u4f60\u597d"}]}`,
		},
		{
			name: "matching words inside output text",
			body: `{"input":[{"type":"function_call_output","name":"read_file","output":"automation_update create_thread codex_apps_open_page"}]}`,
		},
		{
			name: "name and type from different items",
			body: `{"input":[{"type":"function_call","name":"automation_update"},{"type":"function_call","name":"create_thread"},{"type":"function_call","name":"codex_apps_open_page"},{"type":"function_call_output","name":"read_file","output":"ok"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			require.False(t, mayContainCodexCallOutputCandidate(body, "automation_update", "create_thread", "codex_apps_open_page"))
			for _, normalize := range []func([]byte) ([]byte, bool){normalizeCodexPageContext, normalizeCodexAutomationBootstrap, normalizeCodexDelegationBootstrap} {
				got, changed := normalize(body)
				require.False(t, changed)
				require.Equal(t, body, got)
				require.Equal(t, &body[0], &got[0])
			}
		})
	}
}

func TestCodexBootstrapPrecheckPreservesEscapedCandidateFields(t *testing.T) {
	output := codexAutomationBootstrap("wiki-maintenance", "never", automationBootstrapPrompt)
	body := codexAutomationBootstrapBody(t, output, "")
	escaped := strings.NewReplacer(
		`"input"`, `"\u0069nput"`,
		`"type"`, `"\u0074ype"`,
		`"name"`, `"\u006eame"`,
		`"function_call_output"`, `"\u0066unction_call_output"`,
		`"automation_update"`, `"\u0061utomation_update"`,
	).Replace(string(body))
	got, changed := normalizeCodexAutomationBootstrap([]byte(escaped))
	require.True(t, changed)
	require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())

	// The structural precheck must not bypass the original duplicate-key guard,
	// including escaped keys that decode to the same member name.
	duplicate := strings.Replace(escaped, `"\u006eame":`, `"name":"other","\u006eame":`, 1)
	got, changed = normalizeCodexAutomationBootstrap([]byte(duplicate))
	require.False(t, changed)
	require.Equal(t, duplicate, string(got))

	page := []byte(`{"\u0069nput":[{"\u0074ype":"\u0066unction_call_output","namespace":"agent","\u006eame":"codex_apps_selected_page_part_1_of_1","output":"Companion Page snapshot part 1 of 1:\n{\"page_id\":null}"}]}`)
	got, changed = normalizeCodexPageContext(page)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
}

// Real Codex requests advertise these tools even when their current output is
// unrelated. Escaped non-ASCII text must not turn that common no-op into three
// complete decodes of the conversation.
func codexBootstrapLargeNoopBody(items, textBytes int) []byte {
	var body strings.Builder
	body.WriteString(`{"model":"gpt-5","tools":[{"type":"function","name":"automation_update"},{"type":"function","name":"create_thread"},{"type":"function","name":"codex_apps_open_page"}],"input":[`)
	text := strings.Repeat("x", textBytes)
	for i := 0; i < items; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteString(`{"type":"message","role":"user","content":[{"type":"input_text","text":"\u4f60`)
		body.WriteString(text)
		body.WriteString(`"}]}`)
	}
	body.WriteString(`,{"type":"function_call_output","call_id":"call_1","name":"read_file","output":"ok"}]}`)
	return []byte(body.String())
}

func TestNormalizeCodexBootstrapLargeNoopAvoidsFullDecode(t *testing.T) {
	body := codexBootstrapLargeNoopBody(256, 4096)
	for name, normalize := range map[string]func([]byte) ([]byte, bool){
		"page":       normalizeCodexPageContext,
		"automation": normalizeCodexAutomationBootstrap,
		"delegation": normalizeCodexDelegationBootstrap,
	} {
		t.Run(name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(3, func() {
				got, changed := normalize(body)
				if changed || &got[0] != &body[0] {
					t.Fatal("non-candidate body must be returned unchanged")
				}
			})
			// Metadata projection allocates a handful of small buffers; a complete
			// token walk/map decode allocates per input item and exceeds this bound.
			require.Less(t, allocs, float64(100))
		})
	}
}

func BenchmarkNormalizeCodexBootstrapLargeNoop(b *testing.B) {
	body := codexBootstrapLargeNoopBody(256, 32<<10)
	for name, normalize := range map[string]func([]byte) ([]byte, bool){
		"structural_precheck": normalizeCodexAutomationBootstrap,
		// This body matched the old global byte precheck. Calling the unchanged
		// decoder directly measures the former no-op path on identical input.
		"full_decode_baseline": func(body []byte) ([]byte, bool) {
			return normalizeCodexCallOutputBootstrap(body, isCodexAutomationCandidate, false)
		},
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				got, changed := normalize(body)
				if changed || &got[0] != &body[0] {
					b.Fatal("non-candidate body must be returned unchanged")
				}
			}
		})
	}
}
