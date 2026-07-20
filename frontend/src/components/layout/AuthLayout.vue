<template>
  <div
    class="auth-shell relative flex min-h-screen items-center justify-center overflow-hidden p-4"
    @mousemove="onPointerMove"
  >
    <!-- Dynamic light + geek field -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden="true">
      <div class="auth-grid"></div>
      <div class="auth-hex"></div>
      <div class="auth-circuit"></div>

      <div class="auth-stars">
        <span v-for="n in 36" :key="'st-' + n" class="auth-star" :style="starStyle(n)"></span>
      </div>
      <div class="auth-meteors">
        <span v-for="n in 12" :key="'mt-' + n" class="auth-meteor" :style="meteorStyle(n)"></span>
      </div>

      <!-- Binary / hex rain -->
      <div class="auth-rain">
        <div
          v-for="col in rainCols"
          :key="'rn-' + col.id"
          class="auth-rain-col"
          :style="col.style"
        >
          {{ col.text }}
        </div>
      </div>

      <!-- Floating code chips -->
      <div class="auth-codechips">
        <span
          v-for="chip in codeChips"
          :key="chip.id"
          class="auth-codechip"
          :style="chip.style"
        >{{ chip.text }}</span>
      </div>

      <div class="auth-halos">
        <span class="auth-halo auth-halo--1"></span>
        <span class="auth-halo auth-halo--2"></span>
        <span class="auth-halo auth-halo--3"></span>
        <span class="auth-halo auth-halo--4"></span>
        <span class="auth-halo auth-halo--core"></span>
        <span class="auth-halo auth-halo--radar"></span>
        <span class="auth-halo auth-halo--radar-2"></span>
      </div>

      <div class="auth-scan"></div>
      <div class="auth-scan auth-scan--fast"></div>
      <div class="auth-beam auth-beam--a"></div>
      <div class="auth-beam auth-beam--b"></div>
      <div class="auth-beam auth-beam--c"></div>
      <div class="auth-beam auth-beam--d"></div>
      <div class="auth-orb auth-orb--cyan"></div>
      <div class="auth-orb auth-orb--amber"></div>
      <div class="auth-orb auth-orb--blue"></div>
      <div
        class="auth-spotlight"
        :style="{ transform: `translate3d(${pointer.x}px, ${pointer.y}px, 0)` }"
      ></div>

      <!-- HUD corners -->
      <div class="auth-hud auth-hud--tl"></div>
      <div class="auth-hud auth-hud--tr"></div>
      <div class="auth-hud auth-hud--bl"></div>
      <div class="auth-hud auth-hud--br"></div>

      <div class="auth-noise"></div>
      <div class="auth-vignette"></div>
    </div>

    <!-- Content Container -->
    <div class="auth-panel relative z-10 w-full max-w-md">
      <div class="auth-brand mb-6 text-center">
        <template v-if="settingsLoaded">
          <div class="auth-logo-wrap mb-4 inline-flex">
            <span class="auth-logo-rings" aria-hidden="true"><i></i><i></i><i></i></span>
            <div class="auth-logo relative z-10 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl">
              <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
            </div>
          </div>
          <p class="auth-kicker mb-2">
            <span class="auth-kicker-dot"></span>
            SECURE ACCESS // GATEWAY
          </p>
          <h1 class="auth-title mb-2 text-3xl font-bold">
            <span class="auth-title-glow">{{ siteName }}</span>
          </h1>
          <p class="auth-subtitle text-sm">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Geek status strip -->
      <div class="auth-status mb-3" aria-hidden="true">
        <span class="auth-status-item"><i></i>TLS 1.3</span>
        <span class="auth-status-item"><i></i>JWT READY</span>
        <span class="auth-status-item auth-status-item--live"><i></i>NODE ONLINE</span>
        <span class="auth-status-item auth-status-cursor">_</span>
      </div>

      <div class="auth-card rounded-2xl p-8">
        <div class="auth-card-chrome" aria-hidden="true">
          <span>auth.session</span>
          <span>pid://login</span>
        </div>
        <slot />
      </div>

      <div class="auth-terminal mt-3" aria-hidden="true">
        <span class="auth-terminal-line">$ curl -X POST /api/v1/auth/login</span>
        <span class="auth-terminal-line auth-terminal-line--ok">← 200 handshake ok · awaiting credentials<span class="auth-blink">▌</span></span>
      </div>

      <div class="auth-footer-links mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <div class="auth-copy mt-8 text-center text-xs">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

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

const pointer = reactive({ x: -200, y: -200 })
let raf = 0
let targetX = -200
let targetY = -200

const CHIP_TEXTS = [
  'POST /v1/messages',
  'Authorization: Bearer',
  'x-api-key: sk-***',
  '200 OK',
  'route → upstream',
  'sticky_session=1',
  'rpm_limit=60',
  'hash: sha256',
  'TLS handshake',
  'failover ✓'
]

const HEX = '0123456789ABCDEF01C0FFEEDEADBEEF'

function hashUnit(n: number, salt: number) {
  const x = Math.sin(n * 12.9898 + salt * 78.233) * 43758.5453
  return x - Math.floor(x)
}

function starStyle(n: number) {
  return {
    left: `${hashUnit(n, 1) * 100}%`,
    top: `${hashUnit(n, 2) * 100}%`,
    width: `${1.2 + hashUnit(n, 3) * 2.4}px`,
    height: `${1.2 + hashUnit(n, 3) * 2.4}px`,
    animationDelay: `${hashUnit(n, 4) * 5}s`,
    animationDuration: `${2 + hashUnit(n, 5) * 3.6}s`
  }
}

function meteorStyle(n: number) {
  return {
    left: `${6 + hashUnit(n, 6) * 88}%`,
    top: `${hashUnit(n, 7) * 50}%`,
    '--meteor-len': `${60 + hashUnit(n, 8) * 110}px`,
    animationDelay: `${hashUnit(n, 9) * 8}s`,
    animationDuration: `${1.6 + hashUnit(n, 10) * 2.4}s`
  }
}

const rainCols = Array.from({ length: 14 }, (_, i) => {
  let text = ''
  for (let j = 0; j < 18; j++) {
    text += HEX[Math.floor(hashUnit(i + 1, j + 3) * HEX.length)] + (j % 2 === 1 ? '\n' : '')
  }
  return {
    id: i,
    text,
    style: {
      left: `${4 + i * 7}%`,
      animationDelay: `${hashUnit(i + 1, 11) * 6}s`,
      animationDuration: `${8 + hashUnit(i + 1, 12) * 10}s`,
      opacity: String(0.12 + hashUnit(i + 1, 13) * 0.22)
    }
  }
})

const codeChips = CHIP_TEXTS.map((text, i) => ({
  id: i,
  text,
  style: {
    left: `${hashUnit(i + 2, 14) * 86}%`,
    top: `${8 + hashUnit(i + 2, 15) * 78}%`,
    animationDelay: `${hashUnit(i + 2, 16) * 7}s`,
    animationDuration: `${10 + hashUnit(i + 2, 17) * 8}s`
  }
}))

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

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-shell {
  --auth-ink: #e8f7ff;
  --auth-muted: rgba(198, 226, 239, 0.7);
  --auth-cyan: #2ee6d6;
  --auth-line: rgba(46, 230, 214, 0.14);
  font-family: 'Sora', ui-sans-serif, system-ui, sans-serif;
  color: var(--auth-ink);
  background:
    radial-gradient(900px 600px at 15% -5%, rgba(46, 230, 214, 0.2), transparent 55%),
    radial-gradient(700px 500px at 90% 5%, rgba(245, 185, 66, 0.12), transparent 50%),
    radial-gradient(600px 400px at 50% 100%, rgba(56, 189, 248, 0.1), transparent 55%),
    linear-gradient(160deg, #04090f 0%, #0a1622 50%, #061018 100%);
}

:global(html:not(.dark)) .auth-shell {
  --auth-ink: #0b1c24;
  --auth-muted: rgba(15, 48, 58, 0.7);
  --auth-line: rgba(13, 148, 136, 0.12);
  background:
    radial-gradient(900px 600px at 12% -8%, rgba(20, 184, 166, 0.16), transparent 55%),
    radial-gradient(700px 480px at 92% 0%, rgba(245, 158, 11, 0.1), transparent 50%),
    linear-gradient(165deg, #f7fcfd 0%, #eef8f9 48%, #e8f4f6 100%);
}

.auth-kicker,
.auth-title,
.auth-status,
.auth-terminal,
.auth-card-chrome,
.auth-codechip {
  font-family: 'Oxanium', ui-monospace, monospace;
}

.auth-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--auth-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--auth-line) 1px, transparent 1px);
  background-size: 52px 52px;
  mask-image: radial-gradient(ellipse 65% 55% at 50% 45%, #000 25%, transparent 78%);
  animation: auth-grid-drift 26s linear infinite;
}

.auth-hex {
  position: absolute;
  inset: 0;
  opacity: 0.18;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='56' height='100' viewBox='0 0 56 100'%3E%3Cpath d='M28 66L0 50L0 16L28 0L56 16L56 50L28 66L28 100' fill='none' stroke='%232ee6d6' stroke-width='0.6' opacity='0.45'/%3E%3Cpath d='M28 0L28 34L0 50L0 84L28 100L56 84L56 50L28 34' fill='none' stroke='%232ee6d6' stroke-width='0.4' opacity='0.25'/%3E%3C/svg%3E");
  background-size: 56px 100px;
  mask-image: radial-gradient(ellipse 70% 60% at 50% 40%, #000 10%, transparent 75%);
  animation: auth-hex-drift 40s linear infinite;
}

.auth-circuit {
  position: absolute;
  inset: 0;
  opacity: 0.22;
  background:
    linear-gradient(90deg, transparent 48%, rgba(46, 230, 214, 0.35) 49.5%, rgba(46, 230, 214, 0.35) 50.5%, transparent 52%) 12% 20% / 120px 2px no-repeat,
    linear-gradient(0deg, transparent 48%, rgba(46, 230, 214, 0.28) 49.5%, rgba(46, 230, 214, 0.28) 50.5%, transparent 52%) 18% 10% / 2px 160px no-repeat,
    linear-gradient(90deg, transparent 48%, rgba(245, 185, 66, 0.3) 49.5%, rgba(245, 185, 66, 0.3) 50.5%, transparent 52%) 78% 70% / 140px 2px no-repeat,
    linear-gradient(0deg, transparent 48%, rgba(56, 189, 248, 0.28) 49.5%, rgba(56, 189, 248, 0.28) 50.5%, transparent 52%) 82% 55% / 2px 180px no-repeat;
  animation: auth-circuit-pulse 5s ease-in-out infinite;
}

.auth-stars {
  position: absolute;
  inset: 0;
}

.auth-star {
  position: absolute;
  border-radius: 50%;
  background: #dffcff;
  box-shadow: 0 0 8px rgba(46, 230, 214, 0.85);
  opacity: 0.3;
  animation-name: auth-star-twinkle;
  animation-timing-function: ease-in-out;
  animation-iteration-count: infinite;
}

:global(html:not(.dark)) .auth-star {
  background: #0f766e;
  box-shadow: 0 0 6px rgba(13, 148, 136, 0.5);
}

.auth-meteors {
  position: absolute;
  inset: 0;
  overflow: hidden;
}

.auth-meteor {
  --meteor-len: 100px;
  position: absolute;
  width: var(--meteor-len);
  height: 2px;
  border-radius: 999px;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.95), #2ee6d6);
  box-shadow:
    0 0 10px rgba(46, 230, 214, 0.85),
    0 0 20px rgba(46, 230, 214, 0.4);
  transform: rotate(-34deg) translate3d(-140%, 0, 0);
  opacity: 0;
  animation-name: auth-meteor-fall;
  animation-timing-function: cubic-bezier(0.22, 0.61, 0.36, 1);
  animation-iteration-count: infinite;
}

.auth-meteor::after {
  content: '';
  position: absolute;
  right: -2px;
  top: 50%;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  transform: translateY(-50%);
  background: #fff;
  box-shadow: 0 0 12px 3px rgba(46, 230, 214, 0.9);
}

.auth-rain {
  position: absolute;
  inset: 0;
  overflow: hidden;
  mask-image: linear-gradient(180deg, transparent, #000 15%, #000 85%, transparent);
}

.auth-rain-col {
  position: absolute;
  top: -120%;
  white-space: pre;
  font-family: ui-monospace, 'Oxanium', monospace;
  font-size: 11px;
  line-height: 1.35;
  letter-spacing: 0.12em;
  color: rgba(46, 230, 214, 0.55);
  text-shadow: 0 0 8px rgba(46, 230, 214, 0.35);
  animation-name: auth-rain-fall;
  animation-timing-function: linear;
  animation-iteration-count: infinite;
}

.auth-codechips {
  position: absolute;
  inset: 0;
}

.auth-codechip {
  position: absolute;
  padding: 0.2rem 0.45rem;
  border-radius: 0.35rem;
  border: 1px solid rgba(46, 230, 214, 0.28);
  background: rgba(6, 18, 28, 0.55);
  color: rgba(126, 240, 230, 0.75);
  font-size: 0.62rem;
  letter-spacing: 0.04em;
  white-space: nowrap;
  box-shadow: 0 0 12px rgba(46, 230, 214, 0.12);
  animation-name: auth-chip-float;
  animation-timing-function: ease-in-out;
  animation-iteration-count: infinite;
}

:global(html:not(.dark)) .auth-codechip {
  background: rgba(255, 255, 255, 0.65);
  color: rgba(15, 118, 110, 0.8);
}

.auth-halos {
  position: absolute;
  left: 50%;
  top: 42%;
  width: min(680px, 92vw);
  height: min(680px, 92vw);
  transform: translate(-50%, -50%);
}

.auth-halo {
  position: absolute;
  inset: 18%;
  border-radius: 50%;
  border: 1px solid rgba(46, 230, 214, 0.26);
  box-shadow:
    0 0 22px rgba(46, 230, 214, 0.1),
    inset 0 0 22px rgba(46, 230, 214, 0.05);
}

.auth-halo--1 {
  inset: 14%;
  animation: auth-halo-pulse 5.2s ease-out infinite;
}

.auth-halo--2 {
  inset: 4%;
  border-color: rgba(56, 189, 248, 0.22);
  animation: auth-halo-pulse 5.2s ease-out infinite 1s;
}

.auth-halo--3 {
  inset: -6%;
  border-color: rgba(245, 185, 66, 0.16);
  animation: auth-halo-pulse 5.2s ease-out infinite 2s;
}

.auth-halo--4 {
  inset: -16%;
  border-color: rgba(46, 230, 214, 0.1);
  animation: auth-halo-pulse 5.2s ease-out infinite 3s;
}

.auth-halo--core {
  inset: 28%;
  border: none;
  background: radial-gradient(circle, rgba(46, 230, 214, 0.24), transparent 70%);
  filter: blur(10px);
  animation: auth-glow-breathe 3.8s ease-in-out infinite;
}

.auth-halo--radar {
  inset: 8%;
  border-style: dashed;
  border-color: rgba(46, 230, 214, 0.2);
  animation: auth-radar-spin 20s linear infinite;
}

.auth-halo--radar-2 {
  inset: -2%;
  border-style: dotted;
  border-color: rgba(56, 189, 248, 0.16);
  animation: auth-radar-spin 32s linear infinite reverse;
}

.auth-scan {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    180deg,
    transparent 0%,
    rgba(46, 230, 214, 0.05) 48%,
    transparent 52%,
    transparent 100%
  );
  background-size: 100% 220%;
  animation: auth-scan-sweep 7.2s ease-in-out infinite;
}

.auth-scan--fast {
  opacity: 0.55;
  animation-duration: 3.8s;
  background: linear-gradient(
    180deg,
    transparent 0%,
    rgba(56, 189, 248, 0.04) 46%,
    transparent 54%,
    transparent 100%
  );
  background-size: 100% 240%;
}

.auth-beam {
  position: absolute;
  width: 42vw;
  max-width: 580px;
  height: 2px;
  border-radius: 999px;
  opacity: 0.55;
}

.auth-beam--a {
  top: 14%;
  left: -6%;
  background: linear-gradient(90deg, transparent, rgba(46, 230, 214, 0.85), transparent);
  box-shadow: 0 0 24px rgba(46, 230, 214, 0.45);
  transform: rotate(-16deg);
  animation: auth-beam-pulse 4.8s ease-in-out infinite;
}

.auth-beam--b {
  bottom: 16%;
  right: -8%;
  background: linear-gradient(90deg, transparent, rgba(245, 185, 66, 0.7), transparent);
  box-shadow: 0 0 22px rgba(245, 185, 66, 0.35);
  transform: rotate(14deg);
  animation: auth-beam-pulse 6s ease-in-out infinite reverse;
}

.auth-beam--c {
  top: 48%;
  left: 8%;
  width: 30vw;
  background: linear-gradient(90deg, transparent, rgba(56, 189, 248, 0.7), transparent);
  box-shadow: 0 0 18px rgba(56, 189, 248, 0.35);
  transform: rotate(-7deg);
  animation: auth-beam-pulse 4.2s ease-in-out infinite 0.8s;
}

.auth-beam--d {
  top: 72%;
  left: 35%;
  width: 24vw;
  background: linear-gradient(90deg, transparent, rgba(126, 240, 230, 0.55), transparent);
  transform: rotate(22deg);
  animation: auth-beam-pulse 5.5s ease-in-out infinite 1.4s;
}

.auth-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(46px);
}

.auth-orb--cyan {
  top: -10%;
  right: -8%;
  width: 380px;
  height: 380px;
  background: radial-gradient(circle, rgba(46, 230, 214, 0.42), transparent 68%);
  animation: auth-orb-float 12s ease-in-out infinite;
}

.auth-orb--amber {
  bottom: -12%;
  left: -10%;
  width: 340px;
  height: 340px;
  background: radial-gradient(circle, rgba(245, 185, 66, 0.28), transparent 70%);
  animation: auth-orb-float 16s ease-in-out infinite reverse;
}

.auth-orb--blue {
  top: 40%;
  left: 55%;
  width: 240px;
  height: 240px;
  background: radial-gradient(circle, rgba(56, 189, 248, 0.22), transparent 70%);
  animation: auth-orb-float 10s ease-in-out infinite;
}

.auth-spotlight {
  position: absolute;
  top: 0;
  left: 0;
  width: 420px;
  height: 420px;
  margin: -210px 0 0 -210px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(46, 230, 214, 0.18), transparent 65%);
  transition: transform 0.08s linear;
}

.auth-hud {
  position: absolute;
  width: 56px;
  height: 56px;
  border-color: rgba(46, 230, 214, 0.45);
  border-style: solid;
  opacity: 0.7;
}

.auth-hud--tl {
  top: 18px;
  left: 18px;
  border-width: 2px 0 0 2px;
}

.auth-hud--tr {
  top: 18px;
  right: 18px;
  border-width: 2px 2px 0 0;
}

.auth-hud--bl {
  bottom: 18px;
  left: 18px;
  border-width: 0 0 2px 2px;
}

.auth-hud--br {
  bottom: 18px;
  right: 18px;
  border-width: 0 2px 2px 0;
}

.auth-noise {
  position: absolute;
  inset: 0;
  opacity: 0.045;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
  mix-blend-mode: overlay;
}

.auth-vignette {
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at center, transparent 40%, rgba(0, 0, 0, 0.42) 100%);
}

:global(html:not(.dark)) .auth-vignette {
  background: radial-gradient(ellipse at center, transparent 48%, rgba(7, 24, 32, 0.08) 100%);
}

.auth-panel {
  animation: auth-rise-in 0.75s cubic-bezier(0.22, 1, 0.36, 1) both;
}

.auth-logo-wrap {
  position: relative;
}

.auth-logo-rings {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 150%;
  height: 150%;
  transform: translate(-50%, -50%);
  pointer-events: none;
}

.auth-logo-rings i {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  border: 1px solid rgba(46, 230, 214, 0.38);
  box-shadow: 0 0 16px rgba(46, 230, 214, 0.2);
  animation: auth-title-ring 4.2s ease-out infinite;
}

.auth-logo-rings i:nth-child(2) {
  inset: -12%;
  animation-delay: 1s;
  border-color: rgba(56, 189, 248, 0.3);
}

.auth-logo-rings i:nth-child(3) {
  inset: -26%;
  animation-delay: 2s;
  border-color: rgba(245, 185, 66, 0.24);
}

.auth-logo {
  box-shadow:
    0 0 0 1px rgba(46, 230, 214, 0.45),
    0 0 32px rgba(46, 230, 214, 0.4),
    0 12px 28px rgba(0, 0, 0, 0.28);
  background: rgba(8, 22, 32, 0.55);
}

.auth-kicker {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.68rem;
  letter-spacing: 0.18em;
  color: var(--auth-cyan);
  text-transform: uppercase;
}

.auth-kicker-dot {
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 999px;
  background: var(--auth-cyan);
  box-shadow: 0 0 10px var(--auth-cyan);
  animation: auth-pulse-dot 1.6s ease-in-out infinite;
}

.auth-title-glow {
  background: linear-gradient(120deg, #f4fffd 10%, #7ff5ea 42%, #2ee6d6 68%, #f5b942 100%);
  background-size: 200% 200%;
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  animation: auth-title-shine 7s ease-in-out infinite;
  filter: drop-shadow(0 0 16px rgba(46, 230, 214, 0.3));
}

:global(html:not(.dark)) .auth-title-glow {
  background: linear-gradient(120deg, #083344 8%, #0f766e 40%, #0891b2 70%, #b45309 100%);
  background-size: 200% 200%;
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  filter: none;
}

.auth-subtitle,
.auth-copy {
  color: var(--auth-muted);
}

.auth-status {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.55rem 0.85rem;
  font-size: 0.62rem;
  letter-spacing: 0.08em;
  color: rgba(126, 240, 230, 0.75);
}

.auth-status-item {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.auth-status-item i {
  width: 0.35rem;
  height: 0.35rem;
  border-radius: 50%;
  background: #2ee6d6;
  box-shadow: 0 0 8px #2ee6d6;
}

.auth-status-item--live i {
  background: #4ade80;
  box-shadow: 0 0 8px #4ade80;
  animation: auth-pulse-dot 1.2s ease-in-out infinite;
}

.auth-status-cursor {
  color: #2ee6d6;
  animation: auth-blink 1s step-end infinite;
}

.auth-card {
  position: relative;
  border: 1px solid rgba(46, 230, 214, 0.28);
  background: rgba(8, 22, 32, 0.68);
  backdrop-filter: blur(18px);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.05),
    0 0 0 1px rgba(46, 230, 214, 0.08),
    0 24px 48px rgba(0, 0, 0, 0.38),
    0 0 48px rgba(46, 230, 214, 0.12);
  animation: auth-rise-in 0.85s cubic-bezier(0.22, 1, 0.36, 1) 0.08s both;
}

.auth-card::before {
  content: '';
  position: absolute;
  inset: -1px;
  border-radius: inherit;
  padding: 1px;
  background: linear-gradient(135deg, rgba(46, 230, 214, 0.55), transparent 35%, transparent 65%, rgba(245, 185, 66, 0.35));
  -webkit-mask:
    linear-gradient(#fff 0 0) content-box,
    linear-gradient(#fff 0 0);
  -webkit-mask-composite: xor;
  mask-composite: exclude;
  pointer-events: none;
  animation: auth-border-glow 5s linear infinite;
}

.auth-card-chrome {
  display: flex;
  justify-content: space-between;
  margin: -0.5rem 0 1rem;
  font-size: 0.62rem;
  letter-spacing: 0.08em;
  color: rgba(126, 240, 230, 0.55);
  text-transform: lowercase;
}

:global(html:not(.dark)) .auth-card {
  background: rgba(255, 255, 255, 0.8);
  border-color: rgba(13, 148, 136, 0.22);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.75),
    0 20px 40px rgba(15, 118, 110, 0.1);
}

.auth-terminal {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: 0.65rem 0.85rem;
  border-radius: 0.65rem;
  border: 1px solid rgba(46, 230, 214, 0.18);
  background: rgba(4, 12, 18, 0.55);
  font-size: 0.68rem;
  letter-spacing: 0.03em;
  color: rgba(126, 240, 230, 0.7);
  box-shadow: inset 0 0 20px rgba(46, 230, 214, 0.05);
}

.auth-terminal-line--ok {
  color: #86efac;
}

.auth-blink {
  display: inline-block;
  margin-left: 0.15rem;
  color: #2ee6d6;
  animation: auth-blink 1s step-end infinite;
}

:global(html:not(.dark)) .auth-terminal {
  background: rgba(15, 48, 58, 0.06);
  color: rgba(15, 118, 110, 0.8);
}

.auth-footer-links :deep(a) {
  color: #5eead4;
  transition: color 0.2s ease;
}

.auth-footer-links :deep(a:hover) {
  color: #99f6e4;
}

:global(html:not(.dark)) .auth-footer-links :deep(a) {
  color: #0d9488;
}

.auth-footer-links :deep(p) {
  color: var(--auth-muted);
}

@keyframes auth-grid-drift {
  from {
    transform: translateY(0);
  }
  to {
    transform: translateY(52px);
  }
}

@keyframes auth-hex-drift {
  from {
    background-position: 0 0;
  }
  to {
    background-position: 56px 100px;
  }
}

@keyframes auth-circuit-pulse {
  0%,
  100% {
    opacity: 0.14;
  }
  50% {
    opacity: 0.32;
  }
}

@keyframes auth-star-twinkle {
  0%,
  100% {
    opacity: 0.18;
    transform: scale(0.85);
  }
  50% {
    opacity: 0.95;
    transform: scale(1.25);
  }
}

@keyframes auth-meteor-fall {
  0% {
    opacity: 0;
    transform: rotate(-34deg) translate3d(-30%, -20%, 0);
  }
  8% {
    opacity: 1;
  }
  70% {
    opacity: 1;
  }
  100% {
    opacity: 0;
    transform: rotate(-34deg) translate3d(160%, 140%, 0);
  }
}

@keyframes auth-rain-fall {
  from {
    transform: translateY(0);
  }
  to {
    transform: translateY(220%);
  }
}

@keyframes auth-chip-float {
  0%,
  100% {
    transform: translateY(0);
    opacity: 0.35;
  }
  50% {
    transform: translateY(-10px);
    opacity: 0.85;
  }
}

@keyframes auth-halo-pulse {
  0% {
    transform: scale(0.7);
    opacity: 0.7;
  }
  70% {
    opacity: 0.15;
  }
  100% {
    transform: scale(1.18);
    opacity: 0;
  }
}

@keyframes auth-radar-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

@keyframes auth-scan-sweep {
  0%,
  100% {
    background-position: 0% 0%;
  }
  50% {
    background-position: 0% 100%;
  }
}

@keyframes auth-beam-pulse {
  0%,
  100% {
    opacity: 0.2;
  }
  50% {
    opacity: 0.75;
  }
}

@keyframes auth-orb-float {
  0%,
  100% {
    transform: translate3d(0, 0, 0) scale(1);
  }
  50% {
    transform: translate3d(18px, -14px, 0) scale(1.06);
  }
}

@keyframes auth-glow-breathe {
  0%,
  100% {
    opacity: 0.55;
    transform: scale(0.96);
  }
  50% {
    opacity: 1;
    transform: scale(1.06);
  }
}

@keyframes auth-title-ring {
  0% {
    transform: scale(0.7);
    opacity: 0.65;
  }
  100% {
    transform: scale(1.32);
    opacity: 0;
  }
}

@keyframes auth-title-shine {
  0%,
  100% {
    background-position: 0% 50%;
  }
  50% {
    background-position: 100% 50%;
  }
}

@keyframes auth-pulse-dot {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.35;
    transform: scale(0.85);
  }
}

@keyframes auth-blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

@keyframes auth-border-glow {
  0% {
    filter: hue-rotate(0deg);
  }
  100% {
    filter: hue-rotate(40deg);
  }
}

@keyframes auth-rise-in {
  from {
    opacity: 0;
    transform: translateY(16px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .auth-grid,
  .auth-hex,
  .auth-circuit,
  .auth-scan,
  .auth-beam,
  .auth-orb,
  .auth-star,
  .auth-meteor,
  .auth-halo,
  .auth-rain-col,
  .auth-codechip,
  .auth-logo-rings i,
  .auth-title-glow,
  .auth-kicker-dot,
  .auth-panel,
  .auth-card,
  .auth-blink,
  .auth-status-cursor,
  .auth-status-item--live i {
    animation: none !important;
  }

  .auth-panel,
  .auth-card {
    opacity: 1;
  }

  .auth-spotlight,
  .auth-meteors,
  .auth-halos,
  .auth-logo-rings,
  .auth-rain,
  .auth-codechips {
    display: none;
  }
}
</style>
