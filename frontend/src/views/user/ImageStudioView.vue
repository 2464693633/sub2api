<template>
  <div class="mx-auto max-w-6xl space-y-4 p-4 text-gray-900 dark:text-gray-100">
    <!-- 页头 -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold">{{ t('imageStudio.title') }}</h1>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('imageStudio.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <label class="text-xs text-gray-500">{{ t('imageStudio.apiKey') }}</label>
        <select
          v-model="selectedKeyId"
          class="h-9 rounded-lg border border-gray-300 bg-white px-2 text-sm dark:border-dark-600 dark:bg-dark-800"
        >
          <option v-for="k in keys" :key="k.id" :value="k.id">{{ k.name }}（{{ maskKey(k.key) }}）</option>
        </select>
      </div>
    </div>

    <!-- 未选密钥提示 -->
    <div v-if="keys.length === 0" class="card p-6 text-center text-sm text-gray-500">
      {{ t('imageStudio.noKeys') }}
      <router-link to="/keys" class="text-primary-500 hover:underline">{{ t('imageStudio.goCreateKey') }}</router-link>
    </div>

    <div v-else class="grid gap-4 lg:grid-cols-[320px_1fr]">
      <!-- 左侧参数面板 -->
      <div class="card space-y-4 p-4">
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.model') }}</label>
          <select
            v-model="model"
            class="h-9 w-full rounded-lg border border-gray-300 bg-white px-2 text-sm dark:border-dark-600 dark:bg-dark-800"
          >
            <option v-for="m in models" :key="m" :value="m">{{ m }}</option>
          </select>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.size') }}</label>
          <div class="grid grid-cols-3 gap-1.5">
            <button
              v-for="preset in sizePresets"
              :key="preset.value"
              type="button"
              class="rounded-lg border px-1 py-1.5 text-center text-[11px] transition-colors"
              :class="size === preset.value
                ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-gray-400'"
              @click="size = preset.value"
            >
              <div class="font-medium">{{ preset.ratio }}</div>
              <div class="text-[10px] text-gray-400">{{ preset.value }}</div>
            </button>
          </div>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.quantity') }}</label>
          <div class="flex gap-1.5">
            <button
              v-for="q in [1, 2, 4]"
              :key="q"
              type="button"
              class="h-8 flex-1 rounded-lg border text-sm transition-colors"
              :class="quantity === q
                ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-400'"
              @click="quantity = q"
            >
              {{ q }}
            </button>
          </div>
        </div>

        <p class="text-xs text-gray-400">{{ t('imageStudio.feeNote') }}</p>
      </div>

      <!-- 右侧:提示词 + 结果 -->
      <div class="space-y-4">
        <div class="card p-4">
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.prompt') }}</label>
          <textarea
            v-model="prompt"
            rows="4"
            maxlength="4000"
            :placeholder="t('imageStudio.promptPlaceholder')"
            class="w-full rounded-lg border border-gray-300 bg-white p-3 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
          ></textarea>
          <div class="mt-3 flex items-center justify-between">
            <p v-if="errorMessage" class="text-sm text-red-500">{{ errorMessage }}</p>
            <span v-else class="text-xs text-gray-400">{{ t('imageStudio.feeNote2') }}</span>
            <button
              type="button"
              class="btn btn-primary min-w-32"
              :disabled="generating || !prompt.trim() || !selectedKey"
              @click="generate"
            >
              <span v-if="generating" class="mr-1 inline-block h-3 w-3 animate-spin rounded-full border-2 border-white/40 border-t-white"></span>
              {{ generating ? t('imageStudio.generating') : t('imageStudio.generate') }}
            </button>
          </div>
        </div>

        <!-- 结果网格 -->
        <div class="card min-h-64 p-4">
          <div v-if="generating" class="flex min-h-48 items-center justify-center text-sm text-gray-400">
            <span class="mr-2 inline-block h-4 w-4 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600"></span>
            {{ t('imageStudio.generatingWait') }}
          </div>
          <div v-else-if="images.length" class="grid grid-cols-2 gap-3 md:grid-cols-3">
            <div v-for="(img, idx) in images" :key="idx" class="group relative overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
              <img :src="img.src" class="aspect-square w-full object-cover" :alt="img.prompt.slice(0, 30)" />
              <div class="absolute inset-x-0 bottom-0 flex items-center justify-between bg-black/60 px-2 py-1 opacity-0 transition-opacity group-hover:opacity-100">
                <span class="text-[10px] text-white/80">{{ img.size }}</span>
                <a
                  :href="img.src"
                  :download="`image-${idx + 1}.png`"
                  class="text-[10px] text-white underline"
                  v-if="img.src.startsWith('data:')"
                >{{ t('imageStudio.download') }}</a>
                <a :href="img.src" target="_blank" class="text-[10px] text-white underline" v-else>{{ t('imageStudio.open') }}</a>
              </div>
            </div>
          </div>
          <div v-else class="flex min-h-48 items-center justify-center text-sm text-gray-400">
            {{ t('imageStudio.emptyResult') }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

interface KeyItem { id: number; name: string; key: string; status: string }
interface GeneratedImage { src: string; prompt: string; size: string }

const keys = ref<KeyItem[]>([])
const selectedKeyId = ref<number | null>(null)
const models = ref<string[]>([])
const model = ref('')
const size = ref('1024x1024')
const quantity = ref(1)
const prompt = ref('')
const generating = ref(false)
const errorMessage = ref('')
const images = ref<GeneratedImage[]>([])

const sizePresets = [
  { ratio: '1:1', value: '1024x1024' },
  { ratio: '3:2', value: '1536x1024' },
  { ratio: '2:3', value: '1024x1536' },
  { ratio: '16:9', value: '1792x1024' },
  { ratio: '9:16', value: '1024x1792' },
  { ratio: 'auto', value: 'auto' }
]

const selectedKey = computed(() => keys.value.find(k => k.id === selectedKeyId.value) || null)
const gatewayBase = computed(() => appStore.cachedPublicSettings?.api_base_url || window.location.origin)

const IMAGE_MODEL_PATTERN = /(image|dall|flux|seedream|banana| diffusion)/i
const FALLBACK_MODELS = ['gpt-image-2', 'gpt-image-1', 'dall-e-3']

function maskKey(key: string): string {
  if (!key || key.length < 12) return key
  return `${key.slice(0, 8)}...${key.slice(-6)}`
}

async function loadKeys() {
  try {
    const res = await keysAPI.list(1, 100, { status: 'active' })
    keys.value = (res.items || []).map(k => ({ id: k.id, name: k.name, key: k.key, status: k.status }))
    const saved = Number(localStorage.getItem('image_studio_key_id'))
    selectedKeyId.value = keys.value.some(k => k.id === saved)
      ? saved
      : (keys.value[0]?.id ?? null)
  } catch {
    keys.value = []
  }
}

async function loadModels() {
  if (!selectedKey.value) return
  try {
    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 15000)
    const res = await fetch(`${gatewayBase.value}/v1/models`, {
      headers: { Authorization: `Bearer ${selectedKey.value.key}` },
      signal: controller.signal
    })
    window.clearTimeout(timer)
    if (!res.ok) throw new Error(String(res.status))
    const j = await res.json()
    const ids: string[] = (j.data || []).map((m: { id: string }) => m.id)
    const imageModels = ids.filter(id => IMAGE_MODEL_PATTERN.test(id))
    models.value = imageModels.length ? imageModels : FALLBACK_MODELS
    if (!models.value.includes(model.value)) model.value = models.value[0] || FALLBACK_MODELS[0]
  } catch {
    models.value = FALLBACK_MODELS
    model.value = FALLBACK_MODELS[0]
  }
}

async function generate() {
  errorMessage.value = ''
  if (!selectedKey.value || !prompt.value.trim()) return
  generating.value = true
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 300000)
  try {
    const res = await fetch(`${gatewayBase.value}/v1/images/generations`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${selectedKey.value.key}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        model: model.value,
        prompt: prompt.value.trim(),
        size: size.value === 'auto' ? undefined : size.value,
        n: quantity.value
      }),
      signal: controller.signal
    })
    const body = await res.json().catch(() => null)
    if (!res.ok) {
      const msg = body?.error?.message || body?.message || t('imageStudio.generateFailed')
      throw new Error(msg)
    }
    const data: Array<{ b64_json?: string; url?: string }> = body?.data || []
    if (!data.length) throw new Error(t('imageStudio.generateFailed'))
    for (const item of data) {
      images.value.unshift({
        src: item.b64_json ? `data:image/png;base64,${item.b64_json}` : (item.url || ''),
        prompt: prompt.value.trim(),
        size: size.value
      })
    }
  } catch (err: unknown) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      errorMessage.value = t('imageStudio.timeout')
    } else if (err instanceof Error) {
      errorMessage.value = err.message
    } else {
      errorMessage.value = t('imageStudio.generateFailed')
    }
  } finally {
    window.clearTimeout(timer)
    generating.value = false
  }
}

watch(selectedKeyId, () => {
  localStorage.setItem('image_studio_key_id', String(selectedKeyId.value ?? ''))
  loadModels()
})

onMounted(async () => {
  await loadKeys()
  if (selectedKeyId.value !== null) await loadModels()
})
</script>
