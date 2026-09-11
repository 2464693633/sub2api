<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
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

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img
          :src="siteLogo || '/logo.svg'"
          alt="Logo"
          class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- Default Home Page: full-screen video hero with glass card -->
  <div v-else class="relative flex min-h-screen flex-col overflow-hidden text-white">
    <HeroVideoBackground
      ref="heroBackground"
      v-model:active-index="activeThemeIndex"
      :themes="HOME_HERO_THEMES"
    />

    <!-- Header -->
    <header class="relative z-20 px-4 py-4 sm:px-6">
      <nav class="mx-auto flex max-w-6xl items-center justify-between gap-3">
        <div class="flex min-w-0 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-xl object-contain shadow-md"
          />
          <span class="min-w-0 truncate text-base font-semibold drop-shadow-md">{{ siteName }}</span>
        </div>
        <div class="flex shrink-0 items-center gap-2 [&_button]:text-white/80 [&_button:hover]:bg-white/10">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 items-center justify-center rounded-lg text-white/80 transition-colors hover:bg-white/10 hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="flex h-10 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-white/80 transition-colors hover:bg-white/10 hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <!-- 未登录时登录入口即卡片内嵌表单,导航不再重复放置登录按钮 -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex min-h-10 items-center justify-center rounded-xl bg-amber-500/90 px-4 py-2 text-sm font-semibold text-black shadow-lg transition-all hover:bg-amber-400 hover:shadow-[0_0_24px_rgba(245,158,11,0.4)]"
          >
            {{ t('home.dashboard') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Hero: glass card -->
    <main class="relative z-10 flex flex-1 items-center justify-center px-4 py-12 sm:px-6">
      <div
        class="w-full max-w-2xl rounded-3xl border border-white/20 bg-white/10 px-8 py-12 text-center shadow-2xl backdrop-blur-xl sm:px-12"
      >
        <span
          class="inline-flex items-center gap-1.5 rounded-full border border-amber-300/50 bg-amber-400/10 px-3 py-1 text-xs font-medium text-amber-200"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-amber-300"></span>
          {{ siteSubtitle || t('home.hero.badge') }}
        </span>
        <h1 class="mt-5 text-4xl font-bold drop-shadow-lg md:text-5xl">{{ siteName }}</h1>
        <p class="mx-auto mt-4 max-w-md text-sm leading-relaxed text-white/75 md:text-base">
          {{ t('home.hero.description') }}
        </p>

        <!-- 未登录:登录/注册标签页(视频背景透过毛玻璃可见) -->
        <div
          v-if="!isAuthenticated"
          class="mx-auto mt-8 w-full max-w-sm"
        >
          <div
            v-if="registrationEnabled"
            class="mb-4 grid grid-cols-2 gap-1 rounded-xl border border-white/20 bg-white/5 p-1 backdrop-blur"
          >
            <button
              type="button"
              class="h-9 rounded-lg text-sm font-medium transition-all"
              :class="homeAuthMode === 'login'
                ? 'bg-amber-500/90 text-black shadow'
                : 'text-white/70 hover:bg-white/10 hover:text-white'"
              @click="homeAuthMode = 'login'"
            >
              {{ t('auth.signIn') }}
            </button>
            <button
              type="button"
              class="h-9 rounded-lg text-sm font-medium transition-all"
              :class="homeAuthMode === 'register'
                ? 'bg-amber-500/90 text-black shadow'
                : 'text-white/70 hover:bg-white/10 hover:text-white'"
              @click="homeAuthMode = 'register'"
            >
              {{ t('auth.signUp') }}
            </button>
          </div>

          <!-- 登录表单 -->
          <form
            v-if="homeAuthMode === 'login'"
            class="space-y-3 text-left"
            @submit.prevent="handleHomeLogin"
          >
            <input
              v-model="loginEmail"
              type="email"
              autocomplete="email"
              :placeholder="t('auth.emailPlaceholder')"
              class="h-11 w-full rounded-xl border border-white/25 bg-white/10 px-4 text-sm text-white placeholder-white/50 outline-none backdrop-blur transition-colors focus:border-amber-300/70 focus:bg-white/15"
            />
            <input
              v-model="loginPassword"
              type="password"
              autocomplete="current-password"
              :placeholder="t('auth.passwordPlaceholder')"
              class="h-11 w-full rounded-xl border border-white/25 bg-white/10 px-4 text-sm text-white placeholder-white/50 outline-none backdrop-blur transition-colors focus:border-amber-300/70 focus:bg-white/15"
            />
            <p v-if="loginError" class="text-sm text-red-300">{{ loginError }}</p>
            <button
              type="submit"
              :disabled="loginLoading"
              class="inline-flex h-11 w-full items-center justify-center gap-2 rounded-xl bg-amber-500/90 px-6 text-sm font-semibold text-black shadow-lg transition-all hover:bg-amber-400 hover:shadow-[0_0_28px_rgba(245,158,11,0.45)] disabled:cursor-not-allowed disabled:opacity-60"
            >
              {{ loginLoading ? t('auth.signingIn') : t('auth.signIn') }}
            </button>
          </form>

          <!-- 注册表单;实例启用图形验证码(Turnstile/腾讯)时主页无法承载验证流程,引导到完整注册页 -->
          <div v-if="homeAuthMode === 'register' && captchaEnabled" class="space-y-4 text-center">
            <p class="text-sm leading-relaxed text-white/70">
              {{ t('home.hero.captchaRedirectHint') }}
            </p>
            <router-link
              to="/register"
              class="inline-flex min-h-11 w-full items-center justify-center rounded-xl bg-amber-500/90 px-6 text-sm font-semibold text-black shadow-lg transition-all hover:bg-amber-400 hover:shadow-[0_0_28px_rgba(245,158,11,0.45)]"
            >
              {{ t('auth.signUp') }}
            </router-link>
          </div>

          <form
            v-else-if="homeAuthMode === 'register'"
            class="space-y-3 text-left"
            @submit.prevent="handleHomeRegister"
          >
            <input
              v-model="registerEmail"
              type="email"
              autocomplete="email"
              :placeholder="t('auth.emailPlaceholder')"
              class="h-11 w-full rounded-xl border border-white/25 bg-white/10 px-4 text-sm text-white placeholder-white/50 outline-none backdrop-blur transition-colors focus:border-amber-300/70 focus:bg-white/15"
            />
            <input
              v-model="registerPassword"
              type="password"
              autocomplete="new-password"
              :placeholder="t('auth.createPasswordPlaceholder')"
              class="h-11 w-full rounded-xl border border-white/25 bg-white/10 px-4 text-sm text-white placeholder-white/50 outline-none backdrop-blur transition-colors focus:border-amber-300/70 focus:bg-white/15"
            />
            <!-- 邮箱验证码(实例开启邮箱验证时显示) -->
            <div v-if="emailVerifyEnabled" class="flex gap-2">
              <input
                v-model="registerVerifyCode"
                type="text"
                inputmode="numeric"
                maxlength="6"
                :placeholder="t('auth.verificationCodeHint')"
                class="h-11 min-w-0 flex-1 rounded-xl border border-white/25 bg-white/10 px-4 text-sm text-white placeholder-white/50 outline-none backdrop-blur transition-colors focus:border-amber-300/70 focus:bg-white/15"
              />
              <button
                type="button"
                :disabled="verifyCodeSending || verifyCodeCountdown > 0 || !registerEmail.trim()"
                class="h-11 shrink-0 rounded-xl border border-white/25 bg-white/10 px-3 text-xs font-medium text-white backdrop-blur transition-colors hover:border-white/40 hover:bg-white/20 disabled:cursor-not-allowed disabled:opacity-50"
                @click="handleSendVerifyCode"
              >
                {{ verifyCodeSending
                  ? t('auth.sendingCode')
                  : verifyCodeCountdown > 0
                    ? t('auth.resendCountdown', { countdown: verifyCodeCountdown })
                    : t('auth.sendCode') }}
              </button>
            </div>
            <input
              v-if="invitationCodeEnabled"
              v-model="registerInviteCode"
              type="text"
              :placeholder="t('auth.invitationCodePlaceholder')"
              class="h-11 w-full rounded-xl border border-white/25 bg-white/10 px-4 text-sm text-white placeholder-white/50 outline-none backdrop-blur transition-colors focus:border-amber-300/70 focus:bg-white/15"
            />
            <p v-if="registerError" class="text-sm text-red-300">{{ registerError }}</p>
            <button
              type="submit"
              :disabled="registerLoading"
              class="inline-flex h-11 w-full items-center justify-center gap-2 rounded-xl bg-amber-500/90 px-6 text-sm font-semibold text-black shadow-lg transition-all hover:bg-amber-400 hover:shadow-[0_0_28px_rgba(245,158,11,0.45)] disabled:cursor-not-allowed disabled:opacity-60"
            >
              {{ registerLoading ? t('auth.processing') : t('auth.createAccount') }}
            </button>
          </form>
        </div>

        <!-- 已登录:进入控制台 -->
        <div v-else class="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row">
          <router-link
            :to="dashboardPath"
            class="inline-flex min-h-11 w-full items-center justify-center gap-2 rounded-xl bg-amber-500/90 px-6 py-2.5 text-sm font-semibold text-black shadow-lg transition-all hover:bg-amber-400 hover:shadow-[0_0_28px_rgba(245,158,11,0.45)] sm:w-auto"
          >
            {{ t('home.goToDashboard') }}
            <Icon name="arrowRight" size="sm" :stroke-width="2" />
          </router-link>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="inline-flex min-h-11 w-full items-center justify-center rounded-xl border border-white/25 bg-white/10 px-6 py-2.5 text-sm font-medium text-white backdrop-blur transition-colors hover:border-white/40 hover:bg-white/20 sm:w-auto"
          >
            {{ t('nav.modelPlaza') }}
          </router-link>
        </div>
      </div>
    </main>

    <!-- Stats cards -->
    <section class="relative z-10 mx-auto mb-10 w-full max-w-3xl px-4 sm:px-6">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div
          v-for="stat in heroStats"
          :key="stat.label"
          class="rounded-2xl border border-white/15 bg-white/10 px-6 py-5 text-center shadow-lg backdrop-blur-md"
        >
          <div class="text-2xl font-bold drop-shadow md:text-3xl">{{ stat.value }}</div>
          <div class="mt-1 text-xs font-medium uppercase tracking-wide text-white/70">{{ stat.label }}</div>
        </div>
      </div>
    </section>

    <!-- Theme picker -->
    <section
      v-if="HOME_HERO_THEMES.length > 1"
      class="relative z-10 mb-8 flex flex-wrap items-center justify-center gap-3 px-4"
      :aria-label="t('home.hero.themeLabel')"
    >
      <button
        v-for="(theme, index) in HOME_HERO_THEMES"
        :key="theme.key"
        type="button"
        class="group relative h-14 w-28 overflow-hidden rounded-xl border transition-all duration-300 disabled:cursor-not-allowed disabled:opacity-30"
        :class="
          index === activeThemeIndex
            ? 'border-amber-300/80 shadow-lg ring-2 ring-amber-300/50'
            : 'border-white/20 hover:border-white/50 hover:shadow-md'
        "
        :disabled="heroBackground?.isThemeFailed(index)"
        :title="t(theme.labelKey)"
        @click="heroBackground?.selectTheme(index)"
      >
        <span class="absolute inset-0 bg-gradient-to-br transition-transform duration-300 group-hover:scale-110" :class="theme.gradient"></span>
        <span
          class="absolute inset-0 flex items-center justify-center text-xs font-semibold text-black/70 drop-shadow-sm transition-colors group-hover:text-black/85"
          :class="index === activeThemeIndex ? 'text-black/85' : ''"
        >
          {{ t(theme.labelKey) }}
        </span>
      </button>
    </section>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-white/10 px-4 py-5 text-center text-xs text-white/60 sm:px-6">
      &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import HeroVideoBackground from '@/components/home/HeroVideoBackground.vue'
import { HOME_HERO_THEMES, HOME_HERO_STATS } from '@/config/homeThemes'
import { modelPlazaAPI } from '@/api/modelPlaza'
import { authAPI, isTotp2FARequired } from '@/api'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()
const router = useRouter()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Lyozc')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// Video hero
const heroBackground = ref<InstanceType<typeof HeroVideoBackground> | null>(null)
const activeThemeIndex = ref(0)
const modelCount = ref<number | null>(null)

const heroStats = computed(() => [
  {
    value: modelCount.value !== null ? `${modelCount.value}+` : HOME_HERO_STATS.modelsFallback,
    label: t('home.hero.stats.models')
  },
  { value: HOME_HERO_STATS.platformsValue, label: t('home.hero.stats.platforms') },
  { value: HOME_HERO_STATS.uptimeValue, label: t('home.hero.stats.uptime') }
])

async function loadModelCount() {
  if (!modelPlazaEnabled.value) return
  try {
    const data = await modelPlazaAPI.getModelPlaza()
    const names = new Set<string>()
    for (const group of data.groups) {
      for (const model of group.models) names.add(model.name)
    }
    if (names.size > 0) modelCount.value = names.size
  } catch {
    // Model plaza may be disabled or require auth; stats card falls back to the configured value
  }
}

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)

// 主页内嵌登录(视频背景可见;需要验证码/2FA 时跳转完整登录页)
const loginEmail = ref('')
const loginPassword = ref('')
const loginLoading = ref(false)
const loginError = ref('')

// 主页内嵌注册(与登录同卡片切换;开启邮箱验证时增加验证码步骤)
const homeAuthMode = ref<'login' | 'register'>('login')
const registerEmail = ref('')
const registerPassword = ref('')
const registerInviteCode = ref('')
const registerLoading = ref(false)
const registerError = ref('')
const registrationEnabled = computed(() => appStore.cachedPublicSettings?.registration_enabled !== false)
const invitationCodeEnabled = computed(() => appStore.cachedPublicSettings?.invitation_code_enabled === true)
const emailVerifyEnabled = computed(() => appStore.cachedPublicSettings?.email_verify_enabled === true)
// 图形验证码开启时主页注册无法获取 proof,注册流程引导到完整注册页
const captchaEnabled = computed(
  () =>
    appStore.cachedPublicSettings?.turnstile_enabled === true ||
    appStore.cachedPublicSettings?.tencent_captcha_enabled === true ||
    appStore.cachedPublicSettings?.aliyun_captcha_enabled === true,
)

// 邮箱验证码
const registerVerifyCode = ref('')
const verifyCodeSending = ref(false)
const verifyCodeCountdown = ref(0)
let verifyCodeTimer: ReturnType<typeof setInterval> | null = null

async function handleSendVerifyCode() {
  registerError.value = ''
  if (!registerEmail.value.trim()) {
    registerError.value = t('auth.emailRequired')
    return
  }
  verifyCodeSending.value = true
  try {
    const resp = await authAPI.sendVerifyCode({ email: registerEmail.value.trim() })
    verifyCodeCountdown.value = resp.countdown || 60
    if (verifyCodeTimer) clearInterval(verifyCodeTimer)
    verifyCodeTimer = setInterval(() => {
      verifyCodeCountdown.value -= 1
      if (verifyCodeCountdown.value <= 0 && verifyCodeTimer) {
        clearInterval(verifyCodeTimer)
        verifyCodeTimer = null
      }
    }, 1000)
    appStore.showSuccess(t('auth.codeSentSuccess'))
  } catch (error: unknown) {
    registerError.value = extractI18nErrorMessage(error, t, 'auth.errors', t('auth.sendCodeFailed'))
  } finally {
    verifyCodeSending.value = false
  }
}

onBeforeUnmount(() => {
  if (verifyCodeTimer) clearInterval(verifyCodeTimer)
})

async function handleHomeRegister() {
  registerError.value = ''
  if (!registerEmail.value.trim()) {
    registerError.value = t('auth.emailRequired')
    return
  }
  if (!registerPassword.value || registerPassword.value.length < 6) {
    registerError.value = t('auth.passwordMinLength')
    return
  }
  if (emailVerifyEnabled.value && !registerVerifyCode.value.trim()) {
    registerError.value = t('auth.codeRequired')
    return
  }
  registerLoading.value = true
  try {
    const user = await authStore.register({
      email: registerEmail.value.trim(),
      password: registerPassword.value,
      verify_code: emailVerifyEnabled.value ? registerVerifyCode.value.trim() : undefined,
      invitation_code: registerInviteCode.value.trim() || undefined,
    })
    if (user) {
      appStore.showSuccess(t('auth.accountCreatedSuccess', { siteName: siteName.value }))
      await router.push(dashboardPath.value)
    }
  } catch (error: unknown) {
    registerError.value = extractI18nErrorMessage(error, t, 'auth.errors', t('auth.registrationFailed'))
  } finally {
    registerLoading.value = false
  }
}

async function handleHomeLogin() {
  loginError.value = ''
  if (!loginEmail.value.trim()) {
    loginError.value = t('auth.emailRequired')
    return
  }
  if (!loginPassword.value) {
    loginError.value = t('auth.passwordRequired')
    return
  }
  loginLoading.value = true
  try {
    const response = await authStore.login({
      email: loginEmail.value.trim(),
      password: loginPassword.value,
    })
    if (isTotp2FARequired(response)) {
      // 2FA 等增强验证在完整登录页完成
      await router.push('/login')
      return
    }
    appStore.showSuccess(t('auth.loginSuccess'))
    await router.push(dashboardPath.value)
  } catch (error: unknown) {
    loginError.value = extractI18nErrorMessage(error, t, 'auth.errors', t('auth.loginFailed'))
  } finally {
    loginLoading.value = false
  }
}
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value),
)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
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

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }

  void loadModelCount()
})
</script>
