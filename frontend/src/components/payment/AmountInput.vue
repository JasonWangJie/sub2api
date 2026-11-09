<template>
  <div class="space-y-4">
    <!-- Quick Amount Buttons -->
    <div>
      <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.quickAmounts') }}
      </label>
      <div class="grid grid-cols-3 gap-x-4 gap-y-4 pt-2">
        <button
          v-for="{ amount: amt, quote } in quickAmounts"
          :key="amt"
          type="button"
          :class="[
            'relative rounded-lg border-2 px-3 py-3 text-center font-medium transition-colors',
            modelValue === amt
              ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/40 dark:text-primary-300'
              : quote.available && quote.source
                ? 'border-red-200 bg-white text-gray-700 hover:border-red-300 dark:border-red-500/40 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-red-400/60'
                : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500',
          ]"
          :data-testid="`quick-amount-${amt}`"
          @click="selectAmount(amt)"
        >
          <!-- 仅产生优惠的金额显示促销价签 -->
          <span
            v-if="quote.available && quote.source"
            class="pointer-events-none absolute -right-2 -top-3 z-10 rotate-12"
            data-testid="quick-amount-bonus-badge"
          >
            <span
              class="relative flex items-center gap-1 whitespace-nowrap rounded bg-red-600 py-0.5 pl-1.5 pr-1 text-[11px] font-extrabold leading-tight tracking-tight text-white shadow-md ring-2 ring-white before:pointer-events-none before:absolute before:inset-[2px] before:rounded-sm before:border before:border-dotted before:border-white/70 dark:bg-red-500 dark:ring-dark-800"
            >
              <span>{{ badgeText(quote) }}</span>
              <span class="h-1 w-1 shrink-0 rounded-full bg-white"></span>
            </span>
          </span>
          <span class="block">{{ amt }}</span>
          <!-- 有活动时，所有按钮显示到账 USD 或折后实付，保持高度一致 -->
          <span
            v-if="showSecondLine"
            :class="[
              'mt-0.5 block text-[11px] font-normal leading-tight',
              quote.available && quote.source ? 'text-red-600 dark:text-red-300' : 'text-gray-400 dark:text-gray-500',
            ]"
            data-testid="quick-amount-credited"
          >{{ secondLine(quote) }}</span>
        </button>
      </div>
    </div>

    <!-- Custom Amount Input -->
    <div>
      <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.customAmount') }}
      </label>
      <div class="relative">
        <span class="pointer-events-none absolute inset-y-0 right-4 flex items-center text-gray-400 dark:text-dark-500">
          {{ currency }}
        </span>
        <input
          type="text"
          inputmode="decimal"
          :value="customText"
          :placeholder="placeholderText"
          class="input w-full py-3 pl-4 pr-16"
          @input="handleInput"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RechargeBonusTier, RechargeFixedBonus } from '@/types/payment'
import {
  formatRechargeBonusNumber,
  isRechargeFixedBonusEnabled,
  quoteRechargeBonus,
  type RechargeBonusMode,
  type RechargeBonusQuote,
} from '@/utils/rechargeBonus'
import { DEFAULT_PAYMENT_CURRENCY, formatPaymentAmount, paymentCurrencyFractionDigits } from './currency'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  /** 生效的充值优惠阶梯（按 min_amount 升序） */
  bonusTiers?: RechargeBonusTier[]
  /** 阶梯模式：bonus 赠金 / discount 折扣 */
  bonusMode?: RechargeBonusMode
  fixedBonus?: RechargeFixedBonus
  /** 充值倍率（1 支付币种 = multiplier USD），用于计算到账金额 */
  multiplier?: number
  /** 支付币种（折扣模式第二行实付金额的币种与精度） */
  currency?: string
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  bonusTiers: () => [],
  bonusMode: 'bonus',
  multiplier: 1,
  currency: DEFAULT_PAYMENT_CURRENCY,
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const customText = ref('')

// 0 = no limit
const quickAmounts = computed(() => props.amounts
  .filter((amount) => (props.min <= 0 || amount >= props.min) && (props.max <= 0 || amount <= props.max))
  .map((amount) => ({
    amount,
    quote: quoteRechargeBonus(props.bonusTiers, amount, {
      multiplier: props.multiplier,
      mode: props.bonusMode,
      fixedBonus: props.fixedBonus,
      currencyDigits: paymentCurrencyFractionDigits(props.currency),
    }),
  })),
)

const showSecondLine = computed(() => props.bonusTiers.length > 0 || isRechargeFixedBonusEnabled(props.fixedBonus))

function badgeText(quote: RechargeBonusQuote): string {
  if (quote.source === 'fixed') return `+$${quote.bonus.toFixed(2)}`
  const percent = formatRechargeBonusNumber(quote.percent)
  return quote.source === 'discount' ? `${percent}% OFF` : `+${percent}%`
}

function secondLine(quote: RechargeBonusQuote): string {
  if (!quote.available) return t('payment.rechargeBonus.unavailable')
  if (quote.mode === 'discount') {
    return t('payment.rechargeBonus.payShort', { amount: formatPaymentAmount(quote.payBase, props.currency) })
  }
  return t('payment.rechargeBonus.creditedShort', { amount: '$' + quote.credited.toFixed(2) })
}

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const input = e.target as HTMLInputElement
  const val = input.value
  if (!AMOUNT_PATTERN.test(val)) {
    input.value = customText.value
    return
  }
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  if (Number.isFinite(num) && num > 0) {
    emit('update:modelValue', num)
  } else {
    emit('update:modelValue', null)
  }
}

watch(() => props.modelValue, (v) => {
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>
