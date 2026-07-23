<template>
  <AppLayout>
    <div class="plaza-page">
      <header class="plaza-header">
        <div>
          <h1>{{ t('imagePlaza.title') }}</h1>
          <p>{{ t('imageWorkflow.plaza.description') }}</p>
        </div>
        <div class="plaza-header__links">
          <RouterLink to="/image-library" class="btn btn-secondary inline-flex items-center gap-2">
            <Icon name="inbox" size="sm" />
            {{ t('imageWorkflow.library.title') }}
          </RouterLink>
          <RouterLink to="/image-workbench" class="btn btn-primary inline-flex items-center gap-2">
            <Icon name="bolt" size="sm" />
            {{ t('imagePlaza.goWorkbench') }}
          </RouterLink>
        </div>
      </header>

      <section class="plaza-toolbar" :aria-label="t('imageWorkflow.plaza.filters')">
        <label class="plaza-search">
          <Icon name="search" size="sm" aria-hidden="true" />
          <span class="sr-only">{{ t('imagePlaza.searchPlaceholder') }}</span>
          <input v-model="filters.q" type="search" :placeholder="t('imagePlaza.searchPlaceholder')" @keydown.enter="refresh" />
        </label>
        <label class="plaza-select">
          <span>{{ t('imageWorkflow.workbench.platform') }}</span>
          <select v-model="filters.platform" @change="refresh">
            <option value="">{{ t('common.all') }}</option>
            <option value="openai">OpenAI</option>
            <option value="gemini">Gemini</option>
            <option value="grok">Grok</option>
          </select>
        </label>
        <label class="plaza-select">
          <span>{{ t('imageWorkflow.workbench.aspectRatio') }}</span>
          <select v-model="filters.aspectRatio" @change="refresh">
            <option value="">{{ t('common.all') }}</option>
            <option v-for="ratio in ratios" :key="ratio" :value="ratio">{{ ratio }}</option>
          </select>
        </label>
        <label class="plaza-select">
          <span>{{ t('imageWorkflow.plaza.sort') }}</span>
          <select v-model="filters.sort" @change="refresh">
            <option value="newest">{{ t('imageWorkflow.plaza.newest') }}</option>
            <option value="oldest">{{ t('imageWorkflow.plaza.oldest') }}</option>
          </select>
        </label>
        <button type="button" class="plaza-icon-button" :disabled="loading" :title="t('common.refresh')" :aria-label="t('common.refresh')" @click="refresh">
          <Icon name="refresh" size="sm" :class="loading && 'animate-spin'" />
        </button>
      </section>

      <div class="plaza-summary">
        <span>{{ t('imageWorkflow.plaza.approvedOnly') }}</span>
        <strong v-if="total != null">{{ t('imagePlaza.total', { n: total }) }}</strong>
      </div>

      <div v-if="loading && !items.length" class="plaza-empty" role="status">
        <span class="plaza-spinner" aria-hidden="true"></span>
        {{ t('common.loading') }}
      </div>
      <div v-else-if="error && !items.length" class="plaza-empty is-error" role="alert">
        <Icon name="exclamationCircle" size="lg" />
        <span>{{ error }}</span>
        <button type="button" class="btn btn-secondary" @click="refresh">{{ t('common.refresh') }}</button>
      </div>
      <div v-else-if="!items.length" class="plaza-empty">
        <Icon name="grid" size="lg" />
        <strong>{{ t('imageWorkflow.plaza.empty') }}</strong>
        <span>{{ t('imageWorkflow.plaza.emptyHint') }}</span>
      </div>

      <div v-else class="plaza-grid">
        <article v-for="item in items" :key="item.id" class="plaza-item">
          <button type="button" class="plaza-item__media" @click="openPreview(item)">
            <LazyImage
              class="plaza-item__lazy"
              :src="plazaThumbUrl(item)"
              :alt="item.title || t('imageWorkflow.library.untitled')"
              @error="markBroken(item.id)"
            >
              <template #error>
                <span class="plaza-item__broken">
                  <Icon name="exclamationTriangle" size="lg" />
                  {{ t('imageWorkflow.library.imageUnavailable') }}
                </span>
              </template>
            </LazyImage>
            <span class="plaza-item__platform">{{ platformName(item.platform) }}</span>
          </button>
          <div class="plaza-item__body">
            <div class="plaza-item__title-row">
              <h2 :title="item.title">{{ item.title || t('imageWorkflow.library.untitled') }}</h2>
              <span v-if="item.is_owner" class="owner-badge">{{ t('imageWorkflow.plaza.mine') }}</span>
            </div>
            <p class="plaza-item__meta">
              <span>{{ item.model || '—' }}</span>
              <span>{{ item.aspect_ratio || item.size || '—' }}</span>
              <span>{{ formatDate(item.published_at) }}</span>
            </p>
            <p v-if="item.share_prompt && item.prompt" class="plaza-item__prompt">{{ item.prompt }}</p>
            <p v-else class="plaza-item__prompt is-private">{{ t('imageWorkflow.plaza.promptPrivate') }}</p>
            <div class="plaza-item__publisher">
              <Icon name="userCircle" size="sm" />
              <span>{{ item.public_identity }}</span>
            </div>
          </div>
          <div class="plaza-item__actions">
            <button
              type="button"
              class="plaza-action"
              :disabled="!item.share_prompt || !item.prompt"
              :title="item.share_prompt ? t('imagePlaza.reuse') : t('imageWorkflow.plaza.promptPrivate')"
              :aria-label="t('imagePlaza.reuse')"
              @click="reuse(item)"
            >
              <Icon name="refresh" size="sm" />
            </button>
            <a class="plaza-action" :href="resolvePlazaImageUrl(item)" target="_blank" rel="noopener" :title="t('imageWorkbench.download')" :aria-label="t('imageWorkbench.download')">
              <Icon name="download" size="sm" />
            </a>
            <button
              v-if="authStore.isAdmin"
              type="button"
              class="plaza-text-action is-danger"
              :disabled="busyId === String(item.id)"
              @click="adminHide(item)"
            >
              {{ t('common.delete') }}
            </button>
            <button
              v-else
              type="button"
              class="plaza-text-action"
              @click="openReport(item)"
            >
              {{ t('imageWorkflow.plaza.report') }}
            </button>
          </div>
        </article>
      </div>

      <button
        v-if="nextCursor"
        ref="loadMoreSentinel"
        type="button"
        class="plaza-load-more"
        :disabled="loadingMore"
        @click="loadMore"
      >
        <span v-if="loadingMore" class="plaza-spinner" aria-hidden="true"></span>
        {{ t('imageWorkflow.plaza.loadMore') }}
      </button>

      <dialog ref="previewDialog" class="plaza-dialog" @click="closeOnBackdrop">
        <div v-if="previewItem" class="plaza-dialog__surface">
          <header>
            <div class="min-w-0">
              <h2>{{ previewItem.title || t('imageWorkflow.library.untitled') }}</h2>
              <p>{{ previewItem.public_identity }} · {{ formatDate(previewItem.published_at) }}</p>
            </div>
            <button type="button" class="plaza-icon-button" :title="t('common.close')" :aria-label="t('common.close')" @click="closePreview">
              <Icon name="x" size="sm" />
            </button>
          </header>
          <img :src="resolvePlazaImageUrl(previewItem)" :alt="previewItem.title" />
          <p v-if="previewItem.share_prompt && previewItem.prompt" class="plaza-dialog__prompt">{{ previewItem.prompt }}</p>
          <footer>
            <button v-if="previewItem.share_prompt && previewItem.prompt" type="button" class="btn btn-primary" @click="reuse(previewItem)">
              {{ t('imagePlaza.oneClickSame') }}
            </button>
            <a :href="resolvePlazaImageUrl(previewItem)" target="_blank" rel="noopener" class="btn btn-secondary">{{ t('imageWorkbench.view') }}</a>
          </footer>
        </div>
      </dialog>

      <dialog ref="reportDialog" class="plaza-dialog" @click="closeReportOnBackdrop">
        <form class="plaza-dialog__surface plaza-dialog__surface--form" @submit.prevent="submitReport">
          <header>
            <div>
              <h2>{{ t('imageWorkflow.plaza.reportTitle') }}</h2>
              <p>{{ reportItem?.title }}</p>
            </div>
            <button type="button" class="plaza-icon-button" :title="t('common.close')" :aria-label="t('common.close')" @click="closeReport">
              <Icon name="x" size="sm" />
            </button>
          </header>
          <label class="dialog-field">
            <span>{{ t('imageWorkflow.plaza.reportReason') }}</span>
            <select v-model="reportForm.reason" class="input" required>
              <option v-for="reason in IMAGE_PLAZA_REPORT_REASONS" :key="reason" :value="reason">
                {{ t(`imageWorkflow.plaza.reasons.${reason}`) }}
              </option>
            </select>
          </label>
          <label class="dialog-field">
            <span>{{ t('imageWorkflow.plaza.reportDetail') }}</span>
            <textarea v-model="reportForm.detail" class="input" rows="4" maxlength="500"></textarea>
          </label>
          <footer>
            <button type="button" class="btn btn-secondary" @click="closeReport">{{ t('common.cancel') }}</button>
            <button type="submit" class="btn btn-danger" :disabled="reporting">{{ t('imageWorkflow.plaza.submitReport') }}</button>
          </footer>
        </form>
      </dialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LazyImage from '@/components/common/LazyImage.vue'
import { useInView } from '@/composables/useInView'
import {
  IMAGE_PLAZA_REPORT_REASONS,
  listImagePlaza,
  reportImagePlaza,
  resolvePlazaImageUrl,
  type ImagePlazaReportReason,
} from '@/api/imagePlaza'
import { reviewPublication } from '@/api/imageLibrary'
import type { ImagePlazaItem } from '@/features/image-workflow/types'
import { useAppStore, useAuthStore } from '@/stores'
import { buildOssThumbnailUrl } from '@/utils/ossThumbnail'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const ratios = ['1:1', '2:3', '3:2', '3:4', '4:3', '4:5', '5:4', '9:16', '16:9', '21:9']
const loading = ref(false)
const loadingMore = ref(false)
const reporting = ref(false)
const busyId = ref('')
const error = ref('')
const items = ref<ImagePlazaItem[]>([])
const nextCursor = ref<string | null>(null)
const total = ref<number | null>(null)
const broken = ref(new Set<string>())
const previewDialog = ref<HTMLDialogElement | null>(null)
const reportDialog = ref<HTMLDialogElement | null>(null)
const previewItem = ref<ImagePlazaItem | null>(null)
const reportItem = ref<ImagePlazaItem | null>(null)
const { target: loadMoreSentinel, inView: loadMoreInView } = useInView({ rootMargin: '400px 0px', once: false })
const filters = reactive({ q: '', platform: '', aspectRatio: '', sort: 'newest' as 'newest' | 'oldest' })
const reportForm = reactive<{ reason: ImagePlazaReportReason; detail: string }>({
  reason: IMAGE_PLAZA_REPORT_REASONS[0],
  detail: '',
})

function query(cursor?: string) {
  return {
    q: filters.q.trim() || undefined,
    platform: filters.platform || undefined,
    aspect_ratio: filters.aspectRatio || undefined,
    sort: filters.sort,
    cursor,
    limit: 36,
  }
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const page = await listImagePlaza(query())
    items.value = page.items
    nextCursor.value = page.next_cursor
    total.value = page.total ?? null
  } catch (cause: any) {
    error.value = cause?.message || t('imagePlaza.loadFailed')
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (!nextCursor.value || loadingMore.value) return
  loadingMore.value = true
  try {
    const page = await listImagePlaza(query(nextCursor.value))
    const existing = new Set(items.value.map((item) => String(item.id)))
    items.value.push(...page.items.filter((item) => !existing.has(String(item.id))))
    nextCursor.value = page.next_cursor
  } catch (cause: any) {
    appStore.showError(cause?.message || t('imagePlaza.loadFailed'))
  } finally {
    loadingMore.value = false
    await nextTick()
    if (loadMoreInView.value && nextCursor.value) void loadMore()
  }
}

function plazaThumbUrl(item: ImagePlazaItem) {
  if (broken.value.has(String(item.id))) return ''
  return buildOssThumbnailUrl(resolvePlazaImageUrl(item), { width: 480 })
}

function reuse(item: ImagePlazaItem) {
  if (!item.share_prompt || !item.prompt) return
  closePreview()
  router.push({ path: '/image-workbench', query: { prompt: item.prompt, model: item.model, size: item.size || undefined } })
}

async function adminHide(item: ImagePlazaItem) {
  const id = item.publication_id || item.id
  if (!id || !window.confirm(t('imageWorkflow.plaza.adminDeleteConfirm'))) return
  busyId.value = String(item.id)
  try {
    await reviewPublication(id, 'hide', t('imageWorkflow.plaza.adminDeleteReason'))
    items.value = items.value.filter((candidate) => candidate.id !== item.id)
    if (total.value != null) total.value = Math.max(0, total.value - 1)
    appStore.showSuccess(t('imageWorkflow.plaza.adminDeleted'))
  } catch (cause: any) {
    appStore.showError(cause?.message || t('imageWorkflow.library.actionFailed'))
  } finally {
    busyId.value = ''
  }
}

function openPreview(item: ImagePlazaItem) {
  previewItem.value = item
  previewDialog.value?.showModal()
}
function closePreview() { previewDialog.value?.close(); previewItem.value = null }
function closeOnBackdrop(event: MouseEvent) { if (event.target === previewDialog.value) closePreview() }

function openReport(item: ImagePlazaItem) {
  reportItem.value = item
  reportForm.reason = IMAGE_PLAZA_REPORT_REASONS[0]
  reportForm.detail = ''
  reportDialog.value?.showModal()
}
function closeReport() { reportDialog.value?.close(); reportItem.value = null }
function closeReportOnBackdrop(event: MouseEvent) { if (event.target === reportDialog.value) closeReport() }

async function submitReport() {
  if (!reportItem.value) return
  reporting.value = true
  try {
    await reportImagePlaza(reportItem.value.publication_id || reportItem.value.id, {
      reason: reportForm.reason,
      detail: reportForm.detail.trim() || undefined,
    })
    closeReport()
    appStore.showSuccess(t('imageWorkflow.plaza.reportSubmitted'))
  } catch (cause: any) {
    appStore.showError(cause?.message || t('imageWorkflow.plaza.reportFailed'))
  } finally {
    reporting.value = false
  }
}

function markBroken(id: string | number) { broken.value = new Set([...broken.value, String(id)]) }
function platformName(platform: string) { return platform === 'openai' ? 'OpenAI' : platform === 'gemini' ? 'Gemini' : platform === 'grok' ? 'Grok' : platform }
function formatDate(value: string) { const time = Date.parse(value); return Number.isFinite(time) ? new Date(time).toLocaleString() : value }

watch(loadMoreInView, (visible) => {
  if (visible && nextCursor.value && !loadingMore.value && !loading.value) void loadMore()
})

onMounted(refresh)
</script>

<style scoped>
.plaza-page { max-width: 1580px; margin: 0 auto; }
.plaza-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; margin-bottom: 1rem; }
.plaza-header h1 { color: #111827; font-size: 1.5rem; font-weight: 750; line-height: 1.25; }
.dark .plaza-header h1 { color: #f9fafb; }
.plaza-header p { margin-top: 0.3rem; color: #6b7280; font-size: 0.875rem; }
.dark .plaza-header p { color: #9ca3af; }
.plaza-header__links { display: flex; flex-wrap: wrap; gap: 0.5rem; }
.plaza-toolbar { display: grid; grid-template-columns: minmax(220px, 1fr) repeat(3, minmax(120px, auto)) 2.25rem; align-items: end; gap: 0.5rem; padding: 0.65rem; border: 1px solid #e5e7eb; border-radius: 8px; background: #fff; }
.dark .plaza-toolbar { border-color: #374151; background: #111827; }
.plaza-search { display: flex; height: 2.25rem; align-items: center; gap: 0.45rem; padding: 0 0.65rem; border: 1px solid #d1d5db; border-radius: 6px; color: #6b7280; }
.dark .plaza-search { border-color: #4b5563; }
.plaza-search:focus-within { border-color: #0d9488; box-shadow: 0 0 0 2px rgba(13,148,136,0.14); }
.plaza-search input { min-width: 0; flex: 1; border: 0; outline: 0; background: transparent; color: #111827; font-size: 0.78rem; }
.dark .plaza-search input { color: #f3f4f6; }
.plaza-select span { display: block; margin-bottom: 0.25rem; color: #6b7280; font-size: 0.63rem; font-weight: 650; }
.plaza-select select { width: 100%; height: 2.25rem; padding: 0 1.65rem 0 0.55rem; border: 1px solid #d1d5db; border-radius: 6px; background: transparent; color: inherit; font-size: 0.72rem; }
.dark .plaza-select select { border-color: #4b5563; }
.plaza-icon-button,
.plaza-action { display: inline-grid; width: 2.25rem; height: 2.25rem; flex: 0 0 auto; place-items: center; border: 1px solid #d1d5db; border-radius: 6px; color: #4b5563; }
.dark .plaza-icon-button,
.dark .plaza-action { border-color: #4b5563; color: #d1d5db; }
.plaza-icon-button:hover,
.plaza-action:hover { border-color: #0d9488; color: #0f766e; }
.plaza-action:disabled { cursor: not-allowed; opacity: 0.35; }
.plaza-summary { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: 0.65rem 0.15rem; color: #6b7280; font-size: 0.7rem; }
.plaza-summary strong { color: #0f766e; font-weight: 700; }

.plaza-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(245px, 1fr)); gap: 0.75rem; }
.plaza-item { min-width: 0; overflow: hidden; border: 1px solid #e5e7eb; border-radius: 8px; background: #fff; }
.dark .plaza-item { border-color: #374151; background: #111827; }
.plaza-item__media { position: relative; display: block; width: 100%; aspect-ratio: 4 / 3; overflow: hidden; background: #f3f4f6; }
.dark .plaza-item__media { background: #030712; }
.plaza-item__lazy { width: 100%; height: 100%; }
.plaza-item__media :deep(img) { width: 100%; height: 100%; object-fit: cover; transition: transform 0.18s ease; }
.plaza-item__media:hover :deep(img) { transform: scale(1.015); }
.plaza-item__media:focus-visible { outline: 2px solid #0d9488; outline-offset: -2px; }
.plaza-item__platform { position: absolute; left: 0.45rem; bottom: 0.45rem; padding: 0.2rem 0.4rem; border-radius: 4px; background: rgba(17,24,39,0.84); color: #f9fafb; font-size: 0.62rem; font-weight: 700; }
.plaza-item__broken { display: flex; width: 100%; height: 100%; flex-direction: column; align-items: center; justify-content: center; gap: 0.35rem; color: #9ca3af; font-size: 0.7rem; }
.plaza-item__body { padding: 0.7rem 0.7rem 0.4rem; }
.plaza-item__title-row { display: flex; min-width: 0; align-items: flex-start; gap: 0.45rem; }
.plaza-item__title-row h2 { min-width: 0; flex: 1; overflow: hidden; color: #111827; font-size: 0.82rem; font-weight: 750; text-overflow: ellipsis; white-space: nowrap; }
.dark .plaza-item__title-row h2 { color: #f9fafb; }
.owner-badge { flex: 0 0 auto; padding: 0.12rem 0.32rem; border-radius: 4px; background: #ccfbf1; color: #115e59; font-size: 0.6rem; font-weight: 700; }
.plaza-item__meta { display: flex; flex-wrap: wrap; gap: 0.35rem 0.6rem; margin-top: 0.3rem; color: #6b7280; font-size: 0.64rem; }
.plaza-item__prompt { display: -webkit-box; min-height: 2.1rem; margin-top: 0.45rem; overflow: hidden; color: #4b5563; font-size: 0.7rem; line-height: 1.5; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.dark .plaza-item__prompt { color: #d1d5db; }
.plaza-item__prompt.is-private { color: #9ca3af; font-style: italic; }
.plaza-item__publisher { display: flex; min-width: 0; align-items: center; gap: 0.3rem; margin-top: 0.55rem; color: #6b7280; font-size: 0.66rem; }
.plaza-item__publisher span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.plaza-item__actions { display: flex; align-items: center; gap: 0.35rem; padding: 0.4rem 0.7rem 0.7rem; }
.plaza-text-action { min-width: 0; flex: 1; height: 2.25rem; overflow: hidden; padding: 0 0.55rem; border: 1px solid #d1d5db; border-radius: 6px; color: #4b5563; font-size: 0.7rem; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.dark .plaza-text-action { border-color: #4b5563; color: #d1d5db; }
.plaza-text-action:hover { border-color: #0d9488; color: #0f766e; }
.plaza-text-action.is-danger { border-color: #fca5a5; color: #b91c1c; }
.plaza-text-action.is-danger:hover { border-color: #ef4444; color: #991b1b; }
.plaza-empty { display: flex; min-height: 17rem; flex-direction: column; align-items: center; justify-content: center; gap: 0.45rem; border: 1px dashed #d1d5db; border-radius: 8px; color: #6b7280; text-align: center; font-size: 0.78rem; }
.dark .plaza-empty { border-color: #374151; color: #9ca3af; }
.plaza-empty.is-error { color: #b91c1c; }
.plaza-spinner { display: inline-block; width: 1rem; height: 1rem; border: 2px solid #99f6e4; border-top-color: #0f766e; border-radius: 50%; animation: plaza-spin 0.75s linear infinite; }
.plaza-load-more { display: flex; width: 100%; min-height: 2.6rem; align-items: center; justify-content: center; gap: 0.45rem; margin-top: 0.9rem; border: 1px solid #d1d5db; border-radius: 6px; color: #4b5563; font-size: 0.78rem; font-weight: 700; }
.dark .plaza-load-more { border-color: #4b5563; color: #d1d5db; }

.plaza-dialog { width: min(880px, calc(100vw - 2rem)); max-height: calc(100vh - 2rem); padding: 0; overflow: auto; border: 0; border-radius: 8px; background: transparent; }
.plaza-dialog::backdrop { background: rgba(3,7,18,0.72); }
.plaza-dialog__surface { overflow: hidden; border: 1px solid #d1d5db; border-radius: 8px; background: #fff; color: #111827; }
.dark .plaza-dialog__surface { border-color: #374151; background: #111827; color: #f9fafb; }
.plaza-dialog__surface > header { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; padding: 0.8rem; border-bottom: 1px solid #e5e7eb; }
.dark .plaza-dialog__surface > header { border-color: #374151; }
.plaza-dialog__surface > header h2 { overflow-wrap: anywhere; font-size: 0.95rem; font-weight: 750; }
.plaza-dialog__surface > header p { margin-top: 0.15rem; color: #6b7280; font-size: 0.68rem; }
.plaza-dialog__surface > img { width: 100%; max-height: 68vh; object-fit: contain; background: #030712; }
.plaza-dialog__prompt { padding: 0.8rem; color: #4b5563; font-size: 0.78rem; line-height: 1.6; white-space: pre-wrap; }
.dark .plaza-dialog__prompt { color: #d1d5db; }
.plaza-dialog__surface > footer { display: flex; justify-content: flex-end; gap: 0.5rem; padding: 0.8rem; border-top: 1px solid #e5e7eb; }
.dark .plaza-dialog__surface > footer { border-color: #374151; }
.plaza-dialog__surface--form { padding-bottom: 0.1rem; }
.dialog-field { display: block; margin: 0.8rem; }
.dialog-field > span { display: block; margin-bottom: 0.35rem; color: #4b5563; font-size: 0.72rem; font-weight: 650; }
.dark .dialog-field > span { color: #d1d5db; }
.dialog-field .input { width: 100%; }

@keyframes plaza-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .plaza-spinner { animation: none; } .plaza-item__media :deep(img) { transition: none; } }
@media (max-width: 900px) { .plaza-toolbar { grid-template-columns: 1fr 1fr; } .plaza-search { grid-column: 1 / -1; } }
@media (max-width: 640px) { .plaza-header { flex-direction: column; } .plaza-toolbar { grid-template-columns: 1fr; } .plaza-search { grid-column: auto; } .plaza-grid { grid-template-columns: 1fr 1fr; } }
@media (max-width: 430px) { .plaza-grid { grid-template-columns: 1fr; } .plaza-header__links { width: 100%; } .plaza-header__links > * { flex: 1; justify-content: center; } }
</style>
