<template>
  <AppLayout>
    <div class="space-y-4">
      <section class="card p-4 sm:p-5">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('invoice.profile') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('invoice.profileSaved') }}</p>
          </div>
          <div class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-200">
            {{ t('invoice.excludedGrant') }}；{{ t('invoice.excludedAffiliate') }}
          </div>
        </div>
        <div class="mt-4 grid gap-4 md:grid-cols-3">
          <label>
            <span class="input-label">{{ t('invoice.email') }}</span>
            <input v-model.trim="profile.email" type="email" class="input mt-1 w-full" autocomplete="email" />
          </label>
          <label>
            <span class="input-label">{{ t('invoice.taxNumber') }}</span>
            <input v-model.trim="profile.tax_number" type="text" class="input mt-1 w-full" />
          </label>
          <label>
            <span class="input-label">{{ t('invoice.companyName') }}</span>
            <input v-model.trim="profile.company_name" type="text" class="input mt-1 w-full" />
          </label>
        </div>
      </section>

      <section class="card overflow-hidden">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:px-5">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('invoice.records') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('invoice.minimumHint') }}</p>
          </div>
          <div class="flex items-center gap-3">
            <div class="text-right">
              <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.totalSelected') }}</div>
              <div :class="['text-xl font-semibold', selectedTotal >= minimumAmount ? 'text-primary-600 dark:text-primary-400' : 'text-gray-900 dark:text-white']">{{ selectedTotal.toFixed(2) }}</div>
            </div>
            <button class="btn btn-primary inline-flex items-center gap-2" :disabled="submitting || selectedTotal < minimumAmount || selectedRecords.length === 0" @click="openConfirm">
              <Icon name="document" size="sm" />
              {{ submitting ? t('common.processing') : t('invoice.submit') }}
            </button>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[760px] divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="w-12 px-4 py-3 text-center">
                  <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" :checked="allSelectableSelected" :disabled="selectableRecords.length === 0" @change="toggleAll(($event.target as HTMLInputElement).checked)" />
                </th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('invoice.source') }}</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('invoice.applicationNo') }}</th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('invoice.amount') }}</th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('invoice.date') }}</th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('invoice.status') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr v-if="loading"><td colspan="6" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td></tr>
              <tr v-else-if="records.length === 0"><td colspan="6" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('invoice.noRecords') }}</td></tr>
              <tr v-for="record in records" v-else :key="recordKey(record)" :class="record.selectable ? 'hover:bg-gray-50 dark:hover:bg-dark-800' : 'bg-gray-50/60 dark:bg-dark-800/40'">
                <td class="px-4 py-3 text-center">
                  <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40" :checked="isSelected(record)" :disabled="!record.selectable" :title="record.ineligible_reason || ''" @change="toggleRecord(record, ($event.target as HTMLInputElement).checked)" />
                </td>
                <td class="px-4 py-3 text-sm text-gray-900 dark:text-gray-100">
                  <div class="font-medium">{{ sourceLabel(record.source_type) }}</div>
                  <div class="font-mono text-xs text-gray-500 dark:text-gray-400">{{ record.source_reference }}</div>
                </td>
                <td class="px-4 py-3 font-mono text-sm text-gray-700 dark:text-gray-300">{{ record.application_no || '-' }}</td>
                <td class="px-4 py-3 text-right text-sm font-medium text-gray-900 dark:text-gray-100">{{ record.amount.toFixed(2) }}</td>
                <td class="px-4 py-3 text-right text-sm text-gray-500 dark:text-gray-400">{{ formatDate(record.occurred_at) }}</td>
                <td class="px-4 py-3 text-right text-sm">
                  <span v-if="record.selectable" class="text-emerald-600 dark:text-emerald-400">{{ t('invoice.selectable') }}</span>
                  <span v-else class="text-gray-500 dark:text-gray-400" :title="record.ineligible_reason || ''">
                    <span>{{ record.application_status ? statusLabel(record.application_status) : t('invoice.excluded') }}</span>
                    <span v-if="record.ineligible_reason" class="mt-0.5 block text-xs text-amber-600 dark:text-amber-400">{{ reasonLabel(record.ineligible_reason) }}</span>
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="changePage" @update:pageSize="changePageSize" />
      </section>

      <section class="card overflow-hidden">
        <div class="border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:px-5"><h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('invoice.applications') }}</h2></div>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[640px] divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800"><tr><th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">{{ t('invoice.applicationNo') }}</th><th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">{{ t('invoice.totalSelected') }}</th><th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">{{ t('invoice.status') }}</th><th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">{{ t('invoice.rejectReason') }}</th><th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">{{ t('invoice.date') }}</th></tr></thead>
            <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr v-if="applicationsLoading"><td colspan="5" class="px-4 py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</td></tr>
              <tr v-else-if="applications.length === 0"><td colspan="5" class="px-4 py-8 text-center text-sm text-gray-500">{{ t('invoice.noRecords') }}</td></tr>
              <tr v-for="application in applications" v-else :key="application.id"><td class="px-4 py-3 font-mono text-sm text-gray-900 dark:text-gray-100">{{ application.application_no }}</td><td class="px-4 py-3 text-right text-sm text-gray-900 dark:text-gray-100">{{ application.total_amount.toFixed(2) }}</td><td class="px-4 py-3 text-right text-sm" :class="statusClass(application.status)">{{ statusLabel(application.status) }}</td><td class="max-w-xs break-words px-4 py-3 text-left text-sm text-red-600 dark:text-red-400"><span v-if="application.status === 'REJECTED'">{{ application.rejection_reason || '-' }}</span><span v-else class="text-gray-400 dark:text-gray-500">-</span></td><td class="px-4 py-3 text-right text-sm text-gray-500 dark:text-gray-400">{{ formatDate(application.created_at) }}</td></tr>
            </tbody>
          </table>
        </div>
        <Pagination v-if="appPagination.total > 0" :page="appPagination.page" :total="appPagination.total" :page-size="appPagination.page_size" @update:page="changeApplicationPage" @update:pageSize="changeApplicationPageSize" />
      </section>
    </div>

    <BaseDialog :show="showConfirm" :title="t('invoice.confirmTitle')" width="normal" @close="showConfirm = false">
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('invoice.confirmMessage') }}</p>
        <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 rounded-lg bg-gray-50 p-4 text-sm dark:bg-dark-800">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.email') }}</dt><dd class="break-all text-right text-gray-900 dark:text-gray-100">{{ profile.email }}</dd>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.taxNumber') }}</dt><dd class="break-all text-right text-gray-900 dark:text-gray-100">{{ profile.tax_number }}</dd>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.companyName') }}</dt><dd class="break-all text-right text-gray-900 dark:text-gray-100">{{ profile.company_name }}</dd>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.totalSelected') }}</dt><dd class="text-right font-semibold text-primary-600 dark:text-primary-400">{{ selectedTotal.toFixed(2) }}</dd>
        </dl>
      </div>
      <template #footer><div class="flex justify-end gap-3"><button class="btn btn-secondary" @click="showConfirm = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="submitting" @click="submitApplication">{{ submitting ? t('common.processing') : t('invoice.confirmSubmit') }}</button></div></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { paymentAPI, type InvoiceApplication, type InvoiceProfile, type InvoiceRecord } from '@/api/payment'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const minimumAmount = 1000
const loading = ref(false)
const applicationsLoading = ref(false)
const submitting = ref(false)
const showConfirm = ref(false)
const records = ref<InvoiceRecord[]>([])
const applications = ref<InvoiceApplication[]>([])
const selectedKeys = ref<string[]>([])
const selectedRecordMap = reactive<Record<string, InvoiceRecord>>({})
const profile = reactive<InvoiceProfile>({ email: '', tax_number: '', company_name: '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const appPagination = reactive({ page: 1, page_size: 10, total: 0 })

const selectableRecords = computed(() => records.value.filter((record) => record.selectable))
const selectedRecords = computed(() => selectedKeys.value.map((key) => selectedRecordMap[key]).filter(Boolean))
const selectedTotal = computed(() => selectedRecords.value.reduce((sum, record) => sum + Number(record.amount || 0), 0))
const allSelectableSelected = computed(() => selectableRecords.value.length > 0 && selectableRecords.value.every((record) => selectedKeys.value.includes(recordKey(record))))

function recordKey(record: InvoiceRecord) { return `${record.source_type}:${record.source_id}` }
function isSelected(record: InvoiceRecord) { return selectedKeys.value.includes(recordKey(record)) }
function toggleRecord(record: InvoiceRecord, checked: boolean) {
  if (!record.selectable) return
  const next = new Set(selectedKeys.value)
  const key = recordKey(record)
  if (checked) { next.add(key); selectedRecordMap[key] = record } else { next.delete(key); delete selectedRecordMap[key] }
  selectedKeys.value = Array.from(next)
}
function toggleAll(checked: boolean) {
  const next = new Set(selectedKeys.value)
  for (const record of selectableRecords.value) {
    const key = recordKey(record)
    if (checked) { next.add(key); selectedRecordMap[key] = record } else { next.delete(key); delete selectedRecordMap[key] }
  }
  selectedKeys.value = Array.from(next)
}
function sourceLabel(type: string) { return t(`invoice.source${type === 'payment_order' ? 'Payment' : type === 'redeem_code' ? 'Redeem' : type === 'admin_grant' ? 'Grant' : 'Affiliate'}`) }
function statusLabel(status: string) { return status === 'PENDING' ? t('invoice.pending') : status === 'COMPLETED' ? t('invoice.completed') : status === 'REJECTED' ? t('invoice.rejected') : status === 'HISTORICAL_COMPLETED' ? t('invoice.historicalCompleted') : status }
function statusClass(status: string) { return status === 'COMPLETED' ? 'text-emerald-600 dark:text-emerald-400' : status === 'HISTORICAL_COMPLETED' ? 'text-sky-600 dark:text-sky-400' : status === 'REJECTED' ? 'text-red-600 dark:text-red-400' : 'text-amber-600 dark:text-amber-400' }
function reasonLabel(reason: string) { return reason === 'ADMIN_GRANT_EXCLUDED' ? t('invoice.excludedGrant') : reason === 'AFFILIATE_TRANSFER_EXCLUDED' ? t('invoice.excludedAffiliate') : reason === 'HISTORICAL_INVOICE_COMPLETED' ? '' : reason }
function formatDate(value: string) { return value ? new Date(value).toLocaleString() : '-' }

async function loadRecords() {
  loading.value = true
  try {
    const response = await paymentAPI.getInvoiceRecords({ page: pagination.page, page_size: pagination.page_size })
    records.value = response.data.items || []
    pagination.total = response.data.total || 0
    for (const record of records.value) {
      const key = recordKey(record)
      if (!record.selectable) { delete selectedRecordMap[key]; selectedKeys.value = selectedKeys.value.filter((selectedKey) => selectedKey !== key) }
    }
  }
  catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('invoice.errors.load'))) }
  finally { loading.value = false }
}
async function loadApplications() {
  applicationsLoading.value = true
  try { const response = await paymentAPI.getInvoiceApplications({ page: appPagination.page, page_size: appPagination.page_size }); applications.value = response.data.items || []; appPagination.total = response.data.total || 0 }
  catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('invoice.errors.load'))) }
  finally { applicationsLoading.value = false }
}
function changePage(page: number) { pagination.page = page; loadRecords() }
function changePageSize(size: number) { pagination.page_size = size; pagination.page = 1; loadRecords() }
function changeApplicationPage(page: number) { appPagination.page = page; loadApplications() }
function changeApplicationPageSize(size: number) { appPagination.page_size = size; appPagination.page = 1; loadApplications() }
function openConfirm() {
  if (selectedTotal.value < minimumAmount) { appStore.showError(t('invoice.errors.minimum')); return }
  if (!profile.email || !profile.tax_number || !profile.company_name) { appStore.showError(t('invoice.confirmMessage')); return }
  showConfirm.value = true
}
async function submitApplication() {
  if (!selectedRecords.value.length || selectedTotal.value < minimumAmount) return
  submitting.value = true
  try {
    await paymentAPI.createInvoiceApplication({ ...profile, sources: selectedRecords.value.map((record) => ({ source_type: record.source_type, source_id: record.source_id })) })
    showConfirm.value = false; selectedKeys.value = []; Object.keys(selectedRecordMap).forEach((key) => delete selectedRecordMap[key]); appStore.showSuccess(t('common.success')); await Promise.all([loadRecords(), loadApplications()])
  } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('invoice.errors.submit'))) }
  finally { submitting.value = false }
}

onMounted(async () => {
  try { const response = await paymentAPI.getInvoiceProfile(); Object.assign(profile, response.data || {}) }
  catch { /* profile is optional */ }
  await Promise.all([loadRecords(), loadApplications()])
})
</script>
