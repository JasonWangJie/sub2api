import { apiClient } from '../client'
import type { InvoiceApplication } from '@/api/payment'
import type { BasePaginationResponse } from '@/types'

export interface AdminInvoiceRecord {
  user_id: number
  user_email?: string
  user_name?: string
  source_type: string
  source_id: number
  source_reference: string
  amount: number
  occurred_at: string
  selectable: boolean
  ineligible_reason?: string
  application_no?: string
  application_status?: string
  marked_at?: string
  marked_by?: number
  marked_by_email?: string
}

export const adminInvoiceAPI = {
  getApplications(params?: { page?: number; page_size?: number; status?: string; keyword?: string }) {
    return apiClient.get<BasePaginationResponse<InvoiceApplication>>('/admin/payment/invoices', { params })
  },
  getRecords(params?: { page?: number; page_size?: number; user_id?: number; keyword?: string; invoice_status?: string }) {
    return apiClient.get<BasePaginationResponse<AdminInvoiceRecord>>('/admin/payment/invoices/records', { params })
  },
  markHistoricalRecords(sources: Array<{ source_type: string; source_id: number }>) {
    return apiClient.post('/admin/payment/invoices/historical-marks', { sources }, {
      headers: { 'Idempotency-Key': createHistoricalOperationKey(sources) }
    })
  },
  getApplication(id: number) {
    return apiClient.get<InvoiceApplication>(`/admin/payment/invoices/${id}`)
  },
  completeApplication(id: number) {
    return apiClient.post(`/admin/payment/invoices/${id}/complete`, undefined, {
      headers: { 'Idempotency-Key': createInvoiceOperationKey('complete', id) }
    })
  },
  rejectApplication(id: number, reason: string) {
    return apiClient.post(`/admin/payment/invoices/${id}/reject`, { reason }, {
      headers: { 'Idempotency-Key': createInvoiceOperationKey('reject', id) }
    })
  },
}

function createInvoiceOperationKey(action: string, id: number | string): string {
  const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `invoice-${action}-${id}-${requestID}`
}

function createHistoricalOperationKey(sources: Array<{ source_type: string; source_id: number }>): string {
  const input = sources.map((source) => `${source.source_type}:${source.source_id}`).sort().join(',')
  let hash = 2166136261
  for (let index = 0; index < input.length; index += 1) {
    hash ^= input.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return `invoice-historical-marks-${(hash >>> 0).toString(16)}`
}

export default adminInvoiceAPI
