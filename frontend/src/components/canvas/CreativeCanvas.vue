<template>
  <section class="creative-canvas" :aria-label="canvasT('board.title')">
    <div class="creative-canvas-toolbar">
      <div class="creative-canvas-add">
        <button type="button" :disabled="readonly" @click="emit('add', 'text')">+ {{ canvasT('board.text') }}</button>
        <button type="button" :disabled="readonly" @click="emit('add', 'image')">+ {{ canvasT('board.image') }}</button>
        <button type="button" :disabled="readonly" @click="emit('add', 'video')">+ {{ canvasT('board.video') }}</button>
      </div>
      <button type="button" :disabled="readonly" @click="emit('openAssets')">{{ canvasT('board.assets') }}</button>
      <button type="button" class="creative-canvas-template" :disabled="readonly" @click="emit('template')">{{ canvasT('board.template') }}</button>
      <div class="creative-canvas-view-actions">
        <button type="button" :disabled="readonly || !nodes.length" @click="emit('arrange')">{{ canvasT('board.arrange') }}</button>
        <button type="button" :disabled="!nodes.length" @click="fitView()">⛶ {{ canvasT('board.fit') }}</button>
      </div>
    </div>

    <div class="creative-canvas-surface">
      <VueFlow
        :id="flowId"
        v-model:nodes="nodesModel"
        v-model:edges="edgesModel"
        :min-zoom="0.15"
        :max-zoom="2.5"
        :nodes-draggable="!readonly"
        :nodes-connectable="!readonly"
        :selection-key-code="true"
        :multi-selection-key-code="['Shift', 'Control', 'Meta']"
        :pan-on-drag="[1, 2]"
        :edges-updatable="false"
        :delete-key-code="readonly ? null : ['Backspace', 'Delete']"
        :default-viewport="viewport"
        :snap-to-grid="true"
        :snap-grid="[12, 12]"
        :default-edge-options="{ markerEnd: MarkerType.ArrowClosed }"
        :connection-line-style="{ stroke: '#14b8a6', strokeWidth: 2 }"
        :only-render-visible-elements="true"
        @connect="onConnect"
        @node-click="emit('select', $event.node.id)"
        @pane-click="emit('select', null)"
        @viewport-change-end="emit('update:viewport', $event)"
      >
        <template #edge-smoothstep="edgeProps"><RoutedCanvasEdge v-bind="edgeProps" /></template>
        <Background :gap="24" :size="1" pattern-color="#94a3b860" />
        <Controls position="bottom-right" :show-interactive="false" />
        <template v-for="kind in kinds" :key="kind" #[`node-${kind}`]="{ id, data, selected }">
          <CreativeNodeCard
            :kind="kind"
            :data="data"
            :selected="id === selectedNodeId || selected"
            :readonly="readonly"
            @run="emit('run', id)"
            @derive="emit('derive', { id, kind: $event })"
            @duplicate="emit('duplicate', id)"
            @remove="emit('remove', id)"
            @edit-prompt="emit('edit', { id, prompt: $event })"
            @preview="emit('preview', id)"
          />
        </template>
      </VueFlow>

      <div v-if="!nodes.length" class="creative-canvas-empty">
        <div class="creative-canvas-empty-art" aria-hidden="true">
          <span>▧</span><i></i><span>▧</span><i></i><span>▷</span>
        </div>
        <h2>{{ canvasT('board.emptyTitle') }}</h2>
        <p>{{ canvasT('board.emptyHint') }}</p>
        <button type="button" :disabled="readonly" @click="emit('template')">+ {{ canvasT('board.emptyAction') }}</button>
      </div>
      <div v-if="connectionError" role="alert" class="creative-canvas-error">{{ canvasT(`board.connection.${connectionError}`) }}</div>
      <div v-if="nodes.length" class="creative-canvas-count">{{ canvasT('board.count', { n: nodes.length }) }}</div>
    </div>
    <p class="creative-canvas-hint">{{ canvasT('board.hint') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueFlow, MarkerType, useVueFlow, type Connection } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import type {
  CreativeCanvasConnectionError, CreativeCanvasEdge, CreativeCanvasKind,
  CreativeCanvasNode, CreativeCanvasViewport,
} from '@/types/creativeCanvas'
import { validateCreativeCanvasConnection } from '@/types/creativeCanvas'
import CreativeNodeCard from './CreativeCanvasNode.vue'
import RoutedCanvasEdge from './CreativeCanvasEdge.vue'
import { creativeCanvasMessages } from './creativeCanvasMessages'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'

const props = withDefaults(defineProps<{
  nodes: CreativeCanvasNode[]
  edges: CreativeCanvasEdge[]
  selectedNodeId?: string | null
  viewport?: CreativeCanvasViewport
  readonly?: boolean
}>(), { selectedNodeId: null, viewport: () => ({ x: 0, y: 0, zoom: 1 }), readonly: false })

const emit = defineEmits<{
  'update:nodes': [nodes: CreativeCanvasNode[]]
  'update:edges': [edges: CreativeCanvasEdge[]]
  'update:viewport': [viewport: CreativeCanvasViewport]
  select: [id: string | null]
  add: [kind: CreativeCanvasKind]
  openAssets: []
  template: []
  arrange: []
  run: [id: string]
  duplicate: [id: string]
  remove: [id: string]
  derive: [request: { id: string; kind: 'image' | 'video' }]
  edit: [request: { id: string; prompt: string }]
  preview: [id: string]
}>()

const { t: canvasT } = useI18n({ useScope: 'local', messages: creativeCanvasMessages })
const kinds: CreativeCanvasKind[] = ['text', 'image', 'video']
const flowId = `creative-canvas-${Math.random().toString(36).slice(2)}`
const nodesModel = computed({ get: () => props.nodes, set: value => emit('update:nodes', value) })
const edgesModel = computed({ get: () => props.edges, set: value => emit('update:edges', value) })
const connectionError = ref<CreativeCanvasConnectionError | null>(null)
let errorTimer: ReturnType<typeof setTimeout> | undefined
const flow = useVueFlow({ id: flowId })

function fitView() {
  if (!props.nodes.length) return
  // New cards may not have measured dimensions until the next animation frame.
  // Bounds from card positions make template fitting deterministic on first use.
  const x = Math.min(...props.nodes.map(n => n.position.x))
  const y = Math.min(...props.nodes.map(n => n.position.y))
  const right = Math.max(...props.nodes.map(n => n.position.x + 286))
  const bottom = Math.max(...props.nodes.map(n => n.position.y + (n.type === 'text' ? 260 : 370)))
  return flow.fitBounds({ x, y, width: right - x, height: bottom - y }, { padding: 0.15, duration: 250 })
}

function ensureVisible() {
  const { width, height } = flow.dimensions.value
  const { x, y, zoom } = flow.viewport.value
  if (!width || !height || !props.nodes.length) return
  const visible = props.nodes.some(n => {
    const left = n.position.x * zoom + x
    const top = n.position.y * zoom + y
    return left < width && top < height && left + 286 * zoom > 0 && top + 370 * zoom > 0
  })
  if (!visible) void fitView()
}
flow.onPaneReady(() => { requestAnimationFrame(ensureVisible) })
watch(() => flow.dimensions.value.width, (width, previous) => {
  if (width > 0 && previous > 0 && Math.abs(width - previous) > 40) void fitView()
})

function onConnect(connection: Connection) {
  if (props.readonly) return
  connectionError.value = validateCreativeCanvasConnection(connection, props.nodes, props.edges)
  if (errorTimer) clearTimeout(errorTimer)
  if (connectionError.value) {
    errorTimer = setTimeout(() => { connectionError.value = null }, 5000)
    return
  }
  const source = props.nodes.find(node => node.id === connection.source)!
  emit('update:edges', [...props.edges, {
    ...connection,
    id: `edge-${connection.source}-${connection.target}`,
    label: source.data.kind === 'text' ? canvasT('board.promptInput') : canvasT('board.referenceInput'),
    type: 'smoothstep',
    markerEnd: MarkerType.ArrowClosed,
    style: { stroke: '#94a3b8', strokeWidth: 1.5 },
    labelStyle: { fill: '#64748b', fontSize: 10 },
    labelBgStyle: { fill: '#f8fafc', fillOpacity: 0.95 },
  }])
}

onBeforeUnmount(() => { if (errorTimer) clearTimeout(errorTimer) })
function focusNode(id: string) {
  const node = props.nodes.find(n => n.id === id)
  if (node) return flow.setCenter(node.position.x + 143, node.position.y + 170, { zoom: 1, duration: 250 })
}
defineExpose({ fitView, focusNode })
</script>

<style scoped>
.creative-canvas { display: flex; flex-direction: column; height: 100%; min-height: 460px; overflow: hidden; background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 12px; }
.creative-canvas-toolbar { z-index: 2; display: flex; flex-shrink: 0; flex-wrap: wrap; align-items: center; gap: 6px; border-bottom: 1px solid #e2e8f0; padding: 9px 10px; background: #ffffffed; }
.creative-canvas-toolbar button { border-radius: 6px; padding: 6px 9px; color: #475569; font-size: 11px; white-space: nowrap; }
.creative-canvas-toolbar button:hover { background: #f1f5f9; color: #0f172a; }
.creative-canvas-toolbar .creative-canvas-template { background: #f0fdfa; color: #0f766e; }
.creative-canvas-add { display: flex; align-items: center; border: 1px solid #e2e8f0; border-radius: 7px; }
.creative-canvas-view-actions { display: flex; margin-left: auto; }
.creative-canvas-surface { position: relative; flex: 1; min-height: 360px; }
.creative-canvas-empty { position: absolute; left: 50%; top: 50%; transform: translate(-50%, -50%); width: min(440px, 90%); text-align: center; pointer-events: none; }
.creative-canvas-empty-art { display: flex; align-items: center; justify-content: center; margin-bottom: 24px; color: #14b8a6; }
.creative-canvas-empty-art span { display: grid; place-items: center; width: 58px; height: 68px; border: 1px solid #cbd5e1; border-radius: 10px; background: #fff; font-size: 30px; box-shadow: 0 5px 15px #0f172a07; }
.creative-canvas-empty-art span:last-child { color: #8b5cf6; }
.creative-canvas-empty-art i { width: 26px; border-top: 1px dashed #94a3b8; }
.creative-canvas-empty h2 { margin: 0; font-size: 20px; font-weight: 600; letter-spacing: -.03em; color: #334155; }
.creative-canvas-empty p { margin: 12px auto 20px; max-width: 360px; font-size: 12px; line-height: 1.8; color: #64748b; }
.creative-canvas-empty button { pointer-events: auto; border-radius: 8px; background: #0f766e; padding: 10px 16px; color: #fff; font-size: 12px; }
.creative-canvas-hint { flex-shrink: 0; margin: 0; padding: 7px 12px; border-top: 1px solid #e2e8f0; color: #94a3b8; font-size: 10px; }
.creative-canvas-count { position: absolute; left: 12px; bottom: 12px; padding: 5px 9px; border: 1px solid #e2e8f0; border-radius: 6px; background: #ffffffed; color: #64748b; font-size: 10px; pointer-events: none; }
.creative-canvas-error { position: absolute; left: 50%; bottom: 18px; z-index: 3; transform: translateX(-50%); max-width: 80%; border: 1px solid #fecaca; border-radius: 8px; padding: 9px 14px; background: #fff1f2; color: #be123c; font-size: 12px; }
button:disabled { opacity: .45; cursor: not-allowed; }
.creative-canvas :deep(.vue-flow__handle) { width: 10px; height: 10px; border: 2px solid #fff; background: #14b8a6; }
.creative-canvas :deep(.vue-flow__node-video .vue-flow__handle) { background: #8b5cf6; }
.creative-canvas :deep(.vue-flow__edge.selected .vue-flow__edge-path) { stroke: #14b8a6 !important; stroke-width: 2.5 !important; }
.creative-canvas :deep(.vue-flow__controls) { overflow: hidden; border: 1px solid #e2e8f0; border-radius: 8px; box-shadow: 0 3px 9px #0f172a08; }
:global(.dark) .creative-canvas { border-color: #334155; background: #0f1721; }
:global(.dark) .creative-canvas-toolbar { border-color: #334155; background: #17202eed; }
:global(.dark) .creative-canvas-toolbar button { color: #94a3b8; }
:global(.dark) .creative-canvas-toolbar button:hover { color: #e2e8f0; background: #263244; }
:global(.dark) .creative-canvas-toolbar .creative-canvas-template { background: #103735; color: #5eead4; }
:global(.dark) .creative-canvas-add { border-color: #334155; }
:global(.dark) .creative-canvas-empty-art span { border-color: #334155; background: #17202e; }
:global(.dark) .creative-canvas-empty h2 { color: #e2e8f0; }
:global(.dark) .creative-canvas-empty p { color: #94a3b8; }
:global(.dark) .creative-canvas-hint { border-color: #334155; }
:global(.dark) .creative-canvas-count { border-color: #334155; background: #17202e; color: #94a3b8; }
:global(.dark) .creative-canvas :deep(.vue-flow__handle) { border-color: #17202e; }
:global(.dark) .creative-canvas :deep(.vue-flow__controls) { border-color: #334155; }
:global(.dark) .creative-canvas :deep(.vue-flow__controls-button) { border-color: #334155; background: #17202e; fill: #94a3b8; }
</style>
