<template>
  <div class="space-y-2 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="openai-429-mode-settings">
    <div class="flex items-center justify-between gap-4">
      <label for="openai-429-mode-enabled" class="input-label mb-0">{{ t('admin.accounts.mode429.enable') }}</label>
      <input id="openai-429-mode-enabled" type="checkbox" :checked="modelValue" aria-describedby="openai-429-mode-hint"
        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        @change="$emit('update:modelValue', ($event.target as HTMLInputElement).checked)" />
    </div>
    <p id="openai-429-mode-hint" class="input-hint">{{ t('admin.accounts.mode429.hint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ modelValue: boolean }>()
defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { t } = useI18n()
</script>
