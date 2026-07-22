<template>
  <div class="auth-shell">
    <header class="auth-header">
      <router-link to="/" class="auth-home" :aria-label="siteName">
        <span class="auth-logo"><img :src="siteLogo || '/logo.svg'" alt="" /></span>
        <span>{{ siteName }}</span>
      </router-link>
      <div class="auth-tools">
        <LocaleSwitcher />
        <button
          type="button"
          class="auth-tool-button"
          :title="isDark ? 'Light theme' : 'Dark theme'"
          :aria-label="isDark ? 'Light theme' : 'Dark theme'"
          @click="toggleTheme"
        >
          <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
        </button>
      </div>
    </header>

    <main class="auth-main">
      <section class="auth-panel" :aria-label="siteName">
        <div v-if="settingsLoaded" class="auth-brand">
          <router-link to="/" class="auth-brand-logo" tabindex="-1" aria-hidden="true">
            <img :src="siteLogo || '/logo.svg'" alt="" />
          </router-link>
          <p>{{ siteSubtitle }}</p>
        </div>

        <div class="auth-form-surface">
          <slot />
        </div>

        <div v-if="$slots.footer" class="auth-footer-links">
          <slot name="footer" />
        </div>
      </section>
    </main>

    <footer class="auth-footer">&copy; {{ currentYear }} {{ siteName }}</footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const appStore = useAppStore()
const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true })
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform'
)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)
const currentYear = computed(() => new Date().getFullYear())
const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

onMounted(() => {
  const savedTheme = localStorage.getItem('theme')
  const preferDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  isDark.value = savedTheme === 'dark' || (!savedTheme && preferDark)
  document.documentElement.classList.toggle('dark', isDark.value)
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-shell {
  --auth-bg: #f3f5f5;
  --auth-surface: #ffffff;
  --auth-ink: #172023;
  --auth-muted: #68757a;
  --auth-border: #d8dfe1;
  --auth-accent: #0f8f83;
  display: grid;
  min-height: 100dvh;
  grid-template-rows: auto 1fr auto;
  color: var(--auth-ink);
  background: var(--auth-bg);
  font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

:global(html.dark .auth-shell) {
  --auth-bg: #101517;
  --auth-surface: #171e20;
  --auth-ink: #edf2f3;
  --auth-muted: #9aa7aa;
  --auth-border: #2b3538;
  --auth-accent: #43bcae;
}

.auth-header {
  display: flex;
  min-height: 68px;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 12px clamp(16px, 4vw, 48px);
  border-bottom: 1px solid var(--auth-border);
}

.auth-home,
.auth-tools {
  display: flex;
  align-items: center;
}

.auth-home {
  min-width: 0;
  gap: 10px;
  color: var(--auth-ink);
  font-size: 15px;
  font-weight: 750;
}

.auth-logo,
.auth-brand-logo {
  display: grid;
  place-items: center;
  overflow: hidden;
  border: 1px solid var(--auth-border);
  border-radius: 8px;
  background: var(--auth-surface);
}

.auth-logo { width: 38px; height: 38px; }
.auth-logo img,
.auth-brand-logo img { width: 100%; height: 100%; object-fit: contain; }
.auth-tools { gap: 8px; }

.auth-tool-button {
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border: 1px solid var(--auth-border);
  border-radius: 8px;
  color: var(--auth-muted);
  background: var(--auth-surface);
}

.auth-tool-button:hover,
.auth-tool-button:focus-visible { color: var(--auth-ink); border-color: var(--auth-accent); }

.auth-main {
  display: grid;
  place-items: center;
  padding: 40px max(18px, env(safe-area-inset-left)) 48px;
}

.auth-panel { width: min(100%, 520px); }
.auth-brand { margin-bottom: 18px; text-align: center; }
.auth-brand-logo { width: 54px; height: 54px; margin: 0 auto 12px; }
.auth-brand p { color: var(--auth-muted); font-size: 13px; line-height: 1.5; }

.auth-form-surface {
  padding: clamp(24px, 5vw, 36px);
  border: 1px solid var(--auth-border);
  border-radius: 8px;
  background: var(--auth-surface);
  box-shadow: 0 18px 44px rgba(18, 31, 34, 0.08);
}

:global(html.dark .auth-form-surface) { box-shadow: 0 20px 48px rgba(0, 0, 0, 0.2); }
.auth-footer-links { margin-top: 20px; text-align: center; font-size: 14px; }
.auth-footer { padding: 20px; color: var(--auth-muted); text-align: center; font-size: 12px; }

@media (max-width: 480px) {
  .auth-header { min-height: 60px; padding: 10px 14px; }
  .auth-home > span:last-child { display: none; }
  .auth-main { place-items: start center; padding: 24px 0 36px; }
  .auth-panel { width: 100%; }
  .auth-brand { padding-inline: 18px; }
  .auth-form-surface { padding: 24px 18px; border-inline: 0; border-radius: 0; box-shadow: none; }
}

@media (prefers-reduced-motion: reduce) {
  .auth-shell *, .auth-shell *::before, .auth-shell *::after { transition: none !important; }
}
</style>
