<template>
  <article class="creative-node" :class="[{ 'creative-node-selected': selected, 'creative-node-active': active }, `creative-node-${kind}`]" :aria-label="data.title || canvasT(`board.${kind}`)">
    <Handle v-if="kind !== 'text'" type="target" :position="Position.Left" :connectable="!readonly" />
    <header class="creative-node-header">
      <span class="creative-node-kind">{{ canvasT(`board.${kind}`) }}</span>
      <span class="creative-node-title">{{ data.title || canvasT(`board.${kind}`) }}</span>
      <button type="button" class="nodrag creative-node-icon" :disabled="readonly" :aria-label="canvasT('board.duplicate')" :title="canvasT('board.duplicate')" @click.stop="emit('duplicate')">⧉</button>
      <button type="button" class="nodrag creative-node-icon creative-node-delete" :disabled="readonly || active" :aria-label="canvasT('board.remove')" :title="canvasT('board.remove')" @click.stop="emit('remove')">×</button>
    </header>

    <textarea
      v-if="kind === 'text'"
      :value="data.prompt"
      :disabled="readonly"
      :aria-label="canvasT('board.textReady')"
      :placeholder="canvasT('board.prompt')"
      class="nodrag nowheel creative-node-text"
      maxlength="4000"
      @input="emit('editPrompt', ($event.target as HTMLTextAreaElement).value)"
      @keydown.stop
    ></textarea>
    <div v-else class="creative-node-preview">
      <button v-if="kind === 'image' && data.previewUrl" type="button" class="nodrag creative-node-image-button" :aria-label="canvasT('board.preview')" @click.stop="emit('preview')">
        <img :src="data.previewUrl" :alt="data.title" draggable="false" loading="lazy" />
      </button>
      <video v-else-if="kind === 'video' && data.previewUrl" :src="data.previewUrl" class="nodrag nowheel" controls playsinline preload="metadata" @keydown.stop />
      <div v-else class="creative-node-placeholder">
        <span v-if="active" class="creative-node-spinner" aria-hidden="true"></span>
        <svg v-else-if="kind === 'image'" width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.3" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="3" /><circle cx="8.5" cy="8.5" r="1.5" /><path d="m3 17 5-5 4 4 4-6 5 7" /></svg>
        <svg v-else width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.3" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="3" /><path d="m10 8 6 4-6 4z" /></svg>
        <span>{{ active ? statusLabel : canvasT('board.noPreview') }}</span>
        <button v-if="kind === 'video' && (data.assetId || data.resultId)" type="button" class="nodrag creative-node-action creative-node-video-action" @click.stop="emit('preview')">▶ {{ canvasT('board.loadVideo') }}</button>
      </div>
      <span v-if="data.status" class="creative-node-status" :class="`creative-node-status-${data.status}`">{{ statusLabel }}</span>
    </div>

    <div v-if="kind !== 'text'" class="creative-node-description">
      <p v-if="data.prompt" :title="data.prompt">{{ data.prompt }}</p>
      <p v-else class="creative-node-prompt-hint">{{ canvasT('board.prompt') }}</p>
      <p v-if="data.error" class="creative-node-error" :title="data.error">{{ data.error }}</p>
    </div>

    <footer class="creative-node-actions nodrag">
      <button v-if="kind !== 'text'" type="button" class="creative-node-action creative-node-run" :disabled="readonly || active" @click.stop="runOrReview">{{ runLabel }}</button>
      <button v-if="kind !== 'text' && data.assetId" type="button" class="creative-node-action creative-node-timeline-action" :disabled="readonly" draggable="true" @dragstart.stop="emit('dragTimeline', $event)" @click.stop="emit('addTimeline')">+ {{ canvasT('board.timeline') }}</button>
      <button v-if="kind === 'text'" type="button" class="creative-node-action" :disabled="readonly" @click.stop="emit('derive', 'image')">+ {{ canvasT('board.fromText') }}</button>
      <template v-if="kind === 'image'">
        <button type="button" class="creative-node-action" :disabled="readonly" @click.stop="emit('derive', 'image')">+ {{ canvasT('board.details') }}</button>
        <button type="button" class="creative-node-action creative-node-video-action" :disabled="readonly" @click.stop="emit('derive', 'video')">{{ canvasT('board.toVideo') }} →</button>
      </template>
    </footer>
    <Handle v-if="kind !== 'video'" type="source" :position="Position.Right" :connectable="!readonly" />
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Handle, Position } from '@vue-flow/core'
import type { CreativeCanvasKind, CreativeCanvasNodeData } from '@/types/creativeCanvas'
import { creativeCanvasMessages } from './creativeCanvasMessages'

const props = withDefaults(defineProps<{
  kind: CreativeCanvasKind
  data: CreativeCanvasNodeData
  selected?: boolean
  readonly?: boolean
}>(), { selected: false, readonly: false })
const emit = defineEmits<{
  run: []
  review: []
  derive: [kind: 'image' | 'video']
  duplicate: []
  remove: []
  editPrompt: [prompt: string]
  preview: []
  addTimeline: []
  dragTimeline: [event: DragEvent]
}>()
const { t: canvasT } = useI18n({ useScope: 'local', messages: creativeCanvasMessages })
const active = computed(() => ['running', 'queued', 'waiting'].includes(props.data.status || ''))
const statusLabel = computed(() => canvasT(`board.${props.data.status || 'draft'}`))
const runLabel = computed(() => props.data.status === 'interrupted'
  ? canvasT('board.reviewRetry')
  : props.data.status === 'failed' || props.data.status === 'cancelled'
    ? canvasT('board.retry')
    : props.data.assetId ? canvasT('board.regenerate') : active.value ? statusLabel.value : canvasT('board.run'))
function runOrReview() {
  if (props.data.status === 'interrupted') emit('review')
  else emit('run')
}
</script>

<style scoped>
.creative-node { width: 286px; overflow: visible; border: 1px solid #dbe2e9; border-radius: 14px; background: #fff; color: #1e293b; box-shadow: 0 3px 14px #0f172a09; transition: border-color .15s, box-shadow .15s; }
.creative-node:hover { border-color: #94a3b8; }
.creative-node-selected { border-color: #14b8a6; box-shadow: 0 0 0 2px #14b8a62b, 0 5px 18px #0f172a0d; }
.creative-node-header { display: flex; align-items: center; gap: 7px; padding: 11px 12px; cursor: grab; }
.creative-node-kind { flex-shrink: 0; border-radius: 5px; padding: 2px 5px; background: #e0f2fe; color: #0369a1; font-size: 10px; font-weight: 600; }
.creative-node-video .creative-node-kind { background: #ede9fe; color: #7c3aed; }
.creative-node-text .creative-node-kind { background: #fef3c7; color: #92400e; }
.creative-node-title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; font-weight: 600; }
.creative-node-icon { flex-shrink: 0; border-radius: 4px; width: 22px; height: 22px; color: #94a3b8; font-size: 17px; }
.creative-node-icon:hover { background: #f1f5f9; color: #475569; }
.creative-node-delete:hover { color: #ef4444; }
.creative-node-preview { position: relative; height: 184px; margin: 0 9px; overflow: hidden; border-radius: 8px; background: #f1f5f9; }
.creative-node-preview img, .creative-node-preview video { display: block; width: 100%; height: 100%; object-fit: contain; }
.creative-node-preview video { background: #0f172a; }
.creative-node-image-button { display: block; width: 100%; height: 100%; cursor: zoom-in; }
.creative-node-placeholder { height: 100%; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 11px; color: #94a3b8; font-size: 11px; }
.creative-node-status { position: absolute; right: 7px; top: 7px; padding: 3px 7px; border-radius: 5px; background: #ffffffed; color: #64748b; font-size: 10px; box-shadow: 0 1px 4px #0000000c; pointer-events: none; }
.creative-node-status-running, .creative-node-status-queued, .creative-node-status-waiting { color: #0f766e; background: #f0fdfa; }
.creative-node-status-failed { color: #b91c1c; background: #fef2f2; }
.creative-node-status-succeeded { color: #047857; background: #ecfdf5; }
.creative-node-description { padding: 10px 12px 0; }
.creative-node-description p { display: -webkit-box; overflow: hidden; -webkit-line-clamp: 2; -webkit-box-orient: vertical; margin: 0; min-height: 30px; font-size: 11px; line-height: 1.55; color: #64748b; white-space: pre-wrap; }
.creative-node-description .creative-node-error { color: #dc2626; margin-top: 5px; }
.creative-node-description .creative-node-prompt-hint { color: #94a3b8; }
textarea.creative-node-text { display: block; width: calc(100% - 24px); min-height: 146px; margin: 0 12px; resize: vertical; padding: 4px 0; background: transparent; font-size: 12px; line-height: 1.65; outline: none; color: inherit; }
.creative-node-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; padding: 10px; }
.creative-node-action { border-radius: 5px; padding: 5px 7px; color: #64748b; font-size: 10px; line-height: 1.4; }
.creative-node-action:hover { background: #f1f5f9; color: #0f172a; }
.creative-node-run { background: #f0fdfa; color: #0f766e; }
.creative-node-video-action { margin-left: auto; color: #7c3aed; }
button:disabled, textarea:disabled { opacity: .45; cursor: not-allowed; }
.creative-node-spinner { width: 25px; height: 25px; border: 2px solid #ccfbf1; border-top-color: #14b8a6; border-radius: 50%; animation: creative-canvas-spin 1s linear infinite; }
@keyframes creative-canvas-spin { to { transform: rotate(360deg); } }
:global(.dark) .creative-node { border-color: #334155; background: #17202e; color: #e2e8f0; }
:global(.dark) .creative-node-selected { border-color: #2dd4bf; }
:global(.dark) .creative-node-preview { background: #101827; }
:global(.dark) .creative-node-description p { color: #94a3b8; }
:global(.dark) .creative-node-action:hover, :global(.dark) .creative-node-icon:hover { background: #263244; color: #e2e8f0; }
:global(.dark) .creative-node-run { background: #0f3836; color: #5eead4; }
</style>
