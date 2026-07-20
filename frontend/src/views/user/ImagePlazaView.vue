<template>
  <AppLayout>
    <div class="img-plaza space-y-4">
      <div class="img-plaza-hero card">
        <div class="img-plaza-hero__beam" aria-hidden="true"></div>
        <div class="relative flex flex-wrap items-end justify-between gap-4">
          <div>
            <p class="img-plaza-kicker">GALLERY // GLOBAL FEED</p>
            <h1 class="img-plaza-title">{{ t('imagePlaza.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
              {{ t('imagePlaza.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <div class="img-plaza-search">
              <Icon name="search" size="sm" class="text-gray-400" />
              <input
                v-model="query"
                type="search"
                class="img-plaza-search__input"
                :placeholder="t('imagePlaza.searchPlaceholder')"
                @keydown.enter="loadImages"
              />
            </div>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadImages">
              <Icon name="refresh" size="sm" :class="loading && 'animate-spin'" />
            </button>
          </div>
        </div>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-2 px-0.5">
        <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('imagePlaza.sharedHint') }}</span>
        <div class="flex items-center gap-3">
          <button
            v-if="selectedOwnIds.length"
            type="button"
            class="btn btn-danger"
            @click="bulkDelete"
          >
            {{ t('imagePlaza.bulkDelete', { n: selectedOwnIds.length }) }}
          </button>
          <span class="img-plaza-count">{{ t('imagePlaza.total', { n: total }) }}</span>
        </div>
      </div>

      <div v-if="loading" class="img-plaza-empty card">{{ t('common.loading') }}</div>
      <div v-else-if="!images.length" class="img-plaza-empty card">
        <div class="img-plaza-empty__grid" aria-hidden="true"></div>
        <p>{{ t('imagePlaza.empty') }}</p>
        <RouterLink to="/image-workbench" class="btn btn-primary mt-3">
          {{ t('imagePlaza.goWorkbench') }}
        </RouterLink>
      </div>

      <div v-else class="img-plaza-grid">
        <article
          v-for="(item, idx) in images"
          :key="item.id"
          class="img-plaza-card card"
          :style="{ animationDelay: `${Math.min(idx, 12) * 40}ms` }"
        >
          <div class="img-plaza-card__media">
            <label v-if="isMine(item)" class="img-plaza-card__check" @click.stop>
              <input
                type="checkbox"
                :checked="selectedIds.has(item.id)"
                @change="toggleSelect(item.id)"
              />
            </label>
            <img
              :src="resolvePlazaImageUrl(item)"
              :alt="item.title"
              loading="lazy"
              @click="openPreview(item)"
            />
            <div class="img-plaza-card__shine" aria-hidden="true"></div>
          </div>

          <div class="img-plaza-card__body">
            <h3 class="img-plaza-card__title">{{ item.title || item.prompt }}</h3>
            <div class="img-plaza-card__meta">
              <span>{{ item.model }}</span>
              <span>{{ formatTime(item.created_at) }}</span>
              <span class="img-plaza-card__source">{{ maskEmail(item.user_email) }}</span>
            </div>
            <p class="img-plaza-card__prompt">{{ item.prompt }}</p>
          </div>

          <div class="img-plaza-card__actions">
            <button type="button" class="img-plaza-iconbtn" :title="t('imagePlaza.reuse')" @click="goReuse(item)">
              <Icon name="refresh" size="sm" />
            </button>
            <button type="button" class="img-plaza-iconbtn" :title="t('imageWorkbench.download')" @click="download(item)">
              <Icon name="download" size="sm" />
            </button>
            <button type="button" class="btn btn-primary img-plaza-same" @click="goReuse(item)">
              {{ t('imagePlaza.oneClickSame') }}
            </button>
            <button
              v-if="isMine(item)"
              type="button"
              class="img-plaza-iconbtn is-danger"
              :title="t('common.delete')"
              @click="remove(item)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </article>
      </div>
    </div>

    <BaseDialog :show="previewOpen" :title="t('imageWorkbench.view')" width="wide" @close="previewOpen = false">
      <div v-if="previewTarget" class="space-y-3">
        <img :src="resolvePlazaImageUrl(previewTarget)" :alt="previewTarget.title" class="w-full rounded-xl" />
        <p class="font-mono text-xs text-teal-600 dark:text-teal-300">
          {{ previewTarget.model }} · {{ formatTime(previewTarget.created_at) }}
        </p>
        <p class="text-sm text-gray-600 dark:text-dark-300 whitespace-pre-wrap">{{ previewTarget.prompt }}</p>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn btn-primary" @click="goReuse(previewTarget)">
            {{ t('imagePlaza.oneClickSame') }}
          </button>
          <button type="button" class="btn btn-secondary" @click="download(previewTarget)">
            {{ t('imageWorkbench.download') }}
          </button>
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import {
  deleteImagePlaza,
  listImagePlaza,
  resolvePlazaImageUrl,
  type ImagePlazaItem
} from '@/api/imagePlaza'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const router = useRouter()

const loading = ref(false)
const query = ref('')
const images = ref<ImagePlazaItem[]>([])
const total = ref(0)
const selectedIds = ref(new Set<number>())
const previewOpen = ref(false)
const previewTarget = ref<ImagePlazaItem | null>(null)

const myUserId = computed(() => Number(authStore.user?.id || 0))
const selectedOwnIds = computed(() =>
  [...selectedIds.value].filter((id) => images.value.some((item) => item.id === id && isMine(item)))
)

function isMine(item: ImagePlazaItem) {
  return myUserId.value > 0 && item.user_id === myUserId.value
}

function formatTime(value: string) {
  const ts = Date.parse(value)
  return Number.isFinite(ts) ? new Date(ts).toLocaleString() : value
}

function maskEmail(email?: string) {
  const v = (email || '').trim()
  if (!v) return 'user'
  const [name, domain] = v.split('@')
  if (!domain) return v.slice(0, 2) + '***'
  const head = name.slice(0, 2)
  return `${head}***@${domain}`
}

async function loadImages() {
  loading.value = true
  try {
    const res = await listImagePlaza({
      q: query.value.trim() || undefined,
      page: 1,
      page_size: 60
    })
    images.value = res.items || []
    total.value = res.total || 0
    selectedIds.value = new Set()
  } catch (error: any) {
    appStore.showError(error?.message || t('imagePlaza.loadFailed'))
  } finally {
    loading.value = false
  }
}

function toggleSelect(id: number) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

async function remove(item: ImagePlazaItem) {
  if (!isMine(item)) return
  try {
    await deleteImagePlaza(item.id)
    if (previewTarget.value?.id === item.id) {
      previewOpen.value = false
      previewTarget.value = null
    }
    await loadImages()
  } catch (error: any) {
    appStore.showError(error?.message || t('common.error'))
  }
}

async function bulkDelete() {
  const ids = selectedOwnIds.value
  for (const id of ids) {
    try {
      await deleteImagePlaza(id)
    } catch {
      // continue
    }
  }
  await loadImages()
  appStore.showSuccess(t('common.success'))
}

async function download(item: ImagePlazaItem) {
  try {
    const url = resolvePlazaImageUrl(item)
    const res = await fetch(url)
    const blob = await res.blob()
    const objectUrl = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = objectUrl
    link.download = `plaza-${item.id}.${item.format || 'png'}`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(objectUrl)
  } catch (error: any) {
    appStore.showError(error?.message || t('common.error'))
  }
}

function openPreview(item: ImagePlazaItem) {
  previewTarget.value = item
  previewOpen.value = true
}

function goReuse(item: ImagePlazaItem) {
  router.push({
    path: '/image-workbench',
    query: {
      prompt: item.prompt,
      model: item.model
    }
  })
}

onMounted(loadImages)
</script>

<style scoped>
.img-plaza {
  --plaza-cyan: #14b8a6;
  font-family: 'Sora', ui-sans-serif, system-ui, sans-serif;
}

.img-plaza-hero {
  position: relative;
  overflow: hidden;
  padding: 1.15rem 1.25rem;
  border: 1px solid color-mix(in srgb, var(--plaza-cyan) 24%, transparent);
  background:
    radial-gradient(700px 160px at 85% -30%, rgba(20, 184, 166, 0.2), transparent 60%),
    linear-gradient(120deg, rgba(20, 184, 166, 0.05), transparent 40%);
}

.dark .img-plaza-hero {
  background:
    radial-gradient(700px 160px at 85% -30%, rgba(20, 184, 166, 0.22), transparent 60%),
    linear-gradient(120deg, rgba(2, 6, 23, 0.7), rgba(15, 23, 42, 0.25));
}

.img-plaza-hero__beam {
  position: absolute;
  inset: 0 auto 0 -20%;
  width: 40%;
  background: linear-gradient(90deg, transparent, rgba(20, 184, 166, 0.12), transparent);
  transform: skewX(-18deg);
  animation: plaza-beam 7s ease-in-out infinite;
  pointer-events: none;
}

.img-plaza-kicker {
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.7rem;
  letter-spacing: 0.16em;
  color: var(--plaza-cyan);
  margin-bottom: 0.25rem;
}

.img-plaza-title {
  font-family: 'Oxanium', ui-sans-serif, system-ui, sans-serif;
  font-size: 1.45rem;
  font-weight: 700;
}

.img-plaza-search {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  min-width: min(320px, 70vw);
  padding: 0.45rem 0.7rem;
  border-radius: 0.7rem;
  border: 1px solid color-mix(in srgb, var(--plaza-cyan) 28%, transparent);
  background: rgba(15, 23, 42, 0.03);
}

.dark .img-plaza-search {
  background: rgba(2, 6, 23, 0.45);
}

.img-plaza-search__input {
  flex: 1;
  background: transparent;
  border: 0;
  outline: none;
  font-size: 0.85rem;
  color: inherit;
}

.img-plaza-count {
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.78rem;
  color: var(--plaza-cyan);
}

.img-plaza-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 1rem;
}

.img-plaza-card {
  overflow: hidden;
  display: flex;
  flex-direction: column;
  border: 1px solid color-mix(in srgb, var(--plaza-cyan) 14%, transparent);
  animation: plaza-rise 0.45s ease both;
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;
}

.img-plaza-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--plaza-cyan) 45%, transparent);
  box-shadow: 0 12px 30px rgba(20, 184, 166, 0.12);
}

.img-plaza-card__media {
  position: relative;
  aspect-ratio: 16 / 11;
  background: #0b1220;
  overflow: hidden;
}

.img-plaza-card__media img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  cursor: zoom-in;
  transition: transform 0.35s ease;
}

.img-plaza-card:hover .img-plaza-card__media img {
  transform: scale(1.04);
}

.img-plaza-card__shine {
  position: absolute;
  inset: 0;
  background: linear-gradient(120deg, transparent 30%, rgba(255, 255, 255, 0.08), transparent 70%);
  transform: translateX(-120%);
  pointer-events: none;
}

.img-plaza-card:hover .img-plaza-card__shine {
  animation: plaza-shine 0.9s ease;
}

.img-plaza-card__check {
  position: absolute;
  top: 0.55rem;
  left: 0.55rem;
  z-index: 2;
  width: 1.2rem;
  height: 1.2rem;
  display: grid;
  place-items: center;
  border-radius: 0.3rem;
  background: rgba(15, 23, 42, 0.55);
  backdrop-filter: blur(4px);
}

.img-plaza-card__body {
  padding: 0.85rem 0.9rem 0.5rem;
  flex: 1;
}

.img-plaza-card__title {
  font-size: 0.9rem;
  font-weight: 650;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.img-plaza-card__meta {
  margin-top: 0.35rem;
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.68rem;
  color: #64748b;
}

.dark .img-plaza-card__meta {
  color: #94a3b8;
}

.img-plaza-card__source {
  margin-left: auto;
  color: var(--plaza-cyan);
}

.img-plaza-card__prompt {
  margin-top: 0.45rem;
  font-size: 0.75rem;
  color: #64748b;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-height: 2.2em;
}

.dark .img-plaza-card__prompt {
  color: #94a3b8;
}

.img-plaza-card__actions {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.65rem 0.85rem 0.85rem;
}

.img-plaza-iconbtn {
  width: 2rem;
  height: 2rem;
  display: grid;
  place-items: center;
  border-radius: 0.5rem;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: transparent;
  color: inherit;
}

.img-plaza-iconbtn:hover {
  border-color: var(--plaza-cyan);
  color: var(--plaza-cyan);
}

.img-plaza-iconbtn.is-danger:hover {
  border-color: #f87171;
  color: #f87171;
}

.img-plaza-same {
  flex: 1;
  padding-left: 0.6rem;
  padding-right: 0.6rem;
  font-size: 0.78rem;
  box-shadow: 0 0 16px rgba(20, 184, 166, 0.25);
}

.img-plaza-empty {
  position: relative;
  min-height: 220px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  color: #64748b;
}

.img-plaza-empty__grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(20, 184, 166, 0.07) 1px, transparent 1px),
    linear-gradient(90deg, rgba(20, 184, 166, 0.07) 1px, transparent 1px);
  background-size: 26px 26px;
  mask-image: radial-gradient(circle at center, black, transparent 70%);
}

@keyframes plaza-rise {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes plaza-shine {
  to {
    transform: translateX(120%);
  }
}

@keyframes plaza-beam {
  0%,
  100% {
    transform: translateX(0) skewX(-18deg);
    opacity: 0.35;
  }
  50% {
    transform: translateX(160%) skewX(-18deg);
    opacity: 0.7;
  }
}

@media (prefers-reduced-motion: reduce) {
  .img-plaza-card,
  .img-plaza-hero__beam,
  .img-plaza-card__shine {
    animation: none !important;
  }
}
</style>
