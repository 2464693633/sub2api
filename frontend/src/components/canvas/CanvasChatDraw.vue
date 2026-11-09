<template>
  <div class="flex min-h-[calc(100dvh-10rem)] gap-3 lg:min-h-0">
    <!-- 左侧历史栏(桌面,可折叠) -->
    <aside
      v-if="!collapsed"
      class="hidden w-[15.5rem] shrink-0 flex-col overflow-hidden rounded-[1.25rem] border border-gray-200 bg-white/90 p-3 shadow-[0_16px_44px_-36px_rgba(15,23,42,0.45)] dark:border-dark-700 dark:bg-dark-900/80 lg:flex"
      :aria-label="t('canvasChat.history')"
    >
      <ChatDrawHistory
        v-bind="historyProps"
        closable
        close-icon="chevronLeft"
        :close-label="t('canvasChat.collapseHistory')"
        @close="setCollapsed(true)"
        v-on="historyHandlers"
      />
    </aside>

    <!-- 移动端历史抽屉 -->
    <Teleport to="body">
      <Transition enter-from-class="opacity-0" leave-to-class="opacity-0" enter-active-class="transition-opacity duration-200" leave-active-class="transition-opacity duration-150">
        <div v-if="drawerOpen" class="fixed inset-0 z-[260] bg-slate-900/40 p-3 lg:hidden" @click.self="drawerOpen = false">
          <div class="flex h-full max-h-[calc(100dvh-1.5rem)] w-[min(22rem,88vw)] flex-col rounded-2xl border border-gray-200 bg-white p-3 shadow-2xl dark:border-dark-700 dark:bg-dark-900" role="dialog" aria-modal="true" :aria-label="t('canvasChat.history')">
            <ChatDrawHistory
              v-bind="historyProps"
              closable
              close-icon="x"
              :close-label="t('common.close')"
              @close="drawerOpen = false"
              v-on="drawerHandlers"
            />
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 主列 -->
    <main class="relative flex min-w-0 flex-1 flex-col overflow-hidden rounded-[1.25rem] border border-gray-200 bg-white/90 shadow-[0_16px_44px_-36px_rgba(15,23,42,0.45)] dark:border-dark-700 dark:bg-dark-900/80">
      <!-- 顶栏 -->
      <header class="flex min-h-14 shrink-0 items-center justify-between gap-3 border-b border-gray-200 px-3.5 py-2.5 dark:border-dark-700">
        <div class="flex min-w-0 items-center gap-2">
          <button type="button" :class="iconBtn" class="lg:hidden" :aria-label="t('canvasChat.openHistory')" :title="t('canvasChat.openHistory')" @click="drawerOpen = true"><Icon name="menu" size="sm" /></button>
          <button v-if="collapsed" type="button" :class="iconBtn" class="hidden lg:flex" :aria-label="t('canvasChat.expandHistory')" :title="t('canvasChat.expandHistory')" @click="setCollapsed(false)"><Icon name="chevronRight" size="sm" /></button>
          <div class="min-w-0">
            <div class="max-w-[min(32rem,48vw)] truncate text-sm font-bold text-gray-900 dark:text-white">{{ activeSession?.title || t('canvasChat.untitled') }}</div>
            <div class="mt-0.5 text-xs leading-4 text-gray-400">{{ subtitle }}</div>
          </div>
        </div>
        <div class="flex shrink-0 items-center gap-1.5">
          <button type="button" :class="pillBtn" :aria-label="t('canvasChat.newChat')" @click="onNewChat"><Icon name="plus" size="xs" /><span class="hidden sm:inline">{{ t('canvasChat.newChat') }}</span></button>
          <button type="button" :class="pillBtn" :disabled="!turns.length" :aria-label="t('canvasChat.clear')" @click="onClear"><Icon name="trash" size="xs" /><span class="hidden sm:inline">{{ t('canvasChat.clear') }}</span></button>
        </div>
      </header>
      <div v-if="initError" class="shrink-0 border-b border-amber-200 bg-amber-50 px-4 py-1.5 text-[11px] text-amber-700 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-300">
        {{ initError }}
        <button type="button" class="ml-1 underline" @click="initChatDraw">{{ t('imageStudio.retry') }}</button>
      </div>

      <!-- 消息流 -->
      <div ref="scroller" class="relative min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-6 pt-4 sm:px-6" aria-live="polite" @scroll="onScroll">
        <div class="mx-auto flex w-full max-w-[52rem] flex-col gap-5">
          <div v-if="!loaded" class="flex justify-center py-16">
            <span class="inline-block h-6 w-6 animate-spin rounded-full border-2 border-gray-200 border-t-gray-600 dark:border-dark-600 dark:border-t-gray-300" :aria-label="t('canvasChat.loading')"></span>
          </div>
          <div v-else-if="hasOlder" class="flex justify-center">
            <span class="rounded-full border border-gray-200 bg-white px-3.5 py-1 text-xs font-semibold text-gray-400 dark:border-dark-600 dark:bg-dark-800">{{ t('canvasChat.olderHidden', { n: LOAD_LIMIT }) }}</span>
          </div>

          <!-- 空态 -->
          <div v-if="loaded && !turns.length && !polishing" class="flex min-h-[50vh] flex-col items-center justify-center px-4 py-10 text-center">
            <h1 class="text-[clamp(1.8rem,5vw,3rem)] font-bold text-gray-900 dark:text-white">{{ t('canvasChat.emptyTitle') }}</h1>
            <p class="mt-3.5 max-w-lg text-sm leading-[1.8] text-gray-500 dark:text-gray-400">{{ t('canvasChat.emptyDesc') }}</p>
            <div class="mt-6 flex flex-wrap justify-center gap-2">
              <button
                v-for="ex in examples"
                :key="ex"
                type="button"
                class="rounded-full border border-gray-200 bg-white px-3.5 py-1.5 text-xs text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-900 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-400 dark:hover:text-white"
                @click="composer?.setText(ex)"
              >{{ ex }}</button>
            </div>
          </div>

          <template v-for="(turn, ti) in turns" :key="turn.id">
            <!-- 日期分隔 -->
            <div v-if="ti === 0 || dayKey(turns[ti - 1].ts) !== dayKey(turn.ts)" class="flex items-center gap-3 text-[11px] text-gray-400" role="separator">
              <span class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></span>
              <span>{{ dayLabel(t, locale, turn.ts) }}</span>
              <span class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></span>
            </div>

            <!-- 用户消息 -->
            <div class="group/u flex flex-col items-end">
              <div class="mb-1 flex min-h-7 flex-row-reverse items-center gap-1.5">
                <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-emerald-500 text-white dark:bg-emerald-600" aria-hidden="true"><Icon name="user" size="xs" /></span>
                <div :class="msgActions">
                  <button type="button" :class="msgActBtn" :aria-label="t('canvasChat.copyPrompt')" :title="t('canvasChat.copyPrompt')" @click="copyPrompt(turn)"><Icon name="copy" size="xs" /></button>
                  <button type="button" :class="msgActBtn" :aria-label="t('canvasChat.fillPrompt')" :title="t('canvasChat.fillPrompt')" @click="composer?.setText(turn.prompt)"><Icon name="edit" size="xs" /></button>
                </div>
                <span v-if="turn.origin" class="rounded-full bg-gray-100 px-2 py-px text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ turn.origin === 'redraw' ? t('canvasChat.redrawTag') : t('canvasChat.variantTag') }}</span>
              </div>
              <div class="max-w-[min(100%,38rem)] rounded-[18px] rounded-tr-md bg-emerald-500 px-3.5 py-2.5 text-sm leading-[1.75] text-white shadow-[0_8px_24px_-12px_rgba(16,185,129,0.55)] dark:bg-emerald-600">
                <p class="m-0 whitespace-pre-wrap [overflow-wrap:anywhere]">{{ turn.prompt }}</p>
                <details v-if="turn.polished" class="mt-1.5 text-xs opacity-90">
                  <summary class="cursor-pointer select-none">✨ {{ t('canvasChat.polishedPrompt') }}</summary>
                  <div class="mt-1 whitespace-pre-wrap [overflow-wrap:anywhere]">{{ turn.polished }}</div>
                </details>
              </div>
              <div v-if="turn.refs.length" class="ml-auto mt-2 flex max-w-[min(100%,20rem)] flex-wrap justify-end gap-2">
                <button
                  v-for="(r, i) in turn.refs"
                  :key="i"
                  type="button"
                  class="h-20 w-20 cursor-zoom-in overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm transition hover:-translate-y-px hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 sm:h-[5.75rem] sm:w-[5.75rem]"
                  :aria-label="t('canvasChat.refN', { n: i + 1 })"
                  @click="previewSingle(blobUrl(r), t('canvasChat.refN', { n: i + 1 }))"
                ><img :src="blobUrl(r)" class="h-full w-full object-cover" :alt="t('canvasChat.refN', { n: i + 1 })" /></button>
              </div>
            </div>

            <!-- AI 回复 -->
            <div class="group/a flex flex-col items-start">
              <div class="mb-1 flex min-h-7 items-center gap-1.5">
                <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300" aria-hidden="true"><Icon name="sparkles" size="xs" /></span>
                <span v-if="turn.status === 'running'" class="text-[11px] text-gray-400">{{ t('canvasChat.generatingN', { done: doneCount(turn), total: turn.images.length }) }} · {{ t('imageStudio.elapsed', { n: elapsedOf(turn) }) }}</span>
                <span v-else-if="turn.status === 'cancelled'" class="text-[11px] text-amber-500">{{ t('canvasChat.stopped') }}</span>
                <span v-else-if="turn.ms" class="text-[11px] text-gray-400">{{ t('canvasChat.tookSeconds', { n: (turn.ms / 1000).toFixed(1) }) }}</span>
                <div v-if="turn.status !== 'running'" :class="msgActions">
                  <button type="button" :class="msgActBtn" :disabled="busy" :aria-label="t('canvasChat.redraw')" :title="t('canvasChat.redraw')" @click="redrawTurn(turn)"><Icon name="refresh" size="xs" /></button>
                  <button type="button" :class="msgActBtn" :disabled="busy" :aria-label="t('canvasChat.variant')" :title="t('canvasChat.variant')" @click="variantOf(turn)"><Icon name="plus" size="xs" /></button>
                  <button type="button" :class="[msgActBtn, 'hover:!border-red-300 hover:!text-red-500']" :aria-label="t('canvasChat.deleteTurn')" :title="t('canvasChat.deleteTurn')" @click="deleteTurn(turn.id)"><Icon name="trash" size="xs" /></button>
                </div>
              </div>

              <div
                class="grid gap-2.5"
                :class="turn.images.length > 1 ? 'w-[min(100%,36rem)] grid-cols-1 min-[480px]:grid-cols-2' : ''"
                :style="turn.images.length > 1 ? undefined : { width: singleImageWidth(turn.ratio) }"
              >
                <figure v-for="(img, idx) in turn.images" :key="img.id" class="m-0 min-w-0">
                  <!-- 完成 -->
                  <div v-if="img.status === 'done' && img.blob" class="group/img relative overflow-hidden rounded-xl border border-gray-200/80 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                    <button type="button" class="block w-full cursor-zoom-in" :aria-label="t('canvasChat.preview')" @click="openGallery(turn, img)">
                      <img :src="blobUrl(img.blob)" class="block h-auto w-full select-none object-contain" :alt="turn.prompt.slice(0, 60)" loading="lazy" />
                    </button>
                    <span v-if="img.kind === 'variant'" class="absolute left-2 top-2 rounded-full bg-violet-600/85 px-2 py-px text-[10px] font-medium text-white">{{ t('canvasChat.variantTag') }}</span>
                    <!-- 悬浮操作 -->
                    <div class="absolute inset-x-0 bottom-0 flex items-end justify-end gap-1.5 bg-gradient-to-t from-black/55 to-transparent p-2 transition-opacity sm:pointer-events-none sm:opacity-0 sm:group-hover/img:pointer-events-auto sm:group-hover/img:opacity-100 sm:group-focus-within/img:pointer-events-auto sm:group-focus-within/img:opacity-100">
                      <button type="button" :class="overlayBtn" :disabled="draftRefs.length >= MAX_REFS" :aria-label="t('canvasChat.useAsRef')" :title="t('canvasChat.useAsRef')" @click="addImageAsRef(img)"><Icon name="link" size="xs" /></button>
                      <button type="button" :class="overlayBtn" :disabled="busy" :aria-label="t('canvasChat.redraw')" :title="t('canvasChat.redraw')" @click="redrawTurn(turn)"><Icon name="refresh" size="xs" /></button>
                      <button type="button" :class="overlayBtn" :aria-label="t('imageStudio.download')" :title="t('imageStudio.download')" @click="downloadImage(turn, img)"><Icon name="download" size="xs" /></button>
                    </div>
                  </div>
                  <!-- 失败 -->
                  <div
                    v-else-if="img.status === 'failed'"
                    class="flex flex-col items-center justify-center gap-1.5 rounded-xl border border-dashed border-red-300 bg-red-50/60 p-3 text-center dark:border-red-800/70 dark:bg-red-900/15"
                    :style="{ aspectRatio: aspectOf(turn.ratio), minHeight: '8rem' }"
                  >
                    <Icon name="xCircle" size="md" class="text-red-400" />
                    <span class="text-xs font-medium text-red-500 dark:text-red-300">{{ t('canvasChat.imageFailed') }}</span>
                    <span class="line-clamp-4 max-w-full select-text break-all text-[11px] text-red-500/80 dark:text-red-300/80">{{ img.error || t('imageStudio.generateFailed') }}</span>
                    <button type="button" :disabled="busy" class="mt-0.5 rounded-full border border-red-200 bg-white px-2.5 py-0.5 text-[11px] text-red-500 hover:border-red-400 disabled:opacity-50 dark:border-red-800 dark:bg-dark-900" @click="retryImage(turn, img)">{{ t('imageStudio.retry') }}</button>
                  </div>
                  <!-- 生成中:骨架屏 -->
                  <div
                    v-else
                    class="chat-skeleton relative flex flex-col items-center justify-center gap-2.5 overflow-hidden rounded-xl border border-gray-200/80 dark:border-dark-700"
                    :style="{ aspectRatio: aspectOf(turn.ratio), minHeight: '8rem' }"
                  >
                    <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-gray-300 border-t-gray-600 dark:border-dark-500 dark:border-t-gray-300"></span>
                    <span class="text-[13px] text-gray-500 dark:text-gray-400">{{ t('imageStudio.generating') }}</span>
                    <small class="text-[11px] text-gray-400">{{ turn.images.length > 1 ? t('canvasChat.imageN', { n: idx + 1 }) : t('imageStudio.elapsed', { n: elapsedOf(turn) }) }}</small>
                  </div>
                  <!-- 参数小字 -->
                  <figcaption class="mt-1 truncate px-0.5 text-[11px] text-gray-400" :title="caption(turn, img)">{{ caption(turn, img) }}</figcaption>
                </figure>
              </div>
            </div>
          </template>

          <div v-if="polishing" class="flex items-center gap-2 text-xs text-gray-500">
            <span class="flex h-7 w-7 items-center justify-center rounded-full border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800"><span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-gray-300 border-t-gray-600"></span></span>
            {{ t('canvas.aiPrompting') }}
          </div>
        </div>
      </div>

      <!-- 回到最新 -->
      <button
        v-if="!atBottom && turns.length"
        type="button"
        class="absolute left-1/2 z-30 flex h-10 w-10 -translate-x-1/2 items-center justify-center rounded-full border border-gray-200 bg-white/95 text-gray-700 shadow-lg backdrop-blur hover:text-gray-900 dark:border-dark-600 dark:bg-dark-800/95 dark:text-gray-200"
        :style="{ bottom: `${composerHeight + 12}px` }"
        :aria-label="t('canvasChat.scrollLatest')"
        :title="t('canvasChat.scrollLatest')"
        @click="scrollToBottom(true)"
      ><Icon name="arrowDown" size="sm" /></button>

      <!-- 输入区 -->
      <div ref="composerWrap" class="relative shrink-0 px-3 pb-3 pt-1 sm:px-4">
        <div class="pointer-events-none absolute inset-x-0 -top-6 h-6 bg-gradient-to-t from-white/90 to-transparent dark:from-dark-900/80" aria-hidden="true"></div>
        <ChatDrawComposer ref="composer" @preview="url => previewSingle(url, t('canvasChat.refPreview'))" />
      </div>
    </main>

    <ChatDrawLightbox v-model:index="lightIndex" :items="lightItems" @close="lightIndex = -1" @download="onLightDownload" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ChatDrawHistory from './chatdraw/ChatDrawHistory.vue'
import ChatDrawComposer from './chatdraw/ChatDrawComposer.vue'
import ChatDrawLightbox from './chatdraw/ChatDrawLightbox.vue'
import { aspectOf, dayKey, dayLabel, qualityLabelOf, singleImageWidth, type LightboxItem } from './chatdraw/format'
import { useAppStore } from '@/stores/app'
import {
  MAX_REFS, LOAD_LIMIT,
  turns, draftRefs, polishEnabled, polishing, loaded, hasOlder, busy, initError,
  sessions, activeSessionId, activeSession, sessionNotices, runningSessionIds, totalImages,
  initChatDraw, newSession, selectSession, renameSession, deleteSession, clearAllSessions,
  addImageAsRef, redrawTurn, variantOf, retryImage, deleteTurn, clearConversation,
  downloadImage, blobUrl, elapsedOf,
  type ChatImage, type ChatTurn
} from '@/composables/useChatDrawEngine'
import { initAIOptions } from '@/composables/useAIPrompt'

const { t, locale } = useI18n()
const appStore = useAppStore()

const iconBtn = 'flex h-8 w-8 items-center justify-center rounded-xl text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white'
const pillBtn = 'inline-flex h-8 items-center gap-1 rounded-xl border border-gray-200 px-2.5 text-xs font-medium text-gray-700 transition-colors hover:border-gray-400 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:text-gray-200 dark:hover:border-dark-400'
const msgActions = 'flex items-center gap-1 transition-opacity sm:pointer-events-none sm:opacity-0 sm:group-hover/u:pointer-events-auto sm:group-hover/u:opacity-100 sm:group-hover/a:pointer-events-auto sm:group-hover/a:opacity-100 sm:focus-within:pointer-events-auto sm:focus-within:opacity-100'
const msgActBtn = 'flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-500 transition-colors hover:border-gray-400 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:text-white'
const overlayBtn = 'flex h-8 w-8 items-center justify-center rounded-full bg-white/90 text-gray-800 shadow transition-colors hover:bg-white disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-800/90 dark:text-gray-100 dark:hover:bg-dark-700'

const composer = ref<InstanceType<typeof ChatDrawComposer> | null>(null)
const scroller = ref<HTMLElement | null>(null)
const composerWrap = ref<HTMLElement | null>(null)
const composerHeight = ref(160)
const atBottom = ref(true)
const drawerOpen = ref(false)

// ===== 历史栏 =====
const LS_COLLAPSED = 'chat_draw_history_collapsed'
const collapsed = ref(localStorage.getItem(LS_COLLAPSED) === '1')
function setCollapsed(v: boolean) {
  collapsed.value = v
  localStorage.setItem(LS_COLLAPSED, v ? '1' : '0')
}
const historyProps = computed(() => ({
  sessions: sessions.value,
  activeId: activeSessionId.value,
  running: runningSessionIds.value,
  notices: sessionNotices.value,
  totalImages: totalImages.value
}))
async function onSelect(id: string) {
  await selectSession(id)
  scrollToBottom(true)
}
function onNewChat() {
  newSession()
  composer.value?.focus()
}
async function onDeleteSession(id: string) {
  const s = sessions.value.find(x => x.id === id)
  if (s?.turnCount && !window.confirm(t('canvasChat.deleteSessionConfirm', { title: s.title || t('canvasChat.untitled') }))) return
  await deleteSession(id)
  single.value = null
  lightIndex.value = -1
}
async function onClearAll() {
  if (!window.confirm(t('canvasChat.clearAllConfirm'))) return
  await clearAllSessions()
  single.value = null
  lightIndex.value = -1
}
const historyHandlers = {
  create: onNewChat,
  select: onSelect,
  rename: renameSession,
  delete: onDeleteSession,
  clear: onClearAll
}
// 抽屉内选择/新建后自动收起
const drawerHandlers = {
  ...historyHandlers,
  create: () => { drawerOpen.value = false; onNewChat() },
  select: (id: string) => { drawerOpen.value = false; void onSelect(id) }
}

// ===== 顶栏 =====
const examples = computed(() => [t('canvasChat.example1'), t('canvasChat.example2'), t('canvasChat.example3')])
const subtitle = computed(() => {
  if (!loaded.value) return t('canvasChat.loading')
  if (polishing.value) return t('canvas.aiPrompting')
  const run = turns.value.find(x => x.status === 'running')
  if (run) return t('canvasChat.statusRunning', { n: elapsedOf(run) })
  return turns.value.length ? t('canvasChat.statusTurns', { n: activeSession.value?.turnCount ?? turns.value.length }) : t('canvasChat.statusReady')
})
async function onClear() {
  if (!window.confirm(t('canvasChat.clearConfirm'))) return
  await clearConversation()
}

// ===== 消息 =====
function doneCount(turn: ChatTurn): number {
  return turn.images.filter(i => i.status === 'done').length
}
/** 参数小字:模型 · 清晰度 · 尺寸 */
function caption(turn: ChatTurn, img: ChatImage): string {
  const size = img.actualSize || (turn.size !== 'auto' ? turn.size : turn.ratio)
  return [turn.model, qualityLabelOf(t, turn.quality), size].filter(Boolean).join(' · ')
}
async function copyPrompt(turn: ChatTurn) {
  try {
    await navigator.clipboard.writeText(turn.polished || turn.prompt)
    appStore.showSuccess(t('canvasChat.copied'))
  } catch {
    appStore.showError(t('canvasChat.copyFailed'))
  }
}

// ===== 灯箱 =====
interface GalleryEntry { turn: ChatTurn; img: ChatImage }
const lightIndex = ref(-1)
const single = ref<LightboxItem | null>(null)
const gallery = computed<GalleryEntry[]>(() =>
  turns.value.flatMap(turn => turn.images.filter(img => img.status === 'done' && img.blob).map(img => ({ turn, img })))
)
const lightItems = computed<LightboxItem[]>(() => single.value
  ? [single.value]
  : gallery.value.map(g => ({ url: blobUrl(g.img.blob), caption: `${g.turn.prompt.slice(0, 80)} — ${caption(g.turn, g.img)}`, downloadable: true }))
)
function openGallery(turn: ChatTurn, img: ChatImage) {
  single.value = null
  lightIndex.value = gallery.value.findIndex(g => g.img.id === img.id && g.turn.id === turn.id)
}
function previewSingle(url: string, cap: string) {
  single.value = { url, caption: cap }
  lightIndex.value = 0
}
function onLightDownload(i: number) {
  const g = gallery.value[i]
  if (g && !single.value) downloadImage(g.turn, g.img)
}
watch(lightIndex, v => { if (v < 0) single.value = null })
// 会话切换时关闭灯箱,避免指向别的会话的图片
watch(activeSessionId, () => { lightIndex.value = -1 })

// ===== 滚动 =====
const STICK_PX = 160
function onScroll() {
  const el = scroller.value
  if (!el) return
  atBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_PX
}
function scrollToBottom(force = false) {
  if (!force && !atBottom.value) return
  void nextTick(() => {
    const el = scroller.value
    if (!el) return
    el.scrollTop = el.scrollHeight
    atBottom.value = true
  })
}
// 新轮次/新图完成/润色开始时,若用户停在底部则跟随
watch(
  () => [turns.value.length, turns.value.at(-1)?.images.map(i => i.status).join(',') ?? '', polishing.value],
  () => scrollToBottom()
)
// 自己发出的新一轮总是滚到底
watch(() => turns.value.length, (n, o) => { if (n > o) scrollToBottom(true) })
watch(activeSessionId, () => scrollToBottom(true))
watch(loaded, v => { if (v) scrollToBottom(true) })

let ro: ResizeObserver | null = null
onMounted(async () => {
  if (typeof ResizeObserver !== 'undefined') {
    ro = new ResizeObserver(() => { composerHeight.value = composerWrap.value?.offsetHeight ?? 160 })
    if (composerWrap.value) ro.observe(composerWrap.value)
  }
  await initChatDraw()
  if (polishEnabled.value) void initAIOptions()
  scrollToBottom(true)
})
onBeforeUnmount(() => ro?.disconnect())
</script>

<style>
/* 生成中骨架屏(类名唯一,不用 scoped 以便直接匹配 html.dark) */
.chat-skeleton {
  background: linear-gradient(100deg, rgb(243 244 246) 30%, rgb(249 250 251) 50%, rgb(243 244 246) 70%);
  background-size: 200% 100%;
  animation: chat-shimmer 1.6s linear infinite;
}
.dark .chat-skeleton {
  background-image: linear-gradient(100deg, rgb(30 41 59) 30%, rgb(51 65 85 / 0.7) 50%, rgb(30 41 59) 70%);
}
@keyframes chat-shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
@media (prefers-reduced-motion: reduce) {
  .chat-skeleton { animation: none; }
}
</style>
