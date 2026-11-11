package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// 充值优惠阶梯：余额充值订单按用户输入的支付金额命中阶梯（取不超过该金额的最大 MinAmount），
// 整条阶梯只有一种模式（RECHARGE_BONUS_MODE）：
//   - bonus（赠金）：实付不变，在到账基数（输入 × 充值倍率）之上额外赠送 BonusPercent% 的 USD 余额；
//   - discount（折扣）：到账不变（输入 × 倍率），实付基数按 BonusPercent% 打折。
//
// 固定满赠按支付金额的完整倍数计算，与百分比赠金取较高值。两者的门槛都不含手续费。
// amount 为到账总额，bonus_amount 为其中「免费」的 USD 部分，pay_amount 为实收。
// 订阅和专用 USDT 订单不参与。优惠在下单时计算并落库。
const (
	// SettingRechargeBonusEnabled 未设置时沿用历史行为，有阶梯即启用。
	SettingRechargeBonusEnabled = "RECHARGE_BONUS_ENABLED"
	// SettingRechargeBonusTiers 存 JSON 数组（RechargeBonusTier 列表）。
	SettingRechargeBonusTiers = "RECHARGE_BONUS_TIERS"
	// SettingRechargeBonusNotice 充值页金额卡顶部展示的 Markdown 活动文案，空表示不展示。
	SettingRechargeBonusNotice = "RECHARGE_BONUS_NOTICE"
	// SettingRechargeBonusMode 阶梯模式：bonus / discount；空/非法按 bonus 解析（兼容早期配置）。
	SettingRechargeBonusMode = "RECHARGE_BONUS_MODE"
	// SettingRechargeFixedBonus 存固定满赠规则的 JSON 对象。
	SettingRechargeFixedBonus = "RECHARGE_FIXED_BONUS"
)

const (
	RechargeBonusModeBonus    = "bonus"
	RechargeBonusModeDiscount = "discount"
)

const (
	maxRechargeBonusTiers       = 20
	maxRechargeBonusPercent     = 1000
	maxRechargeBonusNoticeRunes = 10000
	maxRechargeFixedBonusAmount = 1e9
	rechargeBonusAmountEpsilon  = 1e-9
	// Redeem-code value is DECIMAL(20,8), so one credit must stay below 1e12.
	rechargeCreditExclusiveLimit = 1e12
)

// RechargeBonusTier 一个优惠档位：支付金额 ≥ MinAmount 时按 BonusPercent% 赠送（bonus）或打折（discount）。
type RechargeBonusTier struct {
	MinAmount    float64 `json:"min_amount"`
	BonusPercent float64 `json:"bonus_percent"`
}

// RechargeFixedBonus 按支付币种配置固定满赠的门槛和每份赠送金额。
type RechargeFixedBonus struct {
	Enabled         bool    `json:"enabled"`
	ThresholdAmount float64 `json:"threshold_amount"`
	BonusAmount     float64 `json:"bonus_amount"`
}

// NormalizeRechargeBonusMode 归一化模式；空按 bonus。第二个返回值表示输入是否合法。
func NormalizeRechargeBonusMode(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", RechargeBonusModeBonus:
		return RechargeBonusModeBonus, true
	case RechargeBonusModeDiscount:
		return RechargeBonusModeDiscount, true
	default:
		return RechargeBonusModeBonus, false
	}
}

// ValidateRechargeBonusTiersForMode 折扣模式下百分比必须 < 100，否则实付为 0 或负数。
func ValidateRechargeBonusTiersForMode(mode string, tiers []RechargeBonusTier) error {
	if mode != RechargeBonusModeDiscount {
		return nil
	}
	for _, tier := range tiers {
		if tier.BonusPercent >= 100 {
			return fmt.Errorf("discount percent must be less than 100 (tier with min amount %s)",
				decimal.NewFromFloat(tier.MinAmount).Round(2).String())
		}
	}
	return nil
}

func rechargeBonusValueValid(v float64, max float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > max {
		return false
	}
	d := decimal.NewFromFloat(v)
	return d.Equal(d.Round(2))
}

// NormalizeRechargeBonusTiers 严格归一化（写路径）：任何非法项直接报错；
// 成功时返回按 MinAmount 升序排序的副本。
func NormalizeRechargeBonusTiers(raw []RechargeBonusTier) ([]RechargeBonusTier, error) {
	if len(raw) == 0 {
		return []RechargeBonusTier{}, nil
	}
	if len(raw) > maxRechargeBonusTiers {
		return nil, fmt.Errorf("recharge bonus tiers exceed limit of %d", maxRechargeBonusTiers)
	}
	out := make([]RechargeBonusTier, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, tier := range raw {
		if !rechargeBonusValueValid(tier.MinAmount, math.MaxFloat64) {
			return nil, fmt.Errorf("recharge bonus tier min amount must be a non-negative number with at most 2 decimals")
		}
		if !rechargeBonusValueValid(tier.BonusPercent, maxRechargeBonusPercent) {
			return nil, fmt.Errorf("recharge bonus tier percent must be between 0 and %d with at most 2 decimals", maxRechargeBonusPercent)
		}
		key := decimal.NewFromFloat(tier.MinAmount).Round(2).String()
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("duplicate recharge bonus tier min amount: %s", key)
		}
		seen[key] = struct{}{}
		out = append(out, RechargeBonusTier{MinAmount: tier.MinAmount, BonusPercent: tier.BonusPercent})
	}
	sortRechargeBonusTiers(out)
	return out, nil
}

func sortRechargeBonusTiers(tiers []RechargeBonusTier) {
	sort.SliceStable(tiers, func(i, j int) bool {
		return tiers[i].MinAmount < tiers[j].MinAmount
	})
}

// encodeRechargeBonusTiers 序列化为设置值；空列表存空串，与「未配置」保持同一形态。
func encodeRechargeBonusTiers(tiers []RechargeBonusTier) (string, error) {
	if len(tiers) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(tiers)
	if err != nil {
		return "", fmt.Errorf("marshal recharge bonus tiers: %w", err)
	}
	return string(raw), nil
}

// parseRechargeBonusTiers 宽松解析（读路径）：非法条目丢弃而非报错，避免历史错配置阻断下单。
// 同一 MinAmount 重复时保留先出现的档位。始终返回非 nil 切片，便于 JSON 输出为 []。
func parseRechargeBonusTiers(raw string) []RechargeBonusTier {
	out := make([]RechargeBonusTier, 0)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	var items []RechargeBonusTier
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		slog.Warn("[Payment] parseRechargeBonusTiers: unmarshal failed", "error", err)
		return out
	}
	seen := make(map[string]struct{}, len(items))
	for _, tier := range items {
		if !rechargeBonusValueValid(tier.MinAmount, math.MaxFloat64) || !rechargeBonusValueValid(tier.BonusPercent, maxRechargeBonusPercent) {
			continue
		}
		key := decimal.NewFromFloat(tier.MinAmount).Round(2).String()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tier)
	}
	sortRechargeBonusTiers(out)
	return out
}

func validateRechargeBonusNotice(notice string) error {
	if utf8.RuneCountInString(notice) > maxRechargeBonusNoticeRunes {
		return fmt.Errorf("recharge bonus notice exceeds %d characters", maxRechargeBonusNoticeRunes)
	}
	return nil
}

func rechargeFixedBonusValid(fixed RechargeFixedBonus) bool {
	return rechargeBonusValueValid(fixed.ThresholdAmount, math.MaxFloat64) &&
		rechargeBonusValueValid(fixed.BonusAmount, math.MaxFloat64) &&
		(!fixed.Enabled || (fixed.ThresholdAmount > 0 && fixed.BonusAmount > 0))
}

func parseRechargeFixedBonus(raw string) RechargeFixedBonus {
	var fixed RechargeFixedBonus
	if strings.TrimSpace(raw) == "" {
		return fixed
	}
	if err := json.Unmarshal([]byte(raw), &fixed); err != nil || !rechargeFixedBonusValid(fixed) {
		return RechargeFixedBonus{}
	}
	return fixed
}

func parseRechargeBonusEnabled(raw string, tiers []RechargeBonusTier) bool {
	if raw == "" {
		return len(tiers) > 0
	}
	return raw == "true"
}

func rechargeDiscountEnabled(enabled bool, mode string, tiers []RechargeBonusTier) bool {
	return enabled && mode == RechargeBonusModeDiscount && len(tiers) > 0
}

// resolveRechargeBonusUpdate 使用更新后的完整规则校验折扣与满赠的兼容性，只返回请求修改的设置。
func (s *PaymentConfigService) resolveRechargeBonusUpdate(ctx context.Context, req UpdatePaymentConfigRequest) (map[string]string, error) {
	updates := make(map[string]string)
	if req.RechargeBonusTiers == nil && req.RechargeBonusMode == nil && req.RechargeBonusEnabled == nil && req.RechargeFixedBonus == nil {
		return updates, nil
	}
	var stored map[string]string
	if s.settingRepo != nil {
		var err error
		stored, err = s.settingRepo.GetMultiple(ctx, []string{
			SettingRechargeBonusTiers, SettingRechargeBonusMode, SettingRechargeBonusEnabled, SettingRechargeFixedBonus,
		})
		if err != nil {
			return nil, fmt.Errorf("get recharge bonus settings: %w", err)
		}
	}
	storedTiers := parseRechargeBonusTiers(stored[SettingRechargeBonusTiers])
	storedMode, _ := NormalizeRechargeBonusMode(stored[SettingRechargeBonusMode])
	storedEnabled := parseRechargeBonusEnabled(stored[SettingRechargeBonusEnabled], storedTiers)

	var tiers []RechargeBonusTier
	if req.RechargeBonusTiers != nil {
		var err error
		tiers, err = NormalizeRechargeBonusTiers(*req.RechargeBonusTiers)
		if err != nil {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", err.Error())
		}
		tiersValue, err := encodeRechargeBonusTiers(tiers)
		if err != nil {
			return nil, err
		}
		updates[SettingRechargeBonusTiers] = tiersValue
	} else {
		tiers = storedTiers
	}

	mode := storedMode
	if req.RechargeBonusMode != nil {
		normalized, ok := NormalizeRechargeBonusMode(*req.RechargeBonusMode)
		if !ok {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_BONUS_MODE", "recharge bonus mode must be bonus or discount")
		}
		mode = normalized
		updates[SettingRechargeBonusMode] = mode
	}

	enabled := parseRechargeBonusEnabled(stored[SettingRechargeBonusEnabled], tiers)
	if req.RechargeBonusEnabled != nil {
		enabled = *req.RechargeBonusEnabled
		updates[SettingRechargeBonusEnabled] = formatBoolOrEmpty(req.RechargeBonusEnabled)
	}
	if req.RechargeBonusTiers != nil || req.RechargeBonusMode != nil || (req.RechargeBonusEnabled != nil && enabled) {
		if err := ValidateRechargeBonusTiersForMode(mode, tiers); err != nil {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", err.Error())
		}
	}

	fixed := parseRechargeFixedBonus(stored[SettingRechargeFixedBonus])
	if fixed.Enabled && rechargeDiscountEnabled(storedEnabled, storedMode, storedTiers) {
		// Persistently clear a conflicting legacy switch so turning discount off
		// later cannot silently activate a previously suppressed fixed gift.
		fixed.Enabled = false
		value, err := json.Marshal(fixed)
		if err != nil {
			return nil, fmt.Errorf("marshal fixed recharge bonus: %w", err)
		}
		updates[SettingRechargeFixedBonus] = string(value)
	}
	if req.RechargeFixedBonus != nil {
		previousFixed := fixed
		next := *req.RechargeFixedBonus
		// 只关闭开关时保留已配置金额，新配置仍可保存 0。
		if !next.Enabled {
			if next.ThresholdAmount == 0 {
				next.ThresholdAmount = fixed.ThresholdAmount
			}
			if next.BonusAmount == 0 {
				next.BonusAmount = fixed.BonusAmount
			}
		}
		fixed = next
		newExcessiveGift := fixed.Enabled && fixed.BonusAmount > maxRechargeFixedBonusAmount &&
			(!previousFixed.Enabled || fixed.BonusAmount != previousFixed.BonusAmount ||
				fixed.ThresholdAmount != previousFixed.ThresholdAmount)
		if !rechargeFixedBonusValid(fixed) || newExcessiveGift {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_FIXED_BONUS", "fixed recharge bonus amounts must be non-negative with at most 2 decimals, positive when enabled, and the bonus amount must not exceed 1 billion")
		}
		value, err := json.Marshal(fixed)
		if err != nil {
			return nil, fmt.Errorf("marshal fixed recharge bonus: %w", err)
		}
		updates[SettingRechargeFixedBonus] = string(value)
	}

	if fixed.Enabled && rechargeDiscountEnabled(enabled, mode, tiers) {
		return nil, infraerrors.BadRequest("RECHARGE_BONUS_MODE_CONFLICT", "recharge discount and fixed recharge bonus cannot both be enabled")
	}
	return updates, nil
}

// matchRechargeBonusTier 返回不超过 paymentAmount 的最大档位；tiers 需已按 MinAmount 升序。
func matchRechargeBonusTier(tiers []RechargeBonusTier, paymentAmount float64) (RechargeBonusTier, bool) {
	if math.IsNaN(paymentAmount) || math.IsInf(paymentAmount, 0) || paymentAmount <= 0 {
		return RechargeBonusTier{}, false
	}
	var matched RechargeBonusTier
	found := false
	for _, tier := range tiers {
		if paymentAmount+rechargeBonusAmountEpsilon < tier.MinAmount {
			break
		}
		matched = tier
		found = true
	}
	return matched, found
}

// calculateRechargeBonus 赠送金额 = 到账基数 × 百分比，保留两位小数（四舍五入）。
func calculateRechargeBonus(baseCredited, bonusPercent float64) float64 {
	if baseCredited <= 0 || bonusPercent <= 0 || math.IsNaN(baseCredited) || math.IsNaN(bonusPercent) {
		return 0
	}
	return decimal.NewFromFloat(baseCredited).
		Mul(decimal.NewFromFloat(bonusPercent)).
		Div(decimal.NewFromInt(100)).
		Round(2).
		InexactFloat64()
}

// addRechargeBonus 到账总额 = 基数 + 赠送，两位小数。
func addRechargeBonus(baseCredited, bonus float64) float64 {
	return decimal.NewFromFloat(baseCredited).
		Add(decimal.NewFromFloat(bonus)).
		Round(2).
		InexactFloat64()
}

// calculateDiscountedPayBase 折扣模式实付基数 = 支付金额 × (1 − 百分比)，按币种精度四舍五入。
func calculateDiscountedPayBase(paymentAmount, discountPercent float64, currency string) float64 {
	digits := int32(payment.CurrencyMaxFractionDigits(currency))
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromInt(100).Sub(decimal.NewFromFloat(discountPercent))).
		Div(decimal.NewFromInt(100)).
		Round(digits).
		InexactFloat64()
}

// rechargeBonusQuote 一笔余额充值的报价结果。
type rechargeBonusQuote struct {
	// PayBase 网关收款基数（支付币种，不含手续费）；赠金模式等于支付金额，折扣模式为折后金额。
	PayBase float64
	// Credited 到账总额（USD），含 Bonus。
	Credited float64
	// Bonus 免费额度（USD）：赠金模式为额外赠送，折扣模式为未付费却到账的部分。
	Bonus float64
	// Percent 命中档位的百分比；未命中或未产生优惠时为 0。
	Percent float64
}

// quoteRechargeBonus 按配置报价。currency 用于折扣模式实付基数的精度。
func quoteRechargeBonus(cfg *PaymentConfig, paymentAmount float64, currency string) (rechargeBonusQuote, error) {
	multiplier := defaultBalanceRechargeMultiplier
	var tiers []RechargeBonusTier
	var fixed RechargeFixedBonus
	mode := RechargeBonusModeBonus
	if cfg != nil {
		multiplier = cfg.BalanceRechargeMultiplier
		if cfg.RechargeBonusEnabled {
			tiers = cfg.RechargeBonusTiers
		}
		fixed = cfg.RechargeFixedBonus
		mode, _ = NormalizeRechargeBonusMode(cfg.RechargeBonusMode)
	}
	base := calculateCreditedBalance(paymentAmount, multiplier)
	if math.IsNaN(base) || math.IsInf(base, 0) || base >= rechargeCreditExclusiveLimit {
		return rechargeBonusQuote{}, invalidRechargeBonusAmount()
	}
	quote := rechargeBonusQuote{PayBase: paymentAmount, Credited: base}
	tier, ok := matchRechargeBonusTier(tiers, paymentAmount)

	// 历史配置若同时启用折扣与满赠，沿用折扣规则。
	if rechargeDiscountEnabled(true, mode, tiers) {
		return quoteRechargeDiscount(quote, tier.BonusPercent, multiplier, currency), nil
	}
	if ok {
		quote.Bonus = calculateRechargeBonus(base, tier.BonusPercent)
	}
	if quote.Bonus > 0 {
		quote.Percent = tier.BonusPercent
	}
	if fixed.Enabled && rechargeFixedBonusValid(fixed) {
		multiples, _ := decimal.NewFromFloat(paymentAmount).QuoRem(decimal.NewFromFloat(fixed.ThresholdAmount), 0)
		fixedCredit := multiples.
			Mul(decimal.NewFromFloat(fixed.BonusAmount)).
			Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
			Round(2)
		if fixedCredit.GreaterThanOrEqual(decimal.NewFromInt(rechargeCreditExclusiveLimit)) {
			return rechargeBonusQuote{}, invalidRechargeBonusAmount()
		}
		bonus := fixedCredit.InexactFloat64()
		if bonus > quote.Bonus {
			quote.Bonus = bonus
			quote.Percent = 0
		}
	}
	if quote.Bonus > 0 {
		quote.Credited = addRechargeBonus(base, quote.Bonus)
	}
	if math.IsNaN(quote.Credited) || math.IsInf(quote.Credited, 0) || quote.Credited >= rechargeCreditExclusiveLimit {
		return rechargeBonusQuote{}, invalidRechargeBonusAmount()
	}
	return quote, nil
}

func invalidRechargeBonusAmount() error {
	return infraerrors.BadRequest("INVALID_RECHARGE_BONUS_AMOUNT", "recharge credit exceeds the supported limit")
}

func quoteRechargeDiscount(quote rechargeBonusQuote, percent, multiplier float64, currency string) rechargeBonusQuote {
	if percent <= 0 || percent >= 100 {
		return quote
	}
	payBase := calculateDiscountedPayBase(quote.PayBase, percent, currency)
	if payBase <= 0 || payBase >= quote.PayBase {
		return quote
	}
	paidCredit := calculateCreditedBalance(payBase, multiplier)
	bonus := decimal.NewFromFloat(quote.Credited).Sub(decimal.NewFromFloat(paidCredit)).Round(2).InexactFloat64()
	quote.PayBase = payBase
	quote.Bonus = math.Max(0, bonus)
	quote.Percent = percent
	return quote
}

// paymentOrderAmountWithoutBonus 订单到账金额剔除免费额度后的实付部分（USD），用于推广返利基数。
func paymentOrderAmountWithoutBonus(o *dbent.PaymentOrder) float64 {
	if o == nil {
		return 0
	}
	if o.OrderType != payment.OrderTypeBalance || o.BonusAmount <= 0 {
		return o.Amount
	}
	base := decimal.NewFromFloat(o.Amount).
		Sub(decimal.NewFromFloat(o.BonusAmount)).
		Round(2).
		InexactFloat64()
	if base < 0 {
		return 0
	}
	return base
}
