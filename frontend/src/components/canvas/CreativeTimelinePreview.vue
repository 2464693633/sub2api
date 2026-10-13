<template>
  <dialog ref="dialog" class="timeline-preview" @cancel="close" @close="stop">
    <header><h2>{{ tr('previewTimeline') }}</h2><button type="button" :aria-label="tr('close')" @click="close">×</button></header>
    <div class="player-frame" :style="{ aspectRatio: snapshot.size.replace('x', ' / ') }">
      <p v-if="loading" role="status">{{ tr('loading') }}</p>
      <template v-else-if="currentAsset && mediaUrl">
        <img v-if="currentAsset.kind === 'image'" :src="mediaUrl" :alt="currentClip?.title || currentAsset.name" />
        <video v-else ref="video" :key="currentClip?.id" :src="mediaUrl" playsinline preload="auto" @loadedmetadata="videoReady" @ended="advance" @error="mediaFailed"></video>
        <span v-if="currentClip?.title" class="caption">{{ currentClip.title }}</span>
      </template>
    </div>
    <audio v-if="audioUrl" ref="music" :src="audioUrl" loop preload="auto" @loadedmetadata="syncMusic"></audio>
    <p v-if="failure" role="alert" class="failure">{{ failure }}</p>
    <div class="player-controls"><button type="button" :disabled="loading || !ready || !!failure" @click="playing ? pause() : play()">{{ playing ? 'Ⅱ' : '▶' }} {{ tr(playing ? 'pausePreview' : 'playPreview') }}</button><span>{{ tr('clip') }} {{ index + 1 }} / {{ snapshot.clips.length }}</span><span>{{ Math.min(total, elapsedBefore + elapsed).toFixed(1) }} / {{ total.toFixed(1) }}s</span><button type="button" :disabled="index === 0 || loading" @click="loadClip(index - 1, false)">←</button><button type="button" :disabled="index >= snapshot.clips.length - 1 || loading" @click="loadClip(index + 1, false)">→</button></div>
    <progress :value="elapsedBefore + elapsed" :max="total || 1" :aria-label="tr('previewProgress')"></progress>
    <p class="hint">{{ tr('previewTimelineHint') }}</p>
  </dialog>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { StudioAsset, StudioTimeline } from '@/api/creativeStudio'
import { creativeStudioMessages } from './creativeStudioMessages'
const props = defineProps<{ timeline: StudioTimeline; assets: StudioAsset[]; resolvePreview: (id: string) => Promise<string> }>()
const { t: tr } = useI18n({ useScope: 'local', messages: creativeStudioMessages })
const dialog = ref<HTMLDialogElement>()
const video = ref<HTMLVideoElement>()
const music = ref<HTMLAudioElement>()
const snapshot = ref<StudioTimeline>({ clips: [], size: '1280x720' })
const index = ref(0)
const elapsed = ref(0)
const playing = ref(false)
const ready = ref(false)
const loading = ref(false)
const failure = ref('')
const mediaUrl = ref('')
const audioUrl = ref('')
const currentClip = computed(() => snapshot.value.clips[index.value])
const currentAsset = computed(() => props.assets.find(a => a.id === currentClip.value?.asset_id))
const total = computed(() => snapshot.value.clips.reduce((sum, c) => sum + c.duration, 0))
const elapsedBefore = computed(() => snapshot.value.clips.slice(0, index.value).reduce((sum, c) => sum + c.duration, 0))
let timer: ReturnType<typeof setInterval> | undefined
let ticket = 0
let session = 0
let playAttempt = 0
let lastTick = 0
let startAfterLoad = false
let ended = false
function stop() {
  ticket++; session++; playAttempt++; playing.value = false; loading.value = false; startAfterLoad = false
  video.value?.pause(); music.value?.pause()
  if (timer) clearInterval(timer)
  timer = undefined
  mediaUrl.value = ''; audioUrl.value = ''
}
function close() { stop(); dialog.value?.close() }
function pause() { playAttempt++; playing.value = false; video.value?.pause(); music.value?.pause() }
async function play() {
  if (ended) { ended = false; await loadClip(0, true); return }
  if (!ready.value || failure.value || loading.value) return
  const attempt = ++playAttempt
  try {
    if (currentAsset.value?.kind === 'video') await video.value?.play()
    if (!dialog.value?.open || attempt !== playAttempt) return
    playing.value = true; lastTick = performance.now()
    syncMusic()
  } catch { playing.value = false; failure.value = tr('mediaPreviewFailed') }
}
function syncMusic() {
  const audio = music.value
  if (!audio || !playing.value) return
  audio.volume = 0.2
  if (Number.isFinite(audio.duration) && audio.duration > 0) audio.currentTime = (elapsedBefore.value + elapsed.value) % audio.duration
  void audio.play().catch(() => {})
}
function mediaFailed() { failure.value = tr('mediaPreviewFailed'); pause() }
function videoReady() {
  const clip = currentClip.value
  const media = video.value
  if (!clip || !media) return
  if (clip.start + clip.duration > media.duration + .1) { failure.value = tr('previewRangeError'); return }
  media.currentTime = clip.start
  ready.value = true
  if (startAfterLoad) { startAfterLoad = false; void play() }
}
async function loadClip(next: number, autoplay: boolean) {
  pause(); ready.value = false; loading.value = true; failure.value = ''; mediaUrl.value = ''; elapsed.value = 0
  index.value = next; ended = false; startAfterLoad = autoplay
  const current = ++ticket
  const clip = currentClip.value
  if (!clip || !Number.isFinite(clip.duration) || clip.duration <= 0 || !Number.isFinite(clip.start) || clip.start < 0) { loading.value = false; failure.value = tr('renderInvalid'); return }
  try {
    const url = await props.resolvePreview(clip.asset_id)
    if (current !== ticket || !dialog.value?.open) return
    mediaUrl.value = url; loading.value = false
    await nextTick()
    if (currentAsset.value?.kind === 'image') { ready.value = true; if (autoplay) await play() }
  } catch (error) { if (current === ticket) { loading.value = false; failure.value = (error as Error).message || tr('mediaPreviewFailed') } }
}
function advance() {
  if (!playing.value || loading.value) return
  if (index.value + 1 < snapshot.value.clips.length) void loadClip(index.value + 1, true)
  else { elapsed.value = currentClip.value?.duration || 0; pause(); ended = true }
}
async function open() {
  stop(); ended = false
  snapshot.value = JSON.parse(JSON.stringify(props.timeline)) as StudioTimeline
  if (!snapshot.value.clips.length) return
  dialog.value?.showModal()
  timer = setInterval(() => {
    if (!playing.value || !ready.value || loading.value || !currentClip.value) return
    const now = performance.now()
    elapsed.value = currentAsset.value?.kind === 'video' ? Math.max(0, (video.value?.currentTime || 0) - currentClip.value.start) : elapsed.value + (now - lastTick) / 1000
    lastTick = now
    if (elapsed.value >= currentClip.value.duration) advance()
  }, 60)
  const currentSession = session
  const soundtrack = snapshot.value.audio_asset_id
  if (soundtrack) void props.resolvePreview(soundtrack).then(async url => {
    if (currentSession !== session || !dialog.value?.open) return
    audioUrl.value = url
    await nextTick(); syncMusic()
  }).catch(() => { if (currentSession === session) failure.value = tr('mediaPreviewFailed') })
  await loadClip(0, false)
}
onBeforeUnmount(stop)
defineExpose({ open, close })
</script>
<style scoped>
.timeline-preview { width:min(940px,94vw);max-height:92vh;overflow:auto;padding:20px;border:1px solid var(--cs-border);border-radius:14px;background:var(--cs-bg);color:var(--cs-text); }
.timeline-preview::backdrop { background:#0f172a88;backdrop-filter:blur(3px); }
header,.player-controls { display:flex;align-items:center;gap:12px;flex-wrap:wrap; }
header { justify-content:space-between;margin-bottom:16px; }
h2 { font-size:15px;font-weight:600; }
button { border:1px solid var(--cs-border);border-radius:6px;padding:7px 12px;background:var(--cs-bg);color:var(--cs-text); }
button:disabled { opacity:.45; }
.player-frame { position:relative;display:flex;align-items:center;justify-content:center;max-height:58vh;margin:auto;max-width:100%;background:#0b1018;border-radius:8px;overflow:hidden;color:#cbd5e1; }
.player-frame img,.player-frame video { width:100%;height:100%;object-fit:contain; }
.caption { position:absolute;bottom:9%;left:8%;right:8%;text-align:center;white-space:pre-wrap;color:white;font-size:clamp(13px,2vw,24px);text-shadow:0 2px 4px #000; }
.player-controls { margin:14px 0;font-size:12px; }
progress { width:100%;height:6px;accent-color:#0f766e; }
.hint { margin-top:10px;color:var(--cs-muted);font-size:11px; }
.failure { color:#be123c;font-size:12px;margin:10px 0; }
</style>
