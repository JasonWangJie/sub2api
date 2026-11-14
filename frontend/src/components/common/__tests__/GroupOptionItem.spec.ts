import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupOptionItem from '../GroupOptionItem.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key === 'groups.maintenanceUnavailable' ? '（维护中·暂不可用）' : key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

describe('GroupOptionItem description layout', () => {
  it('shows a red maintenance suffix after the group name and removes it when maintenance ends', async () => {
    const wrapper = mount(GroupOptionItem, {
      props: { name: 'Example group', platform: 'openai', isMaintenance: true },
      global: { stubs: { PlatformIcon: true } },
    })

    const suffix = wrapper.get('.text-red-600')
    expect(suffix.text()).toBe('（维护中·暂不可用）')
    expect(suffix.classes()).toContain('dark:text-red-400')
    expect(suffix.element.parentElement?.textContent?.trim()).toBe('Example group（维护中·暂不可用）')

    await wrapper.setProps({ isMaintenance: false })
    expect(wrapper.text()).toContain('Example group')
    expect(wrapper.text()).not.toContain('（维护中·暂不可用）')
  })

  it('applies multiline and overflow-safe text styles', () => {
    const description = 'First section\nvery-long-unbroken-description-value-that-must-not-overflow'
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'Example group',
        platform: 'openai',
        description,
      },
      global: {
        stubs: {
          GroupBadge: true,
        },
      },
    })

    const descriptionElement = wrapper
      .findAll('span')
      .find((element) => element.text() === description)

    expect(descriptionElement).toBeDefined()
    expect(descriptionElement?.classes()).toContain('whitespace-pre-line')
    expect(descriptionElement?.classes()).toContain('[overflow-wrap:anywhere]')
    expect(descriptionElement?.classes()).toContain('line-clamp-3')
    expect(wrapper.find('[title]').attributes('title')).toBe(description)
  })
})
