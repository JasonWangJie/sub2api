import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OpenAI429ModeStatus from '../OpenAI429ModeStatus.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => `${key}${params ? JSON.stringify(params) : ''}` }) }))

describe('OpenAI429ModeStatus', () => {
  it('shows the server phase, consecutive count and reliable recovery time', () => {
    const wrapper = mount(OpenAI429ModeStatus, { props: { account: { extra: {
      openai_429_mode_enabled: true,
      openai_429_mode_state: { phase: 'stopped', consecutive_429: 10, recover_at: 2000000000000 }
    } } as any } })
    expect(wrapper.text()).toContain('admin.accounts.mode429.phases.stopped')
    expect(wrapper.text()).toContain('10/10')
    expect(wrapper.text()).toContain('admin.accounts.mode429.recovery')
  })

  it('uses the quota-query label for unknown recovery and hides disabled accounts', async () => {
    const wrapper = mount(OpenAI429ModeStatus, { props: { account: { extra: {
      openai_429_mode_enabled: true,
      openai_429_mode_state: { phase: 'stopped', consecutive_429: 10, unknown_reset: true }
    } } as any } })
    expect(wrapper.text()).toContain('admin.accounts.mode429.unknownRecovery')
    await wrapper.setProps({ account: { extra: { openai_429_mode_enabled: false } } as any })
    expect(wrapper.find('[data-testid="openai-429-mode-status"]').exists()).toBe(false)
  })
})
