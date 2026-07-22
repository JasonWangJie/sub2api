<template>
  <section class="library-panel" :class="{ 'library-panel--compact': compact }" aria-labelledby="image-library-heading">
    <header class="library-panel__header">
      <div class="min-w-0">
        <h2 id="image-library-heading" class="library-panel__title">{{ t('imageWorkflow.library.title') }}</h2>
        <p v-if="!compact" class="library-panel__description">{{ t('imageWorkflow.library.description') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <RouterLink v-if="compact" to="/image-library" class="library-icon-button" :aria-label="t('imageWorkflow.library.openFull')" :title="t('imageWorkflow.library.openFull')">
          <Icon name="externalLink" size="sm" />
        </RouterLink>
        <button type="button" class="library-icon-button" :disabled="loading" :aria-label="t('common.refresh')" :title="t('common.refresh')" @click="refresh">
          <Icon name="refresh" size="sm" :class="loading && 'animate-spin'" />
        </button>
      </div>
    </header>

    <div v-if="!compact" class="library-filters" role="group" :aria-label="t('imageWorkflow.library.filters')">
      <button
        v-for="option in filterOptions"
        :key="option.value"
        type="button"
        class="library-filter"
        :class="{ 'is-active': filter === option.value }"
        @click="setFilter(option.value)"
      >
        {{ option.label }}
      </button>
    </div>

    <section v-if="recoveries.length" class="library-recovery" aria-labelledby="image-library-recovery-heading">
      <div class="library-recovery__heading">
        <span class="library-recovery__icon"><Icon name="exclamationTriangle" size="sm" /></span>
        <div class="min-w-0">
          <h3 id="image-library-recovery-heading">
            {{ t('imageWorkflow.library.recoveryTitle', { count: recoveries.length }) }}
          </h3>
          <p>{{ t('imageWorkflow.library.recoveryLocalHint') }}</p>
        </div>
      </div>
      <div class="library-recovery__list">
        <article v-for="recovery in visibleRecoveries" :key="recovery.id" class="library-recovery__item">
          <span class="library-recovery__preview">
            <img v-if="recoveryPreviewURLs[recovery.id]" :src="recoveryPreviewURLs[recovery.id]" :alt="recovery.title" />
            <Icon v-else name="inbox" size="sm" />
          </span>
          <div class="min-w-0 flex-1">
            <strong :title="recovery.title">{{ recovery.title || t('imageWorkflow.library.untitled') }}</strong>
            <small>{{ recovery.errorMessage || t('imageWorkflow.library.archiveFailed') }}</small>
          </div>
          <button
            type="button"
            class="library-recovery__retry"
            :disabled="recoveryBusyId === recovery.id"
            @click="retryRecovery(recovery)"
          >
            <Icon name="refresh" size="xs" :class="recoveryBusyId === recovery.id && 'animate-spin'" />
            {{ t('imageWorkflow.library.retryArchive') }}
          </button>
          <button
            type="button"
            class="library-action"
            :disabled="recoveryBusyId === recovery.id"
            :title="t('imageWorkflow.library.discardRecovery')"
            :aria-label="t('imageWorkflow.library.discardRecovery')"
            @click="discardRecovery(recovery.id)"
          >
            <Icon name="x" size="sm" />
          </button>
        </article>
      </div>
      <RouterLink v-if="compact && recoveries.length > visibleRecoveries.length" to="/image-library" class="library-recovery__more">
        {{ t('imageWorkflow.library.viewAllRecoveries', { count: recoveries.length }) }}
      </RouterLink>
    </section>

    <div v-if="loading && !items.length" class="library-empty" role="status">
      <span class="library-spinner" aria-hidden="true"></span>
      {{ t('common.loading') }}
    </div>
    <div v-else-if="error && !items.length" class="library-empty library-empty--error" role="alert">
      <Icon name="exclamationCircle" size="lg" />
      <span>{{ error }}</span>
      <button type="button" class="btn btn-secondary" @click="refresh">{{ t('common.refresh') }}</button>
    </div>
    <div v-else-if="!items.length" class="library-empty">
      <Icon name="inbox" size="lg" />
      <span>{{ t('imageWorkflow.library.empty') }}</span>
    </div>

    <div v-else class="library-grid" :class="{ 'library-grid--compact': compact }">
      <article v-for="item in items" :key="item.id" class="library-item">
        <button type="button" class="library-item__media" @click="emit('view', itemForView(item))">
          <img
            v-if="!brokenImages.has(String(item.id)) && resolvedImages[String(item.id)]"
            :src="resolvedImages[String(item.id)]"
            :alt="item.title || t('imageWorkflow.library.untitled')"
            loading="lazy"
            decoding="async"
            @error="markBroken(item.id)"
          />
          <span v-else-if="brokenImages.has(String(item.id))" class="library-item__broken">
            <Icon name="exclamationTriangle" size="lg" />
            {{ t('imageWorkflow.library.imageUnavailable') }}
          </span>
          <span v-else class="library-item__broken"><span class="library-spinner" aria-hidden="true"></span></span>
          <span class="library-item__mode" :class="`is-${item.execution_mode}`">
            <Icon :name="item.execution_mode === 'async' ? 'clock' : 'bolt'" size="xs" />
            {{ modeLabel(item.execution_mode) }}
          </span>
        </button>
        <div class="library-item__body">
          <div class="library-item__title-row">
            <div v-if="editingId === String(item.id)" class="library-title-editor">
              <input
                v-model="editingTitle"
                type="text"
                maxlength="200"
                class="input"
                :aria-label="t('imageWorkflow.library.privateTitle')"
                :disabled="busyId === String(item.id)"
                @keydown.enter.prevent="saveTitle(item)"
                @keydown.esc.prevent="cancelTitleEdit"
              />
              <button type="button" class="library-action" :disabled="busyId === String(item.id)" :title="t('common.save')" :aria-label="t('common.save')" @click="saveTitle(item)">
                <Icon name="check" size="xs" />
              </button>
              <button type="button" class="library-action" :disabled="busyId === String(item.id)" :title="t('common.cancel')" :aria-label="t('common.cancel')" @click="cancelTitleEdit">
                <Icon name="x" size="xs" />
              </button>
            </div>
            <template v-else>
              <h3 class="library-item__title" :title="item.title">{{ item.title || t('imageWorkflow.library.untitled') }}</h3>
              <button type="button" class="library-title-edit" :title="t('imageWorkflow.library.editPrivateTitle')" :aria-label="t('imageWorkflow.library.editPrivateTitle')" @click="startTitleEdit(item)">
                <Icon name="edit" size="xs" />
              </button>
            </template>
            <span class="library-status" :class="statusClass(item.publication_status)">
              {{ publicationLabel(item.publication_status) }}
            </span>
          </div>
          <p class="library-item__meta">{{ item.platform }} · {{ item.model || '—' }}</p>
          <p v-if="!compact && item.prompt" class="library-item__prompt">{{ item.prompt }}</p>
          <p v-if="isArchiveFailed(item)" class="library-item__error" role="alert">
            {{ item.error_message || t('imageWorkflow.library.archiveFailed') }}
          </p>
          <div class="library-item__actions">
            <button type="button" class="library-action" :title="t('imageWorkflow.library.reuse')" :aria-label="t('imageWorkflow.library.reuse')" @click="emit('reuse', item)">
              <Icon name="refresh" size="sm" />
            </button>
            <a class="library-action" :href="resolvedImages[String(item.id)] || '#'" target="_blank" rel="noopener" :title="t('imageWorkbench.download')" :aria-label="t('imageWorkbench.download')">
              <Icon name="download" size="sm" />
            </a>
            <button
              v-if="canPublish(item)"
              type="button"
              class="library-text-action"
              :disabled="busyId === String(item.id)"
              @click="publish(item)"
            >
              {{ t('imageWorkflow.library.submitReview') }}
            </button>
            <button
              v-else-if="canWithdraw(item)"
              type="button"
              class="library-text-action"
              :disabled="busyId === String(item.id)"
              @click="withdraw(item)"
            >
              {{ t('imageWorkflow.library.withdraw') }}
            </button>
            <button type="button" class="library-action is-danger" :disabled="busyId === String(item.id)" :title="t('common.delete')" :aria-label="t('common.delete')" @click="remove(item)">
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </div>
      </article>
    </div>

    <button v-if="nextCursor && !compact" type="button" class="library-load-more" :disabled="loadingMore" @click="loadMore">
      <span v-if="loadingMore" class="library-spinner" aria-hidden="true"></span>
      {{ t('imageWorkflow.library.loadMore') }}
    </button>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import {
  archiveAsyncTask,
  deleteImageLibraryItem,
  importImageFile,
  importImageURL,
  listImageLibrary,
  publishImageLibraryItem,
  resolveImageLibraryViewURL,
  updateImageLibraryItem,
  withdrawImageLibraryItem,
} from '@/api/imageLibrary'
import {
  listPendingImageArchives,
  onPendingImageArchivesChanged,
  removePendingImageArchive,
  savePendingImageArchive,
  type PendingImageArchive,
} from './archiveRecovery'
import type { ImageLibraryItem, ImagePublicationStatus } from './types'

const props = withDefaults(defineProps<{ compact?: boolean; limit?: number }>(), {
  compact: false,
  limit: 24,
})
const emit = defineEmits<{
  (event: 'reuse', item: ImageLibraryItem): void
  (event: 'view', item: ImageLibraryItem): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const loading = ref(false)
const loadingMore = ref(false)
const error = ref('')
const items = ref<ImageLibraryItem[]>([])
const nextCursor = ref<string | null>(null)
const filter = ref('all')
const busyId = ref('')
const brokenImages = ref(new Set<string>())
const resolvedImages = ref<Record<string, string>>({})
const recoveries = ref<PendingImageArchive[]>([])
const recoveryPreviewURLs = ref<Record<string, string>>({})
const recoveryBusyId = ref('')
const editingId = ref('')
const editingTitle = ref('')
let stopRecoveryListener: (() => void) | null = null

const filterOptions = computed(() => [
  { value: 'all', label: t('common.all') },
  { value: 'private', label: t('imageWorkflow.library.private') },
  { value: 'pending_review', label: t('imageWorkflow.library.pendingReview') },
  { value: 'published', label: t('imageWorkflow.library.published') },
])
const visibleRecoveries = computed(() => props.compact ? recoveries.value.slice(0, 3) : recoveries.value)

function queryForFilter(cursor?: string) {
  const params: Record<string, string | number> = { limit: props.limit }
  if (cursor) params.cursor = cursor
  if (filter.value === 'private') params.visibility = 'private'
  if (filter.value === 'pending_review' || filter.value === 'published') {
    params.publication_status = filter.value
  }
  return params
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const page = await listImageLibrary(queryForFilter())
    items.value = page.items
    nextCursor.value = page.next_cursor
    await resolveItems(page.items)
  } catch (cause: any) {
    error.value = cause?.message || t('imageWorkflow.library.loadFailed')
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (!nextCursor.value || loadingMore.value) return
  loadingMore.value = true
  try {
    const page = await listImageLibrary(queryForFilter(nextCursor.value))
    const known = new Set(items.value.map((item) => String(item.id)))
    items.value.push(...page.items.filter((item) => !known.has(String(item.id))))
    nextCursor.value = page.next_cursor
    await resolveItems(page.items)
  } catch (cause: any) {
    appStore.showError(cause?.message || t('imageWorkflow.library.loadFailed'))
  } finally {
    loadingMore.value = false
  }
}

function setFilter(value: string) {
  if (filter.value === value) return
  filter.value = value
  void refresh()
}

function canPublish(item: ImageLibraryItem) {
  return !item.publication_status || ['rejected', 'withdrawn', 'expired'].includes(item.publication_status)
}

function canWithdraw(item: ImageLibraryItem) {
  return ['pending_review', 'published', 'admin_hidden'].includes(String(item.publication_status || ''))
}

async function publish(item: ImageLibraryItem) {
  if (!window.confirm(t('imageWorkflow.library.publishConfirm'))) return
  busyId.value = String(item.id)
  try {
    const sharePrompt = !props.compact && Boolean(item.prompt) && window.confirm(t('imageWorkflow.library.sharePromptConfirm'))
    await publishImageLibraryItem(item.id, {
      title: item.title,
      share_prompt: sharePrompt,
      public_prompt: sharePrompt ? item.prompt || undefined : undefined,
    })
    appStore.showSuccess(t('imageWorkflow.library.submitted'))
    await refresh()
  } catch (cause: any) {
    appStore.showError(cause?.message || t('imageWorkflow.library.actionFailed'))
  } finally {
    busyId.value = ''
  }
}

async function withdraw(item: ImageLibraryItem) {
  if (!window.confirm(t('imageWorkflow.library.withdrawConfirm'))) return
  busyId.value = String(item.id)
  try {
    await withdrawImageLibraryItem(item.id)
    appStore.showSuccess(t('imageWorkflow.library.withdrawn'))
    await refresh()
  } catch (cause: any) {
    appStore.showError(cause?.message || t('imageWorkflow.library.actionFailed'))
  } finally {
    busyId.value = ''
  }
}

async function remove(item: ImageLibraryItem) {
  if (!window.confirm(t('imageWorkflow.library.deleteConfirm'))) return
  busyId.value = String(item.id)
  try {
    await deleteImageLibraryItem(item.id)
    items.value = items.value.filter((candidate) => candidate.id !== item.id)
    appStore.showSuccess(t('common.deleted'))
  } catch (cause: any) {
    appStore.showError(cause?.message || t('imageWorkflow.library.actionFailed'))
  } finally {
    busyId.value = ''
  }
}

function startTitleEdit(item: ImageLibraryItem) {
  editingId.value = String(item.id)
  editingTitle.value = item.title || ''
}

function cancelTitleEdit() {
  editingId.value = ''
  editingTitle.value = ''
}

async function saveTitle(item: ImageLibraryItem) {
  const id = String(item.id)
  if (busyId.value) return
  busyId.value = id
  try {
    const updated = await updateImageLibraryItem(item.id, { title: editingTitle.value.trim() })
    const index = items.value.findIndex((candidate) => String(candidate.id) === id)
    if (index >= 0) items.value[index] = { ...items.value[index], ...updated }
    cancelTitleEdit()
    appStore.showSuccess(t('imageWorkflow.library.titleUpdated'))
  } catch (cause: any) {
    appStore.showError(cause?.message || t('imageWorkflow.library.actionFailed'))
  } finally {
    busyId.value = ''
  }
}

async function loadRecoveries() {
  const userId = Number(authStore.user?.id || 0)
  const previousURLs = recoveryPreviewURLs.value
  const nextURLs: Record<string, string> = {}
  try {
    recoveries.value = await listPendingImageArchives(userId)
    for (const item of recoveries.value) {
      if (item.file) nextURLs[item.id] = URL.createObjectURL(item.file)
      else if (item.previewUrl || item.remoteUrl) nextURLs[item.id] = item.previewUrl || item.remoteUrl || ''
    }
    recoveryPreviewURLs.value = nextURLs
  } catch {
    recoveries.value = []
  } finally {
    Object.values(previousURLs).forEach((url) => {
      if (url.startsWith('blob:') && !Object.values(nextURLs).includes(url)) URL.revokeObjectURL(url)
    })
  }
}

async function retryRecovery(recovery: PendingImageArchive) {
  recoveryBusyId.value = recovery.id
  try {
    if (recovery.kind === 'task') {
      if (!recovery.taskId) throw new Error(t('imageWorkflow.library.recoveryUnavailable'))
      await archiveAsyncTask(recovery.taskId, recovery.resultIndex == null ? undefined : [recovery.resultIndex])
    } else if (recovery.kind === 'file') {
      if (!recovery.file) throw new Error(t('imageWorkflow.library.recoveryUnavailable'))
      const file = recovery.file instanceof File
        ? recovery.file
        : new File([recovery.file], recovery.fileName || 'recovered-image', { type: recovery.file.type })
      await importImageFile(file, recovery.metadata || {}, recovery.id)
    } else {
      if (!recovery.remoteUrl) throw new Error(t('imageWorkflow.library.recoveryUnavailable'))
      await importImageURL(recovery.remoteUrl, recovery.metadata || {}, recovery.id)
    }
    await removePendingImageArchive(recovery.id)
    await refresh()
    appStore.showSuccess(t('imageWorkflow.library.archiveRecovered'))
  } catch (cause: any) {
    const errorMessage = cause?.message || t('imageWorkflow.library.archiveFailed')
    await savePendingImageArchive({ ...recovery, errorMessage }).catch(() => undefined)
    appStore.showError(errorMessage)
  } finally {
    recoveryBusyId.value = ''
  }
}

async function discardRecovery(id: string) {
  if (!window.confirm(t('imageWorkflow.library.discardRecoveryConfirm'))) return
  await removePendingImageArchive(id).catch(() => undefined)
}

function markBroken(id: string | number) {
  brokenImages.value = new Set([...brokenImages.value, String(id)])
}

async function resolveItems(candidates: ImageLibraryItem[]) {
  await Promise.allSettled(candidates.map(async (item) => {
    const id = String(item.id)
    if (resolvedImages.value[id] || brokenImages.value.has(id)) return
    try {
      const access = await resolveImageLibraryViewURL(item.id)
      resolvedImages.value = { ...resolvedImages.value, [id]: access.url }
    } catch {
      markBroken(id)
    }
  }))
}

function itemForView(item: ImageLibraryItem): ImageLibraryItem {
  return { ...item, view_url: resolvedImages.value[String(item.id)] || item.view_url }
}

function modeLabel(mode: string) {
  return mode === 'async' ? t('imageWorkflow.mode.async') : t('imageWorkflow.mode.realtime')
}

function publicationLabel(status?: ImagePublicationStatus | null) {
  if (!status) return t('imageWorkflow.library.private')
  return t(`imageWorkflow.publication.${status}`)
}

function statusClass(status?: ImagePublicationStatus | null) {
  if (status === 'published') return 'is-published'
  if (status === 'pending_review') return 'is-pending'
  if (status === 'rejected' || status === 'admin_hidden') return 'is-blocked'
  return 'is-private'
}

function isArchiveFailed(item: ImageLibraryItem) {
  return item.archive_status === 'failed' || item.archive_status === 'archive_failed'
}

onMounted(() => {
  stopRecoveryListener = onPendingImageArchivesChanged(() => { void loadRecoveries() })
  void Promise.all([refresh(), loadRecoveries()])
})
onUnmounted(() => {
  stopRecoveryListener?.()
  Object.values(recoveryPreviewURLs.value).forEach((url) => {
    if (url.startsWith('blob:')) URL.revokeObjectURL(url)
  })
})
defineExpose({ refresh })
</script>

<style scoped>
.library-panel {
  min-width: 0;
}

.library-panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.875rem;
}

.library-panel__title {
  color: #111827;
  font-size: 0.95rem;
  font-weight: 700;
  line-height: 1.4;
}

.dark .library-panel__title { color: #f3f4f6; }
.library-panel__description { margin-top: 0.2rem; color: #6b7280; font-size: 0.8rem; }
.dark .library-panel__description { color: #9ca3af; }

.library-icon-button,
.library-action {
  display: inline-grid;
  width: 2rem;
  height: 2rem;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  color: #4b5563;
  background: transparent;
}

.dark .library-icon-button,
.dark .library-action { border-color: #374151; color: #d1d5db; }
.library-icon-button:hover,
.library-action:hover { border-color: #0d9488; color: #0f766e; }
.library-action.is-danger:hover { border-color: #dc2626; color: #dc2626; }

.library-filters { display: flex; flex-wrap: wrap; gap: 0.35rem; margin-bottom: 1rem; }
.library-filter {
  min-height: 2rem;
  padding: 0.35rem 0.7rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  color: #4b5563;
  font-size: 0.75rem;
  font-weight: 600;
}
.dark .library-filter { border-color: #374151; color: #d1d5db; }
.library-filter.is-active { border-color: #0f766e; background: #f0fdfa; color: #0f766e; }
.dark .library-filter.is-active { background: rgba(13, 148, 136, 0.14); color: #5eead4; }

.library-recovery { margin-bottom: 1rem; padding: 0.7rem; border: 1px solid #f59e0b; border-radius: 6px; background: #fffbeb; }
.dark .library-recovery { border-color: #92400e; background: rgba(120, 53, 15, 0.14); }
.library-recovery__heading { display: flex; align-items: flex-start; gap: 0.55rem; color: #92400e; }
.dark .library-recovery__heading { color: #fbbf24; }
.library-recovery__icon { display: grid; width: 1.7rem; height: 1.7rem; flex: 0 0 auto; place-items: center; border-radius: 5px; background: #fef3c7; }
.dark .library-recovery__icon { background: rgba(146, 64, 14, 0.35); }
.library-recovery__heading h3 { font-size: 0.76rem; font-weight: 750; }
.library-recovery__heading p { margin-top: 0.15rem; color: #a16207; font-size: 0.66rem; line-height: 1.4; }
.dark .library-recovery__heading p { color: #fcd34d; }
.library-recovery__list { margin-top: 0.55rem; border-top: 1px solid #fde68a; }
.dark .library-recovery__list { border-color: rgba(180, 83, 9, 0.45); }
.library-recovery__item { display: flex; min-width: 0; align-items: center; gap: 0.5rem; padding: 0.5rem 0; border-bottom: 1px solid #fde68a; }
.dark .library-recovery__item { border-color: rgba(180, 83, 9, 0.35); }
.library-recovery__preview { display: grid; width: 2.5rem; height: 2.5rem; flex: 0 0 auto; overflow: hidden; place-items: center; border: 1px solid #fcd34d; border-radius: 5px; color: #a16207; background: #fff; }
.dark .library-recovery__preview { border-color: #92400e; background: #111827; }
.library-recovery__preview img { width: 100%; height: 100%; object-fit: cover; }
.library-recovery__item strong,
.library-recovery__item small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.library-recovery__item strong { color: #78350f; font-size: 0.7rem; }
.dark .library-recovery__item strong { color: #fde68a; }
.library-recovery__item small { margin-top: 0.15rem; color: #a16207; font-size: 0.62rem; }
.dark .library-recovery__item small { color: #fbbf24; }
.library-recovery__retry { display: inline-flex; min-height: 2rem; flex: 0 0 auto; align-items: center; gap: 0.3rem; padding: 0.3rem 0.55rem; border: 1px solid #f59e0b; border-radius: 5px; color: #92400e; font-size: 0.66rem; font-weight: 700; }
.dark .library-recovery__retry { color: #fcd34d; }
.library-recovery__retry:disabled { cursor: wait; opacity: 0.6; }
.library-recovery__more { display: block; padding-top: 0.45rem; color: #92400e; font-size: 0.68rem; font-weight: 700; text-align: center; }
.dark .library-recovery__more { color: #fbbf24; }

.library-empty {
  display: flex;
  min-height: 9rem;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.65rem;
  border: 1px dashed #d1d5db;
  border-radius: 6px;
  color: #6b7280;
  text-align: center;
  font-size: 0.8rem;
}
.dark .library-empty { border-color: #374151; color: #9ca3af; }
.library-empty--error { color: #b91c1c; }

.library-spinner { width: 1rem; height: 1rem; border: 2px solid #99f6e4; border-top-color: #0f766e; border-radius: 50%; animation: library-spin 0.7s linear infinite; }
.library-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(210px, 1fr)); gap: 0.75rem; }
.library-grid--compact { grid-template-columns: 1fr; max-height: 720px; overflow-y: auto; padding-right: 0.2rem; }

.library-item { min-width: 0; overflow: hidden; border: 1px solid #e5e7eb; border-radius: 6px; background: #fff; }
.dark .library-item { border-color: #374151; background: #111827; }
.library-item__media { position: relative; display: block; width: 100%; aspect-ratio: 4 / 3; overflow: hidden; background: #f3f4f6; }
.dark .library-item__media { background: #030712; }
.library-item__media img { width: 100%; height: 100%; object-fit: cover; }
.library-item__media:focus-visible { outline: 2px solid #0d9488; outline-offset: -2px; }
.library-item__broken { display: flex; width: 100%; height: 100%; flex-direction: column; align-items: center; justify-content: center; gap: 0.4rem; color: #9ca3af; font-size: 0.72rem; }
.library-item__mode { position: absolute; left: 0.45rem; bottom: 0.45rem; display: inline-flex; align-items: center; gap: 0.25rem; padding: 0.2rem 0.4rem; border-radius: 4px; background: rgba(17, 24, 39, 0.84); color: #f9fafb; font-size: 0.65rem; font-weight: 700; }
.library-item__mode.is-async { background: rgba(146, 64, 14, 0.9); }
.library-item__body { padding: 0.65rem; }
.library-item__title-row { display: flex; min-width: 0; align-items: flex-start; gap: 0.35rem; }
.library-item__title { min-width: 0; overflow: hidden; color: #111827; font-size: 0.8rem; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.dark .library-item__title { color: #f9fafb; }
.library-title-edit { display: grid; width: 1.5rem; height: 1.5rem; flex: 0 0 auto; place-items: center; border-radius: 4px; color: #6b7280; }
.library-title-edit:hover,
.library-title-edit:focus-visible { color: #0f766e; background: #f0fdfa; }
.dark .library-title-edit:hover,
.dark .library-title-edit:focus-visible { color: #5eead4; background: rgba(13, 148, 136, 0.14); }
.library-title-editor { display: flex; min-width: 0; flex: 1; align-items: center; gap: 0.25rem; }
.library-title-editor .input { min-width: 0; height: 2rem; flex: 1; padding: 0.3rem 0.45rem; font-size: 0.72rem; }
.library-title-editor .library-action { width: 1.75rem; height: 1.75rem; }
.library-item__meta { margin-top: 0.25rem; overflow: hidden; color: #6b7280; font-size: 0.68rem; text-overflow: ellipsis; white-space: nowrap; }
.library-item__prompt { margin-top: 0.45rem; display: -webkit-box; overflow: hidden; color: #6b7280; font-size: 0.72rem; line-height: 1.45; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.library-item__error { margin-top: 0.4rem; color: #b91c1c; font-size: 0.7rem; }
.library-item__actions { display: flex; align-items: center; gap: 0.35rem; margin-top: 0.65rem; }
.library-text-action { min-width: 0; flex: 1; overflow: hidden; padding: 0.35rem 0.5rem; border: 1px solid #99f6e4; border-radius: 6px; color: #0f766e; font-size: 0.7rem; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.dark .library-text-action { border-color: rgba(45, 212, 191, 0.4); color: #5eead4; }
.library-status { flex: 0 0 auto; padding: 0.15rem 0.35rem; border-radius: 4px; background: #f3f4f6; color: #4b5563; font-size: 0.62rem; font-weight: 700; }
.dark .library-status { background: #1f2937; color: #d1d5db; }
.library-status.is-published { background: #dcfce7; color: #166534; }
.library-status.is-pending { background: #fef3c7; color: #92400e; }
.library-status.is-blocked { background: #fee2e2; color: #991b1b; }
.library-load-more { display: flex; width: 100%; min-height: 2.5rem; align-items: center; justify-content: center; gap: 0.5rem; margin-top: 1rem; border: 1px solid #d1d5db; border-radius: 6px; color: #4b5563; font-size: 0.8rem; font-weight: 700; }
.dark .library-load-more { border-color: #374151; color: #d1d5db; }

@keyframes library-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .library-spinner { animation: none; } }
@media (max-width: 640px) { .library-grid { grid-template-columns: 1fr 1fr; } }
@media (max-width: 520px) {
  .library-recovery__item { align-items: flex-start; flex-wrap: wrap; }
  .library-recovery__item > .min-w-0 { min-width: calc(100% - 3rem); }
  .library-recovery__retry { margin-left: 3rem; }
}
@media (max-width: 420px) { .library-grid { grid-template-columns: 1fr; } }
</style>
