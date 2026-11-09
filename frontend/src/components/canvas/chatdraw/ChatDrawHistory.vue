<template>
  <div class="flex h-full min-h-0 flex-col gap-2.5">
    <div class="flex min-h-9 items-center justify-between gap-2">
      <span class="text-[15px] font-bold text-gray-900 dark:text-white">{{ t('canvasChat.history') }}</span>
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="flex h-8 w-8 items-center justify-center rounded-xl text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-red-900/20"
          :disabled="!sessions.length || (sessions.length === 1 && !sessions[0].turnCount)"
          :title="t('canvasChat.clearAll')"
          :aria-label="t('canvasChat.clearAll')"
          @click="emit('clear')"
        ><Icon name="trash" size="sm" /></button>
        <button
          v-if="closable"
          type="button"
          class="flex h-8 w-8 items-center justify-center rounded-xl text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white"
          :title="closeLabel"
          :aria-label="closeLabel"
          @click="emit('close')"
        ><Icon :name="closeIcon ?? 'x'" size="sm" /></button>
      </div>
    </div>

    <button
      type="button"
      class="flex h-10 shrink-0 items-center justify-center gap-1.5 rounded-xl border border-gray-200 bg-white text-sm font-semibold text-gray-800 shadow-sm transition-colors hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100 dark:hover:border-dark-400"
      @click="emit('create')"
    ><Icon name="plus" size="sm" />{{ t('canvasChat.newChat') }}</button>

    <label v-if="sessions.length > 6" class="flex h-9 shrink-0 items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 text-gray-400 dark:border-dark-600 dark:bg-dark-900">
      <Icon name="search" size="xs" class="shrink-0" />
      <input v-model="query" class="min-w-0 flex-1 bg-transparent text-[13px] text-gray-800 outline-none placeholder:text-gray-400 dark:text-gray-100" :placeholder="t('canvasChat.searchHistory')" :aria-label="t('canvasChat.searchHistory')" />
    </label>

    <div class="-mx-1 min-h-0 flex-1 overflow-y-auto px-1" role="list">
      <div
        v-for="s in filtered"
        :key="s.id"
        role="listitem"
        class="group relative mb-1 flex min-h-[3.125rem] items-center rounded-xl border px-2.5 py-2 transition-colors"
        :class="s.id === activeId
          ? 'border-gray-300 bg-gray-100 dark:border-dark-500 dark:bg-dark-700'
          : 'border-transparent hover:border-gray-200 hover:bg-gray-50 dark:hover:border-dark-600 dark:hover:bg-dark-800'"
      >
        <form v-if="editingId === s.id" class="w-full" @submit.prevent="commitRename(s.id)">
          <input
            ref="editInput"
            v-model="editingTitle"
            maxlength="80"
            :aria-label="t('canvasChat.rename')"
            class="h-8 w-full rounded-lg border border-gray-300 bg-white px-2 text-[13px] font-semibold outline-none focus:border-gray-500 dark:border-dark-500 dark:bg-dark-900"
            @keydown.esc.prevent="editingId = ''"
            @blur="commitRename(s.id)"
          />
        </form>
        <template v-else>
          <button
            type="button"
            class="flex min-w-0 flex-1 flex-col gap-0.5 pr-14 text-left"
            :aria-current="s.id === activeId ? 'true' : undefined"
            @click="emit('select', s.id)"
            @dblclick.stop="beginRename(s)"
          >
            <span class="block truncate text-[13px] font-semibold leading-5 text-gray-900 dark:text-gray-100">{{ s.title || t('canvasChat.untitled') }}</span>
            <span class="flex min-w-0 items-center gap-1.5 truncate text-[11px] leading-4 text-gray-400">
              <span>{{ s.turnCount ? t('canvasChat.sessionMeta', { turns: s.turnCount, images: s.imageCount }) : t('canvasChat.sessionEmpty') }}</span>
              <span aria-hidden="true">·</span>
              <span>{{ formatTime(s.lastTs) }}</span>
            </span>
            <span
              v-if="running.has(s.id) || notices[s.id]"
              class="mt-0.5 w-fit rounded-full px-2 py-px text-[11px] font-semibold"
              :class="running.has(s.id)
                ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                : notices[s.id] === 'error'
                  ? 'bg-red-50 text-red-600 dark:bg-red-900/30 dark:text-red-300'
                  : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'"
            >{{ running.has(s.id) ? t('canvasChat.badgeRunning') : notices[s.id] === 'error' ? t('canvasChat.badgeError') : t('canvasChat.badgeDone') }}</span>
          </button>
          <div class="absolute right-2 top-1/2 flex -translate-y-1/2 items-center gap-px transition-opacity lg:opacity-0 lg:focus-within:opacity-100 lg:group-hover:opacity-100">
            <button
              type="button"
              class="flex h-6 w-6 items-center justify-center rounded-md text-gray-400 hover:bg-white hover:text-gray-900 dark:hover:bg-dark-900 dark:hover:text-white"
              :title="t('canvasChat.rename')"
              :aria-label="t('canvasChat.rename')"
              @click.stop="beginRename(s)"
            ><Icon name="edit" size="xs" /></button>
            <button
              type="button"
              class="flex h-6 w-6 items-center justify-center rounded-md text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/30"
              :title="t('canvasChat.deleteSession')"
              :aria-label="t('canvasChat.deleteSession')"
              @click.stop="emit('delete', s.id)"
            ><Icon name="trash" size="xs" /></button>
          </div>
        </template>
      </div>
      <div v-if="!filtered.length" class="px-3 py-4 text-center text-xs leading-relaxed text-gray-400">
        {{ query.trim() ? t('canvasChat.noMatch') : t('canvasChat.historyEmpty') }}
      </div>
    </div>

    <!-- 底部统计 -->
    <div class="shrink-0 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-800/60">
      <div class="flex items-baseline justify-between gap-2">
        <span class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('canvasChat.statsImages') }}</span>
        <span class="text-base font-bold tabular-nums text-gray-900 dark:text-white">{{ totalImages }}</span>
      </div>
      <div class="mt-0.5 flex items-baseline justify-between gap-2">
        <span class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('canvasChat.statsSessions') }}</span>
        <span class="text-xs font-semibold tabular-nums text-gray-600 dark:text-gray-300">{{ savedCount }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ChatSession, SessionNotice } from '@/composables/useChatDrawEngine'

const props = defineProps<{
  sessions: ChatSession[]
  activeId: string
  running: Set<string>
  notices: Record<string, SessionNotice>
  totalImages: number
  closable?: boolean
  closeLabel?: string
  closeIcon?: 'chevronLeft' | 'x'
}>()
const emit = defineEmits<{
  create: []
  select: [id: string]
  rename: [id: string, title: string]
  delete: [id: string]
  clear: []
  close: []
}>()

const { t, locale } = useI18n()
const query = ref('')
const editingId = ref('')
const editingTitle = ref('')
const editInput = ref<HTMLInputElement[] | HTMLInputElement | null>(null)

const filtered = computed(() => {
  const k = query.value.trim().toLowerCase()
  return k ? props.sessions.filter(s => (s.title || '').toLowerCase().includes(k)) : props.sessions
})
const savedCount = computed(() => props.sessions.filter(s => s.turnCount > 0).length)

function formatTime(ts: number): string {
  if (!ts) return ''
  return new Intl.DateTimeFormat(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(ts))
}
function beginRename(s: ChatSession) {
  editingId.value = s.id
  editingTitle.value = s.title || ''
  void nextTick(() => {
    const el = Array.isArray(editInput.value) ? editInput.value[0] : editInput.value
    el?.focus()
    el?.select()
  })
}
function commitRename(id: string) {
  if (editingId.value !== id) return
  const title = editingTitle.value.trim()
  editingId.value = ''
  emit('rename', id, title)
}
</script>
