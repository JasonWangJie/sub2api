//go:build unit

package admin

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// Saving settings is a whole-document PUT. A client that sends only the field it
// cares about must not reset everything else: a payload as small as
// `{"risk_control_enabled":true}` used to clear site_name, after which
// getStringOrDefault rendered the empty value as the built-in default and the
// login page silently changed name.

func TestUpdateSettingsPartialPayloadKeepsUnsentKeys(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName:         "Example Gateway",
		service.SettingKeySiteSubtitle:     "Example Gateway Platform",
		service.SettingKeySMTPHost:         "smtp.example.com",
		service.SettingKeySMTPFrom:         "noreply@example.com",
		service.SettingKeyTurnstileEnabled: "true",
	})

	rec := doUpdateSettings(t, h, map[string]any{"risk_control_enabled": true}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "true", repo.values[service.SettingKeyRiskControlEnabled],
		"the field the caller actually sent must be written")

	require.Equal(t, "Example Gateway", repo.values[service.SettingKeySiteName])
	require.Equal(t, "Example Gateway Platform", repo.values[service.SettingKeySiteSubtitle])
	require.Equal(t, "smtp.example.com", repo.values[service.SettingKeySMTPHost])
	require.Equal(t, "noreply@example.com", repo.values[service.SettingKeySMTPFrom])
	require.Equal(t, "true", repo.values[service.SettingKeyTurnstileEnabled])
}

// A full payload keeps whole-document semantics: fields explicitly set to their
// zero value are still cleared.
func TestUpdateSettingsFullPayloadStillClearsSentEmptyFields(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName: "Example Gateway",
	})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": ""}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "", repo.values[service.SettingKeySiteName],
		"an explicitly sent empty value is a deliberate clear, not an omission")
}

// smtp_from_email is the one request field whose JSON name differs from its
// setting key; the alias keeps it from being treated as always-omitted.
func TestUpdateSettingsSMTPFromAliasIsWritable(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySMTPFrom: "old@example.com",
	})

	rec := doUpdateSettings(t, h, map[string]any{"smtp_from_email": "new@example.com"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "new@example.com", repo.values[service.SettingKeySMTPFrom])
}

func TestUpdateSettingsPartialSEOPayloadKeepsUnsentSEOKeys(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySEOIndexingEnabled:  "true",
		service.SettingKeySEOSiteURL:          "https://example.com/",
		service.SettingKeySEOTitle:            "Old title",
		service.SettingKeySEOKeywords:         `["API","Gateway"]`,
		service.SettingKeySEODescription:      "Old description",
		service.SettingKeySEOSocialImageURL:   "https://example.com/card.jpg",
		service.SettingKeySEOVerificationTags: `<meta name="google-site-verification" content="public-token" />`,
	})

	rec := doUpdateSettings(t, h, map[string]any{"seo_title": "  New title  "}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "New title", repo.values[service.SettingKeySEOTitle])
	require.Equal(t, "true", repo.values[service.SettingKeySEOIndexingEnabled])
	require.Equal(t, "https://example.com/", repo.values[service.SettingKeySEOSiteURL])
	require.Equal(t, `["API","Gateway"]`, repo.values[service.SettingKeySEOKeywords])
	require.Equal(t, "Old description", repo.values[service.SettingKeySEODescription])
	require.Equal(t, "https://example.com/card.jpg", repo.values[service.SettingKeySEOSocialImageURL])
	require.Contains(t, repo.values[service.SettingKeySEOVerificationTags], "public-token")
}

func TestUpdateSettingsSEOFieldsAreNormalizedBeforePersistence(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, nil)
	rec := doUpdateSettings(t, h, map[string]any{
		"seo_site_url":          "https://example.com",
		"seo_keywords":          []string{" API ", "api", "Gateway"},
		"seo_verification_tags": `<meta content="token&amp;value" name="google-site-verification">`,
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "https://example.com/", repo.values[service.SettingKeySEOSiteURL])
	require.Equal(t, `["API","Gateway"]`, repo.values[service.SettingKeySEOKeywords])
	require.Equal(t, `<meta name="google-site-verification" content="token&amp;value" />`, repo.values[service.SettingKeySEOVerificationTags])
}

func TestDiffSettingsReportsVerificationFieldWithoutToken(t *testing.T) {
	const token = "Gqy1e5BDdNB726mmWO_sONlJr5o2wUInFdaykA7Xzac"
	changed := diffSettings(
		&service.SystemSettings{},
		&service.SystemSettings{
			SEOVerificationTags: `<meta name="google-site-verification" content="` + token + `" />`,
		},
		nil,
		nil,
		UpdateSettingsRequest{},
	)

	require.Equal(t, []string{service.SettingKeySEOVerificationTags}, changed)
	require.NotContains(t, strings.Join(changed, ","), token)
}
