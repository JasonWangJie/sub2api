package service

import (
	"context"
	"encoding/json"
	"strings"
)

// GPT-5.5 retired from Codex with ChatGPT sign-in on 2026-10-14 at
// 19:00 UTC. API-key upstreams remain supported. Do not infer upstream
// eligibility from the API key a client uses to authenticate to this gateway.
// https://learn.chatgpt.com/docs/models#gpt-55-retirement
func isRetiredOpenAISubscriptionModel(account *Account, upstreamModel string) bool {
	if account == nil || !account.IsOpenAI() || (!account.IsOpenAIOAuthLike() && !account.IsShadow()) {
		return false
	}
	model := canonicalizeOpenAIModelAliasSpelling(upstreamModel)
	return model == "gpt-5.5" || strings.HasPrefix(model, "gpt-5.5-")
}

// Only remove entries blocked by retirement. Unknown models and other platforms
// retain their existing listing policy. An explicitly mapped replacement is
// allowed, but a public alias targeting retired GPT-5.5 is not.
func openAIModelRetiredForAccounts(accounts []Account, modelID string) bool {
	retired := false
	for i := range accounts {
		account := &accounts[i]
		if !account.IsOpenAI() {
			continue
		}
		if openAIRequestModelRetiredForAllRoutes(account, modelID, false) {
			retired = true
		} else if account.IsModelSupported(modelID) {
			return false
		}
	}
	return retired
}

func filterRetiredOpenAIModelIDs(accounts []Account, models []string) []string {
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if !openAIModelRetiredForAccounts(accounts, model) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

// Entries must already use client-visible names, not raw upstream IDs. Resolve
// each public name exactly as routing does, after projection and before the
// final client ETag is calculated. Preserve unknown fields.
func filterRetiredOpenAIPublicModelsBody(body []byte, accounts []Account, codex bool) ([]byte, bool, error) {
	field, idField := "data", "id"
	if codex {
		field, idField = "models", "slug"
	}
	envelope, entries, err := modelCatalogEntries(body, field)
	if err != nil {
		return nil, false, err
	}
	filtered := make([]json.RawMessage, 0, len(entries))
	for _, raw := range entries {
		var entry map[string]json.RawMessage
		var id string
		if json.Unmarshal(raw, &entry) == nil {
			_ = json.Unmarshal(entry[idField], &id)
		}
		if !openAIModelRetiredForAccounts(accounts, id) {
			filtered = append(filtered, raw)
		}
	}
	if len(filtered) == len(entries) {
		return body, false, nil
	}
	envelope[field], err = json.Marshal(filtered)
	if err != nil {
		return nil, false, err
	}
	encoded, err := json.Marshal(envelope)
	return encoded, true, err
}

// Compact-only administrator mappings are evaluated against their actual
// target, without treating an old client-visible name as a retired target.
func supportsOpenAIRequestModel(account *Account, requestedModel string, requireCompact bool) bool {
	if openAIRequestModelRetiredForAllRoutes(account, requestedModel, requireCompact) {
		return false
	}
	return account.isModelMappingSupported(requestedModel)
}

// Catalogs have no request rollout bucket. Keep a public model visible while
// any branch enabled by its percentage can still serve it, without sampling.
func openAIRequestModelRetiredForAllRoutes(account *Account, requestedModel string, requireCompact bool) bool {
	if account == nil || !account.IsOpenAI() || (!account.IsOpenAIOAuthLike() && !account.IsShadow()) {
		return false
	}
	// A nil context deliberately uses the deterministic, fully mapped resolver.
	mappedTarget := resolveOpenAIAccountUpstreamModelForRequest(nil, account, requestedModel, requireCompact)
	rawChat := shouldForwardOpenAIResponsesViaRawChatCompletions(account)
	if !rawChat && (account.IsOpenAIPassthroughEnabled() ||
		(requireCompact && resolveOpenAICompactForwardModel(account, requestedModel) != strings.TrimSpace(requestedModel))) {
		return isRetiredOpenAISubscriptionModel(account, mappedTarget)
	}
	resolution := account.ResolveMappedModelDetailed(requestedModel)
	if !resolution.ExplicitChanged {
		return isRetiredOpenAISubscriptionModel(account, mappedTarget)
	}
	percent := account.GetModelMappingPercentForModel(resolution.ExplicitSource)
	if percent > 0 && !isRetiredOpenAISubscriptionModel(account, mappedTarget) {
		return false
	}
	if percent < DefaultModelMappingPercent && !isRetiredOpenAISubscriptionModel(account, normalizeOpenAIModelForUpstream(account, requestedModel)) {
		return false
	}
	return true
}

// Admission must inspect the same request or pinned WS-session branch that
// Forward will use. Other platforms retain their existing mapping-only check.
func supportsOpenAIRequestModelForRequest(ctx context.Context, account *Account, requestedModel string, requireCompact bool) bool {
	if account != nil && account.IsOpenAI() && (account.IsOpenAIOAuthLike() || account.IsShadow()) &&
		isRetiredOpenAISubscriptionModel(account, resolveOpenAIAccountUpstreamModelForRequest(ctx, account, requestedModel, requireCompact)) {
		return false
	}
	return account.isModelMappingSupported(requestedModel)
}
