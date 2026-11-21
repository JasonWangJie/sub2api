package service

import (
	"context"
	"slices"
	"strings"
)

func (s *GatewayService) ExpandAnthropicLongContextModels(ctx context.Context, group *Group, models []string) []string {
	var accounts []Account
	var err error
	if group != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	} else {
		accounts, err = s.accountRepo.ListSchedulable(ctx)
	}
	if err != nil {
		return models
	}
	result := cloneStringSlice(models)
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		seen[strings.ToLower(model)] = true
	}
	for _, model := range models {
		if strings.HasSuffix(strings.ToLower(model), claudeCodeLongContextModelSuffix) || strings.Contains(model, "*") {
			continue
		}
		var routedAccountIDs []int64
		if group != nil {
			routedAccountIDs = group.GetRoutingAccountIDs(model)
		}
		for _, account := range accounts {
			if !account.IsAnthropic() || !account.IsModelSupported(model) ||
				(len(routedAccountIDs) > 0 && !slices.Contains(routedAccountIDs, account.ID)) {
				continue
			}
			variant := model + claudeCodeLongContextModelSuffix
			if !seen[strings.ToLower(variant)] && supportsAnthropicLongContextAcrossRollout(&account, model) {
				result = append(result, variant)
				seen[strings.ToLower(variant)] = true
			}
		}
	}
	return result
}

// A model list has no request-specific rollout bucket. Advertise the extra
// context window only when every enabled branch of this account supports it.
func supportsAnthropicLongContextAcrossRollout(account *Account, model string) bool {
	snapshot := account.GetUpstreamModelMetadataSnapshot()
	if snapshot == nil {
		return false
	}
	mapping := account.ResolveMappedModelDetailed(model)
	percent := DefaultModelMappingPercent
	if mapping.ExplicitChanged {
		percent = account.GetModelMappingPercentForModel(mapping.ExplicitSource)
	}
	const longContextWindow = 900_000
	if percent > 0 && snapshot.Models[strings.TrimSpace(mapping.Model)].ContextWindow < longContextWindow {
		return false
	}
	return percent == DefaultModelMappingPercent || snapshot.Models[strings.TrimSpace(model)].ContextWindow >= longContextWindow
}
