<template>
  <div class="mx-auto max-w-2xl space-y-4 py-2">
    <div class="card p-4">
      <div class="mb-2 text-sm font-semibold">{{ t('canvas.configApi') }}</div>
      <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('canvas.configApiDesc') }}</p>
      <div class="mt-3 space-y-2">
        <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-800/60">
          <span class="text-gray-500">{{ t('canvas.configImageKey') }}</span>
          <span class="font-medium text-green-600 dark:text-green-400">✓ {{ t('canvas.configReady') }}</span>
        </div>
        <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-800/60">
          <span class="text-gray-500">{{ t('canvas.configVideoKey') }}</span>
          <span class="font-medium text-green-600 dark:text-green-400">✓ {{ t('canvas.configReady') }}</span>
        </div>
      </div>
      <p class="mt-2 text-[11px] text-gray-400">{{ t('canvas.configNote') }}</p>
    </div>

    <div class="card p-4">
      <div class="mb-2 text-sm font-semibold">{{ t('canvas.configGroups') }}</div>
      <div class="space-y-2">
        <div v-for="g in groups" :key="g.id + g.name" class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-800/60">
          <span class="font-medium">{{ g.name }}</span>
          <span class="text-gray-400">{{ g.platform }}</span>
          <span class="text-gray-400">{{ g.count }} {{ t('canvas.modelsUnit') }}</span>
        </div>
        <div v-if="groups.length === 0" class="text-xs text-gray-400">{{ t('canvas.noGroups') }}</div>
      </div>
    </div>

    <div class="card p-4">
      <div class="mb-2 text-sm font-semibold">{{ t('canvas.configAbout') }}</div>
      <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('canvas.configAboutDesc') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  IMAGE_MODEL_PATTERN, models as imageModels, loadModels as loadImageModels,
} from '@/composables/useImageStudioEngine'
import { models as videoModels, loadModels as loadVideoModels } from '@/composables/useVideoStudioEngine'

const { t } = useI18n()

interface GroupRow { id: number; name: string; platform: string; count: number }
const groups = ref<GroupRow[]>([])

onMounted(async () => {
  await Promise.all([loadImageModels(), loadVideoModels()])
  const map = new Map<number, GroupRow>()
  for (const m of imageModels.value) {
    if (!m.group_id) continue
    const row = map.get(m.group_id) || { id: m.group_id, name: m.group_name || String(m.group_id), platform: 'auto', count: 0 }
    if (!IMAGE_MODEL_PATTERN.test(m.id)) continue
    row.count++
    map.set(m.group_id, row)
  }
  groups.value = [...map.values()]
  void videoModels
})
</script>
