<template>
  <div v-if="enabled" class="mt-1 space-y-0.5 text-xs" data-testid="openai-429-mode-status">
    <div class="flex items-center gap-1.5" :class="phase === 'stopped' ? 'text-amber-700 dark:text-amber-400' : 'text-gray-600 dark:text-gray-300'">
      <span class="inline-block h-1.5 w-1.5 rounded-full" :class="phase === 'stopped' ? 'bg-amber-500' : 'bg-primary-500'" aria-hidden="true" />
      <span>{{ t('admin.accounts.mode429.title') }} · {{ t(`admin.accounts.mode429.phases.${phase}`) }}</span>
      <span class="font-mono tabular-nums">{{ count }}/10</span>
    </div>
    <div v-if="phase === 'stopped'" class="pl-3 text-gray-500 dark:text-gray-400">
      {{ recovery ? t('admin.accounts.mode429.recovery', { time: recovery }) : t('admin.accounts.mode429.unknownRecovery') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const enabled = computed(() => props.account.extra?.openai_429_mode_enabled === true)
const state = computed(() => props.account.extra?.openai_429_mode_state as Record<string, unknown> | undefined)
const phases = ['normal', 'concentrating', 'draining', 'probing', 'waiting_real', 'real_inflight', 'stopped']
const phase = computed(() => phases.includes(String(state.value?.phase)) ? String(state.value?.phase) : 'normal')
const count = computed(() => typeof state.value?.consecutive_429 === 'number' ? state.value.consecutive_429 : 0)
const recovery = computed(() => {
  if (state.value?.unknown_reset || typeof state.value?.recover_at !== 'number' || state.value.recover_at <= 0) return ''
  return new Date(state.value.recover_at).toLocaleString()
})
</script>
