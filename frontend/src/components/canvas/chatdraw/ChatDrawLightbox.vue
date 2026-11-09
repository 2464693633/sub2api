<template>
  <Teleport to="body">
    <div
      v-if="item"
      class="fixed inset-0 z-[300] flex flex-col bg-black/90 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      :aria-label="t('canvasChat.preview')"
      @click.self="emit('close')"
    >
      <div class="flex shrink-0 items-center gap-2 px-4 py-3 text-white">
        <span class="min-w-0 flex-1 truncate text-xs text-white/70">
          <span v-if="items.length > 1" class="mr-2 tabular-nums">{{ index + 1 }} / {{ items.length }}</span>{{ item.caption }}
        </span>
        <button v-if="item.downloadable" type="button" :class="btnCls" :aria-label="t('imageStudio.download')" :title="t('imageStudio.download')" @click="emit('download', index)"><Icon name="download" size="sm" /></button>
        <button ref="closeBtn" type="button" :class="btnCls" :aria-label="t('common.close')" :title="t('common.close')" @click="emit('close')"><Icon name="x" size="sm" /></button>
      </div>
      <div class="relative flex min-h-0 flex-1 items-center justify-center px-4 pb-6" @click.self="emit('close')">
        <button v-if="items.length > 1" type="button" :class="[btnCls, 'absolute left-3 top-1/2 -translate-y-1/2']" :aria-label="t('canvasChat.prevImage')" @click="step(-1)"><Icon name="chevronLeft" size="md" /></button>
        <img :src="item.url" class="max-h-full max-w-full select-none rounded-lg object-contain shadow-2xl" :alt="item.caption" />
        <button v-if="items.length > 1" type="button" :class="[btnCls, 'absolute right-3 top-1/2 -translate-y-1/2']" :aria-label="t('canvasChat.nextImage')" @click="step(1)"><Icon name="chevronRight" size="md" /></button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { LightboxItem } from './format'

const props = defineProps<{ items: LightboxItem[]; index: number }>()
const emit = defineEmits<{ close: []; 'update:index': [i: number]; download: [i: number] }>()
const { t } = useI18n()

const closeBtn = ref<HTMLButtonElement | null>(null)
const btnCls = 'flex h-9 w-9 items-center justify-center rounded-full bg-white/10 text-white transition-colors hover:bg-white/25'
const item = computed(() => (props.index >= 0 ? props.items[props.index] : undefined))

function step(d: number) {
  const n = props.items.length
  if (n > 1) emit('update:index', (props.index + d + n) % n)
}
function onKey(e: KeyboardEvent) {
  if (!item.value) return
  if (e.key === 'Escape') emit('close')
  else if (e.key === 'ArrowLeft') step(-1)
  else if (e.key === 'ArrowRight') step(1)
}
watch(item, (v, old) => {
  if (v && !old) {
    window.addEventListener('keydown', onKey)
    void nextTick(() => closeBtn.value?.focus())
  } else if (!v && old) {
    window.removeEventListener('keydown', onKey)
  }
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>
