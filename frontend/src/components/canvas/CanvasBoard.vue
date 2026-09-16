<template>
  <div class="flex h-full min-h-0 flex-col gap-2">
    <!-- 工具栏 -->
    <div class="flex flex-wrap items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs dark:border-dark-700 dark:bg-dark-800">
      <button type="button" class="rounded-md border border-gray-200 px-2 py-1 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600" @click="addTextNode">{{ t('canvas.addText') }}</button>
      <label class="cursor-pointer rounded-md border border-gray-200 px-2 py-1 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600">
        {{ t('canvas.addImage') }}
        <input type="file" accept="image/*" multiple class="hidden" @change="onUploadImages" />
      </label>
      <label class="cursor-pointer rounded-md border border-gray-200 px-2 py-1 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600">
        {{ t('canvas.addVideo') }}
        <input type="file" accept="video/*" class="hidden" @change="onUploadVideo" />
      </label>
      <button
        type="button"
        class="rounded-md border px-2 py-1 transition-colors"
        :class="linkMode ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600'"
        @click="toggleLinkMode"
      >{{ t('canvas.linkMode') }}</button>
      <button type="button" class="rounded-md border border-gray-200 px-2 py-1 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600" @click="resetView">{{ t('canvas.resetView') }}</button>
      <span class="ml-1 rounded bg-gray-100 px-1.5 py-0.5 tabular-nums text-gray-500 dark:bg-dark-700">{{ Math.round(viewport.scale * 100) }}%</span>
      <span v-if="linkMode && linkFrom" class="text-primary-500">{{ t('canvas.linkPickSecond') }}</span>
      <span class="ml-auto text-gray-400">{{ t('canvas.autoSaved') }}</span>
    </div>

    <!-- 画布区 -->
    <div
      ref="viewportEl"
      class="relative min-h-0 flex-1 cursor-grab overflow-hidden rounded-lg border border-gray-200 bg-[radial-gradient(circle,#9ca3af22_1px,transparent_1px)] [background-size:24px_24px] dark:border-dark-700 dark:bg-dark-900"
      @wheel.prevent="onWheel"
      @pointerdown="onBackgroundPointerDown"
      @dragover.prevent
      @drop.prevent="onDropFile"
    >
      <div
        class="absolute left-0 top-0 origin-top-left"
        :style="{ transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.scale})` }"
      >
        <!-- 连线层 -->
        <svg class="pointer-events-none absolute overflow-visible" width="1" height="1">
          <path
            v-for="edge in doc.edges"
            :key="edge.id"
            :d="edgePath(edge)"
            fill="none"
            stroke="#14b8a6"
            stroke-width="2"
            class="cursor-pointer"
            @dblclick.stop="removeEdge(edge.id)"
          />
        </svg>

        <!-- 节点层 -->
        <div
          v-for="node in doc.nodes"
          :key="node.id"
          class="absolute rounded-lg border-2 bg-white shadow-md dark:bg-dark-800"
          :class="[
            linkMode ? 'cursor-crosshair' : 'cursor-move',
            linkFrom === node.id ? 'border-primary-500' : 'border-gray-300 dark:border-dark-600'
          ]"
          :style="{ left: node.x + 'px', top: node.y + 'px', width: node.w + 'px', height: node.h + 'px' }"
          @pointerdown.stop="onNodePointerDown($event, node)"
        >
          <template v-if="node.type === 'text'">
            <textarea
              v-model="node.text"
              class="h-full w-full resize-none rounded-md bg-transparent p-2 text-sm outline-none"
              :placeholder="t('canvas.textPlaceholder')"
              @pointerdown.stop
              @input="scheduleSave"
            ></textarea>
          </template>
          <template v-else-if="node.type === 'image'">
            <img v-if="nodeSrc(node)" :src="nodeSrc(node)!" class="pointer-events-none h-full w-full rounded-md object-contain" draggable="false" alt="" />
          </template>
          <template v-else-if="node.type === 'video'">
            <video v-if="nodeSrc(node)" :src="nodeSrc(node)!" class="h-full w-full rounded-md object-contain" controls preload="metadata"></video>
          </template>

          <button
            type="button"
            class="absolute -right-2 -top-2 flex h-5 w-5 items-center justify-center rounded-full bg-red-500 text-[10px] text-white opacity-0 transition-opacity hover:bg-red-600 group-hover:opacity-100"
            :style="{ opacity: 1 }"
            @pointerdown.stop
            @click="removeNode(node.id)"
          >✕</button>
          <div
            class="absolute -bottom-1 -right-1 h-3 w-3 cursor-nwse-resize rounded-sm border border-gray-400 bg-white dark:border-dark-500 dark:bg-dark-700"
            @pointerdown.stop="onResizePointerDown($event, node)"
          ></div>
        </div>
      </div>

      <div v-if="doc.nodes.length === 0" class="pointer-events-none absolute inset-0 flex items-center justify-center text-sm text-gray-400">
        {{ t('canvas.emptyBoard') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import {
  type CanvasDoc, type CanvasNode, putAsset, saveCanvas, getAsset, uid,
} from '@/composables/useInfiniteCanvas'
import { useAppStore } from '@/stores/app'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ doc: CanvasDoc }>()
const { t } = useI18n()
const appStore = useAppStore()

const viewportEl = ref<HTMLElement | null>(null)
const viewport = reactive({ ...props.doc.viewport })
const nodeUrls = new Map<string, string>()

function nodeSrc(node: CanvasNode): string | null {
  if (node.type === 'text') return null
  if (!node.assetId) return null
  const cached = nodeUrls.get(node.assetId)
  if (cached) return cached
  void getAsset(node.assetId).then(blob => {
    if (blob) nodeUrls.set(node.assetId!, URL.createObjectURL(blob))
  })
  return null
}

// ===== 保存(防抖 300ms;卸载/关页立即落盘,防整页导航丢数据) =====
let saveTimer: number | null = null
function flushSave() {
  if (saveTimer !== null) {
    window.clearTimeout(saveTimer)
    saveTimer = null
  }
  props.doc.viewport = { ...viewport }
  void saveCanvas(props.doc)
}
function scheduleSave() {
  if (saveTimer !== null) window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(flushSave, 300)
}
function onPageHide() {
  flushSave()
}

// ===== 背景平移 / 缩放 =====
let panning: { startX: number; startY: number; baseX: number; baseY: number } | null = null
function onBackgroundPointerDown(e: PointerEvent) {
  if (e.button !== 0 && e.button !== 1) return
  panning = { startX: e.clientX, startY: e.clientY, baseX: viewport.x, baseY: viewport.y }
  window.addEventListener('pointermove', onPanMove)
  window.addEventListener('pointerup', onPanUp)
}
function onPanMove(e: PointerEvent) {
  if (!panning) return
  viewport.x = panning.baseX + (e.clientX - panning.startX)
  viewport.y = panning.baseY + (e.clientY - panning.startY)
  scheduleSave()
}
function onPanUp() {
  panning = null
  window.removeEventListener('pointermove', onPanMove)
  window.removeEventListener('pointerup', onPanUp)
}
function onWheel(e: WheelEvent) {
  const rect = viewportEl.value?.getBoundingClientRect()
  if (!rect) return
  const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1
  const next = Math.min(4, Math.max(0.2, viewport.scale * factor))
  const mx = e.clientX - rect.left
  const my = e.clientY - rect.top
  viewport.x = mx - ((mx - viewport.x) * next) / viewport.scale
  viewport.y = my - ((my - viewport.y) * next) / viewport.scale
  viewport.scale = next
  scheduleSave()
}
function resetView() {
  viewport.x = 0
  viewport.y = 0
  viewport.scale = 1
  scheduleSave()
}

// ===== 节点拖动 / 缩放 =====
let dragging: { node: CanvasNode; startX: number; startY: number; baseX: number; baseY: number } | null = null
let resizing: { node: CanvasNode; startX: number; startY: number; baseW: number; baseH: number } | null = null

function onNodePointerDown(e: PointerEvent, node: CanvasNode) {
  // 连线模式:依次点选两个节点
  if (linkMode.value) {
    if (!linkFrom.value) {
      linkFrom.value = node.id
    } else if (linkFrom.value !== node.id) {
      props.doc.edges.push({ id: uid(), from: linkFrom.value, to: node.id })
      linkFrom.value = null
      scheduleSave()
    }
    return
  }
  dragging = { node, startX: e.clientX, startY: e.clientY, baseX: node.x, baseY: node.y }
  window.addEventListener('pointermove', onNodeMove)
  window.addEventListener('pointerup', onNodeUp)
}
function onNodeMove(e: PointerEvent) {
  if (!dragging) return
  dragging.node.x = dragging.baseX + (e.clientX - dragging.startX) / viewport.scale
  dragging.node.y = dragging.baseY + (e.clientY - dragging.startY) / viewport.scale
  scheduleSave()
}
function onNodeUp() {
  dragging = null
  window.removeEventListener('pointermove', onNodeMove)
  window.removeEventListener('pointerup', onNodeUp)
}
function onResizePointerDown(e: PointerEvent, node: CanvasNode) {
  resizing = { node, startX: e.clientX, startY: e.clientY, baseW: node.w, baseH: node.h }
  window.addEventListener('pointermove', onResizeMove)
  window.addEventListener('pointerup', onResizeUp)
}
function onResizeMove(e: PointerEvent) {
  if (!resizing) return
  resizing.node.w = Math.max(80, resizing.baseW + (e.clientX - resizing.startX) / viewport.scale)
  resizing.node.h = Math.max(60, resizing.baseH + (e.clientY - resizing.startY) / viewport.scale)
  scheduleSave()
}
function onResizeUp() {
  resizing = null
  window.removeEventListener('pointermove', onResizeMove)
  window.removeEventListener('pointerup', onResizeUp)
}

// ===== 连线 =====
const linkMode = ref(false)
const linkFrom = ref<string | null>(null)
function toggleLinkMode() {
  linkMode.value = !linkMode.value
  linkFrom.value = null
}
function edgePath(edge: { from: string; to: string }): string {
  const a = props.doc.nodes.find(n => n.id === edge.from)
  const b = props.doc.nodes.find(n => n.id === edge.to)
  if (!a || !b) return ''
  const ax = a.x + a.w / 2, ay = a.y + a.h / 2
  const bx = b.x + b.w / 2, by = b.y + b.h / 2
  const dx = Math.max(60, Math.abs(bx - ax) / 2)
  return `M ${ax} ${ay} C ${ax + dx} ${ay}, ${bx - dx} ${by}, ${bx} ${by}`
}
function removeEdge(id: string) {
  const i = props.doc.edges.findIndex(e => e.id === id)
  if (i >= 0) {
    props.doc.edges.splice(i, 1)
    scheduleSave()
  }
}

// ===== 节点增删 =====
function removeNode(id: string) {
  const i = props.doc.nodes.findIndex(n => n.id === id)
  if (i >= 0) {
    props.doc.nodes.splice(i, 1)
    props.doc.edges = props.doc.edges.filter(e => e.from !== id && e.to !== id)
    scheduleSave()
  }
}
function addTextNode() {
  const cx = (viewportEl.value?.clientWidth || 800) / 2 - viewport.x
  const cy = (viewportEl.value?.clientHeight || 600) / 2 - viewport.y
  props.doc.nodes.push({
    id: uid(), type: 'text',
    x: cx / viewport.scale - 110, y: cy / viewport.scale - 70,
    w: 220, h: 140,
    text: ''
  })
  scheduleSave()
}
async function addImageNodes(files: File[]) {
  for (const file of files) {
    const assetId = uid()
    await putAsset(assetId, file)
    const bmp = await createImageBitmap(file).catch(() => null)
    const w = bmp ? Math.min(360, bmp.width) : 300
    const h = bmp ? Math.round(w * bmp.height / bmp.width) : 300
    bmp?.close()
    props.doc.nodes.push({ id: uid(), type: 'image', x: 60 + props.doc.nodes.length * 24, y: 60 + props.doc.nodes.length * 18, w, h, assetId })
  }
  scheduleSave()
}
async function onUploadImages(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) await addImageNodes(Array.from(input.files))
  input.value = ''
}
async function onUploadVideo(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) {
    const assetId = uid()
    await putAsset(assetId, file)
    props.doc.nodes.push({ id: uid(), type: 'video', x: 80, y: 80, w: 420, h: 260, assetId })
    scheduleSave()
    appStore.showSuccess(t('canvas.addedToCanvas', { name: props.doc.name }))
  }
  input.value = ''
}
async function onDropFile(e: DragEvent) {
  const files = Array.from(e.dataTransfer?.files || [])
  const images = files.filter(f => f.type.startsWith('image/'))
  const videos = files.filter(f => f.type.startsWith('video/'))
  if (images.length) await addImageNodes(images)
  for (const v of videos) {
    const assetId = uid()
    await putAsset(assetId, v)
    props.doc.nodes.push({ id: uid(), type: 'video', x: 100 + props.doc.nodes.length * 20, y: 100, w: 420, h: 260, assetId })
  }
  if (images.length || videos.length) scheduleSave()
}

onMounted(() => {
  // 恢复已存在的图片节点 URL
  for (const n of props.doc.nodes) {
    if (n.type === 'image') void nodeSrc(n)
  }
  window.addEventListener('pagehide', onPageHide)
})
onUnmounted(() => {
  window.removeEventListener('pagehide', onPageHide)
  flushSave()
})
</script>
