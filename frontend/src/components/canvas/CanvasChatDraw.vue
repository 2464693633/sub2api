<template>
  <div class="card flex min-h-[calc(100dvh-10rem)] flex-col overflow-hidden lg:min-h-0">
    <!-- 顶部紧凑工具条 -->
    <div class="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-gray-100 px-3 py-2 text-[11px] dark:border-dark-700">
      <span class="mr-1 text-sm font-semibold">{{ t('canvas.tabChat') }}</span>
      <select v-model="model" :title="t('imageStudio.model')" :aria-label="t('imageStudio.model')" class="max-w-44 rounded border border-gray-300 bg-white px-1.5 py-1 outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" @change="saveSettings">
        <option v-for="mo in models" :key="mo.id + ':' + mo.group_id" :value="mo.id">{{ mo.id }}{{ mo.group_name ? ' · ' + mo.group_name : '' }}</option>
      </select>
      <select v-model="ratio" :title="t('canvas.aspectRatio')" :aria-label="t('canvas.aspectRatio')" class="rounded border border-gray-300 bg-white px-1.5 py-1 outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" @change="saveSettings">
        <option v-for="r in CHAT_RATIOS" :key="r.value" :value="r.value">{{ r.value }}</option>
      </select>
      <select v-model="quality" :title="t('imageStudio.quality')" :aria-label="t('imageStudio.quality')" class="rounded border border-gray-300 bg-white px-1.5 py-1 outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" @change="saveSettings">
        <option v-for="q in QUALITIES" :key="q" :value="q">{{ qualityLabel(q) }}</option>
      </select>
      <select v-model.number="count" :title="t('canvasChat.count')" :aria-label="t('canvasChat.count')" class="rounded border border-gray-300 bg-white px-1.5 py-1 outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" @change="saveSettings">
        <option v-for="n in CHAT_COUNTS" :key="n" :value="n">{{ t('canvasChat.countN', { n }) }}</option>
      </select>
      <label class="flex cursor-pointer items-center gap-1 text-gray-500">
        <button
          type="button"
          role="switch"
          :aria-checked="polishEnabled"
          :aria-label="t('canvasChat.polish')"
          class="relative h-4 w-7 rounded-full transition-colors"
          :class="polishEnabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'"
          @click="togglePolish"
        >
          <span class="absolute top-0.5 h-3 w-3 rounded-full bg-white transition-all" :class="polishEnabled ? 'left-3.5' : 'left-0.5'"></span>
        </button>
        ✨ {{ t('canvasChat.polish') }}
      </label>
      <template v-if="polishEnabled">
        <select v-model="aiKeyId" :title="t('canvas.aiPickKey')" :aria-label="t('canvas.aiPickKey')" class="max-w-28 rounded border border-gray-300 bg-white px-1 py-1 outline-none dark:border-dark-600 dark:bg-dark-800" @change="onAIKeyChange">
          <option v-if="aiKeys.length === 0" :value="null">{{ t('canvas.aiNoKeys') }}</option>
          <option v-for="k in aiKeys" :key="k.id" :value="k.id">{{ k.name }}</option>
        </select>
        <select v-model="aiModel" :title="t('canvas.aiPickModel')" :aria-label="t('canvas.aiPickModel')" :disabled="aiModels.length === 0" class="max-w-36 rounded border border-gray-300 bg-white px-1 py-1 outline-none disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800" @change="rememberAIModel">
          <option v-if="aiModels.length === 0" value="">{{ t('canvas.aiNoModels') }}</option>
          <option v-for="m in aiModels" :key="m" :value="m">{{ m }}</option>
        </select>
      </template>
      <button
        v-if="turns.length"
        type="button"
        class="ml-auto rounded border border-gray-200 px-1.5 py-1 text-gray-500 hover:border-red-300 hover:text-red-500 dark:border-dark-600"
        @click="onClear"
      >{{ t('canvasChat.clear') }}</button>
    </div>
    <div v-if="initError" class="shrink-0 border-b border-amber-200 bg-amber-50 px-3 py-1.5 text-[11px] text-amber-700 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-300">
      {{ initError }}
      <button type="button" class="ml-1 underline" @click="initChatDraw">{{ t('imageStudio.retry') }}</button>
    </div>

    <!-- 消息流 -->
    <div ref="scroller" class="min-h-0 flex-1 space-y-4 overflow-y-auto px-3 py-4" aria-live="polite">
      <div v-if="!loaded" class="flex justify-center py-10">
        <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600" :aria-label="t('canvasChat.loading')"></span>
      </div>
      <div v-if="loaded && hasOlder" class="text-center text-[10px] text-gray-400">{{ t('canvasChat.olderHidden', { n: LOAD_LIMIT }) }}</div>
      <div v-if="loaded && turns.length === 0" class="mx-auto flex max-w-md flex-col items-center gap-3 py-10 text-center">
        <span class="text-4xl">💬</span>
        <div class="text-sm font-semibold">{{ t('canvasChat.emptyTitle') }}</div>
        <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('canvasChat.emptyDesc') }}</p>
        <div class="flex flex-wrap justify-center gap-1.5">
          <button
            v-for="ex in examples"
            :key="ex"
            type="button"
            class="rounded-full border border-gray-200 px-2.5 py-1 text-[11px] text-gray-600 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-300"
            @click="useExample(ex)"
          >{{ ex }}</button>
        </div>
      </div>

      <template v-for="turn in turns" :key="turn.id">
        <!-- 用户消息 -->
        <div class="flex justify-end">
          <div class="max-w-[85%] rounded-2xl rounded-br-sm bg-primary-600 px-3 py-2 text-sm text-white sm:max-w-[70%]">
            <div v-if="turn.origin" class="mb-1 text-[10px] opacity-80">{{ turn.origin === 'redraw' ? t('canvasChat.redrawTag') : t('canvasChat.variantTag') }}</div>
            <div class="whitespace-pre-wrap break-words">{{ turn.prompt }}</div>
            <div v-if="turn.refs.length" class="mt-2 flex flex-wrap gap-1">
              <img v-for="(r, i) in turn.refs" :key="i" :src="blobUrl(r)" class="h-10 w-10 rounded border border-white/40 object-cover" :alt="t('canvasChat.refN', { n: i + 1 })" />
            </div>
            <details v-if="turn.polished" class="mt-1.5 text-[11px] opacity-90">
              <summary class="cursor-pointer select-none">✨ {{ t('canvasChat.polishedPrompt') }}</summary>
              <div class="mt-1 whitespace-pre-wrap break-words">{{ turn.polished }}</div>
            </details>
          </div>
        </div>

        <!-- 助手消息 -->
        <div class="flex justify-start">
          <div class="w-full max-w-[95%] rounded-2xl rounded-bl-sm border border-gray-200 bg-white p-2.5 dark:border-dark-700 dark:bg-dark-800/60 sm:max-w-[80%]">
            <div class="mb-2 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-gray-400">
              <span class="font-medium text-gray-600 dark:text-gray-300">{{ turn.model }}</span>
              <span>{{ turn.ratio }}</span>
              <span>{{ qualityLabel(turn.quality) }}</span>
              <span v-if="turn.refs.length">{{ t('canvasChat.withRefs', { n: turn.refs.length }) }}</span>
              <span v-if="turn.status === 'running'" class="text-primary-500">{{ t('imageStudio.elapsed', { n: elapsed }) }}</span>
              <span v-else-if="turn.ms">{{ (turn.ms / 1000).toFixed(1) }}s</span>
              <span v-if="turn.status === 'cancelled'" class="text-amber-500">{{ t('canvasChat.stopped') }}</span>
              <button v-if="turn.status !== 'running'" type="button" class="ml-auto hover:text-red-500" :aria-label="t('canvasChat.deleteTurn')" :title="t('canvasChat.deleteTurn')" @click="deleteTurn(turn.id)">✕</button>
            </div>
            <div class="grid gap-2" :class="turn.images.length > 1 ? 'grid-cols-2' : 'max-w-sm grid-cols-1'">
              <div v-for="(img, idx) in turn.images" :key="img.id" class="overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900/40">
                <template v-if="img.status === 'done' && img.blob">
                  <button type="button" class="block w-full cursor-zoom-in" :aria-label="t('canvasChat.preview')" @click="viewer = blobUrl(img.blob)">
                    <img :src="blobUrl(img.blob)" class="w-full select-none object-contain" :style="{ aspectRatio: aspectOf(turn.ratio) }" :alt="turn.prompt.slice(0, 40)" />
                  </button>
                  <div class="flex flex-wrap items-center gap-1 border-t border-gray-200 px-1.5 py-1 text-[10px] dark:border-dark-700">
                    <span v-if="img.kind === 'variant'" class="rounded bg-violet-100 px-1 text-violet-600 dark:bg-violet-900/30 dark:text-violet-300">{{ t('canvasChat.variantTag') }}</span>
                    <span class="text-gray-400">{{ img.actualSize }}</span>
                    <span class="ml-auto flex flex-wrap gap-1">
                      <button type="button" :class="actionCls" :disabled="draftRefs.length >= MAX_REFS" @click="addImageAsRef(img)">{{ t('canvasChat.useAsRef') }}</button>
                      <button type="button" :class="actionCls" :disabled="busy" @click="redrawTurn(turn)">{{ t('canvasChat.redraw') }}</button>
                      <button type="button" :class="actionCls" :disabled="busy" @click="variantOf(turn)">{{ t('canvasChat.variant') }}</button>
                      <button type="button" :class="actionCls" @click="downloadImage(turn, img)">{{ t('imageStudio.download') }}</button>
                    </span>
                  </div>
                </template>
                <div v-else-if="img.status === 'failed'" class="flex flex-col items-center justify-center gap-1.5 bg-red-50 p-3 text-center dark:bg-red-900/20" :style="{ aspectRatio: aspectOf(turn.ratio), minHeight: '7rem' }">
                  <span class="text-[11px] font-medium text-red-500">{{ t('imageStudio.generateFailed') }} #{{ idx + 1 }}</span>
                  <span class="line-clamp-5 select-text break-all text-[10px] text-red-500/90">{{ img.error || t('imageStudio.generateFailed') }}</span>
                  <button type="button" :disabled="busy" class="text-[10px] text-primary-500 underline disabled:opacity-50" @click="retryImage(turn, img)">{{ t('imageStudio.retry') }}</button>
                </div>
                <div v-else class="flex flex-col items-center justify-center gap-2" :style="{ aspectRatio: aspectOf(turn.ratio), minHeight: '7rem' }">
                  <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600"></span>
                  <span class="text-[10px] text-gray-400">{{ t('imageStudio.generating') }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
      <div v-if="polishing" class="flex justify-start">
        <div class="rounded-2xl rounded-bl-sm border border-gray-200 bg-white px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800/60">
          <span class="mr-1 inline-block h-3 w-3 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600 align-middle"></span>
          {{ t('canvas.aiPrompting') }}
        </div>
      </div>
    </div>

    <!-- 输入区 -->
    <div
      class="shrink-0 border-t border-gray-100 p-2 dark:border-dark-700 sm:p-3"
      :class="dragOver ? 'bg-primary-50/60 dark:bg-primary-900/10' : ''"
      @dragover.prevent="dragOver = true"
      @dragleave="dragOver = false"
      @drop.prevent="onDrop"
    >
      <div v-if="draftRefs.length" class="mb-2 flex flex-wrap items-center gap-1.5">
        <div v-for="(r, i) in draftRefs" :key="r.url" class="group relative h-12 w-12 overflow-hidden rounded border border-gray-200 dark:border-dark-600">
          <img :src="r.url" class="h-full w-full object-cover" draggable="false" :alt="t('canvasChat.refN', { n: i + 1 })" />
          <button
            type="button"
            class="absolute right-0 top-0 flex h-4 w-4 items-center justify-center rounded-bl bg-black/60 text-[9px] text-white"
            :aria-label="t('canvasChat.removeRef')"
            @click="removeDraftRef(i)"
          >✕</button>
        </div>
        <span class="text-[10px] text-gray-400">{{ draftRefs.length }}/{{ MAX_REFS }}</span>
      </div>
      <div class="flex items-end gap-2">
        <label
          class="flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-lg border border-gray-200 text-gray-500 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600"
          :title="t('canvasChat.addRef')"
        >
          <span aria-hidden="true">🖼️</span>
          <span class="sr-only">{{ t('canvasChat.addRef') }}</span>
          <input type="file" accept="image/png,image/jpeg,image/webp" multiple class="hidden" @change="onFiles" />
        </label>
        <textarea
          ref="inputEl"
          v-model="draftText"
          rows="2"
          maxlength="4000"
          :aria-label="t('imageStudio.prompt')"
          :placeholder="t('canvasChat.placeholder')"
          class="max-h-40 min-h-[2.5rem] flex-1 resize-none rounded-lg border border-gray-300 bg-white px-2.5 py-2 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
          @keydown="onKeydown"
          @paste="onPaste"
          @input="autoGrow"
        ></textarea>
        <button
          v-if="running"
          type="button"
          class="h-9 shrink-0 rounded-lg border border-gray-300 px-3 text-sm text-gray-600 hover:border-red-300 hover:text-red-500 dark:border-dark-600 dark:text-gray-300"
          @click="stopAll"
        >{{ t('imageStudio.stopBatch') }}</button>
        <button
          v-else
          type="button"
          class="h-9 shrink-0 rounded-lg bg-primary-600 px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="busy || !draftText.trim() || !model"
          @click="onSend"
        >{{ polishing ? t('canvas.aiPrompting') : t('canvasChat.send') }}</button>
      </div>
      <div class="mt-1 hidden text-[10px] text-gray-400 sm:block">{{ t('canvasChat.hint') }}</div>
    </div>

    <!-- 灯箱 -->
    <div v-if="viewer" class="fixed inset-0 z-[100] flex items-center justify-center bg-black/85 p-6" role="dialog" aria-modal="true" @click="viewer = ''">
      <button type="button" class="absolute right-4 top-4 flex h-9 w-9 items-center justify-center rounded-full bg-white/10 text-lg text-white hover:bg-white/20" :aria-label="t('common.close')" @click="viewer = ''">✕</button>
      <img :src="viewer" class="max-h-[90vh] max-w-[92vw] rounded-lg object-contain" alt="" @click.stop />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { models } from '@/composables/useImageStudioEngine'
import {
  CHAT_RATIOS, CHAT_COUNTS, QUALITIES, MAX_REFS, LOAD_LIMIT,
  turns, draftText, draftRefs, ratio, quality, count, model, polishEnabled, polishing,
  elapsed, loaded, hasOlder, busy, initError,
  initChatDraw, saveSettings, addDraftRefs, removeDraftRef, addImageAsRef,
  sendDraft, redrawTurn, variantOf, retryImage, stopAll, deleteTurn, clearConversation,
  downloadImage, blobUrl,
  type ChatQuality
} from '@/composables/useChatDrawEngine'
import {
  aiKeys, aiKeyId, aiModels, aiModel,
  initAIOptions, onAIKeyChange, rememberAIModel
} from '@/composables/useAIPrompt'

const { t } = useI18n()

const scroller = ref<HTMLElement | null>(null)
const inputEl = ref<HTMLTextAreaElement | null>(null)
const viewer = ref('')
const dragOver = ref(false)
const actionCls = 'rounded border border-gray-200 px-1.5 py-0.5 text-gray-600 hover:border-primary-400 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:text-gray-300'

const running = computed(() => turns.value.some(x => x.status === 'running'))
const examples = computed(() => [t('canvasChat.example1'), t('canvasChat.example2'), t('canvasChat.example3')])

function qualityLabel(q: ChatQuality): string {
  return q === 'auto' ? t('canvas.qualityAuto') : q === 'high' ? t('canvas.qualityHigh') : q === 'medium' ? t('canvas.qualityMedium') : t('canvas.qualityLow')
}
function aspectOf(r: string): string {
  const [w, h] = r.split(':').map(Number)
  return w && h ? `${w} / ${h}` : '1 / 1'
}

function togglePolish() {
  polishEnabled.value = !polishEnabled.value
  saveSettings()
  if (polishEnabled.value) void initAIOptions()
}

function scrollToBottom() {
  void nextTick(() => {
    const el = scroller.value
    if (el) el.scrollTop = el.scrollHeight
  })
}
// 新轮次或新图片完成时滚到底部
watch(() => [turns.value.length, turns.value.at(-1)?.images.map(i => i.status).join(',') ?? '', polishing.value], scrollToBottom)

function autoGrow() {
  const el = inputEl.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 160)}px`
}
async function onSend() {
  const pending = sendDraft()
  void nextTick(autoGrow)
  await pending
}
function onKeydown(e: KeyboardEvent) {
  // Enter 发送,Shift+Enter 换行;输入法组字中不拦截
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    if (!busy.value && draftText.value.trim()) void onSend()
  }
}
function useExample(text: string) {
  draftText.value = text
  inputEl.value?.focus()
  void nextTick(autoGrow)
}

function onFiles(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) void addDraftRefs(Array.from(input.files))
  input.value = ''
}
function onPaste(e: ClipboardEvent) {
  const files = Array.from(e.clipboardData?.files || []).filter(f => f.type.startsWith('image/'))
  if (!files.length) return
  e.preventDefault()
  void addDraftRefs(files)
}
// 支持本机文件,以及页面内拖动结果图(原生 img 拖拽只带 blob: 地址)
async function onDrop(e: DragEvent) {
  dragOver.value = false
  const dt = e.dataTransfer
  if (!dt) return
  if (dt.files?.length) {
    void addDraftRefs(Array.from(dt.files))
    return
  }
  const uri = dt.getData('text/uri-list') || dt.getData('text/plain') || ''
  const url = uri.split('\n').map(s => s.trim()).find(u => u.startsWith('blob:') || u.startsWith(window.location.origin))
  if (!url) return
  try {
    const blob = await (await fetch(url)).blob()
    if (blob.type.startsWith('image/')) void addDraftRefs([blob])
  } catch { /* 无效拖拽源,忽略 */ }
}

async function onClear() {
  if (!window.confirm(t('canvasChat.clearConfirm'))) return
  await clearConversation()
}

function onWindowKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && viewer.value) viewer.value = ''
}
onMounted(async () => {
  window.addEventListener('keydown', onWindowKeydown)
  await initChatDraw()
  if (polishEnabled.value) void initAIOptions()
  scrollToBottom()
})
onUnmounted(() => {
  window.removeEventListener('keydown', onWindowKeydown)
})
</script>
