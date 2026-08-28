/**
 * User Payment API endpoints
 * Handles payment operations for regular users
 */

import { apiClient } from './client'
import type {
  PaymentConfig,
  SubscriptionPlan,
  MethodLimitsResponse,
  CheckoutInfoResponse,
  CreateOrderRequest,
  CreateOrderResult,
  PaymentOrder,
  USDTCheckoutInfo
} from '@/types/payment'
import type { BasePaginationResponse } from '@/types'

export interface InvoiceRecord {
  source_type: 'payment_order' | 'redeem_code' | 'admin_grant' | 'affiliate_transfer' | string
  source_id: number
  source_reference: string
  amount: number
  occurred_at: string
  selectable: boolean
  ineligible_reason?: string
  application_no?: string
  application_status?: string
}

export interface InvoiceProfile {
  email: string
  tax_number: string
  company_name: string
}

export interface InvoiceApplicationItem {
  source_type: string
  source_id: number
  source_reference: string
  amount: number
}

export interface InvoiceApplication {
  id: number
  application_no: string
  user_id: number
  user_email?: string
  email: string
  tax_number: string
  company_name: string
  total_amount: number
  status: 'PENDING' | 'COMPLETED' | 'REJECTED' | string
  rejection_reason?: string
  created_at: string
  completed_at?: string
  items?: InvoiceApplicationItem[]
}

export interface PublicOrderVerifyResult {
  id?: number
  out_trade_no: string
  amount?: number
  pay_amount?: number
  fee_rate?: number
  currency?: string
  payment_type?: string
  order_type?: string
  status: string
  paid: boolean
  created_at: string
  expires_at: string
  usdt_quote?: import('@/types/payment').USDTOrderQuote
}

export const paymentAPI = {
  /** Get payment configuration (enabled types, limits, etc.) */
  getConfig() {
    return apiClient.get<PaymentConfig>('/payment/config')
  },

  /** Get available subscription plans */
  getPlans() {
    return apiClient.get<SubscriptionPlan[]>('/payment/plans')
  },

  /** Get all checkout page data in a single call */
  getCheckoutInfo() {
    return apiClient.get<CheckoutInfoResponse>('/payment/checkout-info')
  },

  getUSDTCheckoutInfo() {
    return apiClient.get<USDTCheckoutInfo>('/payment/usdt/checkout-info')
  },

  /** Get payment method limits and fee rates */
  getLimits() {
    return apiClient.get<MethodLimitsResponse>('/payment/limits')
  },

  /** Create a new payment order */
  createOrder(data: CreateOrderRequest) {
    return apiClient.post<CreateOrderResult>('/payment/orders', data)
  },

  createUSDTOrder(data: CreateOrderRequest) {
    return apiClient.post<CreateOrderResult>('/payment/usdt/orders', { ...data, payment_type: 'usdt', order_type: 'balance' })
  },

  /** Get current user's orders */
  getMyOrders(params?: { page?: number; page_size?: number; status?: string }) {
    return apiClient.get<BasePaginationResponse<PaymentOrder>>('/payment/orders/my', { params })
  },

  /** Get a specific order by ID */
  getOrder(id: number) {
    return apiClient.get<PaymentOrder>(`/payment/orders/${id}`)
  },

  /** Cancel a pending order */
  cancelOrder(id: number) {
    return apiClient.post(`/payment/orders/${id}/cancel`)
  },

  /** Verify order payment status with upstream provider */
  verifyOrder(outTradeNo: string) {
    return apiClient.post<PaymentOrder>('/payment/orders/verify', { out_trade_no: outTradeNo })
  },

  /** Legacy-compatible public order lookup by out_trade_no */
  verifyOrderPublic(outTradeNo: string) {
    return apiClient.post<PublicOrderVerifyResult>('/payment/public/orders/verify', { out_trade_no: outTradeNo })
  },

  /** Resolve an order from a signed resume token without auth */
  resolveOrderPublicByResumeToken(resumeToken: string) {
    return apiClient.post<PublicOrderVerifyResult>('/payment/public/orders/resolve', { resume_token: resumeToken })
  },

  /** Request a refund for a completed order */
  requestRefund(id: number, data: { reason: string }) {
    return apiClient.post(`/payment/orders/${id}/refund-request`, data)
  },

  /** Get provider instance IDs that allow user refund */
  getRefundEligibleProviders() {
    return apiClient.get<{ provider_instance_ids: string[] }>('/payment/orders/refund-eligible-providers')
  },

  getInvoiceProfile() {
    return apiClient.get<InvoiceProfile>('/payment/invoices/profile')
  },

  getInvoiceRecords(params?: { page?: number; page_size?: number }) {
    return apiClient.get<BasePaginationResponse<InvoiceRecord>>('/payment/invoices/records', { params })
  },

  getInvoiceApplications(params?: { page?: number; page_size?: number }) {
    return apiClient.get<BasePaginationResponse<InvoiceApplication>>('/payment/invoices/applications', { params })
  },

  createInvoiceApplication(data: InvoiceProfile & { sources: Array<{ source_type: string; source_id: number }> }) {
    const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    return apiClient.post<InvoiceApplication>('/payment/invoices/applications', data, {
      headers: { 'Idempotency-Key': `invoice-application-${requestID}` }
    })
  }
}
