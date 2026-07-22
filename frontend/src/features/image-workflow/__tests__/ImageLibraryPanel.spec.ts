import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const mocks = vi.hoisted(() => ({
  listImageLibrary: vi.fn(),
  resolveImageLibraryViewURL: vi.fn(),
  updateImageLibraryItem: vi.fn(),
  archiveAsyncTask: vi.fn(),
  importImageFile: vi.fn(),
  importImageURL: vi.fn(),
  deleteImageLibraryItem: vi.fn(),
  publishImageLibraryItem: vi.fn(),
  withdrawImageLibraryItem: vi.fn(),
  listPendingImageArchives: vi.fn(),
  removePendingImageArchive: vi.fn(),
  savePendingImageArchive: vi.fn(),
  onPendingImageArchivesChanged: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/imageLibrary', () => ({
  archiveAsyncTask: mocks.archiveAsyncTask,
  deleteImageLibraryItem: mocks.deleteImageLibraryItem,
  importImageFile: mocks.importImageFile,
  importImageURL: mocks.importImageURL,
  listImageLibrary: mocks.listImageLibrary,
  publishImageLibraryItem: mocks.publishImageLibraryItem,
  resolveImageLibraryViewURL: mocks.resolveImageLibraryViewURL,
  updateImageLibraryItem: mocks.updateImageLibraryItem,
  withdrawImageLibraryItem: mocks.withdrawImageLibraryItem,
}))

vi.mock('../archiveRecovery', () => ({
  listPendingImageArchives: mocks.listPendingImageArchives,
  removePendingImageArchive: mocks.removePendingImageArchive,
  savePendingImageArchive: mocks.savePendingImageArchive,
  onPendingImageArchivesChanged: mocks.onPendingImageArchivesChanged,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showSuccess: mocks.showSuccess, showError: mocks.showError }),
  useAuthStore: () => ({ user: { id: 19 } }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import ImageLibraryPanel from '../ImageLibraryPanel.vue'

const item = {
  id: 'img_1',
  source: 'realtime_import',
  platform: 'openai',
  execution_mode: 'realtime',
  model: 'gpt-image-1',
  title: 'Old private title',
  visibility: 'private',
  archive_status: 'ready',
  view_url: '/view/img_1',
  created_at: '2026-07-21T00:00:00Z',
}

function mountPanel() {
  return mount(ImageLibraryPanel, {
    global: {
      stubs: {
        Icon: { template: '<span />' },
        RouterLink: { template: '<a><slot /></a>' },
      },
    },
  })
}

describe('ImageLibraryPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listImageLibrary.mockResolvedValue({ items: [{ ...item }], next_cursor: null })
    mocks.resolveImageLibraryViewURL.mockResolvedValue({ url: 'https://cdn.example/img_1.png' })
    mocks.listPendingImageArchives.mockResolvedValue([])
    mocks.onPendingImageArchivesChanged.mockReturnValue(vi.fn())
    mocks.removePendingImageArchive.mockResolvedValue(undefined)
  })

  it('updates the private title without changing publication metadata', async () => {
    mocks.updateImageLibraryItem.mockResolvedValue({ ...item, title: 'New private title' })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.get('[aria-label="imageWorkflow.library.editPrivateTitle"]').trigger('click')
    const input = wrapper.get('input[aria-label="imageWorkflow.library.privateTitle"]')
    await input.setValue('  New private title  ')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(mocks.updateImageLibraryItem).toHaveBeenCalledWith('img_1', { title: 'New private title' })
    expect(wrapper.text()).toContain('New private title')
    expect(mocks.showSuccess).toHaveBeenCalledWith('imageWorkflow.library.titleUpdated')
    wrapper.unmount()
  })

  it('removes obsolete local task recovery because durable outbox owns async archival', async () => {
    mocks.listPendingImageArchives.mockResolvedValue([{
      id: 'archive_1',
      userId: 19,
      kind: 'task',
      title: 'Recovered task image',
      taskId: 'asyncimg_123',
      resultIndex: 2,
      createdAt: Date.now(),
      expiresAt: Date.now() + 60_000,
      errorMessage: 'storage temporarily unavailable',
    }])
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('.library-recovery__retry').exists()).toBe(false)
    expect(mocks.archiveAsyncTask).not.toHaveBeenCalled()
    expect(mocks.importImageFile).not.toHaveBeenCalled()
    expect(mocks.importImageURL).not.toHaveBeenCalled()
    expect(mocks.removePendingImageArchive).toHaveBeenCalledWith('archive_1')
    wrapper.unmount()
  })
})
