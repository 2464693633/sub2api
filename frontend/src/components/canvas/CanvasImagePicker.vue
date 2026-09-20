<template>
  <div class="fixed inset-0 z-[110] flex items-center justify-center bg-black/60 p-3 sm:p-6" @click.self="$emit('close')">
    <div class="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-dark-900">
      <div class="flex shrink-0 items-center justify-between border-b border-gray-200 px-4 py-3 dark:border-dark-700">
        <span class="text-sm font-semibold">{{ t('canvas.importFromImage') }}</span>
        <button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="$emit('close')">✕</button>
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto p-4">
        <p class="mb-3 text-[11px] text-gray-400">{{ t('canvas.importHint') }}</p>
        <div v-if="items.length === 0" class="flex h-40 items-center justify-center text-sm text-gray-400">{{ t('canvas.importEmpty') }}</div>
        <div v-else class="grid grid-cols-2 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(140px,1fr))]">
          <button
            v-for="item in items"
            :key="item.id"
            type="button"
            class="group relative overflow-hidden rounded-lg border-2 transition-colors"
            :class="selected.has(item.id) ? 'border-primary-500' : 'border-transparent hover:border-gray-300 dark:hover:border-dark-600'"
            :disabled="!selected.has(item.id) && selected.size >= maxSelect"
            @click="toggle(item.id)"
          >
            <img v-if="objectUrlFor(item)" :src="objectUrlFor(item)" class="aspect-square w-full bg-gray-100 object-cover dark:bg-dark-800" alt="" />
            <span class="absolute left-1 top-1 rounded bg-black/60 px-1 text-[10px] text-white">{{ item.model }}</span>
            <span v-if="selected.has(item.id)" class="absolute right-1 top-1 flex h-5 w-5 items-center justify-center rounded-full bg-primary-500 text-[10px] text-white">✓</span>
            <span class="block truncate px-1 py-1 text-left text-[10px] text-gray-500 dark:text-gray-400">{{ item.prompt || t('imageStudio.prompt') }}</span>
          </button>
        </div>
      </div>
      <div class="flex shrink-0 items-center justify-between border-t border-gray-200 px-4 py-3 dark:border-dark-700">
        <span class="text-xs text-gray-500">{{ t('canvas.importPicked', { n: selected.size, max: maxSelect }) }}</span>
        <div class="flex gap-2">
          <button type="button" class="rounded-lg border border-gray-200 px-3 py-1.5 text-xs text-gray-500 hover:border-gray-300 dark:border-dark-600" @click="$emit('close')">{{ t('common.cancel') }}</button>
          <button type="button" class="rounded-lg bg-primary-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50" :disabled="selected.size === 0" @click="confirmImport">{{ t('canvas.importConfirm') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  history as imageHistory, loadHistory as loadImageHistory, objectUrlFor,
  type HistoryItem,
} from '@/composables/useImageStudioEngine'

const props = defineProps<{ maxSelect: number }>()
const emit = defineEmits<{ close: []; import: [HistoryItem[]] }>()
const { t } = useI18n()

const selected = ref(new Set<number>())

const items = computed(() => imageHistory.value.filter(it => (it.images[0]?.blob && !it.images[0]?.failed)))

onMounted(() => { void loadImageHistory() })

function toggle(id: number) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id)
  else {
    if (next.size >= props.maxSelect) return
    next.add(id)
  }
  selected.value = next
}

function confirmImport() {
  const picked = items.value.filter(it => selected.value.has(it.id))
  if (picked.length) emit('import', picked)
  emit('close')
}
</script>
