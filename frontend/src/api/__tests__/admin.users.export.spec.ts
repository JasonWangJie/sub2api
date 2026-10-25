import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({ apiClient: { get } }))

import usersAPI, { exportUsers, list, type AdminUsersListFilters } from '@/api/admin/users'

describe('admin users export API', () => {
  beforeEach(() => get.mockReset())

  it('uses the list filters and sorting for an unpaginated CSV request', async () => {
    const filters: AdminUsersListFilters = {
      status: 'active', role: 'user', search: 'alice', group_name: 'VIP',
      has_exclusive_groups: true, api_key_group_id: 7,
      attributes: { 3: 'partner', 4: '', 5: '0' },
      include_subscriptions: true, sort_by: 'balance', sort_order: 'desc'
    }
    const controller = new AbortController()
    const blob = new Blob(['id,email,balance\n42,alice@example.com,123.456789'])
    get.mockResolvedValue({ data: blob })

    await list(3, 20, filters)
    const listParams = get.mock.calls[0][1].params
    const result = await exportUsers(filters, { signal: controller.signal })
    const exportOptions = get.mock.calls[1][1]
    const { page, page_size, ...matchingParams } = listParams

    expect(page).toBe(3)
    expect(page_size).toBe(20)
    expect(get.mock.calls[1][0]).toBe('/admin/users/export')
    expect(exportOptions.params).toEqual(matchingParams)
    expect(exportOptions.params).toEqual(expect.objectContaining({
      'attr[3]': 'partner', 'attr[5]': '0', api_key_group_id: 7,
      group_name: 'VIP', has_exclusive_groups: true, sort_by: 'balance', sort_order: 'desc'
    }))
    expect(exportOptions.params).not.toHaveProperty('attributes')
    expect(exportOptions.params).not.toHaveProperty('attr[4]')
    expect(exportOptions.params).not.toHaveProperty('page')
    expect(exportOptions.params).not.toHaveProperty('page_size')
    expect(exportOptions.responseType).toBe('blob')
    expect(exportOptions.timeout).toBe(0)
    expect(exportOptions.signal).toBe(controller.signal)
    expect(result).toBe(blob)
    expect(usersAPI.exportUsers).toBe(exportUsers)
  })

  it('exports every user without requiring filters', async () => {
    const blob = new Blob(['id,email,balance'])
    get.mockResolvedValue({ data: blob })

    expect(await exportUsers()).toBe(blob)
    expect(get).toHaveBeenCalledWith('/admin/users/export', expect.objectContaining({
      params: {}, responseType: 'blob'
    }))
  })
})
