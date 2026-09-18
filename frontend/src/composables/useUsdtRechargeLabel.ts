import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { paymentAPI } from '@/api/payment'
import { isFeatureFlagEnabled, FeatureFlags } from '@/utils/featureFlags'

/** Module-level cache so sidebar, header, and document title share one fetch. */
let cachedBonusRate: number | null = null
let loaded = false
let fetchPromise: Promise<void> | null = null

export function formatUsdtBonusRate(rate: number): string {
  if (!Number.isFinite(rate) || rate <= 0) return ''
  return String(Math.round(rate * 100) / 100)
}

export function getCachedUsdtBonusRate(): number | null {
  return cachedBonusRate
}

/** Pure label resolver — safe for router title helpers outside setup(). */
export function resolveUsdtRechargeNavLabel(
  t: (key: string, values?: Record<string, unknown>) => string,
  rate: number | null = cachedBonusRate,
): string {
  if (typeof rate === 'number' && rate > 0) {
    return t('nav.usdtRechargeWithBonus', { rate: formatUsdtBonusRate(rate) })
  }
  return t('nav.usdtRecharge')
}

/**
 * Loads Epusdt bonus_rate once (when payment is enabled) and exposes a reactive
 * nav label: "USDT充值（加赠{n}%）" / "USDT Recharge (+{n}%)".
 */
export function useUsdtRechargeLabel() {
  const { t } = useI18n()
  const bonusRate = ref<number | null>(cachedBonusRate)

  async function ensureLoaded(force = false): Promise<void> {
    if (!isFeatureFlagEnabled(FeatureFlags.payment)) {
      cachedBonusRate = null
      loaded = false
      bonusRate.value = null
      return
    }
    if (!force && loaded) {
      bonusRate.value = cachedBonusRate
      return
    }
    if (!force && fetchPromise) {
      await fetchPromise
      bonusRate.value = cachedBonusRate
      return
    }

    fetchPromise = (async () => {
      try {
        const { data } = await paymentAPI.getUSDTCheckoutInfo()
        // Prefer configured bonus even when checkout is temporarily disabled
        // (e.g. exchange-rate outage); 0 means "no gift" → plain label.
        const rate = Number(data?.bonus_rate)
        cachedBonusRate = Number.isFinite(rate) ? rate : 0
        loaded = true
      } catch {
        // Leave unloaded so a later mount / force refresh can retry.
      } finally {
        fetchPromise = null
      }
    })()

    await fetchPromise
    bonusRate.value = cachedBonusRate
  }

  const label = computed(() => resolveUsdtRechargeNavLabel(t, bonusRate.value))

  return { label, bonusRate, ensureLoaded }
}

/** Test helper — reset module cache between specs. */
export function __resetUsdtRechargeLabelCacheForTests() {
  cachedBonusRate = null
  loaded = false
  fetchPromise = null
}
