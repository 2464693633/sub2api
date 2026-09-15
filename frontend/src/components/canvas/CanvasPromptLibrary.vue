<template>
  <div class="flex h-full min-h-0 flex-col gap-3">
    <div class="flex flex-wrap items-center gap-2">
      <input v-model="search" type="text" :placeholder="t('canvas.promptSearch')" class="w-56 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" />
      <button type="button" class="rounded-md bg-primary-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-700" @click="onCreate">{{ t('canvas.promptNew') }}</button>
    </div>
    <div class="grid min-h-0 flex-1 grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-3 overflow-y-auto pr-1">
      <div v-for="p in filtered" :key="p.id" class="flex flex-col rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
        <div class="mb-1 flex items-center justify-between gap-2">
          <span class="truncate text-xs font-semibold">{{ p.name }}</span>
          <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-500 dark:bg-dark-700">{{ p.category }}</span>
        </div>
        <p class="line-clamp-4 min-h-0 flex-1 whitespace-pre-wrap text-[11px] leading-relaxed text-gray-500 dark:text-gray-400">{{ p.content }}</p>
        <div class="mt-2 flex items-center gap-2 border-t border-gray-100 pt-2 text-[11px] dark:border-dark-700">
          <button type="button" class="text-primary-500 hover:underline" @click="copy(p)">{{ t('canvas.copy') }}</button>
          <button type="button" class="text-primary-500 hover:underline" @click="useForImage(p)">{{ t('canvas.useForImage') }}</button>
          <button type="button" class="text-primary-500 hover:underline" @click="useForVideo(p)">{{ t('canvas.useForVideo') }}</button>
          <button v-if="!p.builtin" type="button" class="ml-auto text-gray-400 hover:text-red-500" @click="onDelete(p)">{{ t('common.delete') }}</button>
          <span v-if="p.builtin" class="ml-auto text-gray-300 dark:text-dark-500">{{ t('canvas.builtin') }}</span>
        </div>
      </div>
    </div>

    <!-- 新建/编辑弹层 -->
    <div v-if="editing" class="fixed inset-0 z-[110] flex items-center justify-center bg-black/50 p-6" @click.self="editing = null">
      <div class="w-full max-w-xl rounded-xl bg-white p-4 shadow-2xl dark:bg-dark-800">
        <div class="mb-2 flex gap-2">
          <input v-model="editName" type="text" :placeholder="t('canvas.promptName')" class="flex-1 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" />
          <input v-model="editCategory" type="text" :placeholder="t('canvas.promptCategory')" class="w-32 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" />
        </div>
        <textarea v-model="editContent" rows="10" class="w-full rounded-lg border border-gray-300 bg-white p-2.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"></textarea>
        <div class="mt-2 flex justify-end gap-2">
          <button type="button" class="rounded-md border border-gray-200 px-3 py-1.5 text-xs dark:border-dark-600" @click="editing = null">{{ t('common.cancel') }}</button>
          <button type="button" class="rounded-md bg-primary-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-700" @click="onSave">{{ t('common.save') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  prompts, loadPrompts, addPrompt, deletePrompt,
  activeTab,
} from '@/composables/useInfiniteCanvas'
import { prompt as imagePrompt } from '@/composables/useImageStudioEngine'
import { prompt as videoPrompt } from '@/composables/useVideoStudioEngine'

const { t } = useI18n()
const appStore = useAppStore()

const search = ref('')
const editing = ref<{ id?: string; name: string; category: string; content: string } | null>(null)
const editName = ref('')
const editCategory = ref('')
const editContent = ref('')

const filtered = computed(() => {
  const kw = search.value.trim().toLowerCase()
  if (!kw) return prompts.value
  return prompts.value.filter(p => p.name.toLowerCase().includes(kw) || p.category.toLowerCase().includes(kw) || p.content.toLowerCase().includes(kw))
})

onMounted(() => { void loadPrompts() })

function onCreate() {
  editName.value = ''
  editCategory.value = ''
  editContent.value = ''
  editing.value = { name: '', category: '', content: '' }
}
async function onSave() {
  if (!editing.value || !editName.value.trim() || !editContent.value.trim()) return
  await addPrompt(editName.value.trim(), editCategory.value.trim(), editContent.value.trim())
  editing.value = null
}
async function onDelete(p: { id: string }) {
  if (window.confirm(t('canvas.promptDeleteConfirm'))) await deletePrompt(p.id)
}
async function copy(p: { content: string }) {
  try {
    await navigator.clipboard.writeText(p.content)
    appStore.showSuccess(t('imageStudio.copied'))
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}
function useForImage(p: { content: string }) {
  imagePrompt.value = p.content.slice(0, 4000)
  activeTab.value = 'image'
  appStore.showSuccess(t('canvas.sentToImage'))
}
function useForVideo(p: { content: string }) {
  videoPrompt.value = p.content.slice(0, 4000)
  activeTab.value = 'video'
  appStore.showSuccess(t('canvas.sentToVideo'))
}
</script>
