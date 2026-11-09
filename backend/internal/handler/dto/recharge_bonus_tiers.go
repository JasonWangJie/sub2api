package dto

// RechargeBonusTier 充值赠送档位：余额充值支付金额 ≥ MinAmount 时，在到账基数上赠送 BonusPercent%。
type RechargeBonusTier struct {
	MinAmount    float64 `json:"min_amount"`
	BonusPercent float64 `json:"bonus_percent"`
}

// RechargeFixedBonus 保存按充值金额整倍数发放的固定赠金规则。
type RechargeFixedBonus struct {
	Enabled         bool    `json:"enabled"`
	ThresholdAmount float64 `json:"threshold_amount"`
	BonusAmount     float64 `json:"bonus_amount"`
}
