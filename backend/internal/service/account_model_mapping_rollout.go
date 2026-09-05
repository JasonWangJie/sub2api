package service

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/maphash"
	"math"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ModelMappingPercentCredentialKey        = "model_mapping_percent"
	ModelMappingPercentByModelCredentialKey = "model_mapping_percent_by_model"
	DefaultModelMappingPercent              = 100
	modelMappingRolloutBuckets              = 10_000
)

var modelMappingRolloutHashSeed = maphash.MakeSeed()

type modelMappingRolloutPinContextKey struct{}

type modelMappingRolloutPin struct{}

func withModelMappingRolloutPin(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, modelMappingRolloutPinContextKey{}, modelMappingRolloutPin{})
}

// ValidateModelMappingPercentCredentials validates the admin-facing credentials
// payload. Runtime reads remain tolerant so malformed legacy data keeps the
// pre-rollout behavior (100% mapping).
func ValidateModelMappingPercentCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	raw, exists := credentials[ModelMappingPercentCredentialKey]
	if exists {
		if _, ok := parseModelMappingPercent(raw); !ok {
			return infraerrors.New(http.StatusBadRequest, "INVALID_MODEL_MAPPING_PERCENT",
				"model_mapping_percent must be an integer between 0 and 100")
		}
	}
	if rawByModel, exists := credentials[ModelMappingPercentByModelCredentialKey]; exists {
		if !validateModelMappingPercentByModel(rawByModel) {
			return infraerrors.New(http.StatusBadRequest, "INVALID_MODEL_MAPPING_PERCENT_BY_MODEL",
				"model_mapping_percent_by_model must contain integer percentages between 0 and 100")
		}
	}
	return nil
}

func validateModelMappingPercentByModel(raw any) bool {
	switch values := raw.(type) {
	case map[string]any:
		for key, value := range values {
			if strings.TrimSpace(key) == "" {
				return false
			}
			if _, ok := parseModelMappingPercent(value); !ok {
				return false
			}
		}
		return true
	case map[string]int:
		for key, value := range values {
			if strings.TrimSpace(key) == "" {
				return false
			}
			if _, ok := parseModelMappingPercent(value); !ok {
				return false
			}
		}
		return true
	case map[string]float64:
		for key, value := range values {
			if strings.TrimSpace(key) == "" {
				return false
			}
			if _, ok := parseModelMappingPercent(value); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// GetModelMappingPercent returns the account-wide rollout percentage for
// administrator-saved model_mapping rules. Missing or invalid legacy values
// deliberately preserve the old 100% mapping behavior.
func (a *Account) GetModelMappingPercent() int {
	if a == nil || a.Credentials == nil {
		return DefaultModelMappingPercent
	}
	percent, ok := parseModelMappingPercent(a.Credentials[ModelMappingPercentCredentialKey])
	if !ok {
		return DefaultModelMappingPercent
	}
	return percent
}

// GetModelMappingPercentForModel returns the rule-specific rollout percentage
// when configured. Exact source-model keys win; otherwise the longest matching
// wildcard key is used. Invalid or missing entries fall back to the legacy
// account-wide percentage.
func (a *Account) GetModelMappingPercentForModel(sourceModel string) int {
	if a == nil || a.Credentials == nil {
		return DefaultModelMappingPercent
	}
	values := a.Credentials[ModelMappingPercentByModelCredentialKey]
	if values == nil {
		return a.GetModelMappingPercent()
	}
	lookup := func(key string) (int, bool) {
		var raw any
		switch typed := values.(type) {
		case map[string]any:
			raw, _ = typed[key]
		case map[string]int:
			if value, ok := typed[key]; ok {
				raw = value
			}
		case map[string]float64:
			if value, ok := typed[key]; ok {
				raw = value
			}
		default:
			return 0, false
		}
		percent, ok := parseModelMappingPercent(raw)
		return percent, ok
	}

	model := strings.TrimSpace(sourceModel)
	if model == "" {
		return a.GetModelMappingPercent()
	}
	if percent, ok := lookup(model); ok {
		return percent
	}
	bestPattern := ""
	for pattern := range rangeModelMappingPercentKeys(values) {
		if matchWildcard(pattern, model) && (bestPattern == "" || len(pattern) > len(bestPattern) || (len(pattern) == len(bestPattern) && pattern < bestPattern)) {
			bestPattern = pattern
		}
	}
	if bestPattern != "" {
		if percent, ok := lookup(bestPattern); ok {
			return percent
		}
	}
	return a.GetModelMappingPercent()
}

func rangeModelMappingPercentKeys(raw any) map[string]int {
	keys := make(map[string]int)
	switch values := raw.(type) {
	case map[string]any:
		for key := range values {
			keys[key] = 0
		}
	case map[string]int:
		for key := range values {
			keys[key] = 0
		}
	case map[string]float64:
		for key := range values {
			keys[key] = 0
		}
	}
	return keys
}

func parseModelMappingPercent(raw any) (int, bool) {
	var value int64
	switch typed := raw.(type) {
	case int:
		value = int64(typed)
	case int8:
		value = int64(typed)
	case int16:
		value = int64(typed)
	case int32:
		value = int64(typed)
	case int64:
		value = typed
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, false
		}
		value = int64(typed)
	case uint8:
		value = int64(typed)
	case uint16:
		value = int64(typed)
	case uint32:
		value = int64(typed)
	case uint64:
		if typed > math.MaxInt64 {
			return 0, false
		}
		value = int64(typed)
	case float32:
		f := float64(typed)
		if math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f || f < 0 || f > DefaultModelMappingPercent {
			return 0, false
		}
		value = int64(f)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed < 0 || typed > DefaultModelMappingPercent {
			return 0, false
		}
		value = int64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.Trunc(parsed) != parsed || parsed < 0 || parsed > DefaultModelMappingPercent {
			return 0, false
		}
		value = int64(parsed)
	default:
		return 0, false
	}
	if value < 0 || value > DefaultModelMappingPercent {
		return 0, false
	}
	return int(value), true
}

// ResolveMappedModelForRequest applies the account mapping rollout decision.
// matched remains true when a rule is rolled out to the original model so
// lower-priority mapping fallbacks cannot silently replace the chosen branch.
func (a *Account) ResolveMappedModelForRequest(ctx context.Context, requestedModel string) (mappedModel string, matched bool) {
	if a == nil || (a.GetModelMappingPercent() >= DefaultModelMappingPercent && !a.hasModelMappingPercentByModel()) {
		if a == nil {
			return requestedModel, false
		}
		return a.ResolveMappedModel(requestedModel)
	}
	resolution := a.ResolveMappedModelDetailedForRequest(ctx, requestedModel)
	return resolution.Model, resolution.ExplicitMatched || resolution.EffectiveMatched
}

func (a *Account) hasModelMappingPercentByModel() bool {
	if a == nil || a.Credentials == nil {
		return false
	}
	switch values := a.Credentials[ModelMappingPercentByModelCredentialKey].(type) {
	case map[string]any:
		return len(values) > 0
	case map[string]int:
		return len(values) > 0
	case map[string]float64:
		return len(values) > 0
	default:
		return false
	}
}

func (a *Account) GetMappedModelForRequest(ctx context.Context, requestedModel string) string {
	mappedModel, _ := a.ResolveMappedModelForRequest(ctx, requestedModel)
	return mappedModel
}

// ResolveMappedModelDetailedForRequest keeps platform defaults deterministic
// and applies percentage rollout only to an explicit credentials.model_mapping
// rule that actually changes the concrete model.
func (a *Account) ResolveMappedModelDetailedForRequest(ctx context.Context, requestedModel string) AccountModelMappingResolution {
	resolution := a.ResolveMappedModelDetailed(requestedModel)
	if a == nil || !resolution.ExplicitChanged {
		return resolution
	}
	// Use the actual source rule key so normalized requests and wildcard rules
	// select the same per-model percentage as the corresponding mapping entry.
	percent := a.GetModelMappingPercentForModel(resolution.ExplicitSource)
	if percent >= DefaultModelMappingPercent {
		return resolution
	}
	sourceModel := resolution.InputModel
	targetModel := resolution.ExplicitTarget
	if ctx != nil {
		if _, pinned := ctx.Value(modelMappingRolloutPinContextKey{}).(modelMappingRolloutPin); pinned {
			// WebSocket sessions use one stable bucket per account so frames that
			// change models do not change rollout branches. Account failover stays
			// independent because accountID remains part of the hash.
			sourceModel = ""
			targetModel = ""
		}
	}
	if percent <= 0 || !shouldApplyModelMappingPercent(percent, modelMappingRolloutBucket(ctx, a.ID, sourceModel, targetModel)) {
		resolution.Model = requestedModel
		resolution.ExplicitChanged = false
	}
	return resolution
}

func shouldApplyModelMappingPercent(percent int, bucket uint64) bool {
	if percent <= 0 {
		return false
	}
	if percent >= DefaultModelMappingPercent {
		return true
	}
	return bucket%modelMappingRolloutBuckets < uint64(percent*100)
}

func modelMappingRolloutBucket(ctx context.Context, accountID int64, sourceModel, targetModel string) uint64 {
	requestID := "internal"
	if ctx != nil {
		if value, ok := ctx.Value(ctxkey.RequestID).(string); ok && strings.TrimSpace(value) != "" {
			requestID = strings.TrimSpace(value)
		} else if value, ok := ctx.Value(ctxkey.ClientRequestID).(string); ok && strings.TrimSpace(value) != "" {
			requestID = strings.TrimSpace(value)
		}
	}

	var accountBytes [8]byte
	binary.LittleEndian.PutUint64(accountBytes[:], uint64(accountID))
	var hash maphash.Hash
	hash.SetSeed(modelMappingRolloutHashSeed)
	_, _ = hash.Write(accountBytes[:])
	writeModelMappingRolloutHashString(&hash, requestID)
	writeModelMappingRolloutHashString(&hash, sourceModel)
	writeModelMappingRolloutHashString(&hash, targetModel)
	return hash.Sum64() % modelMappingRolloutBuckets
}

func writeModelMappingRolloutHashString(hash *maphash.Hash, value string) {
	var lengthBytes [8]byte
	binary.LittleEndian.PutUint64(lengthBytes[:], uint64(len(value)))
	_, _ = hash.Write(lengthBytes[:])
	_, _ = hash.WriteString(value)
}
