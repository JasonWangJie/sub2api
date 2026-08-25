<template>
  <AppLayout>
    <div class="mx-auto max-w-2xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-emerald-500 border-t-transparent" />
      </div>

      <template v-else-if="paymentPhase === 'paying'">
        <PaymentStatusPanel
          :order-id="paymentState.orderId"
          :amount="paymentState.amount"
          :pay-amount="paymentState.payAmount"
          :qr-code="paymentState.qrCode"
          :expires-at="paymentState.expiresAt"
          :payment-type="paymentState.paymentType"
          :pay-url="paymentState.payUrl"
          :order-type="paymentState.orderType"
          :currency="paymentState.currency || 'CNY'"
          :out-trade-no="paymentState.outTradeNo"
          @success="handlePaymentSuccess"
          @done="resetPayment"
          @settled="clearRecovery"
        />
      </template>

      <template v-else>
        <section class="rounded-2xl border border-emerald-200 bg-white p-6 shadow-sm dark:border-emerald-900/60 dark:bg-dark-800">
          <div class="flex items-start justify-between gap-4">
            <div>
              <p class="text-xs font-semibold uppercase tracking-[0.18em] text-emerald-600 dark:text-emerald-400">USDT</p>
              <h1 class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ t('payment.usdtRechargeTitle') }}</h1>
              <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.usdtRechargeHint') }}</p>
            </div>
            <div class="rounded-xl bg-emerald-50 px-4 py-3 text-right dark:bg-emerald-950/30">
              <p class="text-xs text-emerald-700 dark:text-emerald-300">{{ t('payment.currentBalance') }}</p>
              <p class="mt-1 text-lg font-bold text-emerald-700 dark:text-emerald-300">${{ Number(user?.balance || 0).toFixed(2) }}</p>
            </div>
          </div>
        </section>

        <section v-if="!checkout.enabled" class="rounded-2xl border border-amber-200 bg-amber-50 p-6 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/20 dark:text-amber-200">
          {{ t('payment.usdtUnavailable') }}
        </section>

        <template v-else>
          <section class="rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-800">
            <label class="mb-3 block text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.usdtAmountLabel') }} ({{ checkout.currency }})</label>
            <div class="flex items-center rounded-xl border border-gray-300 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-900">
              <span class="mr-3 text-lg font-semibold text-gray-400">{{ inputSymbol }}</span>
              <input v-model.number="amount" type="number" min="0.01" step="0.01" class="w-full bg-transparent text-2xl font-bold text-gray-900 outline-none dark:text-white" :placeholder="t('payment.enterAmount')" />
            </div>
            <div class="mt-3 flex flex-wrap gap-2">
              <button v-for="quick in quickAmounts" :key="quick" type="button" class="rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 hover:border-emerald-400 hover:text-emerald-600 dark:border-dark-600 dark:text-gray-300" @click="amount = quick">{{ inputSymbol }}{{ quick }}</button>
            </div>
            <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ amountHint }}</p>
          </section>

          <section class="rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-800">
            <label class="mb-3 block text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.usdtNetworkLabel') }}</label>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
              <button v-for="network in checkout.networks" :key="network.code" type="button" :class="['rounded-xl border px-4 py-3 text-sm font-semibold uppercase transition-colors', selectedNetwork === network.code ? 'border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300' : 'border-gray-200 text-gray-600 hover:border-emerald-300 dark:border-dark-600 dark:text-gray-300']" @click="selectedNetwork = network.code">{{ network.display_name || network.code }}</button>
            </div>
            <p class="mt-3 text-xs font-semibold leading-relaxed text-red-600 dark:text-red-400">{{ t('payment.usdtNetworkWarning') }}</p>
          </section>

          <section class="rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-800">
            <div class="flex justify-between text-sm text-gray-500 dark:text-gray-400"><span>{{ t('payment.usdtPaymentAmount') }}</span><span class="font-semibold text-gray-900 dark:text-white">{{ paymentAmountLabel }}</span></div>
            <div v-if="checkout.exchange_rate > 0" class="mt-2 flex justify-between text-sm text-gray-500 dark:text-gray-400"><span>{{ t('payment.usdtExchangeRate') }}</span><span>1 USDT = {{ checkout.exchange_rate.toFixed(6) }} CNY</span></div>
            <div v-if="checkout.exchange_rate > 0" class="mt-2 flex justify-between text-sm text-gray-500 dark:text-gray-400"><span>{{ t('payment.usdtConvertedAmount') }}</span><span>¥{{ convertedCNY.toFixed(2) }}</span></div>
            <div v-if="checkout.bonus_rate > 0" class="mt-2 flex justify-between text-sm text-gray-500 dark:text-gray-400"><span>{{ t('payment.usdtBonus') }} {{ checkout.bonus_rate }}%</span><span>+¥{{ bonusAmount.toFixed(2) }}</span></div>
            <div class="mt-2 flex justify-between text-sm text-gray-500 dark:text-gray-400"><span>{{ t('payment.usdtExpectedCNY') }}</span><span>¥{{ creditedCNY.toFixed(2) }}</span></div>
            <div v-if="feeAmount > 0" class="mt-2 flex justify-between text-sm text-gray-500 dark:text-gray-400"><span>{{ t('payment.fee') }} ({{ checkout.fee_rate }}%)</span><span>{{ inputSymbol }}{{ feeAmount.toFixed(2) }}</span></div>
            <div class="mt-2 flex justify-between border-t border-gray-100 pt-3 text-base dark:border-dark-700"><span class="font-semibold text-gray-700 dark:text-gray-200">{{ t('payment.usdtExpectedBalance') }}</span><span class="font-bold text-emerald-600 dark:text-emerald-400">${{ creditedAmount.toFixed(2) }}</span></div>
            <p v-if="checkout.exchange_rate_at" class="mt-3 text-right text-xs text-gray-400 dark:text-gray-500">{{ checkout.exchange_rate_source || 'Coinbase' }} USDT/CNY · {{ formatQuoteTime(checkout.exchange_rate_at) }}<span v-if="checkout.exchange_rate_stale"> · {{ t('payment.usdtRateStale') }}</span></p>
          </section>

          <p v-if="errorMessage" class="text-sm text-red-600 dark:text-red-400">{{ errorMessage }}</p>
          <button type="button" class="w-full rounded-xl bg-emerald-600 py-3.5 text-base font-semibold text-white transition-colors hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-50" :disabled="!canSubmit || submitting" @click="submit">
            {{ submitting ? t('common.processing') : t('payment.usdtCreateOrder') }}
          </button>
        </template>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { paymentAPI } from '@/api/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY, createPaymentRecoverySnapshot, readPaymentRecoverySnapshot, writePaymentRecoverySnapshot, clearPaymentRecoverySnapshot, type PaymentRecoverySnapshot } from '@/components/payment/paymentFlow'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { USDTCheckoutInfo, USDTOrderQuote } from '@/types/payment'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const amount = ref<number | null>(null)
const selectedNetwork = ref('')
const paymentPhase = ref<'select' | 'paying'>('select')
const paymentState = ref<PaymentRecoverySnapshot>(emptyState())
const checkout = ref<USDTCheckoutInfo>({ enabled: false, currency: 'USDT', min_amount: 0, max_amount: 0, daily_limit: 0, fee_rate: 0, balance_recharge_multiplier: 1, bonus_rate: 0, exchange_rate: 0, exchange_rate_source: 'Coinbase', exchange_rate_at: '', exchange_rate_stale: false, networks: [] })
const usdtQuote = ref<USDTOrderQuote | null>(null)
const quickAmounts = [50, 100, 200, 500, 1000]

function emptyState(): PaymentRecoverySnapshot {
  return { orderId: 0, amount: 0, qrCode: '', expiresAt: '', paymentType: 'usdt', payUrl: '', outTradeNo: '', clientSecret: '', intentId: '', currency: 'USDT', countryCode: '', paymentEnv: '', payAmount: 0, orderType: 'balance', paymentMode: '', resumeToken: '', createdAt: 0 }
}

const baseAmount = computed(() => Math.max(0, Number(amount.value || 0)))
const feeAmount = computed(() => checkout.value.fee_rate > 0 ? Math.ceil(baseAmount.value * checkout.value.fee_rate) / 100 : 0)
const payableAmount = computed(() => Math.round((baseAmount.value + feeAmount.value) * 100) / 100)
const convertedCNY = computed(() => checkout.value.currency === 'USDT' ? round2(baseAmount.value * checkout.value.exchange_rate) : round2(baseAmount.value))
const bonusAmount = computed(() => round2(convertedCNY.value * (checkout.value.bonus_rate || 0) / 100))
const creditedCNY = computed(() => round2(convertedCNY.value + bonusAmount.value))
const creditedAmount = computed(() => round2(creditedCNY.value * (checkout.value.balance_recharge_multiplier || 1)))
const inputSymbol = computed(() => checkout.value.currency === 'USDT' ? '₮' : '¥')
const paymentAmountLabel = computed(() => {
  if (checkout.value.currency === 'USDT') return `${payableAmount.value.toFixed(2)} USDT`
  if (checkout.value.exchange_rate > 0) return `${(payableAmount.value / checkout.value.exchange_rate).toFixed(2)} USDT`
  return `${payableAmount.value.toFixed(2)} CNY`
})
const canSubmit = computed(() => checkout.value.enabled && baseAmount.value > 0 && (!checkout.value.min_amount || baseAmount.value >= checkout.value.min_amount) && (!checkout.value.max_amount || baseAmount.value <= checkout.value.max_amount) && !!selectedNetwork.value)
const amountHint = computed(() => {
  const symbol = checkout.value.currency === 'USDT' ? '₮' : '¥'
  const min = checkout.value.min_amount > 0 ? `${symbol}${checkout.value.min_amount.toFixed(2)}` : `${symbol}0.01`
  const max = checkout.value.max_amount > 0 ? ` - ${symbol}${checkout.value.max_amount.toFixed(2)}` : ''
  return t('payment.usdtAmountHint', { range: `${min}${max}` })
})

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  errorMessage.value = ''
  try {
    const result = await paymentAPI.createUSDTOrder({ amount: baseAmount.value, payment_type: 'usdt', order_type: 'balance', network: selectedNetwork.value, return_url: `${window.location.origin}/payment/result`, payment_source: 'usdt_recharge', is_mobile: /Mobi|Android/i.test(navigator.userAgent) })
    usdtQuote.value = result.data.usdt_quote || null
    if (result.data.usdt_quote) {
      checkout.value = {
        ...checkout.value,
        currency: result.data.usdt_quote.currency,
        bonus_rate: result.data.usdt_quote.bonus_rate,
        exchange_rate: result.data.usdt_quote.exchange_rate,
        exchange_rate_at: result.data.usdt_quote.exchange_rate_at,
      }
    }
    const snapshot = createPaymentRecoverySnapshot({ ...emptyState(), orderId: result.data.order_id, amount: result.data.amount, payAmount: result.data.pay_amount, expiresAt: result.data.expires_at, payUrl: result.data.pay_url || '', outTradeNo: result.data.out_trade_no || '', currency: result.data.currency || checkout.value.currency, usdtQuote: result.data.usdt_quote })
    paymentState.value = snapshot
    writePaymentRecoverySnapshot(window.localStorage, snapshot, PAYMENT_RECOVERY_STORAGE_KEY)
    if (snapshot.payUrl) {
      window.location.assign(snapshot.payUrl)
    } else {
      paymentPhase.value = 'paying'
    }
  } catch (error: unknown) {
    errorMessage.value = extractApiErrorMessage(error, t('payment.usdtCreateFailed'))
  } finally {
    submitting.value = false
  }
}

function clearRecovery() { clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY) }
function resetPayment() { clearRecovery(); paymentPhase.value = 'select'; paymentState.value = emptyState() }
async function handlePaymentSuccess() { clearRecovery(); await authStore.refreshUser(); router.push({ path: '/payment/result', query: { order_id: String(paymentState.value.orderId) } }) }

onMounted(async () => {
  try {
    const recovery = readPaymentRecoverySnapshot(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY))
    if (recovery?.paymentType === 'usdt' && recovery.orderType === 'balance' && recovery.expiresAt && new Date(recovery.expiresAt).getTime() > Date.now()) {
      paymentState.value = recovery
      paymentPhase.value = 'paying'
    }
    const { data } = await paymentAPI.getUSDTCheckoutInfo()
    checkout.value = data
    selectedNetwork.value = data.networks[0]?.code || ''
    startRateRefresh()
  } catch (error: unknown) {
    errorMessage.value = extractApiErrorMessage(error, t('payment.usdtUnavailable'))
  } finally { loading.value = false }
})

let rateRefreshTimer: ReturnType<typeof setInterval> | null = null
function startRateRefresh() {
  if (rateRefreshTimer) clearInterval(rateRefreshTimer)
  rateRefreshTimer = setInterval(async () => {
    try {
      const { data } = await paymentAPI.getUSDTCheckoutInfo()
      checkout.value = data
      if (!data.networks.some(network => network.code === selectedNetwork.value)) {
        selectedNetwork.value = data.networks[0]?.code || ''
      }
    } catch { /* keep the last quote while the server cache is valid */ }
  }, 60_000)
}
function round2(value: number) { return Math.round(value * 100) / 100 }
function formatQuoteTime(value: string) { const date = new Date(value); return Number.isFinite(date.getTime()) ? date.toLocaleString() : value }
onUnmounted(() => { if (rateRefreshTimer) clearInterval(rateRefreshTimer) })
</script>
