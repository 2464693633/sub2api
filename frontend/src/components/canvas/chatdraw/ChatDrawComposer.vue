<template>
  <div
    class="relative mx-auto w-full max-w-3xl"
    @dragenter.prevent="dragOver = true"
    @dragover.prevent="dragOver = true"
    @dragleave="onDragLeave"
    @drop.prevent="onDrop"
  >
    <!-- 折叠参数设置 -->
    <div class="mb-2 flex items-center px-1">
      <div ref="settingsAnchor" class="relative min-w-0" @keydown.esc="onSettingsEscape">
        <button
          ref="settingsButton"
          type="button"
          class="inline-flex h-8 max-w-full items-center gap-1.5 rounded-full border bg-white/95 px-3 text-xs text-gray-700 shadow-sm backdrop-blur transition-colors hover:border-gray-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:bg-dark-800/95 dark:text-gray-200 dark:hover:border-dark-400"
          :class="settingsOpen ? 'border-gray-900 dark:border-gray-200' : 'border-gray-200 dark:border-dark-600'"
          :title="`${t('canvasChat.settings')}: ${settingsSummary}`"
          :aria-label="`${t('canvasChat.settings')}: ${settingsSummary}`"
          :aria-expanded="settingsOpen"
          @click="settingsOpen = !settingsOpen"
        >
          <Icon name="slidersHorizontal" size="sm" class="shrink-0" aria-hidden="true" />
          <span class="min-w-0 truncate font-medium">{{ settingsSummary }}</span>
          <Icon name="chevronDown" size="xs" class="shrink-0 transition-transform" :class="settingsOpen ? 'rotate-180' : ''" aria-hidden="true" />
        </button>
        <div
          v-if="settingsOpen"
          role="group"
          :aria-label="t('canvasChat.settings')"
          class="absolute bottom-[calc(100%+0.5rem)] left-0 z-40 w-[min(28rem,calc(100vw-3rem))] space-y-4 rounded-2xl border border-gray-200 bg-white p-3.5 shadow-xl dark:border-dark-600 dark:bg-dark-800"
        >
          <section>
            <h3 class="mb-2 text-xs font-bold text-gray-500 dark:text-gray-400">{{ t('canvasChat.imageSettings') }}</h3>
            <div class="flex flex-wrap items-center gap-2">
              <ChatDrawToolbarSelect :model-value="model" :options="modelOptions" :label="t('imageStudio.model')" :empty-text="t('imageStudio.noImageModels')" @change="setModel" />
              <ChatDrawToolbarSelect :model-value="ratio" :options="ratioOptions" :label="t('canvas.aspectRatio')" @change="setRatio" />
            </div>
            <div class="mt-3 space-y-3">
              <div>
                <div class="mb-1.5 text-[11px] font-semibold text-gray-500 dark:text-gray-400">{{ t('canvasChat.resolution') }}</div>
                <div class="grid grid-cols-3 gap-1.5">
                  <button
                    v-for="option in resolutionOptions"
                    :key="option.value"
                    type="button"
                    class="h-8 rounded-lg border px-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40"
                    :class="ratio === 'auto'
                      ? 'border-gray-200 bg-gray-100 text-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-500'
                      : resolution === option.value
                        ? 'border-gray-900 bg-gray-900 text-white dark:border-white dark:bg-white dark:text-gray-900'
                        : 'border-gray-200 bg-white text-gray-600 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-400'"
                    :disabled="ratio === 'auto'"
                    :aria-pressed="resolution === option.value"
                    @click="setResolution(option.value)"
                  >{{ option.label }}</button>
                </div>
              </div>
              <div>
                <div class="mb-1.5 text-[11px] font-semibold text-gray-500 dark:text-gray-400">{{ t('imageStudio.quality') }}</div>
                <div class="grid grid-cols-4 gap-1.5">
                  <button
                    v-for="option in qualityOptions"
                    :key="option.value"
                    type="button"
                    class="h-8 rounded-lg border px-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
                    :class="quality === option.value
                      ? 'border-gray-900 bg-gray-900 text-white dark:border-white dark:bg-white dark:text-gray-900'
                      : 'border-gray-200 bg-white text-gray-600 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-400'"
                    :aria-pressed="quality === option.value"
                    @click="setQuality(option.value)"
                  >{{ option.label }}</button>
                </div>
              </div>
              <div v-if="!singleOutput">
                <div class="mb-1.5 text-[11px] font-semibold text-gray-500 dark:text-gray-400">{{ t('canvasChat.count') }}</div>
                <div class="grid grid-cols-4 gap-1.5">
                  <button
                    v-for="n in CHAT_COUNTS"
                    :key="n"
                    type="button"
                    class="h-8 rounded-lg border px-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
                    :class="count === n
                      ? 'border-gray-900 bg-gray-900 text-white dark:border-white dark:bg-white dark:text-gray-900'
                      : 'border-gray-200 bg-white text-gray-600 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-400'"
                    :aria-pressed="count === n"
                    @click="setCount(n)"
                  >{{ t('canvasChat.countN', { n }) }}</button>
                </div>
                <label class="mt-2 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                  <span class="shrink-0">{{ t('canvasChat.customCount') }}</span>
                  <input
                    v-model="countDraft"
                    type="number"
                    min="1"
                    :max="MAX_CHAT_COUNT"
                    step="1"
                    inputmode="numeric"
                    class="h-8 min-w-0 flex-1 rounded-lg border border-gray-200 bg-white px-2 text-xs text-gray-700 outline-none focus:border-gray-500 focus:ring-2 focus:ring-primary-500/30 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
                    :aria-label="t('canvasChat.customCount')"
                    @blur="commitCount"
                    @keydown.enter.stop.prevent="commitCount"
                  />
                </label>
                <p class="mt-1 text-[10px] text-gray-400 dark:text-gray-500">{{ t('canvasChat.customCountHint', { max: MAX_CHAT_COUNT }) }}</p>
              </div>
              <div>
                <div class="mb-1.5 text-[11px] font-semibold text-gray-500 dark:text-gray-400">{{ t('canvasChat.outputFormat') }}</div>
                <div class="grid grid-cols-4 gap-1.5">
                  <button
                    v-for="option in formatOptions"
                    :key="option.value"
                    type="button"
                    class="h-8 rounded-lg border px-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
                    :class="format === option.value
                      ? 'border-gray-900 bg-gray-900 text-white dark:border-white dark:bg-white dark:text-gray-900'
                      : 'border-gray-200 bg-white text-gray-600 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-400'"
                    :aria-pressed="format === option.value"
                    @click="setFormat(option.value)"
                  >{{ option.label }}</button>
                </div>
              </div>
              <div v-if="format === 'gif'">
                <div class="mb-1.5 text-[11px] font-semibold text-gray-500 dark:text-gray-400">{{ t('canvasChat.gifParams') }}</div>
                <div class="flex items-center gap-2">
                  <label class="flex flex-1 items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span class="shrink-0">{{ t('canvasChat.gifFrames') }}</span>
                    <input
                      v-model="gifFramesDraft"
                      type="number"
                      :min="GIF_FRAMES_MIN"
                      :max="GIF_FRAMES_MAX"
                      step="1"
                      inputmode="numeric"
                      class="h-8 min-w-0 flex-1 rounded-lg border border-gray-200 bg-white px-2 text-xs text-gray-700 outline-none focus:border-gray-500 focus:ring-2 focus:ring-primary-500/30 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
                      :aria-label="t('canvasChat.gifFrames')"
                      @blur="commitGifFrames"
                      @keydown.enter.stop.prevent="commitGifFrames"
                    />
                  </label>
                  <label class="flex flex-1 items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span class="shrink-0">{{ t('canvasChat.gifFrameMs') }}</span>
                    <input
                      v-model="gifFrameMsDraft"
                      type="number"
                      :min="GIF_FRAME_MS_MIN"
                      :max="GIF_FRAME_MS_MAX"
                      step="10"
                      inputmode="numeric"
                      class="h-8 min-w-0 flex-1 rounded-lg border border-gray-200 bg-white px-2 text-xs text-gray-700 outline-none focus:border-gray-500 focus:ring-2 focus:ring-primary-500/30 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
                      :aria-label="t('canvasChat.gifFrameMs')"
                      @blur="commitGifFrameMs"
                      @keydown.enter.stop.prevent="commitGifFrameMs"
                    />
                  </label>
                </div>
                <p class="mt-1 text-[10px] text-gray-400 dark:text-gray-500">{{ t('canvasChat.gifHint') }}</p>
              </div>
              <p v-if="format === 'svg'" class="text-[10px] text-gray-400 dark:text-gray-500">
                {{ aiKeyId ? t('canvasChat.svgHint') : t('canvasChat.svgNeedKey') }}
              </p>
            </div>
          </section>
          <section class="border-t border-gray-100 pt-3 dark:border-dark-600">
            <h3 class="mb-2 text-xs font-bold text-gray-500 dark:text-gray-400">{{ t('canvasChat.polishSettings') }}</h3>
            <div class="flex flex-wrap items-center gap-2">
              <button
                type="button"
                role="switch"
                :aria-checked="polishEnabled"
                class="inline-flex h-8 items-center gap-1 rounded-full border px-3 text-xs shadow-sm backdrop-blur transition-colors"
                :class="polishEnabled
                  ? 'border-violet-300 bg-violet-50 text-violet-700 dark:border-violet-700 dark:bg-violet-900/30 dark:text-violet-200'
                  : 'border-gray-200 bg-white/95 text-gray-600 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800/95 dark:text-gray-300'"
                @click="togglePolish"
              ><Icon name="sparkles" size="xs" />{{ t('canvasChat.polish') }}</button>
              <template v-if="polishEnabled">
                <ChatDrawToolbarSelect :model-value="aiKeyId" :options="keyOptions" :label="t('canvasChat.polishKey')" :empty-text="t('canvas.aiNoKeys')" @change="setAIKey" />
                <ChatDrawToolbarSelect :model-value="aiModel" :options="aiModelOptions" :label="t('canvasChat.polishModel')" :disabled="!aiModels.length" :empty-text="t('canvas.aiNoModels')" @change="setAIModel" />
              </template>
            </div>
          </section>
        </div>
      </div>
    </div>

    <!-- 输入卡片 -->
    <form
      class="relative flex flex-col gap-2 rounded-[1.4rem] border border-gray-200/90 bg-white p-2.5 shadow-[0_10px_30px_-12px_rgba(15,23,42,0.18)] transition-colors dark:border-dark-600 dark:bg-dark-900 dark:shadow-[0_10px_28px_rgba(0,0,0,0.38)]"
      @submit.prevent="submit"
      @click="focus"
    >
      <!-- 参考图缩略条 -->
      <div v-if="draftRefs.length" class="flex items-center gap-2 overflow-x-auto px-0.5 pb-0.5 pt-0.5" @click.stop>
        <div
          v-for="(r, i) in draftRefs"
          :key="r.url"
          class="group relative h-16 w-16 shrink-0 overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 shadow-sm dark:border-dark-600 dark:bg-dark-800"
        >
          <button type="button" class="block h-full w-full cursor-zoom-in" :aria-label="t('canvasChat.refN', { n: i + 1 })" @click="emit('preview', r.url)">
            <img :src="r.url" class="h-full w-full object-cover" draggable="false" :alt="t('canvasChat.refN', { n: i + 1 })" />
          </button>
          <button
            type="button"
            class="absolute right-1 top-1 flex h-5 w-5 items-center justify-center rounded-full bg-slate-900/70 text-white shadow transition-opacity hover:bg-red-600 focus-visible:opacity-100 sm:opacity-0 sm:group-hover:opacity-100"
            :aria-label="t('canvasChat.removeRef')"
            :title="t('canvasChat.removeRef')"
            @click="removeDraftRef(i)"
          ><Icon name="x" size="xs" /></button>
        </div>
        <button
          v-if="draftRefs.length < MAX_REFS"
          type="button"
          class="flex h-16 w-16 shrink-0 flex-col items-center justify-center gap-0.5 rounded-2xl border border-dashed border-gray-300 text-gray-400 transition-colors hover:border-gray-500 hover:text-gray-700 dark:border-dark-500 dark:hover:border-dark-300 dark:hover:text-gray-200"
          :aria-label="t('canvasChat.addRef')"
          :title="t('canvasChat.addRef')"
          @click="pickFiles"
        ><Icon name="plus" size="sm" /><span class="text-[10px] tabular-nums">{{ draftRefs.length }}/{{ MAX_REFS }}</span></button>
      </div>

      <textarea
        ref="inputEl"
        v-model="draftText"
        rows="1"
        maxlength="4000"
        :aria-label="t('imageStudio.prompt')"
        :placeholder="draftRefs.length ? t('canvasChat.placeholderWithRefs') : t('canvasChat.placeholder')"
        class="max-h-[8.5rem] min-h-[2.75rem] w-full resize-none overflow-y-hidden bg-transparent px-1.5 py-1 text-[15px] leading-[1.5] text-gray-900 outline-none placeholder:text-gray-400 dark:text-gray-100 dark:placeholder:text-gray-500 max-sm:text-base"
        @keydown="onKeydown"
        @paste="onPaste"
        @input="autoGrow"
      ></textarea>

      <div class="flex items-end justify-between gap-2" @click.stop>
        <div class="flex min-w-0 items-center gap-2">
          <button
            type="button"
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-gray-200 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:border-dark-400 dark:hover:text-white"
            :disabled="draftRefs.length >= MAX_REFS"
            :aria-label="t('canvasChat.addRef')"
            :title="t('canvasChat.addRef')"
            @click="pickFiles"
          ><Icon name="plus" size="md" /></button>
          <span class="hidden truncate text-[11px] text-gray-400 sm:inline">{{ t('canvasChat.hint') }}</span>
        </div>
        <div class="flex shrink-0 items-center gap-2">
        <button
          v-if="polishEnabled && !running"
          type="button"
          class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full border transition-all"
          :class="canPolish
            ? 'border-amber-300/70 bg-amber-50 text-amber-600 hover:bg-amber-100 dark:border-amber-700/60 dark:bg-amber-900/30 dark:text-amber-300'
            : 'cursor-not-allowed border-gray-200 bg-gray-100 text-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-500'"
          :disabled="!canPolish"
          :aria-label="t('canvasChat.polishDraft')"
          :title="t('canvasChat.polishDraft')"
          @click="polishDraft"
        >
          <span v-if="polishing" class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"></span>
          <span v-else class="text-xs font-bold" aria-hidden="true">AI</span>
        </button>
        <button
          v-if="running"
          type="button"
          class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full border border-red-300/60 bg-red-50 text-red-600 transition-colors hover:bg-red-100 dark:border-red-800 dark:bg-red-900/30 dark:text-red-300"
          :aria-label="t('imageStudio.stopBatch')"
          :title="t('imageStudio.stopBatch')"
          @click="stopAll"
        ><span class="block h-3.5 w-3.5 rounded-[3px] bg-current"></span></button>
        <button
          v-else
          type="submit"
          class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full transition-all"
          :class="canSend
            ? 'bg-gray-900 text-white shadow-[0_10px_24px_-14px_rgba(15,23,42,0.9)] hover:-translate-y-px hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200'
            : 'cursor-not-allowed border border-gray-200 bg-gray-100 text-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-500'"
          :disabled="!canSend"
          :aria-label="t('canvasChat.send')"
          :title="t('canvasChat.send')"
        >
          <Icon name="arrowUp" size="md" />
        </button>
        </div>
      </div>

      <div
        v-if="dragOver"
        class="pointer-events-none absolute inset-0 z-10 flex items-center justify-center gap-2 rounded-[1.4rem] border-2 border-dashed border-gray-400 bg-white/90 text-sm font-semibold text-gray-700 backdrop-blur-sm dark:border-dark-400 dark:bg-dark-900/90 dark:text-gray-200"
      ><Icon name="upload" size="sm" />{{ t('canvasChat.dropHere') }}</div>
    </form>
    <input ref="fileEl" type="file" accept="image/png,image/jpeg,image/webp" multiple class="hidden" @change="onFiles" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ChatDrawToolbarSelect from './ChatDrawToolbarSelect.vue'
import { qualityLabelOf, type ToolbarOption, type ToolbarValue } from './format'
import { models } from '@/composables/useImageStudioEngine'
import {
  CHAT_RATIOS, CHAT_RESOLUTIONS, CHAT_COUNTS, QUALITIES, MAX_REFS, MAX_CHAT_COUNT,
  GIF_FRAMES_MIN, GIF_FRAMES_MAX, GIF_FRAME_MS_MIN, GIF_FRAME_MS_MAX,
  draftText, draftRefs, ratio, resolution, quality, count, format, gifFrames, gifFrameMs, model, polishEnabled, polishing, busy, turns, polishDraft,
  saveSettings, addDraftRefs, removeDraftRef, sendDraft, stopAll,
  clampCount, clampGifFrames, clampGifFrameMs, type ChatQuality, type ChatFormat, type ChatResolution
} from '@/composables/useChatDrawEngine'
import { aiKeys, aiKeyId, aiModels, aiModel, initAIOptions, onAIKeyChange, rememberAIModel } from '@/composables/useAIPrompt'

const emit = defineEmits<{ preview: [url: string] }>()
const { t } = useI18n()

const inputEl = ref<HTMLTextAreaElement | null>(null)
const fileEl = ref<HTMLInputElement | null>(null)
const dragOver = ref(false)
const settingsOpen = ref(false)
const settingsAnchor = ref<HTMLElement | null>(null)
const settingsButton = ref<HTMLButtonElement | null>(null)

const running = computed(() => turns.value.some(x => x.status === 'running'))
const canSend = computed(() => !busy.value && Boolean(draftText.value.trim()) && Boolean(model.value))
const canPolish = computed(() => !polishing.value && Boolean(draftText.value.trim()))

const modelOptions = computed<ToolbarOption[]>(() => {
  const seen = new Set<string>()
  const out: ToolbarOption[] = []
  for (const m of models.value) {
    if (seen.has(m.id)) continue
    seen.add(m.id)
    out.push({ value: m.id, label: m.id, hint: m.group_name || undefined })
  }
  return out
})
const ratioOptions = computed<ToolbarOption[]>(() => [
  { value: 'auto', label: t('canvasChat.ratioAuto') },
  ...CHAT_RATIOS.map(r => ({ value: r.value, label: r.value }))
])
const resolutionOptions = computed(() => CHAT_RESOLUTIONS.map(value => ({ value, label: t(`canvasChat.resolution${value.toUpperCase()}`) })))
const qualityOptions = computed<{ value: string; label: string }[]>(() => QUALITIES.map(q => ({ value: q, label: qualityLabelOf(t, q) })))
const formatOptions = computed(() => [
  { value: 'auto' as const, label: t('canvasChat.formatAuto') },
  { value: 'png' as const, label: 'PNG' },
  { value: 'jpeg' as const, label: 'JPG' },
  { value: 'webp' as const, label: 'WebP' },
  { value: 'gif' as const, label: 'GIF' },
  // SVG 由对话模型编写矢量代码,带参考图时不支持(参照 Studio 行为)
  ...(draftRefs.value.length === 0 ? [{ value: 'svg' as const, label: 'SVG' }] : [])
])
// 加参考图后 SVG 不再可用,自动回落
watch(() => draftRefs.value.length, n => {
  if (n > 0 && format.value === 'svg') {
    format.value = 'auto'
    saveSettings()
  }
})
const singleOutput = computed(() => format.value === 'gif' || format.value === 'svg')
const gifFramesDraft = ref(String(gifFrames.value))
const gifFrameMsDraft = ref(String(gifFrameMs.value))
watch(gifFrames, n => { gifFramesDraft.value = String(n) })
watch(gifFrameMs, n => { gifFrameMsDraft.value = String(n) })
function commitGifFrames() {
  gifFrames.value = clampGifFrames(gifFramesDraft.value)
  gifFramesDraft.value = String(gifFrames.value)
  saveSettings()
}
function commitGifFrameMs() {
  gifFrameMs.value = clampGifFrameMs(gifFrameMsDraft.value)
  gifFrameMsDraft.value = String(gifFrameMs.value)
  saveSettings()
}
const countDraft = ref(String(count.value))
watch(count, n => { countDraft.value = String(n) })
const keyOptions = computed<ToolbarOption[]>(() => aiKeys.value.map(k => ({ value: k.id, label: k.name })))
const aiModelOptions = computed<ToolbarOption[]>(() => aiModels.value.map(m => ({ value: m, label: m })))
const settingsSummary = computed(() => {
  const summary = t('canvasChat.settingsSummary', {
    model: model.value || t('imageStudio.noImageModels'),
    ratio: ratio.value,
    resolution: resolutionOptions.value.find(option => option.value === resolution.value)?.label,
    quality: quality.value,
    count: t('canvasChat.countN', { n: count.value })
  })
  const selectedFormat = formatOptions.value.find(option => option.value === format.value)
  return format.value === 'auto' ? summary : `${summary} · ${selectedFormat?.label}`
})

function onSettingsPointer(e: PointerEvent) {
  if (settingsAnchor.value && !settingsAnchor.value.contains(e.target as Node)) settingsOpen.value = false
}
function onSettingsEscape(e: KeyboardEvent) {
  // 子下拉先处理 Escape，再次按下时关闭设置面板。
  if (e.defaultPrevented || !settingsOpen.value) return
  e.preventDefault()
  settingsOpen.value = false
  settingsButton.value?.focus()
}
watch(settingsOpen, open => {
  if (open) document.addEventListener('pointerdown', onSettingsPointer)
  else document.removeEventListener('pointerdown', onSettingsPointer)
})
onBeforeUnmount(() => document.removeEventListener('pointerdown', onSettingsPointer))

// 工具条选择回写引擎设置
function setModel(v: ToolbarValue) { model.value = String(v); saveSettings() }
function setRatio(v: ToolbarValue) { ratio.value = String(v); saveSettings() }
function setResolution(v: ChatResolution) { resolution.value = v; saveSettings() }
function setQuality(v: ToolbarValue) { quality.value = v as ChatQuality; saveSettings() }
function setCount(v: ToolbarValue) {
  count.value = clampCount(v)
  countDraft.value = String(count.value)
  saveSettings()
}
function commitCount() { setCount(countDraft.value) }
function setFormat(v: ChatFormat) { format.value = v; saveSettings() }
function setAIKey(v: ToolbarValue) { aiKeyId.value = v === null ? null : Number(v); void onAIKeyChange() }
function setAIModel(v: ToolbarValue) { aiModel.value = String(v); rememberAIModel() }

function togglePolish() {
  polishEnabled.value = !polishEnabled.value
  saveSettings()
  if (polishEnabled.value) void initAIOptions()
}

function autoGrow() {
  requestAnimationFrame(() => {
    const el = inputEl.value
    if (!el) return
    el.style.height = 'auto'
    const max = parseFloat(getComputedStyle(el).maxHeight) || 136
    el.style.height = `${Math.min(el.scrollHeight, max)}px`
    el.style.overflowY = el.scrollHeight > max + 1 ? 'auto' : 'hidden'
  })
}
watch([draftText, () => draftRefs.value.length], () => void nextTick(autoGrow))

function focus() {
  inputEl.value?.focus()
}
function setText(text: string) {
  draftText.value = text
  focus()
}
defineExpose({ focus, setText })

async function submit() {
  if (!canSend.value) return
  await sendDraft()
}
function onKeydown(e: KeyboardEvent) {
  // Enter 发送,Shift+Enter 换行;输入法组字中不拦截
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    void submit()
  }
}
function pickFiles() {
  fileEl.value?.click()
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
function onDragLeave(e: DragEvent) {
  const to = e.relatedTarget as Node | null
  if (!to || !(e.currentTarget as HTMLElement).contains(to)) dragOver.value = false
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
</script>
