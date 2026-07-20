<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page — Signal Gateway aesthetic -->
  <div
    v-else
    class="home-shell relative flex min-h-screen flex-col overflow-hidden"
    @mousemove="onPointerMove"
  >
    <!-- Dynamic light field -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden="true">
      <div class="home-grid"></div>
      <div class="home-stars">
        <span v-for="n in 28" :key="'st-' + n" class="home-star" :style="starStyle(n)"></span>
      </div>
      <div class="home-meteors">
        <span v-for="n in 8" :key="'mt-' + n" class="home-meteor" :style="meteorStyle(n)"></span>
      </div>
      <div class="home-halos home-halos--hero">
        <span class="home-halo home-halo--1"></span>
        <span class="home-halo home-halo--2"></span>
        <span class="home-halo home-halo--3"></span>
        <span class="home-halo home-halo--core"></span>
      </div>
      <div class="home-halos home-halos--terminal">
        <span class="home-halo home-halo--radar-1"></span>
        <span class="home-halo home-halo--radar-2"></span>
        <span class="home-halo home-halo--radar-3"></span>
      </div>
      <div class="home-scan"></div>
      <div class="home-beam home-beam--a"></div>
      <div class="home-beam home-beam--b"></div>
      <div class="home-beam home-beam--c"></div>
      <div class="home-orb home-orb--cyan"></div>
      <div class="home-orb home-orb--amber"></div>
      <div class="home-orb home-orb--mint"></div>
      <div
        class="home-spotlight"
        :style="{
          transform: `translate3d(${pointer.x}px, ${pointer.y}px, 0)`
        }"
      ></div>
      <div class="home-noise"></div>
      <div class="home-vignette"></div>
    </div>

    <!-- Header -->
    <header class="relative z-20 px-6 py-4">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="home-logo-ring h-10 w-10 overflow-hidden rounded-xl">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="home-brand-mark hidden text-sm font-semibold tracking-[0.18em] sm:inline">
            {{ siteName }}
          </span>
        </div>

        <div class="flex items-center gap-3">
          <LocaleSwitcher />

          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="home-icon-btn"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <button
            @click="toggleTheme"
            class="home-icon-btn"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="home-cta-pill inline-flex items-center gap-1.5 py-1 pl-1 pr-2.5"
          >
            <span class="home-avatar flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-semibold">
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium">{{ t('home.dashboard') }}</span>
            <svg class="h-3 w-3 opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25" />
            </svg>
          </router-link>
          <router-link v-else to="/login" class="home-cta-pill inline-flex items-center px-3 py-1 text-xs font-medium">
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="relative z-10 flex-1 px-6 py-14 md:py-16">
      <div class="mx-auto max-w-6xl">
        <div class="mb-12 flex flex-col items-center justify-between gap-12 lg:flex-row lg:gap-16">
          <div class="home-reveal flex-1 text-center lg:text-left" style="--delay: 0.05s">
            <p class="home-kicker mb-4">
              <span class="home-kicker-dot"></span>
              AI API GATEWAY
            </p>
            <h1 class="home-title mb-5 text-4xl font-bold md:text-5xl lg:text-6xl">
              <span class="home-title-wrap">
                <span class="home-title-rings" aria-hidden="true">
                  <i></i><i></i><i></i>
                </span>
                <span class="home-title-glow">{{ siteName }}</span>
              </span>
            </h1>
            <p class="home-subtitle mb-8 text-lg md:text-xl">
              {{ siteSubtitle }}
            </p>

            <div class="flex flex-col items-center gap-4 sm:flex-row lg:justify-start">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="home-primary-btn inline-flex items-center px-8 py-3 text-base"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
              <span class="home-signal-hint hidden sm:inline">SIGNAL · ROUTING · BILLING</span>
            </div>
          </div>

          <div class="home-reveal flex flex-1 justify-center lg:justify-end" style="--delay: 0.18s">
            <div class="terminal-container">
              <div class="terminal-glow"></div>
              <div class="terminal-window">
                <div class="terminal-header">
                  <div class="terminal-buttons">
                    <span class="btn-close"></span>
                    <span class="btn-minimize"></span>
                    <span class="btn-maximize"></span>
                  </div>
                  <span class="terminal-title">gateway · live</span>
                </div>
                <div class="terminal-body">
                  <div class="code-line line-1">
                    <span class="code-prompt">$</span>
                    <span class="code-cmd">curl</span>
                    <span class="code-flag">-X POST</span>
                    <span class="code-url">/v1/messages</span>
                  </div>
                  <div class="code-line line-2">
                    <span class="code-comment"># Routing to upstream...</span>
                  </div>
                  <div class="code-line line-3">
                    <span class="code-success">200 OK</span>
                    <span class="code-response">{ "content": "Hello!" }</span>
                  </div>
                  <div class="code-line line-4">
                    <span class="code-prompt">$</span>
                    <span class="cursor"></span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="home-reveal mb-12 flex flex-wrap items-center justify-center gap-4 md:gap-6" style="--delay: 0.28s">
          <div class="home-chip">
            <Icon name="swap" size="sm" class="text-cyan-300" />
            <span>{{ t('home.tags.subscriptionToApi') }}</span>
          </div>
          <div class="home-chip">
            <Icon name="shield" size="sm" class="text-cyan-300" />
            <span>{{ t('home.tags.stickySession') }}</span>
          </div>
          <div class="home-chip">
            <Icon name="chart" size="sm" class="text-cyan-300" />
            <span>{{ t('home.tags.realtimeBilling') }}</span>
          </div>
        </div>

        <div class="home-reveal mb-12 grid gap-6 md:grid-cols-3" style="--delay: 0.36s">
          <div class="home-card group">
            <div class="home-card-icon home-card-icon--blue">
              <Icon name="server" size="lg" class="text-white" />
            </div>
            <h3 class="mb-2 text-lg font-semibold">{{ t('home.features.unifiedGateway') }}</h3>
            <p class="text-sm leading-relaxed opacity-70">{{ t('home.features.unifiedGatewayDesc') }}</p>
          </div>

          <div class="home-card group">
            <div class="home-card-icon home-card-icon--teal">
              <svg class="h-6 w-6 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold">{{ t('home.features.multiAccount') }}</h3>
            <p class="text-sm leading-relaxed opacity-70">{{ t('home.features.multiAccountDesc') }}</p>
          </div>

          <div class="home-card group">
            <div class="home-card-icon home-card-icon--amber">
              <svg class="h-6 w-6 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold">{{ t('home.features.balanceQuota') }}</h3>
            <p class="text-sm leading-relaxed opacity-70">{{ t('home.features.balanceQuotaDesc') }}</p>
          </div>
        </div>

        <div class="home-reveal mb-8 text-center" style="--delay: 0.44s">
          <h2 class="mb-3 text-2xl font-bold">{{ t('home.providers.title') }}</h2>
          <p class="text-sm opacity-65">{{ t('home.providers.description') }}</p>
        </div>

        <div class="home-reveal mb-16 flex flex-wrap items-center justify-center gap-4" style="--delay: 0.5s">
          <div class="home-provider">
            <div class="home-provider-badge from-orange-400 to-orange-500"><span>C</span></div>
            <span>{{ t('home.providers.claude') }}</span>
            <span class="home-provider-tag">{{ t('home.providers.supported') }}</span>
          </div>
          <div class="home-provider">
            <div class="home-provider-badge from-emerald-500 to-emerald-600"><span>G</span></div>
            <span>GPT</span>
            <span class="home-provider-tag">{{ t('home.providers.supported') }}</span>
          </div>
          <div class="home-provider">
            <div class="home-provider-badge from-sky-500 to-blue-600"><span>G</span></div>
            <span>{{ t('home.providers.gemini') }}</span>
            <span class="home-provider-tag">{{ t('home.providers.supported') }}</span>
          </div>
          <div class="home-provider">
            <div class="home-provider-badge from-rose-500 to-pink-600"><span>A</span></div>
            <span>{{ t('home.providers.antigravity') }}</span>
            <span class="home-provider-tag">{{ t('home.providers.supported') }}</span>
          </div>
          <div class="home-provider home-provider--muted">
            <div class="home-provider-badge from-slate-500 to-slate-600"><span>+</span></div>
            <span>{{ t('home.providers.more') }}</span>
            <span class="home-provider-tag home-provider-tag--soon">{{ t('home.providers.soon') }}</span>
          </div>
        </div>
      </div>
    </main>

    <footer class="home-footer relative z-10 px-6 py-8">
      <div class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left">
        <p class="text-sm opacity-55">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm opacity-55 transition hover:opacity-100"
          >
            {{ t('home.docs') }}
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm opacity-55 transition hover:opacity-100"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
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
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform'
)
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

const currentYear = computed(() => new Date().getFullYear())

const pointer = reactive({ x: -200, y: -200 })
let raf = 0
let targetX = -200
let targetY = -200

function hashUnit(n: number, salt: number) {
  const x = Math.sin(n * 12.9898 + salt * 78.233) * 43758.5453
  return x - Math.floor(x)
}

function starStyle(n: number) {
  const left = hashUnit(n, 1) * 100
  const top = hashUnit(n, 2) * 100
  const size = 1.2 + hashUnit(n, 3) * 2.4
  const delay = hashUnit(n, 4) * 6
  const dur = 2.4 + hashUnit(n, 5) * 3.6
  return {
    left: `${left}%`,
    top: `${top}%`,
    width: `${size}px`,
    height: `${size}px`,
    animationDelay: `${delay}s`,
    animationDuration: `${dur}s`
  }
}

function meteorStyle(n: number) {
  const left = 8 + hashUnit(n, 6) * 82
  const top = hashUnit(n, 7) * 48
  const delay = hashUnit(n, 8) * 10
  const dur = 1.8 + hashUnit(n, 9) * 2.2
  const len = 70 + hashUnit(n, 10) * 90
  return {
    left: `${left}%`,
    top: `${top}%`,
    '--meteor-len': `${len}px`,
    animationDelay: `${delay}s`,
    animationDuration: `${dur}s`
  }
}

function onPointerMove(e: MouseEvent) {
  targetX = e.clientX
  targetY = e.clientY
  if (!raf) {
    raf = requestAnimationFrame(() => {
      pointer.x = targetX
      pointer.y = targetY
      raf = 0
    })
  }
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.home-shell {
  --home-bg: #071018;
  --home-ink: #e8f7ff;
  --home-muted: rgba(198, 226, 239, 0.72);
  --home-cyan: #2ee6d6;
  --home-amber: #f5b942;
  --home-line: rgba(46, 230, 214, 0.14);
  font-family: 'Sora', ui-sans-serif, system-ui, sans-serif;
  color: var(--home-ink);
  background:
    radial-gradient(1200px 700px at 12% -10%, rgba(46, 230, 214, 0.16), transparent 55%),
    radial-gradient(900px 600px at 90% 10%, rgba(245, 185, 66, 0.1), transparent 50%),
    linear-gradient(160deg, #050b12 0%, #0a1622 48%, #071018 100%);
}

:global(html:not(.dark)) .home-shell {
  --home-bg: #f4fbfc;
  --home-ink: #0b1c24;
  --home-muted: rgba(15, 48, 58, 0.72);
  --home-line: rgba(13, 148, 136, 0.12);
  background:
    radial-gradient(1000px 640px at 8% -8%, rgba(20, 184, 166, 0.18), transparent 55%),
    radial-gradient(800px 520px at 92% 0%, rgba(245, 158, 11, 0.12), transparent 50%),
    linear-gradient(165deg, #f7fcfd 0%, #eef8f9 45%, #e8f4f6 100%);
  color: var(--home-ink);
}

.home-brand-mark,
.home-title,
.home-kicker {
  font-family: 'Oxanium', 'Sora', sans-serif;
}

.home-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--home-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--home-line) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: radial-gradient(ellipse 70% 60% at 50% 35%, #000 20%, transparent 75%);
  animation: grid-drift 28s linear infinite;
}

/* Star field */
.home-stars {
  position: absolute;
  inset: 0;
}

.home-star {
  position: absolute;
  border-radius: 50%;
  background: #dffcff;
  box-shadow: 0 0 8px rgba(46, 230, 214, 0.85);
  opacity: 0.35;
  animation-name: star-twinkle;
  animation-timing-function: ease-in-out;
  animation-iteration-count: infinite;
}

:global(html:not(.dark)) .home-star {
  background: #0f766e;
  box-shadow: 0 0 6px rgba(13, 148, 136, 0.55);
}

/* Meteors / shooting lights */
.home-meteors {
  position: absolute;
  inset: 0;
  overflow: hidden;
}

.home-meteor {
  --meteor-len: 100px;
  position: absolute;
  width: var(--meteor-len);
  height: 2px;
  border-radius: 999px;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.95), #2ee6d6);
  box-shadow:
    0 0 10px rgba(46, 230, 214, 0.85),
    0 0 22px rgba(46, 230, 214, 0.45);
  transform: rotate(-32deg) translate3d(-140%, 0, 0);
  opacity: 0;
  animation-name: meteor-fall;
  animation-timing-function: cubic-bezier(0.22, 0.61, 0.36, 1);
  animation-iteration-count: infinite;
}

.home-meteor::after {
  content: '';
  position: absolute;
  right: -2px;
  top: 50%;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  transform: translateY(-50%);
  background: #fff;
  box-shadow: 0 0 14px 4px rgba(46, 230, 214, 0.9);
}

:global(html:not(.dark)) .home-meteor {
  background: linear-gradient(90deg, transparent, rgba(15, 118, 110, 0.2), #0d9488);
  box-shadow: 0 0 10px rgba(13, 148, 136, 0.4);
}

/* Expanding halos */
.home-halos {
  position: absolute;
  width: min(560px, 70vw);
  height: min(560px, 70vw);
  pointer-events: none;
}

.home-halos--hero {
  top: 4%;
  left: 4%;
  transform: translate(-18%, -8%);
}

.home-halos--terminal {
  top: 18%;
  right: 2%;
  width: min(420px, 55vw);
  height: min(420px, 55vw);
  transform: translate(12%, 0);
}

.home-halo {
  position: absolute;
  inset: 12%;
  border-radius: 50%;
  border: 1px solid rgba(46, 230, 214, 0.28);
  box-shadow:
    0 0 24px rgba(46, 230, 214, 0.12),
    inset 0 0 24px rgba(46, 230, 214, 0.06);
}

.home-halo--1 {
  inset: 8%;
  animation: halo-pulse 6s ease-out infinite;
}

.home-halo--2 {
  inset: 0%;
  border-color: rgba(56, 189, 248, 0.22);
  animation: halo-pulse 6s ease-out infinite 1.4s;
}

.home-halo--3 {
  inset: -10%;
  border-color: rgba(245, 185, 66, 0.18);
  animation: halo-pulse 6s ease-out infinite 2.8s;
}

.home-halo--core {
  inset: 28%;
  border: none;
  background: radial-gradient(circle, rgba(46, 230, 214, 0.22), transparent 70%);
  filter: blur(8px);
  animation: glow-breathe 4s ease-in-out infinite;
}

.home-halo--radar-1,
.home-halo--radar-2,
.home-halo--radar-3 {
  inset: 18%;
  border-style: dashed;
  border-color: rgba(46, 230, 214, 0.22);
  animation: radar-spin 18s linear infinite;
}

.home-halo--radar-2 {
  inset: 8%;
  animation-duration: 26s;
  animation-direction: reverse;
  border-color: rgba(56, 189, 248, 0.18);
}

.home-halo--radar-3 {
  inset: -4%;
  animation-duration: 34s;
  border-color: rgba(245, 185, 66, 0.14);
}

.home-scan {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    180deg,
    transparent 0%,
    rgba(46, 230, 214, 0.045) 48%,
    transparent 52%,
    transparent 100%
  );
  background-size: 100% 220%;
  animation: scan-sweep 7.5s ease-in-out infinite;
  opacity: 0.7;
}

.home-beam {
  position: absolute;
  width: 42vw;
  max-width: 640px;
  height: 2px;
  border-radius: 999px;
  filter: blur(0.5px);
  opacity: 0.55;
}

.home-beam--a {
  top: 18%;
  left: -8%;
  background: linear-gradient(90deg, transparent, rgba(46, 230, 214, 0.85), transparent);
  box-shadow: 0 0 28px rgba(46, 230, 214, 0.45);
  transform: rotate(-18deg);
  animation: beam-pulse 5.5s ease-in-out infinite;
}

.home-beam--b {
  bottom: 22%;
  right: -10%;
  background: linear-gradient(90deg, transparent, rgba(245, 185, 66, 0.7), transparent);
  box-shadow: 0 0 26px rgba(245, 185, 66, 0.35);
  transform: rotate(16deg);
  animation: beam-pulse 6.8s ease-in-out infinite reverse;
}

.home-beam--c {
  top: 52%;
  left: 20%;
  width: 28vw;
  background: linear-gradient(90deg, transparent, rgba(56, 189, 248, 0.65), transparent);
  box-shadow: 0 0 20px rgba(56, 189, 248, 0.35);
  transform: rotate(-8deg);
  animation: beam-pulse 4.8s ease-in-out infinite 1s;
}

.home-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(48px);
  will-change: transform, opacity;
}

.home-orb--cyan {
  top: -8%;
  right: -6%;
  width: 420px;
  height: 420px;
  background: radial-gradient(circle, rgba(46, 230, 214, 0.45), transparent 68%);
  animation: orb-float 14s ease-in-out infinite;
}

.home-orb--amber {
  bottom: -12%;
  left: -8%;
  width: 380px;
  height: 380px;
  background: radial-gradient(circle, rgba(245, 185, 66, 0.28), transparent 70%);
  animation: orb-float 18s ease-in-out infinite reverse;
}

.home-orb--mint {
  top: 42%;
  left: 38%;
  width: 260px;
  height: 260px;
  background: radial-gradient(circle, rgba(56, 189, 248, 0.22), transparent 70%);
  animation: orb-float 11s ease-in-out infinite;
}

.home-spotlight {
  position: absolute;
  top: 0;
  left: 0;
  width: 420px;
  height: 420px;
  margin: -210px 0 0 -210px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(46, 230, 214, 0.16), transparent 65%);
  transition: transform 0.08s linear;
  pointer-events: none;
}

.home-noise {
  position: absolute;
  inset: 0;
  opacity: 0.045;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
  mix-blend-mode: overlay;
}

.home-vignette {
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at center, transparent 40%, rgba(0, 0, 0, 0.35) 100%);
}

:global(html:not(.dark)) .home-vignette {
  background: radial-gradient(ellipse at center, transparent 45%, rgba(7, 24, 32, 0.08) 100%);
}

.home-logo-ring {
  box-shadow:
    0 0 0 1px rgba(46, 230, 214, 0.35),
    0 0 18px rgba(46, 230, 214, 0.25);
}

.home-brand-mark {
  color: var(--home-muted);
  text-transform: uppercase;
}

.home-icon-btn {
  border-radius: 0.65rem;
  padding: 0.5rem;
  color: var(--home-muted);
  transition:
    color 0.2s ease,
    background 0.2s ease,
    box-shadow 0.2s ease;
}

.home-icon-btn:hover {
  color: var(--home-ink);
  background: rgba(46, 230, 214, 0.1);
  box-shadow: 0 0 0 1px rgba(46, 230, 214, 0.2);
}

.home-cta-pill {
  border-radius: 999px;
  color: #041016;
  background: linear-gradient(120deg, #7ff5ea, #2ee6d6 45%, #1bb8c7);
  box-shadow:
    0 0 0 1px rgba(46, 230, 214, 0.35),
    0 8px 24px rgba(46, 230, 214, 0.28);
  transition:
    transform 0.2s ease,
    box-shadow 0.2s ease;
}

.home-cta-pill:hover {
  transform: translateY(-1px);
  box-shadow:
    0 0 0 1px rgba(46, 230, 214, 0.5),
    0 12px 28px rgba(46, 230, 214, 0.35);
}

.home-avatar {
  background: linear-gradient(145deg, #0b2a30, #134e4a);
  color: #dffcf8;
}

.home-kicker {
  display: inline-flex;
  align-items: center;
  gap: 0.55rem;
  font-size: 0.72rem;
  letter-spacing: 0.22em;
  color: var(--home-cyan);
  text-transform: uppercase;
}

.home-kicker-dot {
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 999px;
  background: var(--home-cyan);
  box-shadow: 0 0 12px var(--home-cyan);
  animation: pulse-dot 2s ease-in-out infinite;
}

.home-title-wrap {
  position: relative;
  display: inline-block;
}

.home-title-rings {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 140%;
  height: 180%;
  transform: translate(-50%, -50%);
  pointer-events: none;
  z-index: -1;
}

.home-title-rings i {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  border: 1px solid rgba(46, 230, 214, 0.35);
  box-shadow: 0 0 18px rgba(46, 230, 214, 0.18);
  animation: title-ring 4.8s ease-out infinite;
}

.home-title-rings i:nth-child(2) {
  inset: -8%;
  animation-delay: 1.2s;
  border-color: rgba(56, 189, 248, 0.28);
}

.home-title-rings i:nth-child(3) {
  inset: -18%;
  animation-delay: 2.4s;
  border-color: rgba(245, 185, 66, 0.22);
}

.home-title-glow {
  background: linear-gradient(120deg, #f4fffd 10%, #7ff5ea 42%, #2ee6d6 68%, #f5b942 100%);
  background-size: 200% 200%;
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  animation: title-shine 8s ease-in-out infinite;
  filter: drop-shadow(0 0 18px rgba(46, 230, 214, 0.28));
}

:global(html:not(.dark)) .home-title-glow {
  background: linear-gradient(120deg, #083344 8%, #0f766e 40%, #0891b2 70%, #b45309 100%);
  background-size: 200% 200%;
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  filter: none;
}

.home-subtitle {
  color: var(--home-muted);
  max-width: 34rem;
}

.home-primary-btn {
  border-radius: 999px;
  font-weight: 600;
  color: #041016;
  background: linear-gradient(120deg, #9af8ef, #2ee6d6 50%, #14b8a6);
  box-shadow:
    0 0 0 1px rgba(46, 230, 214, 0.4),
    0 12px 32px rgba(46, 230, 214, 0.3),
    inset 0 1px 0 rgba(255, 255, 255, 0.35);
  transition:
    transform 0.2s ease,
    box-shadow 0.2s ease;
}

.home-primary-btn:hover {
  transform: translateY(-2px);
  box-shadow:
    0 0 0 1px rgba(46, 230, 214, 0.55),
    0 16px 36px rgba(46, 230, 214, 0.4),
    inset 0 1px 0 rgba(255, 255, 255, 0.4);
}

.home-signal-hint {
  font-family: 'Oxanium', monospace;
  font-size: 0.68rem;
  letter-spacing: 0.16em;
  color: var(--home-muted);
}

.home-chip,
.home-card,
.home-provider {
  border: 1px solid rgba(46, 230, 214, 0.18);
  background: rgba(8, 22, 32, 0.55);
  backdrop-filter: blur(14px);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.04),
    0 10px 30px rgba(0, 0, 0, 0.18);
}

:global(html:not(.dark)) .home-chip,
:global(html:not(.dark)) .home-card,
:global(html:not(.dark)) .home-provider {
  background: rgba(255, 255, 255, 0.72);
  border-color: rgba(13, 148, 136, 0.18);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.7),
    0 10px 28px rgba(15, 118, 110, 0.08);
}

.home-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.65rem;
  border-radius: 999px;
  padding: 0.65rem 1.2rem;
  font-size: 0.875rem;
  font-weight: 500;
}

.home-card {
  border-radius: 1.1rem;
  padding: 1.4rem;
  transition:
    transform 0.25s ease,
    border-color 0.25s ease,
    box-shadow 0.25s ease;
}

.home-card:hover {
  transform: translateY(-4px);
  border-color: rgba(46, 230, 214, 0.4);
  box-shadow:
    0 0 0 1px rgba(46, 230, 214, 0.12),
    0 18px 40px rgba(46, 230, 214, 0.12);
}

.home-card-icon {
  display: flex;
  width: 3rem;
  height: 3rem;
  align-items: center;
  justify-content: center;
  margin-bottom: 1rem;
  border-radius: 0.85rem;
  transition: transform 0.25s ease;
}

.home-card:hover .home-card-icon {
  transform: scale(1.08);
}

.home-card-icon--blue {
  background: linear-gradient(145deg, #38bdf8, #0284c7);
  box-shadow: 0 10px 24px rgba(14, 165, 233, 0.35);
}

.home-card-icon--teal {
  background: linear-gradient(145deg, #2ee6d6, #0d9488);
  box-shadow: 0 10px 24px rgba(20, 184, 166, 0.35);
}

.home-card-icon--amber {
  background: linear-gradient(145deg, #fbbf24, #d97706);
  box-shadow: 0 10px 24px rgba(245, 158, 11, 0.35);
}

.home-provider {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  border-radius: 0.9rem;
  padding: 0.7rem 1.1rem;
  font-size: 0.875rem;
  font-weight: 500;
}

.home-provider--muted {
  opacity: 0.55;
}

.home-provider-badge {
  display: flex;
  width: 2rem;
  height: 2rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.55rem;
  background-image: linear-gradient(145deg, var(--tw-gradient-stops));
  font-size: 0.7rem;
  font-weight: 700;
  color: white;
}

.home-provider-tag {
  border-radius: 0.3rem;
  padding: 0.1rem 0.35rem;
  font-size: 0.62rem;
  font-weight: 600;
  color: #041016;
  background: rgba(46, 230, 214, 0.85);
}

.home-provider-tag--soon {
  background: rgba(148, 163, 184, 0.35);
  color: inherit;
}

.home-footer {
  border-top: 1px solid rgba(46, 230, 214, 0.12);
}

.home-reveal {
  opacity: 0;
  animation: rise-in 0.8s cubic-bezier(0.22, 1, 0.36, 1) forwards;
  animation-delay: var(--delay, 0s);
}

/* Terminal */
.terminal-container {
  position: relative;
  display: inline-block;
}

.terminal-glow {
  position: absolute;
  inset: -18% -10%;
  background: radial-gradient(circle, rgba(46, 230, 214, 0.28), transparent 65%);
  filter: blur(18px);
  animation: glow-breathe 4.5s ease-in-out infinite;
}

.terminal-window {
  position: relative;
  width: min(420px, 88vw);
  background: linear-gradient(155deg, rgba(15, 28, 42, 0.96) 0%, rgba(7, 14, 24, 0.98) 100%);
  border-radius: 14px;
  border: 1px solid rgba(46, 230, 214, 0.28);
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.55),
    0 0 0 1px rgba(255, 255, 255, 0.04),
    0 0 48px rgba(46, 230, 214, 0.18),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
  overflow: hidden;
  transform: perspective(1000px) rotateX(2deg) rotateY(-2deg);
  transition: transform 0.3s ease;
}

.terminal-window:hover {
  transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px);
}

.terminal-header {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: rgba(12, 24, 36, 0.85);
  border-bottom: 1px solid rgba(46, 230, 214, 0.12);
}

.terminal-buttons {
  display: flex;
  gap: 8px;
}

.terminal-buttons span {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.btn-close {
  background: #ef4444;
}
.btn-minimize {
  background: #eab308;
}
.btn-maximize {
  background: #22c55e;
}

.terminal-title {
  flex: 1;
  text-align: center;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #64748b;
  letter-spacing: 0.08em;
  margin-right: 52px;
}

.terminal-body {
  padding: 20px 24px;
  font-family: ui-monospace, 'Fira Code', monospace;
  font-size: 14px;
  line-height: 2;
}

.code-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  opacity: 0;
  animation: line-appear 0.5s ease forwards;
}

.line-1 {
  animation-delay: 0.3s;
}
.line-2 {
  animation-delay: 1s;
}
.line-3 {
  animation-delay: 1.8s;
}
.line-4 {
  animation-delay: 2.5s;
}

@keyframes line-appear {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.code-prompt {
  color: #2ee6d6;
  font-weight: bold;
  text-shadow: 0 0 10px rgba(46, 230, 214, 0.45);
}
.code-cmd {
  color: #38bdf8;
}
.code-flag {
  color: #a5b4fc;
}
.code-url {
  color: #5eead4;
}
.code-comment {
  color: #64748b;
  font-style: italic;
}
.code-success {
  color: #4ade80;
  background: rgba(34, 197, 94, 0.15);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
  box-shadow: 0 0 14px rgba(74, 222, 128, 0.2);
}
.code-response {
  color: #fbbf24;
}

.cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: #2ee6d6;
  box-shadow: 0 0 10px rgba(46, 230, 214, 0.7);
  animation: blink 1s step-end infinite;
}

@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

@keyframes grid-drift {
  from {
    transform: translateY(0);
  }
  to {
    transform: translateY(56px);
  }
}

@keyframes scan-sweep {
  0%,
  100% {
    background-position: 0% 0%;
  }
  50% {
    background-position: 0% 100%;
  }
}

@keyframes beam-pulse {
  0%,
  100% {
    opacity: 0.25;
    filter: blur(1px);
  }
  50% {
    opacity: 0.7;
    filter: blur(0.2px);
  }
}

@keyframes star-twinkle {
  0%,
  100% {
    opacity: 0.2;
    transform: scale(0.85);
  }
  50% {
    opacity: 0.95;
    transform: scale(1.25);
  }
}

@keyframes meteor-fall {
  0% {
    opacity: 0;
    transform: rotate(-32deg) translate3d(-30%, -20%, 0);
  }
  8% {
    opacity: 1;
  }
  70% {
    opacity: 1;
  }
  100% {
    opacity: 0;
    transform: rotate(-32deg) translate3d(160%, 140%, 0);
  }
}

@keyframes halo-pulse {
  0% {
    transform: scale(0.72);
    opacity: 0.7;
  }
  70% {
    opacity: 0.2;
  }
  100% {
    transform: scale(1.18);
    opacity: 0;
  }
}

@keyframes radar-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

@keyframes title-ring {
  0% {
    transform: scale(0.7);
    opacity: 0.65;
  }
  100% {
    transform: scale(1.25);
    opacity: 0;
  }
}

@keyframes orb-float {
  0%,
  100% {
    transform: translate3d(0, 0, 0) scale(1);
  }
  50% {
    transform: translate3d(24px, -18px, 0) scale(1.08);
  }
}

@keyframes glow-breathe {
  0%,
  100% {
    opacity: 0.55;
    transform: scale(0.96);
  }
  50% {
    opacity: 1;
    transform: scale(1.04);
  }
}

@keyframes title-shine {
  0%,
  100% {
    background-position: 0% 50%;
  }
  50% {
    background-position: 100% 50%;
  }
}

@keyframes pulse-dot {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.45;
    transform: scale(0.85);
  }
}

@keyframes rise-in {
  from {
    opacity: 0;
    transform: translateY(18px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-grid,
  .home-scan,
  .home-beam,
  .home-orb,
  .home-title-glow,
  .terminal-glow,
  .home-reveal,
  .code-line,
  .cursor,
  .home-kicker-dot,
  .home-star,
  .home-meteor,
  .home-halo,
  .home-title-rings i {
    animation: none !important;
  }

  .home-reveal,
  .code-line {
    opacity: 1;
  }

  .home-spotlight,
  .home-meteors,
  .home-halos,
  .home-title-rings {
    display: none;
  }
}
</style>
