<template>
  <AppLayout>
    <div class="image-library-page">
      <header class="image-library-page__header">
        <div>
          <h1>{{ t('imageWorkflow.library.title') }}</h1>
          <p>{{ t('imageWorkflow.library.description') }}</p>
        </div>
        <RouterLink to="/image-workbench" class="btn btn-primary inline-flex items-center gap-2">
          <Icon name="arrowLeft" size="sm" />
          {{ t('imageWorkflow.library.backWorkbench') }}
        </RouterLink>
      </header>

      <div class="image-library-page__surface">
        <ImageLibraryPanel @reuse="reuse" />
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ImageLibraryPanel from '@/features/image-workflow/ImageLibraryPanel.vue'
import type { ImageLibraryItem } from '@/features/image-workflow/types'

const { t } = useI18n()
const router = useRouter()

function reuse(item: ImageLibraryItem) {
  router.push({
    path: '/image-workbench',
    query: {
      prompt: item.prompt || undefined,
      model: item.model || undefined,
      size: item.requested_size || undefined,
    },
  })
}
</script>

<style scoped>
.image-library-page { max-width: 1480px; margin: 0 auto; }
.image-library-page__header { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; margin-bottom: 1.25rem; }
.image-library-page__header h1 { color: #111827; font-size: 1.5rem; font-weight: 750; line-height: 1.25; }
.dark .image-library-page__header h1 { color: #f9fafb; }
.image-library-page__header p { margin-top: 0.3rem; color: #6b7280; font-size: 0.875rem; }
.dark .image-library-page__header p { color: #9ca3af; }
.image-library-page__surface { padding: 1rem; border: 1px solid #e5e7eb; border-radius: 8px; background: #fff; }
.dark .image-library-page__surface { border-color: #374151; background: #111827; }
@media (max-width: 640px) { .image-library-page__header { flex-direction: column; } }
</style>
