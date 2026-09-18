import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  __resetUsdtRechargeLabelCacheForTests,
  formatUsdtBonusRate,
  resolveUsdtRechargeNavLabel,
} from '@/composables/useUsdtRechargeLabel'

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getUSDTCheckoutInfo: vi.fn(),
  },
}))

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: { payment: 'payment' },
  isFeatureFlagEnabled: vi.fn(() => true),
}))

describe('formatUsdtBonusRate', () => {
  it('formats integers and decimals without trailing noise', () => {
    expect(formatUsdtBonusRate(1)).toBe('1')
    expect(formatUsdtBonusRate(1.5)).toBe('1.5')
    expect(formatUsdtBonusRate(1.25)).toBe('1.25')
  })

  it('returns empty for non-positive rates', () => {
    expect(formatUsdtBonusRate(0)).toBe('')
    expect(formatUsdtBonusRate(-1)).toBe('')
    expect(formatUsdtBonusRate(Number.NaN)).toBe('')
  })
})

describe('resolveUsdtRechargeNavLabel', () => {
  beforeEach(() => {
    __resetUsdtRechargeLabelCacheForTests()
  })

  it('uses the with-bonus key when rate is positive', () => {
    const t = vi.fn((key: string, values?: Record<string, unknown>) => {
      if (key === 'nav.usdtRechargeWithBonus') return `USDT充值（加赠${values?.rate}%）`
      return 'USDT充值'
    })
    expect(resolveUsdtRechargeNavLabel(t, 1)).toBe('USDT充值（加赠1%）')
    expect(resolveUsdtRechargeNavLabel(t, 2.5)).toBe('USDT充值（加赠2.5%）')
  })

  it('falls back to the plain label when rate is missing or zero', () => {
    const t = vi.fn((key: string) => (key === 'nav.usdtRecharge' ? 'USDT充值' : key))
    expect(resolveUsdtRechargeNavLabel(t, null)).toBe('USDT充值')
    expect(resolveUsdtRechargeNavLabel(t, 0)).toBe('USDT充值')
  })
})
