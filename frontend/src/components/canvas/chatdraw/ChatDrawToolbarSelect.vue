<template>
  <div ref="root" class="relative inline-flex shrink-0">
    <button
      type="button"
      class="inline-flex h-8 max-w-[14rem] items-center gap-1.5 whitespace-nowrap rounded-full border bg-white/95 px-3 text-xs text-gray-700 shadow-sm backdrop-blur transition-colors hover:border-gray-400 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-800/95 dark:text-gray-200 dark:hover:border-dark-400"
      :class="open ? 'border-gray-900 dark:border-gray-200' : 'border-gray-200 dark:border-dark-600'"
      :disabled="disabled"
      :title="label"
      :aria-label="`${label}: ${currentLabel}`"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="toggle"
      @keydown.down.prevent="openAndFocus"
    >
      <span class="text-gray-400 dark:text-gray-500">{{ label }}</span>
      <span class="min-w-0 truncate font-medium">{{ currentLabel }}</span>
      <Icon name="chevronDown" size="xs" class="shrink-0 transition-transform" :class="open ? 'rotate-180' : ''" />
    </button>
    <div
      v-if="open"
      ref="menu"
      role="listbox"
      :aria-label="label"
      class="absolute left-0 z-40 max-h-72 min-w-[10rem] max-w-[min(22rem,calc(100vw-2rem))] overflow-y-auto rounded-2xl border border-gray-200 bg-white p-1.5 shadow-xl dark:border-dark-600 dark:bg-dark-800"
      :class="openUp ? 'bottom-[calc(100%+0.5rem)]' : 'top-[calc(100%+0.5rem)]'"
      @keydown.esc.prevent="close(true)"
      @keydown.down.prevent="move(1)"
      @keydown.up.prevent="move(-1)"
    >
      <button
        v-for="opt in options"
        :key="String(opt.value)"
        type="button"
        role="option"
        :aria-selected="opt.value === modelValue"
        class="flex w-full items-center gap-2 rounded-xl px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-gray-100 focus:bg-gray-100 focus:outline-none dark:hover:bg-dark-700 dark:focus:bg-dark-700"
        :class="opt.value === modelValue ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-600 dark:text-gray-300'"
        @click="pick(opt.value)"
      >
        <span class="min-w-0 flex-1 truncate">{{ opt.label }}</span>
        <span v-if="opt.hint" class="shrink-0 text-[10px] text-gray-400">{{ opt.hint }}</span>
        <Icon v-if="opt.value === modelValue" name="check" size="xs" class="shrink-0 text-primary-600 dark:text-primary-400" />
      </button>
      <div v-if="!options.length" class="px-2.5 py-1.5 text-xs text-gray-400">{{ emptyText }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
/** 输入区上方的胶囊下拉(仿参照 StudioToolbarSelectButton:按钮触发 + 向上弹出的选项面板) */
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import type { ToolbarOption, ToolbarValue } from './format'

type T = ToolbarValue
const props = defineProps<{
  modelValue: T
  options: ToolbarOption[]
  label: string
  disabled?: boolean
  emptyText?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: T]; change: [value: T] }>()

const open = ref(false)
/** 弹出方向:按钮上方空间放得下菜单则向上,否则向下(设置面板里向上弹会被视口顶边截断) */
const openUp = ref(true)
const root = ref<HTMLElement | null>(null)
const menu = ref<HTMLElement | null>(null)
const currentLabel = computed(() => props.options.find(o => o.value === props.modelValue)?.label ?? (props.emptyText || '—'))

function toggle() {
  if (open.value) close()
  else open.value = true
}
/** 菜单渲染后按实际高度选方向;打开前先用最大高度预估一次,避免先向上闪现再翻转 */
function fitDirection() {
  const rect = root.value?.getBoundingClientRect()
  if (!rect) return
  const menuH = Math.min(menu.value?.offsetHeight ?? 288, 288)
  openUp.value = rect.top > menuH + 16
}
function close(refocus = false) {
  open.value = false
  if (refocus) root.value?.querySelector('button')?.focus()
}
function pick(v: T) {
  emit('update:modelValue', v)
  emit('change', v)
  close(true)
}
async function openAndFocus() {
  open.value = true
  await nextTick()
  const items = optionEls()
  const idx = props.options.findIndex(o => o.value === props.modelValue)
  items[Math.max(0, idx)]?.focus()
}
function optionEls(): HTMLElement[] {
  return Array.from(menu.value?.querySelectorAll<HTMLElement>('[role="option"]') ?? [])
}
function move(step: number) {
  const items = optionEls()
  if (!items.length) return
  const cur = items.indexOf(document.activeElement as HTMLElement)
  items[(cur + step + items.length) % items.length].focus()
}
function onDocPointer(e: PointerEvent) {
  if (root.value && !root.value.contains(e.target as Node)) close()
}
watch(open, v => {
  if (v) {
    // 先按菜单上限预估方向,渲染完成后按实际高度再校准一次(选项少时菜单矮,向上更自然)
    fitDirection()
    void nextTick(() => fitDirection())
    document.addEventListener('pointerdown', onDocPointer)
  } else {
    document.removeEventListener('pointerdown', onDocPointer)
  }
})
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocPointer))
</script>
