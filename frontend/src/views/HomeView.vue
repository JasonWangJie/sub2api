<template>
  <div v-if="homeContent" class="min-h-screen bg-white dark:bg-dark-950">
    <iframe
      v-if="isHomeContentUrl && iframeHomeUrl"
      :src="iframeHomeUrl"
      class="h-screen w-full border-0"
      sandbox="allow-forms allow-scripts allow-popups"
      referrerpolicy="no-referrer"
      title="Custom home content"
    />
    <div v-else class="custom-home-content" v-html="sanitizedHomeContent"></div>
  </div>

  <div v-else class="home-shell min-h-screen">
    <header class="home-header">
      <nav class="home-nav" :aria-label="siteName">
        <router-link to="/" class="home-brand" :aria-label="siteName">
          <span class="home-logo">
            <img :src="siteLogo || '/logo.svg'" alt="" />
          </span>
          <span>{{ siteName }}</span>
        </router-link>

        <div class="home-actions">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="home-icon-button"
            :title="t('home.viewDocs')"
            :aria-label="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <button
            type="button"
            class="home-icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
          </button>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="home-session-link">
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
            <Icon name="arrowRight" size="sm" />
          </router-link>
        </div>
      </nav>
    </header>

    <main>
      <section class="home-hero">
        <img
          v-if="!heroImageFailed"
          src="/images/sub2api-workbench.webp"
          alt=""
          class="home-hero-image"
          fetchpriority="high"
          @error="heroImageFailed = true"
        />
        <div class="home-hero-shade" aria-hidden="true"></div>
        <div class="home-hero-content">
          <p class="home-category">AI API Gateway</p>
          <h1>{{ siteName }}</h1>
          <p class="home-subtitle">{{ siteSubtitle }}</p>
          <div class="home-hero-actions">
            <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="home-primary-action">
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <Icon name="arrowRight" size="md" />
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="home-secondary-action"
            >
              {{ t('home.viewDocs') }}
            </a>
          </div>
        </div>
        <div v-if="heroImageFailed" class="home-hero-fallback" aria-hidden="true">
          <div class="home-fallback-toolbar"></div>
          <div class="home-fallback-columns"><i></i><i></i><i></i></div>
        </div>
      </section>

      <section class="home-capabilities" :aria-label="siteSubtitle">
        <div class="home-capability-row">
          <span class="home-capability-icon"><Icon name="swap" size="lg" /></span>
          <div>
            <h2>{{ t('home.features.unifiedGateway') }}</h2>
            <p>{{ t('home.features.unifiedGatewayDesc') }}</p>
          </div>
        </div>
        <div class="home-capability-row">
          <span class="home-capability-icon home-capability-icon--amber"><Icon name="chart" size="lg" /></span>
          <div>
            <h2>{{ t('home.features.balanceQuota') }}</h2>
            <p>{{ t('home.features.balanceQuotaDesc') }}</p>
          </div>
        </div>
        <div class="home-capability-row">
          <span class="home-capability-icon home-capability-icon--green"><Icon name="shield" size="lg" /></span>
          <div>
            <h2>{{ t('home.features.multiAccount') }}</h2>
            <p>{{ t('home.features.multiAccountDesc') }}</p>
          </div>
        </div>
      </section>

      <section class="home-provider-band">
        <div>
          <p class="home-provider-label">{{ t('home.providers.title') }}</p>
          <p class="home-provider-copy">{{ t('home.providers.description') }}</p>
        </div>
        <ul class="home-provider-list" aria-label="Supported providers">
          <li>Anthropic</li>
          <li>OpenAI</li>
          <li>Gemini</li>
          <li>Grok</li>
          <li>Antigravity</li>
        </ul>
      </section>
    </main>

    <footer class="home-footer">
      <span>&copy; {{ currentYear }} {{ siteName }}</span>
      <div>
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
        <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub</a>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { useAppStore, useAuthStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => /^https?:\/\//i.test(homeContent.value.trim()))
const iframeHomeUrl = computed(() => (isHomeContentUrl.value ? sanitizeUrl(homeContent.value.trim()) : ''))
const sanitizedHomeContent = computed(() =>
  DOMPurify.sanitize(homeContent.value, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['script', 'iframe', 'object', 'embed', 'base', 'form'],
    FORBID_ATTR: ['srcdoc']
  })
)

const isDark = ref(document.documentElement.classList.contains('dark'))
const heroImageFailed = ref(false)
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
const currentYear = computed(() => new Date().getFullYear())

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  const preferDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  isDark.value = savedTheme === 'dark' || (!savedTheme && preferDark)
  document.documentElement.classList.toggle('dark', isDark.value)
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})
</script>

<style scoped>
.home-shell {
  --home-bg: #f5f7f8;
  --home-surface: #ffffff;
  --home-ink: #172023;
  --home-muted: #647176;
  --home-border: #dbe1e3;
  --home-accent: #0f8f83;
  color: var(--home-ink);
  background: var(--home-bg);
  font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

:global(html.dark .home-shell) {
  --home-bg: #101517;
  --home-surface: #171e20;
  --home-ink: #edf2f3;
  --home-muted: #9aa7aa;
  --home-border: #2b3538;
  --home-accent: #39b8aa;
}

.home-header {
  position: absolute;
  inset: 0 0 auto;
  z-index: 20;
  padding: 18px clamp(18px, 4vw, 56px);
}

.home-nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  max-width: 1440px;
  margin: 0 auto;
}

.home-brand,
.home-actions,
.home-session-link,
.home-hero-actions,
.home-primary-action,
.home-secondary-action {
  display: flex;
  align-items: center;
}

.home-brand {
  min-width: 0;
  gap: 10px;
  color: #fff;
  font-size: 15px;
  font-weight: 700;
}

.home-logo {
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.32);
  border-radius: 8px;
  background: rgba(8, 18, 20, 0.7);
}

.home-logo img { width: 100%; height: 100%; object-fit: contain; }
.home-actions { gap: 8px; }

.home-icon-button {
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border: 1px solid rgba(255, 255, 255, 0.25);
  border-radius: 8px;
  color: #fff;
  background: rgba(8, 18, 20, 0.68);
}

.home-icon-button:hover,
.home-icon-button:focus-visible { border-color: rgba(255, 255, 255, 0.62); }

.home-session-link {
  min-height: 38px;
  gap: 7px;
  padding: 0 14px;
  border-radius: 8px;
  color: #09201d;
  background: #62d4c5;
  font-size: 13px;
  font-weight: 700;
}

.home-hero {
  position: relative;
  display: flex;
  min-height: 78dvh;
  align-items: flex-end;
  overflow: hidden;
  background: #182225;
}

.home-hero-image,
.home-hero-shade,
.home-hero-fallback { position: absolute; inset: 0; width: 100%; height: 100%; }
.home-hero-image { object-fit: cover; object-position: center top; }
.home-hero-shade { background: rgba(4, 10, 12, 0.68); }

.home-hero-content {
  position: relative;
  z-index: 2;
  width: min(760px, 100%);
  margin: 0 clamp(20px, 8vw, 130px) clamp(52px, 9vh, 96px);
  color: #fff;
}

.home-category {
  margin-bottom: 13px;
  color: #87e2d6;
  font-size: 13px;
  font-weight: 700;
  text-transform: uppercase;
}

.home-hero h1 {
  max-width: 100%;
  font-size: clamp(42px, 7vw, 82px);
  font-weight: 750;
  line-height: 1.02;
  overflow-wrap: anywhere;
}

.home-subtitle {
  max-width: 660px;
  margin-top: 18px;
  color: rgba(255, 255, 255, 0.78);
  font-size: clamp(17px, 2vw, 22px);
  line-height: 1.55;
}

.home-hero-actions { flex-wrap: wrap; gap: 10px; margin-top: 28px; }
.home-primary-action,
.home-secondary-action {
  min-height: 44px;
  gap: 9px;
  padding: 0 18px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 700;
}
.home-primary-action { color: #09201d; background: #62d4c5; }
.home-secondary-action { border: 1px solid rgba(255, 255, 255, 0.42); color: #fff; background: rgba(5, 12, 14, 0.55); }

.home-hero-fallback { padding: 110px 8vw 48px; opacity: 0.32; }
.home-fallback-toolbar { height: 48px; border: 1px solid #7a9297; }
.home-fallback-columns { display: grid; height: calc(100% - 64px); grid-template-columns: 1fr 2fr 1fr; gap: 16px; margin-top: 16px; }
.home-fallback-columns i { border: 1px solid #7a9297; }

.home-capabilities {
  display: grid;
  max-width: 1440px;
  margin: 0 auto;
  grid-template-columns: repeat(3, 1fr);
  border-inline: 1px solid var(--home-border);
}

.home-capability-row {
  display: grid;
  min-height: 190px;
  grid-template-columns: 42px 1fr;
  gap: 18px;
  align-content: center;
  padding: 34px;
  border-right: 1px solid var(--home-border);
  background: var(--home-surface);
}
.home-capability-row:last-child { border-right: 0; }
.home-capability-icon { color: var(--home-accent); }
.home-capability-icon--amber { color: #b87518; }
.home-capability-icon--green { color: #347b52; }
.home-capability-row h2 { font-size: 17px; font-weight: 750; }
.home-capability-row p { margin-top: 8px; color: var(--home-muted); font-size: 14px; line-height: 1.6; }

.home-provider-band {
  display: flex;
  max-width: 1440px;
  margin: 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 32px;
  padding: 38px clamp(24px, 4vw, 56px);
  border: 1px solid var(--home-border);
  border-top: 0;
}
.home-provider-label { font-size: 16px; font-weight: 750; }
.home-provider-copy { margin-top: 5px; color: var(--home-muted); font-size: 13px; }
.home-provider-list { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.home-provider-list li { padding: 7px 10px; border: 1px solid var(--home-border); border-radius: 6px; color: var(--home-muted); font-size: 12px; background: var(--home-surface); }

.home-footer {
  display: flex;
  max-width: 1440px;
  margin: 0 auto;
  justify-content: space-between;
  gap: 24px;
  padding: 28px clamp(24px, 4vw, 56px);
  color: var(--home-muted);
  font-size: 12px;
}
.home-footer div { display: flex; gap: 18px; }
.home-footer a:hover { color: var(--home-ink); }

.custom-home-content { min-height: 100vh; }

@media (max-width: 860px) {
  .home-actions > :deep(.locale-switcher) { display: none; }
  .home-capabilities { grid-template-columns: 1fr; }
  .home-capability-row { min-height: 140px; border-right: 0; border-bottom: 1px solid var(--home-border); }
  .home-capability-row:last-child { border-bottom: 0; }
  .home-provider-band { align-items: flex-start; flex-direction: column; }
  .home-provider-list { justify-content: flex-start; }
}

@media (max-width: 520px) {
  .home-header { padding: 14px; }
  .home-brand > span:last-child { display: none; }
  .home-actions { gap: 6px; }
  .home-session-link { padding: 0 10px; }
  .home-hero { min-height: 76dvh; }
  .home-hero-content { margin: 0 18px 42px; }
  .home-hero h1 { font-size: 42px; }
  .home-capability-row { grid-template-columns: 34px 1fr; padding: 26px 20px; }
  .home-footer { flex-direction: column; }
}

@media (prefers-reduced-motion: reduce) {
  .home-shell *, .home-shell *::before, .home-shell *::after { scroll-behavior: auto !important; transition: none !important; }
}
</style>
