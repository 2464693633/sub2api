<template>
  <div class="space-y-2">
    <div class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800" :aria-label="t('imageStudio.generationPlan')">
      <button type="button" :aria-pressed="!imageSetEnabled" :disabled="generating" class="rounded-md px-2 py-1.5 text-xs font-medium disabled:opacity-50" :class="!imageSetEnabled ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-400' : 'text-gray-500'" @click="imageSetEnabled = false">
        {{ t('imageStudio.samePromptMode') }}
      </button>
      <button type="button" :aria-pressed="imageSetEnabled" :disabled="generating" class="rounded-md px-2 py-1.5 text-xs font-medium disabled:opacity-50" :class="imageSetEnabled ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-400' : 'text-gray-500'" @click="initializeImageSet">
        {{ t('imageStudio.imageSetMode') }}
      </button>
    </div>

    <div v-if="imageSetEnabled" class="space-y-2 rounded-lg border border-primary-200 bg-primary-50/40 p-2 dark:border-primary-800 dark:bg-primary-900/10">
      <div class="flex items-center justify-between gap-2">
        <span class="text-xs font-semibold text-primary-700 dark:text-primary-300">{{ t('imageStudio.setSummary', { n: plannedImageCount - 1 }) }}</span>
        <span class="text-[10px] text-gray-500">{{ t('imageStudio.totalImages', { n: plannedImageCount }) }}</span>
      </div>
      <p class="text-[11px] leading-relaxed text-gray-500 dark:text-gray-400">{{ t('imageStudio.imageSetHint') }}</p>
      <div v-for="(_, index) in detailPrompts" :key="index" class="rounded-md border border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-1 flex items-center justify-between">
          <label :for="`image-detail-${index}`" class="text-xs font-medium">{{ t('imageStudio.detailImageN', { n: index + 1 }) }}</label>
          <button type="button" :disabled="generating || detailPrompts.length <= 1" :aria-label="t('imageStudio.removeDetailN', { n: index + 1 })" class="text-gray-400 hover:text-red-500 disabled:opacity-40" @click="detailPrompts.splice(index, 1)">×</button>
        </div>
        <textarea :id="`image-detail-${index}`" v-model="detailPrompts[index]" :disabled="generating" rows="2" maxlength="2000" :placeholder="t('imageStudio.detailPlaceholder')" class="w-full resize-y rounded border border-gray-200 bg-transparent px-2 py-1 text-xs leading-relaxed outline-none focus:border-primary-500 disabled:opacity-60 dark:border-dark-600"></textarea>
      </div>
      <button type="button" :disabled="generating || detailPrompts.length >= MAX_DETAIL_IMAGES" class="text-xs font-medium text-primary-600 disabled:opacity-40 dark:text-primary-400" @click="detailPrompts.push('')">+ {{ t('imageStudio.addDetail') }}</button>
      <p v-if="!validImagePlan" role="alert" class="text-[11px] text-amber-700 dark:text-amber-400">{{ t('imageStudio.detailRequired') }}</p>
    </div>

    <div v-else class="flex items-center justify-between gap-2">
      <label for="image-quantity" class="text-xs font-medium text-gray-500">{{ t('imageStudio.quantity') }}</label>
      <input id="image-quantity" v-model.number="quantity" type="number" min="1" max="50" step="1" :disabled="generating" class="w-20 rounded-lg border border-gray-300 bg-white px-2 py-1 text-sm outline-none focus:border-primary-500 disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import {
  detailPrompts, generating, imageSetEnabled, initializeImageSet, MAX_DETAIL_IMAGES,
  plannedImageCount, quantity, validImagePlan
} from '@/composables/useImageStudioEngine'

const { t } = useI18n()
</script>
