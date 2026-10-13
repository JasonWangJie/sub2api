import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import InvoicesView from '../user/InvoicesView.vue'
import AdminInvoicesView from '../admin/AdminInvoicesView.vue'
import type { InvoiceRecord } from '@/api/payment'
import type { AdminInvoiceRecord } from '@/api/admin/invoices'

const { getInvoiceRecords, getRecords, createInvoiceApplication, markHistoricalRecords, showError, showSuccess } = vi.hoisted(() => ({
  getInvoiceRecords: vi.fn(),
  getRecords: vi.fn(),
  createInvoiceApplication: vi.fn(),
  markHistoricalRecords: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/payment', () => {
  const paymentAPI = {
    getInvoiceProfile: vi.fn().mockResolvedValue({ data: { email: 'invoice@example.com', tax_number: 'TAX', company_name: 'Example' } }),
    getInvoiceApplications: vi.fn().mockResolvedValue({ data: { items: [], total: 0 } }),
    getInvoiceRecords,
    createInvoiceApplication,
  }
  return { paymentAPI, default: paymentAPI }
})
vi.mock('@/api/admin/invoices', () => {
  const adminInvoiceAPI = {
    getApplications: vi.fn().mockResolvedValue({ data: { items: [], total: 0 } }),
    getRecords,
    markHistoricalRecords,
  }
  return { adminInvoiceAPI, default: adminInvoiceAPI }
})
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  const { default: common } = await import('@/i18n/locales/zh/common')
  const invoice = common.invoice as Record<string, unknown>
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const value = key.startsWith('invoice.') ? invoice[key.slice('invoice.'.length)] : undefined
        return typeof value === 'string' ? value.replace('{count}', String(params?.count ?? '')) : key
      },
    }),
  }
})

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
  Icon: true,
  Select: true,
  Pagination: {
    props: ['page'],
    emits: ['update:page'],
    template: '<div><button data-testid="previous-page" @click="$emit(\'update:page\', page - 1)">Previous</button><button data-testid="next-page" @click="$emit(\'update:page\', page + 1)">Next</button></div>',
  },
}

let wrappers: VueWrapper[] = []
let pages: Record<number, InvoiceRecord[]>

function row(source_type: string, source_id: number, amount: number, selectable: boolean): InvoiceRecord {
  return {
    source_type, source_id, source_reference: `RECHARGE-${source_id}`, amount, selectable,
    occurred_at: '2026-10-13T10:00:00Z',
    ...(!selectable ? { ineligible_reason: 'ADMIN_RECHARGE_MINIMUM_NOT_MET' } : {}),
  }
}

function button(wrapper: VueWrapper, label: string) {
  const target = wrapper.findAll('button').find((item) => item.text() === label)
  if (!target) throw new Error(`Button not found: ${label}`)
  return target
}

beforeEach(() => {
  vi.clearAllMocks()
  pages = {
    1: [row('admin_grant', 1, 100, true), row('admin_grant', 2, 99.99, false), row('admin_grant', 3, 99.99999999, false)],
    2: [row('payment_order', 4, 900, true)],
  }
  getInvoiceRecords.mockImplementation(({ page }: { page: number }) => Promise.resolve({ data: { items: pages[page], total: 4 } }))
  getRecords.mockImplementation(({ page }: { page: number }) => Promise.resolve({
    data: { items: pages[page].map((record): AdminInvoiceRecord => ({ ...record, user_id: 42, user_email: 'invoice@example.com' })), total: 4 },
  }))
  createInvoiceApplication.mockResolvedValue({ data: {} })
  markHistoricalRecords.mockResolvedValue({ data: {} })
})

afterEach(() => {
  wrappers.forEach((wrapper) => wrapper.unmount())
  wrappers = []
})

describe('admin recharge invoice eligibility', () => {
  it('disables small user-page records and submits 100 + 900 across pages', async () => {
    const wrapper = mount(InvoicesView, { global: { stubs } })
    wrappers.push(wrapper)
    await flushPromises()
    expect(wrapper.text()).toContain('管理员充值单笔达到 100.00 可开票')
    expect(wrapper.text()).toContain('管理员充值单笔不足 100.00，不可开票')
    const checkboxes = wrapper.findAll('tbody input[type="checkbox"]')
    expect(checkboxes.map((checkbox) => (checkbox.element as HTMLInputElement).disabled)).toEqual([false, true, true])
    await wrapper.get('thead input[type="checkbox"]').setValue(true)
    expect(checkboxes.map((checkbox) => (checkbox.element as HTMLInputElement).checked)).toEqual([true, false, false])
    expect(button(wrapper, '提交开票申请').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="next-page"]').trigger('click')
    await flushPromises()
    await wrapper.get('tbody input[type="checkbox"]').setValue(true)
    expect(button(wrapper, '提交开票申请').attributes('disabled')).toBeUndefined()
    await button(wrapper, '提交开票申请').trigger('click')
    await button(wrapper, '确认无误并提交').trigger('click')
    await flushPromises()
    expect(createInvoiceApplication).toHaveBeenCalledWith(expect.objectContaining({
      sources: [{ source_type: 'admin_grant', source_id: 1 }, { source_type: 'payment_order', source_id: 4 }],
    }))
  })

  it('drops a user-page selection when refreshing its page returns an applied record', async () => {
    const wrapper = mount(InvoicesView, { global: { stubs } })
    wrappers.push(wrapper)
    await flushPromises()
    await wrapper.get('thead input[type="checkbox"]').setValue(true)
    await wrapper.get('[data-testid="next-page"]').trigger('click')
    await flushPromises()
    await wrapper.get('tbody input[type="checkbox"]').setValue(true)
    pages[1] = [{ ...row('admin_grant', 1, 100, false), ineligible_reason: undefined, application_status: 'PENDING', application_no: 'INV-1' }]
    await wrapper.get('[data-testid="previous-page"]').trigger('click')
    await flushPromises()
    expect((wrapper.get('tbody input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.text()).toContain('申请中')
    expect(button(wrapper, '提交开票申请').attributes('disabled')).toBeDefined()
    expect(createInvoiceApplication).not.toHaveBeenCalled()
  })

  it('backfills a single eligible 100 record while excluding smaller records from select-all', async () => {
    const wrapper = mount(AdminInvoicesView, { global: { stubs } })
    wrappers.push(wrapper)
    await flushPromises()
    await button(wrapper, '历史开票补录').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('管理员充值单笔不足 100.00，不可开票')
    expect(wrapper.text()).toContain('不可开票')
    expect(wrapper.findAll('tbody input[type="checkbox"]').map((checkbox) => (checkbox.element as HTMLInputElement).disabled)).toEqual([false, true, true])
    await wrapper.get('thead input[type="checkbox"]').setValue(true)
    await button(wrapper, '标记已开票').trigger('click')
    await button(wrapper, '确认标记且不可撤销').trigger('click')
    await flushPromises()
    expect(markHistoricalRecords).toHaveBeenCalledWith([{ source_type: 'admin_grant', source_id: 1 }])
  })

  it('preserves admin selections across pages and removes newly backfilled records on refresh', async () => {
    const wrapper = mount(AdminInvoicesView, { global: { stubs } })
    wrappers.push(wrapper)
    await flushPromises()
    await button(wrapper, '历史开票补录').trigger('click')
    await flushPromises()
    await wrapper.get('thead input[type="checkbox"]').setValue(true)
    await wrapper.get('[data-testid="next-page"]').trigger('click')
    await flushPromises()
    await wrapper.get('thead input[type="checkbox"]').setValue(true)
    await button(wrapper, '标记已开票').trigger('click')
    await button(wrapper, '确认标记且不可撤销').trigger('click')
    await flushPromises()
    expect(markHistoricalRecords).toHaveBeenCalledWith([{ source_type: 'admin_grant', source_id: 1 }, { source_type: 'payment_order', source_id: 4 }])

    await wrapper.get('[data-testid="previous-page"]').trigger('click')
    await flushPromises()
    await wrapper.get('thead input[type="checkbox"]').setValue(true)
    pages[1] = [{ ...row('admin_grant', 1, 100, false), ineligible_reason: 'HISTORICAL_INVOICE_COMPLETED', application_status: 'HISTORICAL_COMPLETED' }]
    await button(wrapper, 'common.refresh').trigger('click')
    await flushPromises()
    expect(button(wrapper, '标记已开票').attributes('disabled')).toBeDefined()
    expect((wrapper.get('tbody input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.text()).toContain('已开票（历史补录）')
    expect(wrapper.text()).not.toContain('HISTORICAL_INVOICE_COMPLETED')
  })
})
