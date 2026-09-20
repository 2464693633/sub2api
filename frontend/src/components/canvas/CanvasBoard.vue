<template>
  <div class="flex h-full min-h-0 flex-col gap-2">
    <!-- 工具栏 -->
    <div class="flex flex-wrap items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs dark:border-dark-700 dark:bg-dark-800">
      <button type="button" class="rounded-md border border-gray-200 px-2 py-1 hover:border-primary-400 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600" :disabled="!canUndo" :title="t('canvas.undo') + ' (Ctrl+Z)'" @click="undo">↶ {{ t('canvas.undo') }}</button>
      <button type="button" class="rounded-md border border-gray-200 px-2 py-1 hover:border-primary-400 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600" :disabled="!canRedo" :title="t('canvas.redo') + ' (Ctrl+Shift+Z)'" @click="redo">↷ {{ t('canvas.redo') }}</button>
      <span class="mx-1 h-4 w-px bg-gray-200 dark:bg-dark-600"></span>
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
      <span class="mx-1 h-4 w-px bg-gray-200 dark:bg-dark-600"></span>
      <div class="flex items-center overflow-hidden rounded-md border border-gray-200 dark:border-dark-600">
        <button type="button" class="px-2 py-1 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" :title="t('canvas.zoomOut')" @click="zoomStep(1 / 1.2)">−</button>
        <span class="min-w-12 text-center tabular-nums text-gray-500">{{ Math.round(viewport.scale * 100) }}%</span>
        <button type="button" class="px-2 py-1 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" :title="t('canvas.zoomIn')" @click="zoomStep(1.2)">+</button>
      </div>
      <button type="button" class="rounded-md border border-gray-200 px-2 py-1 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600" @click="fitView">{{ t('canvas.fitView') }}</button>
      <button type="button" class="rounded-md border border-gray-200 px-2 py-1 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600" @click="resetView">{{ t('canvas.resetView') }}</button>
      <button type="button" class="rounded-md border border-gray-200 px-2 py-1 text-gray-400 hover:border-red-300 hover:text-red-500 dark:border-dark-600" @click="onClearBoard">{{ t('canvas.clearBoard') }}</button>
      <span v-if="linkMode && linkFrom" class="text-primary-500">{{ t('canvas.linkPickSecond') }}</span>
      <span v-else-if="selection.size" class="rounded bg-primary-50 px-1.5 py-0.5 text-primary-600 dark:bg-primary-900/30">{{ t('canvas.selectedCount', { n: selection.size }) }}</span>
      <span class="ml-auto text-gray-400">{{ t('canvas.autoSaved') }}</span>
      <button type="button" class="flex h-6 w-6 items-center justify-center rounded-full border border-gray-200 text-[10px] text-gray-400 hover:text-primary-600 dark:border-dark-600" :title="t('canvas.shortcuts')" @click="showShortcuts = true">?</button>
    </div>

    <!-- 画布区 -->
    <div
      ref="viewportEl"
      class="relative min-h-0 flex-1 cursor-grab overflow-hidden rounded-lg border border-gray-200 bg-[radial-gradient(circle,#9ca3af22_1px,transparent_1px)] [background-size:24px_24px] dark:border-dark-700 dark:bg-dark-900"
      :class="{ '!cursor-grabbing': panning, '!cursor-crosshair': marquee }"
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
            linkFrom === node.id || selection.has(node.id) ? 'border-primary-500 ring-2 ring-primary-300/60' : 'border-gray-300 dark:border-dark-600'
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
            @click="removeNodes([node.id])"
          >✕</button>
          <div
            class="absolute -bottom-1 -right-1 h-3 w-3 cursor-nwse-resize rounded-sm border border-gray-400 bg-white dark:border-dark-500 dark:bg-dark-700"
            @pointerdown.stop="onResizePointerDown($event, node)"
          ></div>
        </div>
      </div>

      <!-- 框选矩形 -->
      <div
        v-if="marquee"
        class="pointer-events-none absolute border border-primary-500 bg-primary-500/10"
        :style="marqueeStyle"
      ></div>

      <!-- 节点缩略图侧栏:点击定位到节点 -->
      <div v-if="doc.nodes.length" class="absolute bottom-2 left-2 top-2 z-10 hidden w-[68px] flex-col gap-1.5 overflow-y-auto rounded-lg border border-gray-200/80 bg-white/85 p-1.5 backdrop-blur dark:border-dark-700/80 dark:bg-dark-800/85 sm:flex">
        <button
          v-for="node in doc.nodes"
          :key="'thumb-' + node.id"
          type="button"
          class="relative h-14 w-full shrink-0 overflow-hidden rounded-md border bg-gray-100 dark:bg-dark-700"
          :class="selection.has(node.id) ? 'border-primary-500' : 'border-gray-200 dark:border-dark-600'"
          :title="node.type === 'text' ? (node.text || '').slice(0, 40) : t('canvas.nodeLocate')"
          @click="centerOnNode(node)"
        >
          <img v-if="node.type === 'image' && nodeSrc(node)" :src="nodeSrc(node)!" class="h-full w-full object-cover" alt="" />
          <video v-else-if="node.type === 'video' && nodeSrc(node)" :src="nodeSrc(node)!" class="h-full w-full object-cover" preload="metadata" muted></video>
          <div v-else-if="node.type === 'text'" class="flex h-full w-full items-center justify-center overflow-hidden p-1 text-center text-[9px] leading-tight text-gray-500">{{ (node.text || '···').slice(0, 26) }}</div>
          <span class="absolute bottom-0 left-0 rounded-tr bg-black/55 px-1 text-[8px] text-white">{{ node.type === 'image' ? '图' : node.type === 'video' ? '视' : '文' }}</span>
        </button>
      </div>

      <div v-if="doc.nodes.length === 0" class="pointer-events-none absolute inset-0 flex items-center justify-center text-sm text-gray-400">
        {{ t('canvas.emptyBoard') }}
      </div>
    </div>

    <!-- 快捷键面板 -->
    <div v-if="showShortcuts" class="fixed inset-0 z-[110] flex items-center justify-center bg-black/50 p-6" @click.self="showShortcuts = false">
      <div class="w-full max-w-md rounded-xl bg-white p-5 shadow-2xl dark:bg-dark-800">
        <div class="mb-3 flex items-center justify-between">
          <span class="text-sm font-semibold">{{ t('canvas.shortcuts') }}</span>
          <button type="button" class="text-gray-400 hover:text-gray-600" @click="showShortcuts = false">✕</button>
        </div>
        <ul class="space-y-2 text-xs text-gray-600 dark:text-gray-300">
          <li v-for="sc in SHORTCUTS" :key="sc.k" class="flex items-center justify-between gap-4">
            <span>{{ sc.k }}</span><span class="text-gray-400">{{ sc.v }}</span>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import {
  type CanvasDoc, type CanvasNode, type CanvasEdge, putAsset, saveCanvas, getAsset, uid,
} from '@/composables/useInfiniteCanvas'
import { useAppStore } from '@/stores/app'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ doc: CanvasDoc }>()
const { t } = useI18n()
const appStore = useAppStore()

const viewportEl = ref<HTMLElement | null>(null)
const viewport = reactive({ ...props.doc.viewport })
const nodeUrls = new Map<string, string>()

// ===== 快捷键面板 =====
const showShortcuts = ref(false)
const SHORTCUTS = computed(() => [
  { k: t('canvas.scDelete'), v: '' },
  { k: t('canvas.scUndo'), v: '' },
  { k: t('canvas.scCopy'), v: '' },
  { k: t('canvas.scAll'), v: '' },
  { k: t('canvas.scEsc'), v: '' },
  { k: t('canvas.scMarquee'), v: '' },
  { k: t('canvas.scPan'), v: '' },
  { k: t('canvas.scWheel'), v: t('canvas.zoom') },
])

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

// ===== 撤销 / 重做(快照栈,上限 60) =====
const undoStack = ref<string[]>([])
const redoStack = ref<string[]>([])
const canUndo = computed(() => undoStack.value.length > 0)
const canRedo = computed(() => redoStack.value.length > 0)
function snapshot(): string {
  return JSON.stringify({ nodes: props.doc.nodes, edges: props.doc.edges })
}
function applySnapshot(json: string) {
  const parsed = JSON.parse(json) as { nodes: CanvasNode[]; edges: CanvasEdge[] }
  props.doc.nodes = parsed.nodes
  props.doc.edges = parsed.edges
  for (const id of [...selection.value]) if (!props.doc.nodes.some(n => n.id === id)) selection.value.delete(id)
  selection.value = new Set(selection.value)
  for (const n of props.doc.nodes) if (n.type === 'image') void nodeSrc(n)
  scheduleSave()
}
function pushUndo() {
  undoStack.value.push(snapshot())
  if (undoStack.value.length > 60) undoStack.value.shift()
  redoStack.value = []
}
let pendingSnap: string | null = null
function armUndo() {
  pendingSnap = snapshot()
}
function commitUndo() {
  if (pendingSnap) {
    undoStack.value.push(pendingSnap)
    if (undoStack.value.length > 60) undoStack.value.shift()
    redoStack.value = []
    pendingSnap = null
  }
}
function discardPendingUndo() {
  pendingSnap = null
}
function undo() {
  if (!undoStack.value.length) return
  redoStack.value.push(snapshot())
  applySnapshot(undoStack.value.pop()!)
}
function redo() {
  if (!redoStack.value.length) return
  undoStack.value.push(snapshot())
  applySnapshot(redoStack.value.pop()!)
}

// ===== 选择(多选 + 框选 + 键盘) =====
const selection = ref(new Set<string>())
const marquee = ref<{ x0: number; y0: number; x1: number; y1: number } | null>(null)
const marqueeStyle = computed(() => {
  if (!marquee.value) return {}
  const x = Math.min(marquee.value.x0, marquee.value.x1)
  const y = Math.min(marquee.value.y0, marquee.value.y1)
  return { left: x + 'px', top: y + 'px', width: Math.abs(marquee.value.x1 - marquee.value.x0) + 'px', height: Math.abs(marquee.value.y1 - marquee.value.y0) + 'px' }
})

// ===== 背景平移 / 框选 / 缩放 =====
let panning: { startX: number; startY: number; baseX: number; baseY: number } | null = null
let spaceHeld = false
let marqueeDrag: { startX: number; startY: number } | null = null
function onBackgroundPointerDown(e: PointerEvent) {
  if (e.button === 1 || spaceHeld) {
    // 平移:中键或空格+左键
    panning = { startX: e.clientX, startY: e.clientY, baseX: viewport.x, baseY: viewport.y }
    window.addEventListener('pointermove', onPanMove)
    window.addEventListener('pointerup', onPanUp)
    return
  }
  if (e.button !== 0) return
  // 左键空白:框选
  const rect = viewportEl.value?.getBoundingClientRect()
  if (!rect) return
  marqueeDrag = { startX: e.clientX - rect.left, startY: e.clientY - rect.top }
  marquee.value = { x0: marqueeDrag.startX, y0: marqueeDrag.startY, x1: marqueeDrag.startX, y1: marqueeDrag.startY }
  if (!e.shiftKey) selection.value = new Set()
  window.addEventListener('pointermove', onMarqueeMove)
  window.addEventListener('pointerup', onMarqueeUp)
}
function onPanMove(e: PointerEvent) {
  if (!panning) return
  viewport.x = panning.baseX + (e.clientX - panning.startX)
  viewport.y = panning.baseY + (e.clientY - panning.startY)
}
function onPanUp() {
  panning = null
  window.removeEventListener('pointermove', onPanMove)
  window.removeEventListener('pointerup', onPanUp)
}
function onMarqueeMove(e: PointerEvent) {
  if (!marqueeDrag || !marquee.value) return
  const rect = viewportEl.value?.getBoundingClientRect()
  if (!rect) return
  marquee.value.x1 = e.clientX - rect.left
  marquee.value.y1 = e.clientY - rect.top
}
function onMarqueeUp() {
  window.removeEventListener('pointermove', onMarqueeMove)
  window.removeEventListener('pointerup', onMarqueeUp)
  if (marquee.value && marqueeDrag) {
    const x0 = Math.min(marquee.value.x0, marquee.value.x1)
    const x1 = Math.max(marquee.value.x0, marquee.value.x1)
    const y0 = Math.min(marquee.value.y0, marquee.value.y1)
    const y1 = Math.max(marquee.value.y0, marquee.value.y1)
    // 屏幕坐标 → 画布坐标
    const cx0 = (x0 - viewport.x) / viewport.scale
    const cx1 = (x1 - viewport.x) / viewport.scale
    const cy0 = (y0 - viewport.y) / viewport.scale
    const cy1 = (y1 - viewport.y) / viewport.scale
    if (Math.abs(x1 - x0) > 4 && Math.abs(y1 - y0) > 4) {
      const next = new Set(selection.value)
      for (const n of props.doc.nodes) {
        if (n.x + n.w >= cx0 && n.x <= cx1 && n.y + n.h >= cy0 && n.y <= cy1) next.add(n.id)
      }
      selection.value = next
    }
  }
  marqueeDrag = null
  marquee.value = null
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
/** 以画布中心为锚点缩放一步(工具栏 +/-) */
function zoomStep(factor: number) {
  const rect = viewportEl.value?.getBoundingClientRect()
  if (!rect) return
  const mx = rect.width / 2
  const my = rect.height / 2
  const next = Math.min(4, Math.max(0.2, viewport.scale * factor))
  viewport.x = mx - ((mx - viewport.x) * next) / viewport.scale
  viewport.y = my - ((my - viewport.y) * next) / viewport.scale
  viewport.scale = next
  scheduleSave()
}
/** 缩放并平移到刚好容纳全部节点(留 60px 边距) */
function fitView() {
  const rect = viewportEl.value?.getBoundingClientRect()
  if (!rect || !props.doc.nodes.length) { resetView(); return }
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  for (const n of props.doc.nodes) {
    minX = Math.min(minX, n.x)
    minY = Math.min(minY, n.y)
    maxX = Math.max(maxX, n.x + n.w)
    maxY = Math.max(maxY, n.y + n.h)
  }
  const pad = 60
  const scale = Math.min(4, Math.max(0.2, Math.min((rect.width - pad * 2) / (maxX - minX), (rect.height - pad * 2) / (maxY - minY))))
  viewport.scale = scale
  viewport.x = (rect.width - (maxX - minX) * scale) / 2 - minX * scale
  viewport.y = (rect.height - (maxY - minY) * scale) / 2 - minY * scale
  scheduleSave()
}
/** 缩略图点击:居中并适度放大该节点 */
function centerOnNode(node: CanvasNode) {
  const rect = viewportEl.value?.getBoundingClientRect()
  if (!rect) return
  const scale = Math.min(1.5, Math.max(0.4, Math.min(rect.width / (node.w + 240), rect.height / (node.h + 240))))
  viewport.scale = scale
  viewport.x = rect.width / 2 - (node.x + node.w / 2) * scale
  viewport.y = rect.height / 2 - (node.y + node.h / 2) * scale
  scheduleSave()
}

// ===== 节点拖动 / 缩放(支持多选拖动) =====
let dragging: { startX: number; startY: number; armed: boolean; items: Array<{ node: CanvasNode; baseX: number; baseY: number }> } | null = null
let resizing: { node: CanvasNode; startX: number; startY: number; baseW: number; baseH: number } | null = null

function onNodePointerDown(e: PointerEvent, node: CanvasNode) {
  // 连线模式:依次点选两个节点
  if (linkMode.value) {
    if (!linkFrom.value) {
      linkFrom.value = node.id
    } else if (linkFrom.value !== node.id) {
      pushUndo()
      props.doc.edges.push({ id: uid(), from: linkFrom.value, to: node.id })
      linkFrom.value = null
      scheduleSave()
    }
    return
  }
  if (e.shiftKey) {
    // Shift 切换选中,不拖动
    const next = new Set(selection.value)
    if (next.has(node.id)) next.delete(node.id)
    else next.add(node.id)
    selection.value = next
    return
  }
  if (!selection.value.has(node.id)) selection.value = new Set([node.id])
  else selection.value = new Set(selection.value)
  // 多选:整体拖动
  const items: Array<{ node: CanvasNode; baseX: number; baseY: number }> = []
  for (const id of selection.value) {
    const n = props.doc.nodes.find(nn => nn.id === id)
    if (n) items.push({ node: n, baseX: n.x, baseY: n.y })
  }
  dragging = { startX: e.clientX, startY: e.clientY, armed: false, items }
  window.addEventListener('pointermove', onNodeMove)
  window.addEventListener('pointerup', onNodeUp)
}
function onNodeMove(e: PointerEvent) {
  if (!dragging) return
  const dx = (e.clientX - dragging.startX) / viewport.scale
  const dy = (e.clientY - dragging.startY) / viewport.scale
  if (!dragging.armed && (Math.abs(dx) > 2 || Math.abs(dy) > 2)) {
    armUndo()
    dragging.armed = true
  }
  if (dragging.armed) {
    for (const it of dragging.items) {
      it.node.x = it.baseX + dx
      it.node.y = it.baseY + dy
    }
    scheduleSave()
  }
}
function onNodeUp() {
  if (dragging?.armed) commitUndo()
  else discardPendingUndo()
  dragging = null
  window.removeEventListener('pointermove', onNodeMove)
  window.removeEventListener('pointerup', onNodeUp)
}
function onResizePointerDown(e: PointerEvent, node: CanvasNode) {
  armUndo()
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
  commitUndo()
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
  pushUndo()
  const i = props.doc.edges.findIndex(e => e.id === id)
  if (i >= 0) {
    props.doc.edges.splice(i, 1)
    scheduleSave()
  }
}

// ===== 节点增删 / 多选删除 / 复制粘贴 / 清空 =====
function removeNodes(ids: string[]) {
  if (!ids.length) return
  pushUndo()
  const set = new Set(ids)
  props.doc.nodes = props.doc.nodes.filter(n => !set.has(n.id))
  props.doc.edges = props.doc.edges.filter(e => !set.has(e.from) && !set.has(e.to))
  const next = new Set(selection.value)
  for (const id of ids) next.delete(id)
  selection.value = next
  scheduleSave()
}
function onClearBoard() {
  if (!props.doc.nodes.length && !props.doc.edges.length) return
  if (!window.confirm(t('canvas.clearConfirm'))) return
  pushUndo()
  props.doc.nodes = []
  props.doc.edges = []
  selection.value = new Set()
  scheduleSave()
}
let nodeClipboard: { nodes: CanvasNode[]; edges: CanvasEdge[] } | null = null
function copySelection() {
  if (!selection.value.size) return
  const nodes = props.doc.nodes.filter(n => selection.value.has(n.id)).map(n => JSON.parse(JSON.stringify(n)) as CanvasNode)
  const ids = new Set(nodes.map(n => n.id))
  const edges = props.doc.edges.filter(e => ids.has(e.from) && ids.has(e.to)).map(e => JSON.parse(JSON.stringify(e)) as CanvasEdge)
  nodeClipboard = { nodes, edges }
  appStore.showSuccess(t('canvas.copiedNodes', { n: nodes.length }))
}
function pasteNodes() {
  if (!nodeClipboard?.nodes.length) return
  pushUndo()
  const idMap = new Map<string, string>()
  const pasted: CanvasNode[] = []
  for (const n of nodeClipboard.nodes) {
    const nid = uid()
    idMap.set(n.id, nid)
    pasted.push({ ...n, id: nid, x: n.x + 24, y: n.y + 24 })
  }
  const edges = nodeClipboard.edges.map(e => ({ id: uid(), from: idMap.get(e.from) || e.from, to: idMap.get(e.to) || e.to }))
  props.doc.nodes.push(...pasted)
  props.doc.edges.push(...edges)
  selection.value = new Set(pasted.map(n => n.id))
  for (const n of pasted) if (n.type === 'image') void nodeSrc(n)
  scheduleSave()
}

// ===== 键盘 =====
let keydownHandler: ((e: KeyboardEvent) => void) | null = null
function isTypingTarget(t: EventTarget | null): boolean {
  const el = t as HTMLElement
  return !!el && (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT' || el.isContentEditable)
}
function onKeydown(e: KeyboardEvent) {
  if (e.key === ' ' && !isTypingTarget(e.target)) {
    spaceHeld = true
    return
  }
  if (isTypingTarget(e.target)) return
  const mod = e.ctrlKey || e.metaKey
  if (mod && !e.shiftKey && e.key.toLowerCase() === 'z') { e.preventDefault(); undo(); return }
  if ((mod && e.shiftKey && e.key.toLowerCase() === 'z') || (mod && e.key.toLowerCase() === 'y')) { e.preventDefault(); redo(); return }
  if (mod && e.key.toLowerCase() === 'c') { copySelection(); return }
  if (mod && e.key.toLowerCase() === 'a') { e.preventDefault(); selection.value = new Set(props.doc.nodes.map(n => n.id)); return }
  if (mod && e.key.toLowerCase() === 'v') {
    // 内部节点剪贴板优先;粘贴外部文本/图片由 paste 事件处理
    if (nodeClipboard?.nodes.length) { e.preventDefault(); pasteNodes() }
    return
  }
  if (e.key === 'Delete' || e.key === 'Backspace') {
    if (selection.value.size) { e.preventDefault(); removeNodes([...selection.value]) }
    return
  }
  if (e.key === 'Escape') {
    selection.value = new Set()
    linkMode.value = false
    linkFrom.value = null
    showShortcuts.value = false
    return
  }
}
function onPaste(e: ClipboardEvent) {
  if (isTypingTarget(e.target)) return
  const dt = e.clipboardData
  if (!dt) return
  const files = Array.from(dt.files || []).filter(f => f.type.startsWith('image/'))
  if (files.length) {
    void addImageNodes(files)
    return
  }
  const text = dt.getData('text/plain') || ''
  if (!text.trim()) return
  pushUndo()
  const rect = viewportEl.value?.getBoundingClientRect()
  const cx = ((rect?.width || 800) / 2 - viewport.x) / viewport.scale
  const cy = ((rect?.height || 600) / 2 - viewport.y) / viewport.scale
  props.doc.nodes.push({ id: uid(), type: 'text', x: cx - 110, y: cy - 70, w: 220, h: 140, text: text.slice(0, 4000) })
  scheduleSave()
}
function onKeyUp(e: KeyboardEvent) {
  if (e.key === ' ') spaceHeld = false
}

// ===== 节点增删 =====
function addTextNode() {
  const rect = viewportEl.value?.getBoundingClientRect()
  const cx = ((rect?.width || 800) / 2 - viewport.x) / viewport.scale
  const cy = ((rect?.height || 600) / 2 - viewport.y) / viewport.scale
  pushUndo()
  props.doc.nodes.push({ id: uid(), type: 'text', x: cx - 110, y: cy - 70, w: 220, h: 140, text: '' })
  selection.value = new Set([props.doc.nodes[props.doc.nodes.length - 1].id])
  scheduleSave()
}
async function addImageNodes(files: File[]) {
  pushUndo()
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
    pushUndo()
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
  if (images.length || videos.length) pushUndo()
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
  keydownHandler = onKeydown
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('keyup', onKeyUp)
  window.addEventListener('paste', onPaste)
})
onUnmounted(() => {
  window.removeEventListener('pagehide', onPageHide)
  if (keydownHandler) window.removeEventListener('keydown', keydownHandler)
  window.removeEventListener('keyup', onKeyUp)
  window.removeEventListener('paste', onPaste)
  flushSave()
})
</script>
