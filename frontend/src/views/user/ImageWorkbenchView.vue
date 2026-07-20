<template>
  <AppLayout>
    <div class="img-lab space-y-4">
      <div class="img-lab-hero card">
        <div class="img-lab-hero__glow" aria-hidden="true"></div>
        <div class="relative flex flex-wrap items-start justify-between gap-3">
          <div>
            <p class="img-lab-kicker">IMAGE // WORKBENCH</p>
            <h1 class="img-lab-title">{{ t('imageWorkbench.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
              {{ t('imageWorkbench.description') }}
            </p>
          </div>
          <button type="button" class="btn btn-secondary" :disabled="loadingKeys || generating" @click="refreshAll">
            <Icon name="refresh" size="sm" class="mr-1.5" :class="loadingKeys && 'animate-spin'" />
            {{ t('common.refresh') }}
          </button>
        </div>
      </div>

      <div class="img-lab-grid">
        <!-- Left: key / model / reference -->
        <aside class="img-lab-col space-y-4">
          <section class="card img-lab-panel">
            <h2 class="img-lab-panel__title mb-1">{{ t('imageWorkbench.selectKey') }}</h2>
            <p class="img-lab-keyhint mb-3">
              {{ t('imageWorkbench.detectedImageKeys', { n: imageCapableKeys.length }) }}
            </p>
            <Select
              v-model="form.apiKeyId"
              :options="apiKeyOptions"
              :disabled="loadingKeys || generating"
              :searchable="true"
              :placeholder="t('imageWorkbench.selectKeyPlaceholder')"
              class="w-full"
            >
              <template #selected="{ option }">
                <span v-if="option" class="truncate font-medium">{{ option.label }}</span>
                <span v-else class="text-gray-400">{{ t('imageWorkbench.selectKeyPlaceholder') }}</span>
              </template>
              <template #option="{ option, selected }">
                <div
                  class="img-lab-keyopt"
                  :class="{ 'is-disabled': option.disabled, 'is-selected': selected }"
                >
                  <div class="img-lab-keyopt__main">
                    <div class="img-lab-keyopt__row">
                      <span class="img-lab-keyopt__label">{{ option.label }}</span>
                      <span v-if="option.imageEnabled" class="img-lab-keyopt__badge">
                        {{ t('imageWorkbench.imageEnabledBadge') }}
                      </span>
                    </div>
                    <p v-if="option.subtitle" class="img-lab-keyopt__sub">{{ option.subtitle }}</p>
                  </div>
                  <Icon
                    v-if="selected && !option.disabled"
                    name="check"
                    size="sm"
                    class="img-lab-keyopt__check"
                    :stroke-width="2"
                  />
                </div>
              </template>
            </Select>

            <div v-if="selectedKey && selectedKeyAllowsImage" class="img-lab-keycard mt-3">
              <div class="img-lab-keycard__tags">
                <span class="img-lab-tag img-lab-tag--group">
                  {{ selectedKey.group?.name || selectedKey.name }}
                </span>
                <span class="img-lab-tag img-lab-tag--ok">
                  {{ t('imageWorkbench.usableForImage') }}
                </span>
                <span class="img-lab-tag img-lab-tag--rate">
                  {{ rateMultiplierLabel }}
                </span>
                <span v-if="price2kLabel" class="img-lab-tag img-lab-tag--price">
                  {{ price2kLabel }}
                </span>
              </div>
              <div class="img-lab-keycard__rows">
                <div class="img-lab-keycard__row">
                  <span>{{ t('imageWorkbench.apiKeyLabel') }}</span>
                  <span class="font-mono">{{ maskApiKey(selectedKey.key) }}</span>
                </div>
                <div class="img-lab-keycard__row">
                  <span>{{ t('imageWorkbench.price2kLabel') }}</span>
                  <span class="font-mono text-teal-700 dark:text-teal-300">{{ price2kValue }}</span>
                </div>
                <div class="img-lab-keycard__row">
                  <span>{{ t('imageWorkbench.statusLabel') }}</span>
                  <span :class="selectedKey.status === 'active' ? 'text-emerald-600 dark:text-emerald-400' : ''">
                    {{ selectedKey.status }}
                  </span>
                </div>
              </div>
            </div>
            <p v-else-if="!loadingKeys && !imageCapableKeys.length" class="mt-3 text-xs text-amber-600 dark:text-amber-400">
              {{ t('imageWorkbench.noImageKeyHint') }}
            </p>
            <p v-else-if="!selectedKey" class="mt-3 text-xs text-gray-500 dark:text-dark-400">
              {{ t('imageWorkbench.noKeyHint') }}
            </p>
          </section>

          <section class="card img-lab-panel">
            <h2 class="img-lab-panel__title">{{ t('imageWorkbench.model') }}</h2>
            <div class="img-lab-models">
              <button
                v-for="m in MODEL_OPTIONS"
                :key="m"
                type="button"
                class="img-lab-model"
                :class="{ 'is-active': form.model === m }"
                :disabled="generating"
                @click="form.model = m"
              >
                {{ m }}
              </button>
            </div>
          </section>

          <section class="card img-lab-panel">
            <div class="mb-3 flex items-center justify-between gap-2">
              <h2 class="img-lab-panel__title mb-0">{{ t('imageWorkbench.reference') }}</h2>
              <span class="img-lab-refcount">{{ referenceImages.length }}/{{ MAX_REFERENCE_IMAGES }}</span>
            </div>

            <label
              class="img-lab-upload"
              :class="{ 'is-disabled': generating || referenceImages.length >= MAX_REFERENCE_IMAGES }"
            >
              <input
                ref="fileInputRef"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                multiple
                class="hidden"
                :disabled="generating || referenceImages.length >= MAX_REFERENCE_IMAGES"
                @change="onReferenceChange"
              />
              <Icon name="upload" size="lg" class="text-primary-500" />
              <span class="img-lab-upload__title">{{ t('imageWorkbench.uploadTitle') }}</span>
              <span class="img-lab-upload__hint">{{ t('imageWorkbench.uploadHint') }}</span>
            </label>

            <div v-if="referenceImages.length" class="img-lab-reflist">
              <div v-for="item in referenceImages" :key="item.id" class="img-lab-refitem">
                <div class="img-lab-refitem__media">
                  <img :src="item.previewUrl" :alt="item.name" loading="lazy" decoding="async" />
                  <button
                    type="button"
                    class="img-lab-refitem__remove"
                    :title="t('common.delete')"
                    :disabled="generating"
                    @click.stop.prevent="removeReference(item.id)"
                  >
                    <Icon name="x" size="sm" />
                  </button>
                </div>
                <div class="img-lab-refitem__name" :title="item.name">{{ truncateFileName(item.name) }}</div>
              </div>
            </div>
          </section>
        </aside>

        <!-- Center: prompt + preview -->
        <main class="img-lab-col img-lab-col--main space-y-4">
          <section class="card img-lab-panel">
            <div class="mb-3 flex items-center justify-between gap-2">
              <h2 class="img-lab-panel__title mb-0">{{ t('imageWorkbench.promptParams') }}</h2>
              <span class="img-lab-signal" aria-hidden="true">
                <i></i>{{ generating ? 'RENDERING' : 'READY' }}
              </span>
            </div>

            <div class="img-lab-endpoint mb-3" role="status">
              <span class="img-lab-endpoint__dot" aria-hidden="true"></span>
              <span>
                {{
                  referenceImages.length
                    ? t('imageWorkbench.endpointEdits', { url: gatewayEndpointEdits })
                    : t('imageWorkbench.endpointGenerations', { url: gatewayEndpointGenerations })
                }}
              </span>
            </div>

            <textarea
              v-model="form.prompt"
              class="input img-lab-prompt"
              rows="5"
              :placeholder="t('imageWorkbench.promptPlaceholder')"
              :disabled="generating"
            />

            <div class="img-lab-params mt-4">
              <label class="img-lab-field">
                <span>{{ t('imageWorkbench.size') }}</span>
                <Select
                  v-model="form.size"
                  :options="sizeOptions"
                  :searchable="true"
                  :disabled="generating"
                />
              </label>
              <label class="img-lab-field">
                <span>{{ t('imageWorkbench.count') }}</span>
                <Select v-model="form.n" :options="countOptions" :disabled="generating" />
              </label>
              <label class="img-lab-field">
                <span>{{ t('imageWorkbench.quality') }}</span>
                <Select v-model="form.quality" :options="qualityOptions" :disabled="generating" />
              </label>
              <label class="img-lab-field">
                <span>{{ t('imageWorkbench.format') }}</span>
                <Select v-model="form.format" :options="formatOptions" :disabled="generating" />
              </label>
              <label class="img-lab-field">
                <span>{{ t('imageWorkbench.background') }}</span>
                <Select v-model="form.background" :options="backgroundOptions" :disabled="generating" />
              </label>
              <label class="img-lab-field">
                <span>{{ t('imageWorkbench.style') }}</span>
                <Select v-model="form.style" :options="styleOptions" :disabled="generating" />
              </label>
            </div>

            <div class="mt-4 flex flex-wrap items-center gap-3">
              <label class="img-lab-switch">
                <input v-model="form.syncPlaza" type="checkbox" :disabled="generating" />
                <span>{{ t('imageWorkbench.syncPlaza') }}</span>
              </label>
            </div>

            <div class="mt-4 flex flex-wrap gap-2">
              <button type="button" class="btn btn-primary img-lab-go" :disabled="!canGenerate" @click="startGenerate">
                <span v-if="generating" class="img-lab-go__spin" aria-hidden="true"></span>
                {{ generating ? t('imageWorkbench.generating') : t('imageWorkbench.start') }}
              </button>
              <button type="button" class="btn btn-secondary" :disabled="generating" @click="form.prompt = ''">
                {{ t('imageWorkbench.clearPrompt') }}
              </button>
              <button type="button" class="btn btn-ghost" :disabled="generating" @click="resetForm">
                {{ t('imageWorkbench.reset') }}
              </button>
            </div>
          </section>

          <section class="card img-lab-panel">
            <h2 class="img-lab-panel__title">{{ t('imageWorkbench.resultPreview') }}</h2>
            <div class="img-lab-preview">
              <div v-if="latestImage" class="img-lab-preview__frame">
                <img :src="latestImage.imageDataUrl" :alt="latestImage.title" />
                <div class="img-lab-preview__scan" aria-hidden="true"></div>
              </div>
              <div v-else class="img-lab-preview__empty">
                <div class="img-lab-preview__grid" aria-hidden="true"></div>
                <p>{{ t('imageWorkbench.emptyPreview') }}</p>
              </div>
            </div>
            <div v-if="latestImage" class="mt-3 flex flex-wrap gap-2">
              <button type="button" class="btn btn-secondary" @click="downloadImage(latestImage)">
                <Icon name="download" size="sm" class="mr-1" />{{ t('imageWorkbench.download') }}
              </button>
              <button type="button" class="btn btn-secondary" @click="togglePublic(latestImage)">
                {{ latestImage.public ? t('imageWorkbench.unpublish') : t('imageWorkbench.publish') }}
              </button>
              <button type="button" class="btn btn-secondary" @click="openPreview(latestImage)">
                <Icon name="eye" size="sm" class="mr-1" />{{ t('imageWorkbench.view') }}
              </button>
              <button type="button" class="btn btn-primary" @click="reuseRecord(latestImage)">
                {{ t('imageWorkbench.oneClick') }}
              </button>
            </div>
          </section>
        </main>

        <!-- Right: history -->
        <aside class="img-lab-col">
          <section class="card img-lab-panel img-lab-history">
            <div class="mb-3 flex items-center justify-between">
              <h2 class="img-lab-panel__title mb-0">{{ t('imageWorkbench.history') }}</h2>
              <span class="text-xs text-gray-500 dark:text-dark-400">{{ history.length }}</span>
            </div>
            <div v-if="!history.length" class="img-lab-history__empty">
              {{ t('imageWorkbench.emptyHistory') }}
            </div>
            <ul v-else class="img-lab-history__list">
              <li v-for="item in history" :key="item.id" class="img-lab-history__item">
                <button type="button" class="img-lab-history__thumb" @click="selectHistory(item)">
                  <img :src="item.imageDataUrl" :alt="item.title" />
                </button>
                <div class="img-lab-history__body">
                  <p class="img-lab-history__prompt">{{ item.title || item.prompt }}</p>
                  <p class="img-lab-history__meta">
                    {{ item.model }} · {{ formatTime(item.createdAt) }}
                  </p>
                  <div class="img-lab-history__actions">
                    <button type="button" @click="selectHistory(item)">{{ t('imageWorkbench.detail') }}</button>
                    <button type="button" @click="togglePublic(item)">
                      {{ item.public ? t('imageWorkbench.unpublish') : t('imageWorkbench.publish') }}
                    </button>
                    <button type="button" @click="downloadImage(item)">{{ t('imageWorkbench.download') }}</button>
                    <button type="button" class="is-danger" @click="removeHistory(item)">{{ t('common.delete') }}</button>
                  </div>
                </div>
              </li>
            </ul>
          </section>
        </aside>
      </div>
    </div>

    <BaseDialog :show="previewOpen" :title="t('imageWorkbench.view')" width="wide" @close="previewOpen = false">
      <div v-if="previewTarget" class="space-y-3">
        <img :src="previewTarget.imageDataUrl" :alt="previewTarget.title" class="w-full rounded-xl" />
        <p class="text-sm text-gray-600 dark:text-dark-300 whitespace-pre-wrap">{{ previewTarget.prompt }}</p>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { keysAPI } from '@/api'
import * as imageAPI from '@/api/imageWorkbench'
import { publishImagePlaza } from '@/api/imagePlaza'
import { getSiteGatewayBase } from '@/api/client'
import { useAppStore } from '@/stores'
import type { ApiKey } from '@/types'
import type { SelectOption } from '@/components/common/Select.vue'
import { maskApiKey } from '@/utils/maskApiKey'
import { getDefaultImagePreviewPrice } from '@/views/admin/groupsImagePricing'
import {
  deleteGalleryImage,
  downloadDataUrl,
  listGalleryImages,
  saveGalleryImage,
  truncatePrompt,
  updateGalleryImage,
  type GalleryImageRecord
} from '@/utils/imageGalleryStore'

interface ApiKeySelectOption extends SelectOption {
  imageEnabled?: boolean
  subtitle?: string
  keyId?: number
}

const MODEL_OPTIONS = ['gpt-image-2', 'gpt-image-1.5', 'gpt-image-1'] as const
const MAX_REFERENCE_IMAGES = 5

interface ReferenceImageItem {
  id: string
  file: File
  name: string
  previewUrl: string
}

const SIZE_MAP: Record<string, string> = {
  '1k_square': '1024x1024',
  '1k_landscape': '1536x1024',
  '1k_portrait': '1024x1536',
  '2k_square': '2048x2048',
  '2k_landscape': '2048x1152',
  '2k_portrait': '1152x2048',
  '4k_square': '4096x4096',
  '4k_landscape': '4096x2304',
  '4k_portrait': '2304x4096'
}

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const router = useRouter()

const loadingKeys = ref(false)
const generating = ref(false)
const apiKeys = ref<ApiKey[]>([])
const history = ref<GalleryImageRecord[]>([])
const latestImage = ref<GalleryImageRecord | null>(null)
const referenceImages = ref<ReferenceImageItem[]>([])
const fileInputRef = ref<HTMLInputElement | null>(null)
const previewOpen = ref(false)
const previewTarget = ref<GalleryImageRecord | null>(null)
let abortCtrl: AbortController | null = null

const form = reactive({
  apiKeyId: '' as string | number,
  model: 'gpt-image-2' as string,
  prompt: '',
  size: '1k_square',
  n: '1',
  quality: 'auto',
  format: 'png',
  background: 'auto',
  style: 'auto',
  syncPlaza: true
})

const gatewayBase = computed(() => getSiteGatewayBase())
const gatewayEndpointGenerations = computed(() => `${gatewayBase.value}/images/generations`)
const gatewayEndpointEdits = computed(() => `${gatewayBase.value}/images/edits`)

const sizeOptions = computed<SelectOption[]>(() => [
  { value: '1k_square', label: t('imageWorkbench.sizes.square1k') },
  { value: '1k_landscape', label: t('imageWorkbench.sizes.landscape1k') },
  { value: '1k_portrait', label: t('imageWorkbench.sizes.portrait1k') },
  { value: '2k_square', label: t('imageWorkbench.sizes.square2k') },
  { value: '2k_landscape', label: t('imageWorkbench.sizes.landscape2k') },
  { value: '2k_portrait', label: t('imageWorkbench.sizes.portrait2k') },
  { value: '4k_square', label: t('imageWorkbench.sizes.square4k') },
  { value: '4k_landscape', label: t('imageWorkbench.sizes.landscape4k') },
  { value: '4k_portrait', label: t('imageWorkbench.sizes.portrait4k') }
])

const countOptions: SelectOption[] = [
  { value: '1', label: '1' },
  { value: '2', label: '2' },
  { value: '3', label: '3' },
  { value: '4', label: '4' }
]

const qualityOptions = computed<SelectOption[]>(() => [
  { value: 'auto', label: t('imageWorkbench.auto') },
  { value: 'high', label: t('imageWorkbench.qualityHigh') },
  { value: 'medium', label: t('imageWorkbench.qualityMedium') },
  { value: 'low', label: t('imageWorkbench.qualityLow') }
])

const formatOptions: SelectOption[] = [
  { value: 'png', label: 'PNG' },
  { value: 'jpeg', label: 'JPEG' },
  { value: 'webp', label: 'WEBP' }
]

const backgroundOptions = computed<SelectOption[]>(() => [
  { value: 'auto', label: t('imageWorkbench.auto') },
  { value: 'transparent', label: t('imageWorkbench.backgroundTransparent') },
  { value: 'opaque', label: t('imageWorkbench.backgroundOpaque') }
])

const styleOptions = computed<SelectOption[]>(() => [
  { value: 'auto', label: t('imageWorkbench.auto') },
  { value: 'vivid', label: t('imageWorkbench.styleVivid') },
  { value: 'natural', label: t('imageWorkbench.styleNatural') }
])

function keyAllowsImage(key: ApiKey): boolean {
  return key.status === 'active' && key.group?.allow_image_generation === true
}

function keySubtitle(key: ApiKey): string {
  const groupName = key.group?.name || t('imageWorkbench.ungrouped')
  const platform = key.group?.platform || '—'
  return `${groupName} · ${platform}`
}

function keyOptionLabel(key: ApiKey): string {
  return `${key.name} ${maskApiKey(key.key)}`
}

const imageCapableKeys = computed(() => apiKeys.value.filter(keyAllowsImage))

const apiKeyOptions = computed<ApiKeySelectOption[]>(() => {
  if (!apiKeys.value.length) {
    return [{ value: '', label: t('imageWorkbench.noKeys'), disabled: true }]
  }
  // Image-capable keys first, then disabled others
  const sorted = [...apiKeys.value].sort((a, b) => {
    const ae = keyAllowsImage(a) ? 0 : 1
    const be = keyAllowsImage(b) ? 0 : 1
    if (ae !== be) return ae - be
    return b.id - a.id
  })
  return sorted.map((key) => {
    const imageEnabled = keyAllowsImage(key)
    return {
      value: String(key.id),
      label: keyOptionLabel(key),
      subtitle: keySubtitle(key),
      imageEnabled,
      disabled: !imageEnabled,
      keyId: key.id
    }
  })
})

const selectedKey = computed(() => {
  const id = Number(form.apiKeyId || 0)
  return apiKeys.value.find((k) => k.id === id) || null
})

const selectedKeyAllowsImage = computed(() => !!selectedKey.value && keyAllowsImage(selectedKey.value))

const rateMultiplierLabel = computed(() => {
  const group = selectedKey.value?.group
  if (!group) return '1.00x'
  const rate =
    group.image_rate_independent && group.image_rate_multiplier > 0
      ? group.image_rate_multiplier
      : group.rate_multiplier || 1
  return `${Number(rate).toFixed(2)}x`
})

const price2kAmount = computed(() => {
  const group = selectedKey.value?.group
  if (!group) return null
  if (group.image_price_2k != null && Number.isFinite(Number(group.image_price_2k))) {
    return Number(group.image_price_2k)
  }
  return getDefaultImagePreviewPrice(group.platform || 'openai', 'image_price_2k')
})

const price2kValue = computed(() => {
  const amount = price2kAmount.value
  if (amount == null) return '—'
  return `$${amount.toFixed(3)}`
})

const price2kLabel = computed(() => {
  const amount = price2kAmount.value
  if (amount == null) return ''
  return `2K ${price2kValue.value}`
})

const canGenerate = computed(
  () =>
    !!selectedKey.value?.key &&
    selectedKeyAllowsImage.value &&
    !!form.prompt.trim() &&
    !generating.value
)

function formatTime(ts: number) {
  return new Date(ts).toLocaleString()
}

async function loadKeys() {
  loadingKeys.value = true
  try {
    const res = await keysAPI.list(1, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
    apiKeys.value = res.items || []
    const currentOk =
      form.apiKeyId && imageCapableKeys.value.some((k) => String(k.id) === String(form.apiKeyId))
    if (!currentOk) {
      form.apiKeyId = imageCapableKeys.value.length ? String(imageCapableKeys.value[0].id) : ''
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('imageWorkbench.loadKeysFailed'))
  } finally {
    loadingKeys.value = false
  }
}

async function loadHistory() {
  history.value = await listGalleryImages()
  if (!latestImage.value && history.value.length) {
    latestImage.value = history.value[0]
  }
}

async function refreshAll() {
  await Promise.all([loadKeys(), loadHistory()])
}

function truncateFileName(name: string, max = 14): string {
  const text = (name || '').trim()
  if (text.length <= max) return text
  const extIdx = text.lastIndexOf('.')
  if (extIdx > 0 && text.length - extIdx <= 5) {
    const ext = text.slice(extIdx)
    const keep = Math.max(4, max - ext.length - 1)
    return `${text.slice(0, keep)}…${ext}`
  }
  return `${text.slice(0, max - 1)}…`
}

function readFileAsDataURL(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read image'))
    reader.readAsDataURL(file)
  })
}

async function cloneImageFile(file: File): Promise<File> {
  const buffer = await file.arrayBuffer()
  const type = file.type || 'image/png'
  return new File([buffer], file.name || 'image.png', {
    type,
    lastModified: file.lastModified || Date.now()
  })
}

async function onReferenceChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files || [])
  // Clear immediately so the same file can be re-selected; cloned copies keep previews alive.
  input.value = ''
  if (!files.length) return

  const room = MAX_REFERENCE_IMAGES - referenceImages.value.length
  if (room <= 0) {
    appStore.showError(t('imageWorkbench.referenceLimit', { n: MAX_REFERENCE_IMAGES }))
    return
  }

  const slice = files.slice(0, room)
  const accepted: ReferenceImageItem[] = []

  try {
    for (const file of slice) {
      const mime = file.type || ''
      const okType =
        ['image/png', 'image/jpeg', 'image/webp'].includes(mime) ||
        /\.(png|jpe?g|webp)$/i.test(file.name || '')
      if (!okType) {
        appStore.showError(t('imageWorkbench.invalidImage'))
        continue
      }

      const cloned = await cloneImageFile(file)
      const previewUrl = await readFileAsDataURL(cloned)
      if (!previewUrl.startsWith('data:image/')) {
        appStore.showError(t('imageWorkbench.invalidImage'))
        continue
      }

      accepted.push({
        id: `ref_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
        file: cloned,
        name: cloned.name || 'image.png',
        previewUrl
      })
    }
  } catch {
    appStore.showError(t('imageWorkbench.invalidImage'))
    return
  }

  if (accepted.length) {
    referenceImages.value = [...referenceImages.value, ...accepted]
  }
  if (files.length > room) {
    appStore.showError(t('imageWorkbench.referenceLimit', { n: MAX_REFERENCE_IMAGES }))
  }
}

function removeReference(id: string) {
  referenceImages.value = referenceImages.value.filter((item) => item.id !== id)
}

function clearReference() {
  referenceImages.value = []
  if (fileInputRef.value) fileInputRef.value.value = ''
}

function resetForm() {
  form.model = 'gpt-image-2'
  form.prompt = ''
  form.size = '1k_square'
  form.n = '1'
  form.quality = 'auto'
  form.format = 'png'
  form.background = 'auto'
  form.style = 'auto'
  form.syncPlaza = true
  clearReference()
}

function selectHistory(item: GalleryImageRecord) {
  latestImage.value = item
}

function openPreview(item: GalleryImageRecord) {
  previewTarget.value = item
  previewOpen.value = true
}

function downloadImage(item: GalleryImageRecord) {
  const ext = item.format === 'jpeg' ? 'jpg' : item.format || 'png'
  downloadDataUrl(item.imageDataUrl, `sub2api-${item.id}.${ext}`)
}

async function togglePublic(item: GalleryImageRecord) {
  const next = await updateGalleryImage(item.id, { public: !item.public })
  if (!next) return
  await loadHistory()
  if (latestImage.value?.id === item.id) latestImage.value = next
  appStore.showSuccess(next.public ? t('imageWorkbench.published') : t('imageWorkbench.unpublished'))
}

async function removeHistory(item: GalleryImageRecord) {
  await deleteGalleryImage(item.id)
  if (latestImage.value?.id === item.id) latestImage.value = null
  await loadHistory()
}

function reuseRecord(item: GalleryImageRecord) {
  form.model = item.model
  form.prompt = item.prompt
  form.quality = item.quality || 'auto'
  form.format = item.format || 'png'
  form.n = String(item.n || 1)
  form.background = item.background || item.sampling || 'auto'
  form.style = item.style || 'auto'
  const sizeKey = Object.entries(SIZE_MAP).find(([, v]) => v === item.size)?.[0]
  if (sizeKey) form.size = sizeKey
  appStore.showSuccess(t('imageWorkbench.reused'))
}

async function startGenerate() {
  const key = selectedKey.value
  if (!key?.key || !form.prompt.trim() || !keyAllowsImage(key)) {
    if (key && !keyAllowsImage(key)) {
      appStore.showError(t('imageWorkbench.keyNotAllowImage'))
    }
    return
  }

  generating.value = true
  abortCtrl?.abort()
  abortCtrl = new AbortController()

  try {
    const size = SIZE_MAP[form.size] || '1024x1024'
    const n = Math.min(4, Math.max(1, Number(form.n) || 1))
    const res = referenceImages.value.length
      ? await imageAPI.editImage(
          key.key,
          {
            model: form.model,
            prompt: form.prompt.trim(),
            imageFiles: referenceImages.value.map((item) => item.file),
            n,
            size,
            quality: form.quality,
            background: form.background,
            style: form.style,
            response_format: 'b64_json'
          },
          abortCtrl.signal
        )
      : await imageAPI.generateImage(
          key.key,
          {
            model: form.model,
            prompt: form.prompt.trim(),
            n,
            size,
            quality: form.quality,
            response_format: 'b64_json',
            output_format: form.format,
            background: form.background,
            style: form.style
          },
          abortCtrl.signal
        )

    const items = res?.data || []
    if (!items.length) throw new Error(t('imageWorkbench.emptyResult'))

    let last: GalleryImageRecord | null = null
    for (const item of items) {
      const dataUrl = imageAPI.resultToDataUrl(item, form.format)
      if (!dataUrl) continue
      last = await saveGalleryImage({
        prompt: form.prompt.trim(),
        title: truncatePrompt(form.prompt.trim()),
        model: form.model,
        size,
        quality: form.quality,
        format: form.format,
        n,
        background: form.background,
        style: form.style,
        apiKeyId: key.id,
        apiKeyName: key.name,
        imageDataUrl: dataUrl,
        public: form.syncPlaza,
        source: 'workbench'
      })

      if (form.syncPlaza) {
        try {
          await publishImagePlaza({
            prompt: form.prompt.trim(),
            title: truncatePrompt(form.prompt.trim()),
            model: form.model,
            size,
            quality: form.quality,
            format: form.format,
            background: form.background,
            style: form.style,
            image: dataUrl
          })
        } catch (publishErr: any) {
          appStore.showError(publishErr?.message || t('imageWorkbench.publishFailed'))
        }
      }
    }

    await loadHistory()
    if (last) latestImage.value = last
    appStore.showSuccess(t('imageWorkbench.generateSuccess'))
  } catch (error: any) {
    if (error?.name === 'AbortError') return
    appStore.showError(error?.message || t('imageWorkbench.generateFailed'))
  } finally {
    generating.value = false
  }
}

function applyQueryPrefill() {
  const q = route.query
  if (typeof q.prompt === 'string' && q.prompt) form.prompt = q.prompt
  if (typeof q.model === 'string' && MODEL_OPTIONS.includes(q.model as any)) form.model = q.model
  if (typeof q.size === 'string' && SIZE_MAP[q.size]) form.size = q.size
}

async function getAndReuse(id: string) {
  const { getGalleryImage } = await import('@/utils/imageGalleryStore')
  const item = await getGalleryImage(id)
  if (item) reuseRecord(item)
}

onMounted(async () => {
  await refreshAll()
  applyQueryPrefill()
  if (typeof route.query.reuse === 'string' && route.query.reuse) {
    await getAndReuse(route.query.reuse)
  }
  if (Object.keys(route.query).length) {
    router.replace({ path: route.path, query: {} })
  }
})

onUnmounted(() => {
  clearReference()
  abortCtrl?.abort()
})
</script>

<style scoped>
.img-lab {
  --lab-cyan: #14b8a6;
  --lab-line: color-mix(in srgb, var(--lab-cyan) 28%, transparent);
  font-family: 'Sora', ui-sans-serif, system-ui, sans-serif;
}

.img-lab-hero {
  position: relative;
  overflow: hidden;
  padding: 1.1rem 1.25rem;
  border: 1px solid var(--lab-line);
  background:
    radial-gradient(900px 180px at 10% -40%, rgba(20, 184, 166, 0.18), transparent 60%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.04), transparent);
}

.dark .img-lab-hero {
  background:
    radial-gradient(900px 180px at 10% -40%, rgba(20, 184, 166, 0.22), transparent 60%),
    linear-gradient(180deg, rgba(2, 6, 23, 0.55), rgba(15, 23, 42, 0.2));
}

.img-lab-hero__glow {
  position: absolute;
  inset: auto -10% -60% auto;
  width: 280px;
  height: 280px;
  background: radial-gradient(circle, rgba(20, 184, 166, 0.25), transparent 70%);
  filter: blur(20px);
  pointer-events: none;
}

.img-lab-kicker {
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.7rem;
  letter-spacing: 0.16em;
  color: var(--lab-cyan);
  margin-bottom: 0.25rem;
}

.img-lab-title {
  font-family: 'Oxanium', ui-sans-serif, system-ui, sans-serif;
  font-size: 1.45rem;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.img-lab-grid {
  display: grid;
  grid-template-columns: minmax(220px, 0.9fr) minmax(0, 1.5fr) minmax(260px, 1fr);
  gap: 1rem;
  align-items: start;
}

@media (max-width: 1180px) {
  .img-lab-grid {
    grid-template-columns: 1fr 1fr;
  }
  .img-lab-col--main {
    grid-column: 1 / -1;
  }
}

@media (max-width: 760px) {
  .img-lab-grid {
    grid-template-columns: 1fr;
  }
}

.img-lab-panel {
  padding: 1rem 1.1rem;
  border: 1px solid color-mix(in srgb, var(--lab-cyan) 16%, transparent);
}

.img-lab-panel__title {
  font-family: 'Oxanium', ui-sans-serif, system-ui, sans-serif;
  font-size: 0.92rem;
  font-weight: 650;
  margin-bottom: 0.75rem;
  letter-spacing: 0.02em;
}

.img-lab-keyhint {
  font-size: 0.78rem;
  color: #64748b;
}

.dark .img-lab-keyhint {
  color: #94a3b8;
}

.img-lab-keyopt {
  display: flex;
  width: 100%;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.6rem;
}

.img-lab-keyopt.is-disabled {
  opacity: 0.95;
}

.img-lab-keyopt__main {
  min-width: 0;
  flex: 1;
}

.img-lab-keyopt__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.4rem;
}

.img-lab-keyopt__label {
  font-size: 0.84rem;
  font-weight: 600;
  line-height: 1.3;
}

.img-lab-keyopt.is-disabled .img-lab-keyopt__label {
  color: #94a3b8;
  font-weight: 500;
}

.img-lab-keyopt__badge {
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.65rem;
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  background: rgba(16, 185, 129, 0.15);
  color: #059669;
  border: 1px solid rgba(16, 185, 129, 0.35);
}

.dark .img-lab-keyopt__badge {
  color: #6ee7b7;
  background: rgba(16, 185, 129, 0.2);
}

.img-lab-keyopt__sub {
  margin-top: 0.2rem;
  font-size: 0.7rem;
  color: #64748b;
  line-height: 1.35;
}

.dark .img-lab-keyopt__sub {
  color: #94a3b8;
}

.img-lab-keyopt__check {
  margin-top: 0.15rem;
  flex-shrink: 0;
  color: #14b8a6;
}

.img-lab-keycard {
  border-radius: 0.75rem;
  border: 1px solid color-mix(in srgb, var(--lab-cyan) 22%, transparent);
  background: rgba(255, 255, 255, 0.72);
  padding: 0.75rem 0.85rem;
}

.dark .img-lab-keycard {
  background: rgba(15, 23, 42, 0.55);
}

.img-lab-keycard__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  margin-bottom: 0.7rem;
}

.img-lab-tag {
  font-size: 0.68rem;
  font-weight: 600;
  padding: 0.18rem 0.5rem;
  border-radius: 999px;
  line-height: 1.3;
}

.img-lab-tag--group {
  background: rgba(20, 184, 166, 0.12);
  color: #0f766e;
  border: 1px solid rgba(20, 184, 166, 0.28);
}

.dark .img-lab-tag--group {
  color: #5eead4;
}

.img-lab-tag--ok {
  background: rgba(16, 185, 129, 0.14);
  color: #047857;
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.dark .img-lab-tag--ok {
  color: #6ee7b7;
}

.img-lab-tag--rate {
  background: rgba(245, 158, 11, 0.12);
  color: #b45309;
  border: 1px solid rgba(245, 158, 11, 0.28);
}

.dark .img-lab-tag--rate {
  color: #fcd34d;
}

.img-lab-tag--price {
  font-family: 'Oxanium', ui-monospace, monospace;
  background: rgba(20, 184, 166, 0.1);
  color: #0f766e;
  border: 1px solid rgba(20, 184, 166, 0.3);
}

.dark .img-lab-tag--price {
  color: #5eead4;
}

.img-lab-keycard__rows {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.img-lab-keycard__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  font-size: 0.78rem;
  color: #64748b;
}

.img-lab-keycard__row > span:last-child {
  color: #0f172a;
  font-weight: 500;
}

.dark .img-lab-keycard__row {
  color: #94a3b8;
}

.dark .img-lab-keycard__row > span:last-child {
  color: #e2e8f0;
}

.img-lab-keymeta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.img-lab-chip {
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.68rem;
  padding: 0.15rem 0.45rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--lab-cyan) 30%, transparent);
  color: #0f766e;
  background: rgba(20, 184, 166, 0.08);
}

.dark .img-lab-chip {
  color: #5eead4;
}

.img-lab-chip.is-live {
  border-color: rgba(16, 185, 129, 0.45);
  color: #059669;
  background: rgba(16, 185, 129, 0.12);
}

.dark .img-lab-chip.is-live {
  color: #6ee7b7;
}

.img-lab-chip.is-off {
  border-color: rgba(248, 113, 113, 0.4);
  color: #dc2626;
}

.img-lab-models {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
}

.img-lab-model {
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.75rem;
  padding: 0.35rem 0.65rem;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: transparent;
  color: inherit;
  transition: all 0.2s ease;
}

.img-lab-model:hover {
  border-color: var(--lab-cyan);
}

.img-lab-model.is-active {
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.95), rgba(13, 148, 136, 0.95));
  border-color: transparent;
  color: white;
  box-shadow: 0 0 18px rgba(20, 184, 166, 0.35);
}

.img-lab-refcount {
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.75rem;
  color: #64748b;
}

.dark .img-lab-refcount {
  color: #94a3b8;
}

.img-lab-upload {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.35rem;
  min-height: 132px;
  border: 1.5px dashed color-mix(in srgb, var(--lab-cyan) 45%, transparent);
  border-radius: 0.9rem;
  cursor: pointer;
  position: relative;
  overflow: hidden;
  text-align: center;
  padding: 0.9rem 0.75rem;
  color: #64748b;
  background: rgba(20, 184, 166, 0.03);
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background 0.2s ease;
}

.img-lab-upload:hover:not(.is-disabled) {
  border-color: var(--lab-cyan);
  box-shadow: inset 0 0 24px rgba(20, 184, 166, 0.08);
  background: rgba(20, 184, 166, 0.06);
}

.img-lab-upload.is-disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.img-lab-upload__title {
  font-size: 0.86rem;
  font-weight: 600;
  color: #334155;
}

.dark .img-lab-upload__title {
  color: #e2e8f0;
}

.img-lab-upload__hint {
  font-size: 0.7rem;
  line-height: 1.4;
  opacity: 0.85;
  max-width: 16rem;
}

.img-lab-reflist {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(88px, 1fr));
  gap: 0.55rem;
  margin-top: 0.75rem;
}

.img-lab-refitem {
  border-radius: 0.65rem;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--lab-cyan) 18%, transparent);
  background: rgba(255, 255, 255, 0.7);
}

.dark .img-lab-refitem {
  background: rgba(15, 23, 42, 0.55);
}

.img-lab-refitem__media {
  position: relative;
  aspect-ratio: 1;
  background: #0b1220;
}

.img-lab-refitem__media img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  background: #111827;
}

.img-lab-refitem__remove {
  position: absolute;
  top: 0.28rem;
  right: 0.28rem;
  width: 1.35rem;
  height: 1.35rem;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.72);
  color: #fff;
  cursor: pointer;
}

.img-lab-refitem__remove:hover:not(:disabled) {
  background: rgba(239, 68, 68, 0.9);
}

.img-lab-refitem__remove:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.img-lab-refitem__name {
  padding: 0.28rem 0.35rem;
  font-size: 0.65rem;
  line-height: 1.2;
  color: #64748b;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background: rgba(148, 163, 184, 0.12);
}

.dark .img-lab-refitem__name {
  color: #94a3b8;
  background: rgba(15, 23, 42, 0.55);
}

.img-lab-endpoint {
  display: flex;
  align-items: flex-start;
  gap: 0.45rem;
  padding: 0.55rem 0.7rem;
  border-radius: 0.65rem;
  border: 1px solid rgba(20, 184, 166, 0.28);
  background: rgba(20, 184, 166, 0.08);
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.72rem;
  line-height: 1.45;
  color: #0f766e;
  word-break: break-all;
}

.dark .img-lab-endpoint {
  color: #5eead4;
  background: rgba(20, 184, 166, 0.12);
  border-color: rgba(20, 184, 166, 0.35);
}

.img-lab-endpoint__dot {
  width: 0.45rem;
  height: 0.45rem;
  margin-top: 0.3rem;
  border-radius: 999px;
  flex-shrink: 0;
  background: var(--lab-cyan);
  box-shadow: 0 0 8px var(--lab-cyan);
}

.img-lab-prompt {
  width: 100%;
  resize: vertical;
  min-height: 120px;
  font-family: 'Sora', ui-sans-serif, system-ui, sans-serif;
}

.img-lab-params {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem 0.9rem;
}

@media (max-width: 560px) {
  .img-lab-params {
    grid-template-columns: 1fr;
  }
}

.img-lab-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  font-size: 0.75rem;
  color: #64748b;
}

.dark .img-lab-field {
  color: #94a3b8;
}

.img-lab-switch {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  font-size: 0.82rem;
  cursor: pointer;
}

.img-lab-signal {
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.68rem;
  letter-spacing: 0.12em;
  color: var(--lab-cyan);
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.img-lab-signal i {
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 999px;
  background: var(--lab-cyan);
  box-shadow: 0 0 10px var(--lab-cyan);
  animation: lab-pulse 1.6s ease-in-out infinite;
}

.img-lab-go {
  min-width: 140px;
  position: relative;
}

.img-lab-go__spin {
  display: inline-block;
  width: 0.85rem;
  height: 0.85rem;
  margin-right: 0.4rem;
  border: 2px solid rgba(255, 255, 255, 0.35);
  border-top-color: white;
  border-radius: 999px;
  animation: lab-spin 0.7s linear infinite;
  vertical-align: -0.1rem;
}

.img-lab-preview {
  min-height: 260px;
  border-radius: 0.9rem;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--lab-cyan) 18%, transparent);
  background: radial-gradient(circle at 30% 20%, rgba(20, 184, 166, 0.08), transparent 45%),
    linear-gradient(160deg, rgba(15, 23, 42, 0.04), rgba(15, 23, 42, 0.02));
}

.dark .img-lab-preview {
  background: radial-gradient(circle at 30% 20%, rgba(20, 184, 166, 0.12), transparent 45%),
    linear-gradient(160deg, #0b1220, #111827);
}

.img-lab-preview__frame {
  position: relative;
}

.img-lab-preview__frame img {
  display: block;
  width: 100%;
  max-height: 420px;
  object-fit: contain;
  background: #0b1220;
}

.img-lab-preview__scan {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, transparent, rgba(20, 184, 166, 0.12), transparent);
  background-size: 100% 220%;
  animation: lab-scan 3.2s linear infinite;
  pointer-events: none;
  mix-blend-mode: screen;
}

.img-lab-preview__empty {
  min-height: 260px;
  display: grid;
  place-items: center;
  position: relative;
  color: #64748b;
  font-size: 0.85rem;
}

.img-lab-preview__grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(20, 184, 166, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(20, 184, 166, 0.08) 1px, transparent 1px);
  background-size: 28px 28px;
  mask-image: radial-gradient(circle at center, black, transparent 75%);
}

.img-lab-history {
  max-height: calc(100vh - 12rem);
  display: flex;
  flex-direction: column;
}

.img-lab-history__list {
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding-right: 0.15rem;
}

.img-lab-history__item {
  display: grid;
  grid-template-columns: 72px 1fr;
  gap: 0.65rem;
  padding: 0.55rem;
  border-radius: 0.75rem;
  border: 1px solid color-mix(in srgb, var(--lab-cyan) 14%, transparent);
  background: rgba(20, 184, 166, 0.03);
}

.img-lab-history__thumb {
  width: 72px;
  height: 72px;
  border-radius: 0.55rem;
  overflow: hidden;
  padding: 0;
  border: 0;
  cursor: pointer;
}

.img-lab-history__thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.img-lab-history__prompt {
  font-size: 0.8rem;
  font-weight: 600;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.img-lab-history__meta {
  margin-top: 0.2rem;
  font-family: 'Oxanium', ui-monospace, monospace;
  font-size: 0.68rem;
  color: #64748b;
}

.dark .img-lab-history__meta {
  color: #94a3b8;
}

.img-lab-history__actions {
  margin-top: 0.35rem;
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.img-lab-history__actions button {
  font-size: 0.68rem;
  padding: 0.15rem 0.4rem;
  border-radius: 0.35rem;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: transparent;
  color: inherit;
}

.img-lab-history__actions button:hover {
  border-color: var(--lab-cyan);
  color: var(--lab-cyan);
}

.img-lab-history__actions .is-danger:hover {
  border-color: #f87171;
  color: #f87171;
}

.img-lab-history__empty {
  font-size: 0.82rem;
  color: #64748b;
  padding: 1.5rem 0.5rem;
  text-align: center;
}

@keyframes lab-pulse {
  0%,
  100% {
    opacity: 0.45;
    transform: scale(0.9);
  }
  50% {
    opacity: 1;
    transform: scale(1.15);
  }
}

@keyframes lab-spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes lab-scan {
  0% {
    background-position: 0 -40%;
  }
  100% {
    background-position: 0 140%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .img-lab-signal i,
  .img-lab-go__spin,
  .img-lab-preview__scan {
    animation: none !important;
  }
}
</style>
