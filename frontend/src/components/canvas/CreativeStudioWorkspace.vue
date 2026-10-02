<template>
  <section class="creative-workspace" :aria-label="tr('workspace')">
    <header class="project-bar">
      <div class="project-identity">
        <span class="project-icon" aria-hidden="true">◈</span>
        <select :value="project?.id || ''" :aria-label="tr('selectProject')" :disabled="busy || loading" @change="open(($event.target as HTMLSelectElement).value)">
          <option v-if="!projects.length" value="">{{ tr('project') }}</option>
          <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
        <input v-if="project" v-model="project.name" :aria-label="tr('rename')" :placeholder="tr('unnamed')" maxlength="80" class="project-name" />
        <button type="button" :disabled="busy || loading" @click="create">+ {{ tr('newProject') }}</button>
      </div>
      <div class="project-actions">
        <button v-if="project" type="button" :disabled="busy || loading" :title="tr('deleteProject')" @click="deleteDialog?.showModal()">{{ tr('deleteProject') }}</button>
        <span class="save-status" :class="{ warning: saveState === 'saveError' || saveState === 'conflict' }" :title="tr('cloud')">{{ tr(saveState) }}</span>
        <button v-if="saveState === 'saveError'" type="button" @click="save">{{ tr('tryAgain') }}</button>
        <span v-if="running.length" class="task-count">{{ tr('background') }} · {{ running.length }}</span>
        <button type="button" class="primary" :disabled="!project || busy || loading || (saveState === 'conflict' && !pendingRun)" @click="run()">▶ {{ pendingRun ? tr('tryAgain') : tr('runPending') }}</button>
      </div>
    </header>

    <div v-if="saveState === 'conflict'" role="alert" class="message warning">
      <span>{{ tr('conflict') }}</span><button type="button" @click="exportDraft">{{ tr('exportDraft') }}</button>
      <button type="button" :disabled="busy" @click="project && open(project.id, true)">{{ tr('reload') }}</button>
    </div>
    <div v-if="error" role="alert" class="message error"><span>{{ error }}</span><button v-if="!project" type="button" @click="initialize">{{ tr('tryAgain') }}</button><button type="button" :aria-label="tr('close')" @click="error = ''">×</button></div>
    <p v-if="notice" class="message notice" role="status">{{ notice }}</p>
    <p v-if="pendingRun" class="message warning">{{ tr('requestRetry') }}</p>
    <div v-if="loading" class="loading" role="status">{{ tr('loading') }}</div>

    <template v-if="project && !loading">
      <div class="workspace-grid">
        <CreativeCanvas
          :key="project.id" ref="board" v-model:nodes="project.nodes" v-model:edges="project.edges"
          v-model:viewport="project.viewport" :selected-node-id="selectedId" :readonly="saveState === 'conflict'"
          @select="selectedId = $event" @add="add($event)" @open-assets="showAssets"
          @template="addTemplate" @arrange="arrangeBoard" @run="runNode($event)" @duplicate="duplicate"
          @remove="remove" @derive="derive($event.id, $event.kind)" @edit="editPrompt" @preview="previewNode"
        />
        <aside class="inspector" :aria-label="tr('inspector')">
          <div class="panel-heading"><h2>{{ tr('inspector') }}</h2><span v-if="selected" class="kind-tag">{{ tr(selected.type === 'text' ? 'text' : selected.type === 'image' ? 'images' : 'videos') }}</span></div>
          <p v-if="!selected" class="empty-panel">{{ tr('selectHint') }}</p>
          <template v-else>
            <label>{{ tr('title') }}<input v-model="selected.data.title" maxlength="80" /></label>
            <label>{{ tr(selected.type === 'text' ? 'textPrompt' : selected.type === 'video' ? 'motion' : 'prompt') }}
              <textarea v-model="selected.data.prompt" rows="5" maxlength="4000" :placeholder="tr(selected.type === 'video' ? 'motionPlaceholder' : 'promptPlaceholder')"></textarea>
            </label>
            <template v-if="selected.type !== 'text' && selectedParams">
              <label>{{ tr('model') }}
                <select :value="modelKey(selectedParams)" @change="selectModel(($event.target as HTMLSelectElement).value)">
                  <option v-if="!selectedModels.some(m => modelKey(m) === modelKey(selectedParams!))" :value="modelKey(selectedParams)">{{ selectedParams.model || tr('noModel') }}</option>
                  <option v-for="m in selectedModels" :key="modelKey(m)" :value="modelKey(m)">{{ m.id }} · {{ m.group_name }}</option>
                </select>
              </label>
              <p v-if="!selectedModels.length" class="warning">{{ tr('noModel') }}</p>
              <div v-if="selected.type === 'image' && !grokSelected" class="field-grid">
                <label>{{ tr('size') }}<select v-model="selectedParams.size"><option value="auto">{{ tr('auto') }}</option><option v-for="size in ['1024x1024', '1536x1024', '1024x1536', '1792x1024', '1024x1792']" :key="size">{{ size }}</option></select></label>
                <label>{{ tr('quality') }}<select v-model="selectedParams.quality"><option v-for="q in ['auto', 'high', 'medium', 'low']" :key="q" :value="q">{{ tr(q) }}</option></select></label>
              </div>
              <div v-if="selected.type === 'video'" class="field-grid">
                <label>{{ tr('seconds') }}<input v-model.number="selectedParams.seconds" type="number" min="1" max="30" step="1" /></label>
                <label>{{ tr('resolution') }}<select v-model="selectedParams.resolution"><option v-for="res in ['480p','720p','1080p']" :key="res">{{ res }}</option></select></label>
                <label>{{ tr('size') }}<select v-model="selectedParams.size"><option value="auto">{{ tr('auto') }}</option><option v-for="size in ['1280x720','720x1280','1024x1024']" :key="size">{{ size }}</option></select></label>
              </div>
              <h3>{{ tr('refs') }}</h3>
              <p v-if="!references.length" class="muted">{{ tr('noRefs') }}</p>
              <div v-for="reference in references" :key="reference.id" class="reference-row"><span>{{ reference.data.title }}</span><button type="button" :aria-label="tr('remove')" @click="disconnect(reference.id)">×</button></div>
              <p v-if="selected.type === 'video'" class="muted">{{ tr('singleImage') }}</p>
              <p v-if="selected.data.error" class="error">{{ selected.data.error }}</p>
              <p v-if="selectedJob?.status === 'interrupted'" class="warning">{{ tr('interruptedHint') }}</p>
              <button v-if="!selectedJob || !['failed','cancelled','interrupted'].includes(selectedJob.status)" type="button" class="primary full" :disabled="busy || selectedActive || saveState === 'conflict'" @click="runNode(selected!.id)">▶ {{ selected.data.assetId ? tr('rerun') : tr('run') }}</button>
              <button v-if="selectedJob && ['failed','cancelled','interrupted'].includes(selectedJob.status)" type="button" class="full" :disabled="busy" @click="retrySelected">{{ selectedJob.status === 'interrupted' ? tr('reviewedRetry') : tr('retry') }}</button>
              <p class="muted">{{ tr('runHint') }}</p>
              <template v-if="selected.data.assetId">
                <div class="button-row"><button type="button" @click="previewNode(selected.id)">{{ tr('preview') }}</button><button type="button" :disabled="busy" @click="downloadById(selected.data.assetId!)">{{ tr('download') }}</button></div>
                <button type="button" class="full" @click="addTimeline(selected.data.assetId!, selected.data.title)">+ {{ tr('addTimeline') }}</button>
              </template>
              <div v-if="selected.type === 'image'" class="button-row"><button type="button" @click="derive(selected!.id, 'image')">+ {{ tr('useAsImage') }}</button><button type="button" @click="derive(selected!.id, 'video')">▷ {{ tr('useAsVideo') }}</button></div>
              <template v-if="versions.length">
                <h3>{{ tr('versions') }}</h3><p class="muted">{{ tr('keepVersions') }}</p>
                <button v-for="(asset, index) in versions" :key="asset.id" type="button" class="version" :class="{ chosen: selected.data.assetId === asset.id }" @click="useVersion(asset)">
                  <span>v{{ versions.length - index }} · {{ date(asset.created_at) }}</span><span>{{ selected.data.assetId === asset.id ? '✓' : tr('useVersion') }}</span>
                </button>
              </template>
            </template>
          </template>
          <button v-if="selectedImages.length > 1" type="button" class="full" @click="selectedImages.forEach(n => derive(n.id, 'video'))">{{ tr('multiVideos') }} · {{ selectedImages.length }}</button>
        </aside>
      </div>

      <details class="timeline-panel" open>
        <summary><span>▤ {{ tr('timeline') }}</span><span class="muted">{{ timeline.clips.length }} {{ tr('clip') }} · {{ totalDuration }}s</span></summary>
        <div class="timeline-controls">
          <select v-model="timeline.size" :aria-label="tr('size')"><option value="1280x720">16:9 · 720p</option><option value="720x1280">9:16 · 720p</option><option value="720x720">1:1 · 720p</option></select>
          <label class="soundtrack">♫ {{ tr('soundtrack') }}<select v-model="timeline.audio_asset_id"><option value="">{{ tr('noAudio') }}</option><option v-for="asset in audioAssets" :key="asset.id" :value="asset.id">{{ asset.name }}</option></select></label>
          <button type="button" :disabled="busy" @click="audioInput?.click()">+ {{ tr('uploadAudio') }}</button>
          <button type="button" class="primary export-button" :disabled="busy || !timeline.clips.length || saveState === 'conflict'" @click="render">↓ {{ tr('render') }}</button>
        </div>
        <p v-if="!timeline.clips.length" class="timeline-empty">{{ tr('timelineEmpty') }}</p>
        <div v-else class="clips">
          <article v-for="(clip, index) in timeline.clips" :key="clip.id" class="clip">
            <div class="clip-top"><span class="clip-number">{{ String(index + 1).padStart(2, '0') }}</span><span>{{ assetById(clip.asset_id)?.kind === 'image' ? '▧' : '▷' }}</span><button type="button" :disabled="index === 0" :aria-label="tr('moveLeft')" @click="moveClip(index, -1)">←</button><button type="button" :disabled="index === timeline.clips.length - 1" :aria-label="tr('moveRight')" @click="moveClip(index, 1)">→</button><button type="button" :aria-label="tr('remove')" @click="timeline.clips.splice(index, 1)">×</button></div>
            <img v-if="previewUrls[clip.asset_id] && assetById(clip.asset_id)?.kind === 'image'" :src="previewUrls[clip.asset_id]" :alt="clip.title" class="clip-thumb" />
            <p class="clip-asset" :title="assetById(clip.asset_id)?.name">{{ assetById(clip.asset_id)?.name }}</p>
            <label>{{ tr('caption') }}<input v-model="clip.title" maxlength="160" /></label>
            <div class="field-grid"><label>{{ tr('start') }}<input v-model.number="clip.start" type="number" min="0" max="3600" step="0.1" :disabled="assetById(clip.asset_id)?.kind === 'image'" /></label><label>{{ tr('duration') }}<input v-model.number="clip.duration" type="number" min="1" max="30" step="0.1" /></label></div>
          </article>
        </div>
        <p class="muted timeline-help">{{ tr('renderHint') }}</p>
        <div v-if="exports.length" class="export-list"><h3>{{ tr('exports') }}</h3><div v-for="job in exports" :key="job.id" class="job-row"><span>{{ date(job.created_at) }}</span><span>{{ tr(job.status) }}</span><span v-if="job.error" class="error">{{ job.error }}</span><button v-if="job.asset_id" type="button" @click="previewAsset(job.asset_id)">{{ tr('preview') }}</button><button v-if="job.asset_id" type="button" :disabled="busy" @click="downloadById(job.asset_id)">{{ tr('download') }}</button><button v-if="['failed','interrupted'].includes(job.status)" type="button" :disabled="busy" @click="jobAction(job, 'retry')">{{ tr('retry') }}</button></div></div>
      </details>
      <details v-if="jobs.some(j => j.kind !== 'compose')" class="task-panel"><summary>{{ tr('taskList') }} · {{ jobs.filter(j => j.kind !== 'compose').length }}</summary><div v-for="job in jobs.filter(j => j.kind !== 'compose').slice(0, 60)" :key="job.id" class="job-row"><button type="button" @click="selectedId = job.node_id">{{ project.nodes.find(n => n.id === job.node_id)?.data.title || job.node_id.slice(0, 8) }}</button><span>{{ tr(job.status) }}</span><span v-if="job.error" class="error">{{ job.error }}</span><button v-if="['waiting','queued'].includes(job.status)" type="button" :disabled="busy" @click="jobAction(job, 'cancel')">{{ tr('stopQueue') }}</button></div></details>
    </template>

    <input ref="fileInput" type="file" multiple class="hidden-input" accept="image/png,image/jpeg,image/webp,video/mp4,video/webm,audio/mpeg,audio/wav,audio/ogg" @change="uploadFiles" />
    <input ref="audioInput" type="file" class="hidden-input" accept="audio/mpeg,audio/wav,audio/ogg" @change="uploadFiles" />
    <dialog ref="deleteDialog" class="delete-dialog"><h2>{{ tr('deleteProject') }}</h2><p>{{ tr('deleteWarning') }}</p><div class="button-row"><button type="button" @click="deleteDialog?.close()">{{ tr('close') }}</button><button type="button" :disabled="busy || running.length > 0" @click="deleteDialog?.close(); deleteProject()">{{ tr('confirmDelete') }}</button></div><p v-if="running.length" class="muted">{{ tr('deleteBusy') }}</p></dialog>
    <dialog ref="assetDialog" class="asset-dialog">
      <div class="dialog-heading"><h2>{{ tr('assets') }}</h2><button type="button" :aria-label="tr('close')" @click="assetDialog?.close()">×</button></div>
      <div class="button-row"><button type="button" class="primary" :disabled="busy" @click="fileInput?.click()">+ {{ busy ? tr('uploading') : tr('upload') }}</button><button type="button" :disabled="localLoading" @click="loadLocalHistory">{{ tr('localHistory') }}</button></div>
      <p class="muted">{{ tr('supportedFiles') }} {{ tr('fileTooLarge') }}</p>
      <h3>{{ tr('projectAssets') }} · {{ assets.length }}</h3>
      <p v-if="!assets.length" class="muted">{{ tr('noAssets') }}</p>
      <div class="asset-grid"><article v-for="asset in assets" :key="asset.id" class="asset-tile"><img v-if="asset.kind === 'image' && previewUrls[asset.id]" :src="previewUrls[asset.id]" :alt="asset.name" loading="lazy" /><span v-else class="asset-symbol">{{ asset.kind === 'image' ? '▧' : asset.kind === 'video' ? '▷' : '♫' }}</span><p :title="asset.name">{{ asset.name }}</p><div class="button-row"><button v-if="asset.kind !== 'audio'" type="button" @click="addAssetNode(asset)">+ {{ tr('workspace') }}</button><button type="button" @click="previewAsset(asset.id)">{{ tr('preview') }}</button></div></article></div>
      <template v-if="localLoaded"><h3>{{ tr('localHistory') }}</h3><p v-if="!localItems.length" class="muted">{{ tr('noAssets') }}</p><div class="local-history"><label v-for="item in localItems" :key="item.id"><input v-model="localSelected" type="checkbox" :value="item.id" /><span>{{ item.kind === 'image' ? '▧' : '▷' }} {{ item.prompt.slice(0, 100) || item.name }}</span></label></div><button type="button" class="primary" :disabled="busy || !localSelected.length" @click="importLocal">{{ tr('importSelected') }} · {{ localSelected.length }}</button></template>
      <p v-if="error" role="alert" class="error">{{ error }}</p>
    </dialog>
    <dialog ref="previewDialog" class="preview-dialog" @cancel="closePreview"><div class="dialog-heading"><h2>{{ previewMedia?.name || tr('preview') }}</h2><button type="button" :aria-label="tr('close')" @click="closePreview">×</button></div><p v-if="previewLoading" role="status">{{ tr('loading') }}</p><template v-if="previewMedia && previewUrls[previewMedia.id]"><img v-if="previewMedia.kind === 'image'" :src="previewUrls[previewMedia.id]" :alt="previewMedia.name" /><video v-else-if="previewMedia.kind === 'video'" :src="previewUrls[previewMedia.id]" controls playsinline preload="metadata"></video><audio v-else :src="previewUrls[previewMedia.id]" controls></audio><button type="button" :disabled="busy" @click="download(previewMedia)">{{ tr('download') }}</button></template><p v-if="previewError" role="alert" class="error">{{ previewError }}</p></dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import CreativeCanvas from './CreativeCanvas.vue'
import { creativeStudioMessages } from './creativeStudioMessages'
import { useCreativeStudio } from '@/composables/useCreativeStudio'
import { isGrokImageModel } from '@/composables/studioModels'
import { sessionOwnerKey } from '@/composables/studioSession'
import type { StudioAsset, StudioModel, StudioParams } from '@/api/creativeStudio'

const { t: tr, locale } = useI18n({ useScope: 'local', messages: creativeStudioMessages })
const { projects, project, assets, jobs, imageModels, videoModels, selectedId, selected, loading, busy, error, notice, saveState, pendingRun, previewUrls, running, exports, timeline,
  initialize, create, deleteProject, open, save, preview, add, derive, template, remove, duplicate, arrange, run, runNode, jobAction, useVersion, addAssetNode, upload, addTimeline, render, download, exportDraft } = useCreativeStudio(tr)
const board = ref<InstanceType<typeof CreativeCanvas>>()
const fileInput = ref<HTMLInputElement>()
const audioInput = ref<HTMLInputElement>()
const deleteDialog = ref<HTMLDialogElement>()
const assetDialog = ref<HTMLDialogElement>()
const previewDialog = ref<HTMLDialogElement>()
const previewMedia = ref<StudioAsset>()
const previewLoading = ref(false)
const previewError = ref('')
const selectedParams = computed(() => selected.value?.data.params as StudioParams | undefined)
const selectedModels = computed(() => selected.value?.type === 'video' ? videoModels.value : imageModels.value)
const grokSelected = computed(() => selectedParams.value?.platform === 'grok' || isGrokImageModel(selectedParams.value?.model || ''))
const references = computed(() => project.value?.nodes.filter(n => project.value?.edges.some(e => e.source === n.id && e.target === selectedId.value)) || [])
const versions = computed(() => assets.value.filter(a => a.node_id === selectedId.value || a.id === selected.value?.data.assetId).sort((a, b) => b.created_at - a.created_at))
const selectedJob = computed(() => jobs.value.filter(j => j.node_id === selectedId.value).sort((a, b) => b.created_at - a.created_at || b.id.localeCompare(a.id))[0])
const selectedActive = computed(() => selectedJob.value && ['running','queued','waiting'].includes(selectedJob.value.status))
const selectedImages = computed(() => project.value?.nodes.filter(n => n.selected && n.type === 'image') || [])
const audioAssets = computed(() => assets.value.filter(a => a.kind === 'audio'))
const totalDuration = computed(() => Math.round(timeline.value.clips.reduce((sum, c) => sum + (Number(c.duration) || 0), 0) * 10) / 10)
const modelKey = (m: Pick<StudioModel, 'id' | 'group_id'> | StudioParams) => JSON.stringify([m.group_id, 'id' in m ? m.id : m.model])
const assetById = (id: string) => assets.value.find(a => a.id === id)
const date = (value: number) => new Date(value).toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
function selectModel(key: string) {
  const model = selectedModels.value.find(m => modelKey(m) === key)
  if (model && selectedParams.value) Object.assign(selectedParams.value, { model: model.id, group_id: model.group_id, platform: model.platform })
}
function disconnect(id: string) { if (project.value) project.value.edges = project.value.edges.filter(e => !(e.source === id && e.target === selectedId.value)) }
function editPrompt({ id, prompt }: { id: string; prompt: string }) {
  const node = project.value?.nodes.find(n => n.id === id)
  if (node) node.data.prompt = prompt
}
async function addTemplate() { template(); await nextTick(); void board.value?.fitView() }
async function arrangeBoard() { arrange(); await nextTick(); void board.value?.fitView() }
function moveClip(index: number, offset: number) {
  const clips = timeline.value.clips
  const [clip] = clips.splice(index, 1)
  if (clip) clips.splice(index + offset, 0, clip)
}
function retrySelected() { if (selectedJob.value) void jobAction(selectedJob.value, 'retry') }
async function uploadFiles(event: Event) {
  const input = event.target as HTMLInputElement
  await upload(Array.from(input.files || [])); input.value = ''
}
async function showAssets() {
  assetDialog.value?.showModal()
  const id = project.value?.id
  for (const asset of assets.value.filter(a => a.kind === 'image').slice(0, 60)) {
    if (project.value?.id !== id || !assetDialog.value?.open) return
    try { await preview(asset.id) } catch { /* allow individual preview failure */ }
  }
}
async function previewAsset(id: string) {
  previewMedia.value = assetById(id); previewError.value = ''; previewLoading.value = true
  previewDialog.value?.showModal()
  try { await preview(id) } catch (e) { previewError.value = (e as Error).message || tr('errorPrefix') }
  finally { previewLoading.value = false }
}
function previewNode(id: string) { const node = project.value?.nodes.find(n => n.id === id); if (node?.data.assetId) void previewAsset(node.data.assetId) }
function closePreview() { previewDialog.value?.querySelectorAll('video,audio').forEach(el => (el as HTMLMediaElement).pause()); previewDialog.value?.close() }
function downloadById(id: string) { const asset = assetById(id); if (asset) void download(asset) }
interface LocalItem { id: string; kind: 'image' | 'video'; name: string; prompt: string; blob: Blob }
const localItems = ref<LocalItem[]>([])
const localSelected = ref<string[]>([])
const localLoaded = ref(false)
const localLoading = ref(false)
let disposed = false
async function loadLocalHistory() {
  const owner = sessionOwnerKey()
  localLoading.value = true
  try {
    const [images, videos] = await Promise.all([import('@/composables/useImageStudioEngine'), import('@/composables/useVideoStudioEngine')])
    await Promise.all([images.loadHistory(), videos.loadHistory()])
    if (disposed || sessionOwnerKey() !== owner) return
    localItems.value = [
      ...images.history.value.flatMap(item => item.images.filter(image => !image.failed).map((image, index) => ({ id: `image-${item.id}-${index}`, kind: 'image' as const, name: `image-${item.id}-${index}.${image.blob.type.split('/')[1] || 'png'}`, prompt: item.prompt, blob: image.blob }))),
      ...videos.history.value.map(item => ({ id: `video-${item.id}`, kind: 'video' as const, name: `video-${item.id}.mp4`, prompt: item.prompt, blob: item.blob }))
    ]
    localLoaded.value = true
  } catch (e) { if (!disposed && sessionOwnerKey() === owner) error.value = (e as Error).message }
  finally { localLoading.value = false }
}
async function importLocal() {
  const items = localItems.value.filter(item => localSelected.value.includes(item.id))
  await upload(items.map(item => new File([item.blob], item.name, { type: item.blob.type })), items.map(item => item.prompt))
  if (!error.value) localSelected.value = []
}
function endSession() { localItems.value = []; localSelected.value = []; assetDialog.value?.close(); deleteDialog.value?.close(); closePreview(); previewMedia.value = undefined }
window.addEventListener('studio-session-ended', endSession)
onBeforeUnmount(() => { disposed = true; endSession(); window.removeEventListener('studio-session-ended', endSession) })
</script>

<style scoped>
.creative-workspace { --cs-bg: #fff; --cs-surface: #f8fafc; --cs-border: #e2e8f0; --cs-text: #334155; --cs-muted: #64748b; color: var(--cs-text); height: 100%; overflow-y: auto; padding-right: 2px; font-size: 12px; }
.project-bar, .project-identity, .project-actions, .button-row, .panel-heading, .dialog-heading, .clip-top, .timeline-controls, .job-row { display: flex; align-items: center; gap: 8px; }
.project-bar { justify-content: space-between; flex-wrap: wrap; margin-bottom: 12px; }
.project-identity { flex-wrap: wrap; min-width: 0; }
.project-icon { color: #0f766e; font-size: 25px; }
.project-name { max-width: 180px; }
.project-identity select { max-width: 160px; }
.project-actions { flex-wrap: wrap; margin-left: auto; }
.save-status, .muted { color: var(--cs-muted); font-size: 11px; line-height: 1.65; }
.task-count { color: #0f766e; padding: 4px 7px; background: #ccfbf144; border-radius: 5px; }
button { border: 1px solid var(--cs-border); border-radius: 7px; padding: 7px 10px; background: var(--cs-bg); color: var(--cs-text); font-size: 11px; line-height: 1.4; }
button:hover { border-color: #14b8a6; color: #0f766e; }
button.primary { background: #0f766e; border-color: #0f766e; color: #fff; }
button:disabled { opacity: .45; cursor: not-allowed; }
input, select, textarea { border: 1px solid var(--cs-border); border-radius: 6px; padding: 7px 8px; min-width: 0; width: 100%; background: var(--cs-bg); color: var(--cs-text); font-size: 12px; }
input:focus, select:focus, textarea:focus { outline: 2px solid #14b8a650; outline-offset: 1px; }
.project-bar input, .project-bar select, .timeline-controls select { width: auto; }
textarea { resize: vertical; line-height: 1.65; }
label { display: flex; flex-direction: column; gap: 6px; color: var(--cs-muted); font-size: 11px; }
h2 { margin: 0; font-size: 13px; font-weight: 600; }
h3 { margin: 18px 0 8px; font-size: 12px; font-weight: 600; }
p { margin: 8px 0; }
.workspace-grid { display: grid; grid-template-columns: minmax(0, 1fr) 282px; gap: 12px; min-height: 540px; height: min(720px, 68vh); }
.inspector { overflow-y: auto; padding: 15px; border: 1px solid var(--cs-border); border-radius: 12px; background: var(--cs-bg); }
.panel-heading { justify-content: space-between; margin-bottom: 16px; }
.inspector > label { margin-bottom: 14px; }
.inspector .field-grid { margin: 14px 0; }
.kind-tag { border-radius: 5px; padding: 3px 7px; color: #0f766e; background: #ccfbf144; font-size: 10px; }
.empty-panel { padding: 45px 8px; text-align: center; line-height: 1.9; color: var(--cs-muted); }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; }
.reference-row, .version { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.reference-row { padding: 5px 8px; background: var(--cs-surface); margin-bottom: 5px; border-radius: 5px; }
.reference-row button { border: 0; padding: 0 4px; background: transparent; }
.full { width: 100%; margin-top: 8px; }
.button-row { flex-wrap: wrap; margin: 8px 0; }
.button-row button { flex: 1; }
.version { width: 100%; font-size: 10px; margin-bottom: 5px; }
.version.chosen { border-color: #14b8a6; color: #0f766e; }
.message { display: flex; align-items: center; gap: 10px; padding: 9px 12px; border-radius: 7px; border: 1px solid var(--cs-border); margin: 8px 0; overflow-wrap: anywhere; }
.message > span:first-child { flex: 1; }
.error { color: #be123c; overflow-wrap: anywhere; }
.message.error { background: #fff1f2; border-color: #fecdd3; }
.warning { color: #b45309; }
.message.warning { background: #fffbeb; border-color: #fde68a; }
.notice { background: #f0fdfa; color: #0f766e; border-color: #99f6e4; }
.loading { padding: 70px 20px; text-align: center; color: var(--cs-muted); }
.timeline-panel, .task-panel { margin-top: 12px; border: 1px solid var(--cs-border); border-radius: 10px; background: var(--cs-bg); padding: 12px; }
summary { cursor: pointer; font-size: 13px; font-weight: 600; }
summary > span + span { margin-left: 14px; font-weight: 400; }
.timeline-controls { flex-wrap: wrap; margin: 13px 0; }
.soundtrack { flex-direction: row; align-items: center; }
.soundtrack select { max-width: 150px; }
.export-button { margin-left: auto; }
.timeline-empty { border: 1px dashed var(--cs-border); text-align: center; color: var(--cs-muted); padding: 22px 10px; border-radius: 8px; }
.clips { display: flex; gap: 10px; overflow-x: auto; padding-bottom: 10px; }
.clip { flex: 0 0 222px; padding: 10px; border: 1px solid var(--cs-border); border-radius: 9px; background: var(--cs-surface); }
.clip-number { font-size: 15px; font-weight: 600; color: #0f766e; margin-right: auto; }
.clip-top button { padding: 3px 7px; }
.clip-thumb { width: 100%; height: 92px; object-fit: cover; border-radius: 5px; margin-top: 8px; }
.clip-asset { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-size: 10px; color: var(--cs-muted); }
.clip .field-grid { margin-top: 8px; }
.timeline-help { margin-bottom: 0; }
.job-row { flex-wrap: wrap; border-top: 1px solid var(--cs-border); padding: 8px 0; font-size: 11px; }
.job-row .error { flex-basis: 100%; }
.job-row button { padding: 3px 8px; }
.hidden-input { display: none; }
dialog { border: 1px solid var(--cs-border); border-radius: 14px; padding: 22px; background: var(--cs-bg); color: var(--cs-text); width: min(850px, 94vw); max-height: 90vh; overflow: auto; }
dialog::backdrop { background: #0f172a80; backdrop-filter: blur(3px); }
.dialog-heading { justify-content: space-between; margin-bottom: 16px; }
.dialog-heading h2 { overflow-wrap: anywhere; }
.dialog-heading button { font-size: 18px; padding: 1px 9px; }
.asset-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); gap: 12px; }
.asset-tile { overflow: hidden; border: 1px solid var(--cs-border); border-radius: 9px; padding: 9px; }
.asset-tile img, .asset-symbol { width: 100%; height: 110px; object-fit: cover; border-radius: 5px; background: var(--cs-surface); }
.asset-symbol { display: grid; place-items: center; color: #14b8a6; font-size: 30px; }
.asset-tile p { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: 11px; }
.asset-tile button { padding: 5px; font-size: 10px; }
.local-history { max-height: 240px; overflow: auto; margin-bottom: 12px; }
.local-history label { flex-direction: row; padding: 9px 3px; border-bottom: 1px solid var(--cs-border); }
.local-history input { width: 15px; flex-shrink: 0; }
.preview-dialog img, .preview-dialog video { display: block; max-width: 100%; max-height: 66vh; margin: 0 auto 15px; border-radius: 8px; }
:global(.dark) .creative-workspace { --cs-bg: #17202e; --cs-surface: #101926; --cs-border: #334155; --cs-text: #cbd5e1; --cs-muted: #94a3b8; }
:global(.dark) .message.error { background: #471d2c; }
:global(.dark) .message.warning { background: #382d18; color: #fbbf24; }
:global(.dark) .notice { background: #103735; color: #5eead4; }
:global(.dark) .error { color: #fb7185; }
@media (max-width: 1050px) { .project-name { display: none; } .workspace-grid { grid-template-columns: minmax(0, 1fr) 245px; } }
@media (max-width: 760px) { .creative-workspace { overflow: visible; height: auto; } .workspace-grid { display: flex; flex-direction: column; height: auto; min-height: 0; } .workspace-grid > :first-child { height: 540px; flex-shrink: 0; } .inspector { max-height: none; } .empty-panel { padding: 12px 6px; } .project-actions { margin-left: 0; width: 100%; } .project-actions .primary { margin-left: auto; } .save-status { font-size: 10px; } .project-identity { width: 100%; } .message { flex-wrap: wrap; } .timeline-controls { gap: 6px; } .export-button { margin-left: 0; } dialog { padding: 15px; } }
</style>
