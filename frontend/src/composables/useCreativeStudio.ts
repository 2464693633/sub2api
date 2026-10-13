import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { creativeStudioAPI as api, type StudioProject, type StudioAsset, type StudioJob, type StudioSnapshot, type StudioModel, type StudioParams, type StudioTimeline, type StudioSettings, type StudioPreflight } from '@/api/creativeStudio'
import type { CreativeCanvasKind, CreativeCanvasNode } from '@/types/creativeCanvas'
import { sessionOwnerKey } from './studioSession'
import { isGrokImageModel } from './studioModels'
import { readVideoDuration } from './studioMedia'
import type { QuickComposeFlow, QuickComposeInput } from '@/types/creativeQuickCompose'

type Translate = (key: string, args?: Record<string, unknown>) => string
interface PendingRun { requestId: string; nodeIds: string[]; revision: number }
const uid = () => typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : `c-${Date.now()}-${Math.random().toString(36).slice(2)}`
const active = (job: StudioJob) => ['queued', 'waiting', 'running'].includes(job.status)
const timelineLimit = 12
export const emptyTimeline = (): StudioTimeline => ({ clips: [], size: '1280x720' })

/** Exclude renderer state, object URLs and credentials from drafts and server storage. */
export function persistedProject(p: StudioProject): StudioProject {
  return JSON.parse(JSON.stringify({ ...p,
    nodes: p.nodes.map(n => ({ id: n.id, type: n.type, position: { x: n.position.x, y: n.position.y }, data: {
      kind: n.data.kind, title: n.data.title, prompt: n.data.prompt,
      assetId: n.data.assetId, resultId: n.data.resultId, params: n.data.params, paramsMode: n.data.paramsMode, preferredAssetId: n.data.preferredAssetId
    } })),
    edges: p.edges.map(e => ({ id: e.id, source: e.source, target: e.target, label: typeof e.label === 'string' ? e.label : undefined, type: e.type })),
    viewport: { x: p.viewport.x, y: p.viewport.y, zoom: p.viewport.zoom },
    timeline: p.timeline || emptyTimeline()
  })) as StudioProject
}
function signature(p: StudioProject) {
  const { revision: _revision, updated_at: _updated, ...content } = persistedProject(p)
  return JSON.stringify(content)
}
function errorStatus(e: unknown) {
  const err = e as { status?: number; response?: { status?: number } }
  return err?.status || err?.response?.status
}
function errorMessage(e: unknown) {
  return (e as { message?: string })?.message || String(e)
}

export function useCreativeStudio(tr: Translate) {
  const projects = ref<StudioProject[]>([])
  const project = ref<StudioProject | null>(null)
  const assets = ref<StudioAsset[]>([])
  const jobs = ref<StudioJob[]>([])
  const imageModels = ref<StudioModel[]>([])
  const videoModels = ref<StudioModel[]>([])
  const selectedId = ref<string | null>(null)
  const loading = ref(false)
  const busy = ref(false)
  const error = ref('')
  const notice = ref('')
  const saveState = ref<'saved' | 'saving' | 'saveError' | 'conflict' | 'localDraft'>('saved')
  const pendingRun = ref<PendingRun | null>(null)
  const previewUrls = ref<Record<string, string>>({})
  const preflight = ref<StudioPreflight | null>(null)
  let owner = sessionOwnerKey()
  let epoch = 0
  let controller = new AbortController()
  let stopped = false
  let switching = false
  let snapshotSequence = 0
  let acceptedSequence = 0
  let savedSignature = ''
  let saveTimer: ReturnType<typeof setTimeout> | undefined
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let savePromise: Promise<boolean> | undefined
  const previewPromises = new Map<string, Promise<string>>()
  const durationPromises = new Map<string, Promise<number>>()
  let autoTimelinePromise: Promise<void> | undefined
  let autoTimelineFull = false
  const selected = computed<CreativeCanvasNode | undefined>(() =>
    project.value?.nodes.find((n: CreativeCanvasNode) => n.id === selectedId.value))
  const running = computed(() => jobs.value.filter(active))
  const exports = computed(() => jobs.value.filter(j => j.kind === 'compose'))
  const selectedNodes = computed(() => project.value?.nodes.filter(n => n.selected) || [])
  const pendingNodeIds = computed(() => project.value?.nodes.filter(n => n.type !== 'text' && !n.data.assetId && !jobs.value.some(j => j.node_id === n.id)).map(n => n.id) || [])
  const timeline = computed(() => project.value?.timeline || emptyTimeline())
  const key = (id: string) => `creative-studio:${owner}:${id}`
  const current = (ticket: number) => !stopped && ticket === epoch && owner === sessionOwnerKey()
  function readLocal(k: string) { try { return localStorage.getItem(k) } catch { return null } }
  function writeLocal(k: string, value: string | null) {
    try { if (value === null) localStorage.removeItem(k); else localStorage.setItem(k, value) } catch { /* cloud save remains available */ }
  }
  function retainDraft() {
    if (project.value && owner === sessionOwnerKey()) {
      const dirty = signature(project.value) !== savedSignature
      writeLocal(key(project.value.id), dirty || pendingRun.value ? JSON.stringify({ project: persistedProject(project.value), pendingRun: pendingRun.value }) : null)
    }
  }
  function reset() {
    if (saveTimer) clearTimeout(saveTimer)
    if (pollTimer) clearTimeout(pollTimer)
    controller.abort()
    controller = new AbortController()
    epoch++
    snapshotSequence = 0; acceptedSequence = 0
    for (const url of Object.values(previewUrls.value)) URL.revokeObjectURL(url)
    previewUrls.value = {}
    previewPromises.clear()
    durationPromises.clear()
    autoTimelinePromise = undefined; autoTimelineFull = false
    project.value = null
    assets.value = []; jobs.value = []; selectedId.value = null
    preflight.value = null
    pendingRun.value = null; savePromise = undefined; savedSignature = ''
  }
  function sessionEnded() {
    reset(); stopped = true; projects.value = []; imageModels.value = []; videoModels.value = []
    error.value = ''; notice.value = ''; loading.value = false; busy.value = false
  }
  function acceptResults(snapshot: StudioSnapshot, sequence = ++snapshotSequence) {
    if (sequence < acceptedSequence) return
    acceptedSequence = sequence
    const media = new Map(assets.value.map(a => [a.id, a]))
    for (const asset of snapshot.assets || []) media.set(asset.id, asset)
    assets.value = [...media.values()].sort((a, b) => b.created_at - a.created_at)
    const knownJobs = new Map(jobs.value.map(j => [j.id, j]))
    for (const job of snapshot.jobs || []) {
      const old = knownJobs.get(job.id)
      if (!old || job.updated_at >= old.updated_at) knownJobs.set(job.id, job)
    }
    jobs.value = [...knownJobs.values()].sort((a, b) => b.created_at - a.created_at || b.id.localeCompare(a.id))
    const p = project.value
    if (!p) return
    for (const node of p.nodes) {
      const matching = jobs.value.filter(j => j.node_id === node.id && j.kind === node.type)
      const latest = matching.reduce<StudioJob | undefined>((a, b) => !a || b.created_at > a.created_at || (b.created_at === a.created_at && b.id > a.id) ? b : a, undefined)
      node.data.status = latest?.status || (node.data.assetId ? 'succeeded' : 'draft')
      node.data.error = latest?.error
      // Keep a manually chosen older version after this job has already been applied.
      if (latest?.status === 'succeeded' && latest.asset_id && node.data.resultId !== latest.id) {
        node.data.assetId = latest.asset_id
        node.data.resultId = latest.id
      }
      node.data.previewUrl = node.data.assetId ? previewUrls.value[node.data.assetId] : undefined
    }
    addAutomaticTimeline()
    void loadImagePreviews()
  }
  function addAutomaticTimeline() {
    const p = project.value
    if (!p?.timeline?.auto_add_node_ids?.length || autoTimelinePromise) return
    const ticket = epoch
    autoTimelinePromise = (async () => {
      for (const node of p.nodes) {
        if (!current(ticket) || project.value !== p) return
        if (!p.timeline?.auto_add_node_ids?.includes(node.id) || !node.data.assetId || node.data.status !== 'succeeded') continue
        // Serialize metadata reads and consume IDs only after successful insertion.
        // Full timelines retain pending results for the next save or poll.
        await insertTimeline(node.data.assetId, node.data.title, undefined, node.id)
      }
    })().finally(() => { if (current(ticket)) autoTimelinePromise = undefined })
  }
  async function preview(assetId: string) {
    if (previewUrls.value[assetId]) return previewUrls.value[assetId]!
    const existing = previewPromises.get(assetId)
    if (existing) return existing
    const p = project.value
    if (!p) return ''
    const ticket = epoch
    const media = assets.value.find(a => a.id === assetId)
    const promise = api.asset(p.id, assetId, controller.signal, media?.kind === 'image').then(blob => {
      if (!current(ticket)) return ''
      const url = URL.createObjectURL(blob)
      previewUrls.value[assetId] = url
      for (const node of project.value?.nodes || []) if (node.data.assetId === assetId) node.data.previewUrl = url
      return url
    }).finally(() => { if (current(ticket)) previewPromises.delete(assetId) })
    previewPromises.set(assetId, promise)
    return promise
  }
  async function loadImagePreviews() {
    const ticket = epoch
    // Limit decode/download pressure on the browser and server.
    const ids = new Set(project.value?.nodes.filter(n => n.type === 'image' && n.data.assetId).map(n => n.data.assetId!) || [])
    for (const id of ids) {
      if (!current(ticket)) return
      try { await preview(id) } catch { /* a missing preview must not block saving or generation */ }
    }
  }
  async function save(): Promise<boolean> {
    if (savePromise) return savePromise
    if (!project.value || !current(epoch) || saveState.value === 'conflict') return false
    const ticket = epoch
    const work = async () => {
      while (current(ticket) && project.value && signature(project.value) !== savedSignature) {
        const submitted = persistedProject(project.value)
        const submittedSignature = signature(submitted)
        saveState.value = 'saving'
        try {
          const sequence = ++snapshotSequence
          const result = await api.save(submitted, controller.signal)
          if (!current(ticket) || !project.value) return false
          project.value.revision = result.project.revision
          project.value.updated_at = result.project.updated_at
          savedSignature = submittedSignature
          const index = projects.value.findIndex(p => p.id === submitted.id)
          if (index >= 0) projects.value[index] = result.project
          acceptResults(result, sequence)
          retainDraft()
        } catch (e) {
          if (current(ticket)) {
            saveState.value = errorStatus(e) === 409 ? 'conflict' : 'saveError'
            error.value = errorMessage(e); retainDraft()
          }
          return false
        }
      }
      if (current(ticket)) saveState.value = 'saved'
      return current(ticket)
    }
    savePromise = work().finally(() => { if (current(ticket)) savePromise = undefined })
    return savePromise
  }
  watch(() => project.value ? signature(project.value) : '', () => {
    if (!project.value || loading.value || !current(epoch)) return
    preflight.value = null
    retainDraft()
    if (signature(project.value) === savedSignature || saveState.value === 'conflict') return
    saveState.value = 'localDraft'
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(() => { void save() }, 650)
  }, { flush: 'sync' })

  async function poll() {
    const ticket = epoch
    if (!project.value || !current(ticket)) return
    try {
      const sequence = ++snapshotSequence
      const snapshot = await api.get(project.value.id, controller.signal)
      if (current(ticket) && project.value && sequence >= acceptedSequence && !savePromise) {
        // Another tab's graph may be newer; never overwrite this tab's draft.
        if (snapshot.project.revision > project.value.revision) {
          saveState.value = 'conflict'; retainDraft()
        }
        acceptResults(snapshot, sequence)
      }
    } catch { /* retain visible data and retry after transient network failures */ }
    if (current(ticket)) pollTimer = setTimeout(() => { void poll() }, 4500)
  }
  async function open(id: string, discardDraft = false) {
    if (loading.value || busy.value || switching) return
    switching = true
    const previousTicket = epoch
    if (project.value && !discardDraft) {
      await save(); retainDraft()
    }
    if (!current(previousTicket)) { switching = false; return }
    reset(); stopped = false; owner = sessionOwnerKey(); switching = false
    const ticket = epoch
    loading.value = true; error.value = ''; notice.value = ''; saveState.value = 'saved'
    try {
      const snapshot = await api.get(id, controller.signal)
      if (!current(ticket)) return
      snapshot.project.timeline ||= emptyTimeline()
      snapshot.project.settings ||= defaultSettings(snapshot.project)
      savedSignature = signature(snapshot.project)
      project.value = snapshot.project
      if (discardDraft) writeLocal(key(id), null)
      else {
        try {
          const draft = JSON.parse(readLocal(key(id)) || 'null') as { project: StudioProject; pendingRun?: PendingRun } | null
          if (draft?.project.id === id) {
            draft.project.settings ||= defaultSettings(draft.project)
            project.value = draft.project
            pendingRun.value = draft.pendingRun || null
            if (draft.project.revision !== snapshot.project.revision) saveState.value = 'conflict'
            else { saveState.value = 'localDraft'; notice.value = tr('restored') }
          }
        } catch { /* ignore malformed local drafts */ }
      }
      acceptResults(snapshot)
      writeLocal(`creative-studio:${owner}:last`, id)
    } catch (e) { if (current(ticket)) error.value = errorMessage(e) }
    finally {
      if (current(ticket)) {
        loading.value = false
        if (project.value && saveState.value !== 'conflict' && signature(project.value) !== savedSignature) void save()
        if (project.value) pollTimer = setTimeout(() => { void poll() }, 4500)
      }
    }
  }
  async function initialize() {
    const ticket = epoch
    error.value = ''
    try {
      const [list, images, videos] = await Promise.all([
        api.list(controller.signal), api.models('image', controller.signal).catch(() => []), api.models('video', controller.signal).catch(() => [])
      ])
      if (!current(ticket)) return
      projects.value = list
      imageModels.value = images.filter(m => m.group_id > 0 && (/(image|dall|flux|seedream|banana|diffusion)/i.test(m.id) || isGrokImageModel(m.id)))
      videoModels.value = videos.filter(m => m.group_id > 0 && /(video|imagine|seedance|veo|sora|kling)/i.test(m.id) && !/image/i.test(m.id.replace(/imagine/ig, '')) && !isGrokImageModel(m.id))
      const last = readLocal(`creative-studio:${owner}:last`)
      if (list.length) await open(list.find(p => p.id === last)?.id || list[0]!.id)
      else await create()
    } catch (e) { if (current(ticket)) error.value = errorMessage(e) }
  }
  async function create() {
    if (busy.value || loading.value || switching) return
    switching = true
    const ticket = epoch
    if (project.value) { await save(); retainDraft() }
    if (!current(ticket)) { switching = false; return }
    busy.value = true
    try {
      const snapshot = await api.create(tr('unnamed'), controller.signal)
      if (!current(ticket)) return
      projects.value.unshift(snapshot.project)
      busy.value = false; switching = false
      await open(snapshot.project.id)
    } catch (e) { if (current(ticket)) error.value = errorMessage(e) }
    finally { switching = false; if (current(ticket)) busy.value = false }
  }
  async function deleteProject() {
    await action(async ticket => {
      const id = project.value!.id
      await api.remove(id, controller.signal)
      if (!current(ticket)) return
      writeLocal(key(id), null)
      writeLocal(`${key(id)}:export`, null)
      projects.value = projects.value.filter(p => p.id !== id)
      reset()
      busy.value = false
      if (projects.value.length) await open(projects.value[0]!.id)
    })
  }
  function params(kind: CreativeCanvasKind): StudioParams {
    const model = (kind === 'video' ? videoModels : imageModels).value[0]
    return { model: model?.id || '', group_id: model?.group_id || 0, platform: model?.platform, size: 'auto', quality: 'auto', format: 'png', seconds: 6, resolution: '720p' }
  }
  function defaultSettings(p: StudioProject): StudioSettings {
    const previous = (kind: 'image' | 'video') => p.nodes.find(n => n.type === kind)?.data.params as StudioParams | undefined
    return { subject: '', style: '', image_params: { ...(previous('image') || params('image')) }, video_params: { ...(previous('video') || params('video')) } }
  }
  function effectiveParams(node: CreativeCanvasNode): StudioParams {
    if (node.data.paramsMode === 'project' && project.value?.settings) return node.type === 'video' ? project.value.settings.video_params : project.value.settings.image_params
    return node.data.params as StudioParams
  }
  function setParamsMode(id: string, mode: 'project' | 'custom') {
    const node = project.value?.nodes.find(n => n.id === id)
    if (!node || node.type === 'text') return
    if (mode === 'custom') node.data.params = { ...effectiveParams(node) }
    node.data.paramsMode = mode
  }
  function applyProjectDefaults(ids: string[]) {
    for (const id of ids) setParamsMode(id, 'project')
  }
  function selectNodes(ids: string[]) {
    for (const node of project.value?.nodes || []) node.selected = ids.includes(node.id)
  }
  function markPreferred(asset: StudioAsset) {
    if (!selected.value || selected.value.type !== asset.kind) return
    selected.value.data.preferredAssetId = selected.value.data.preferredAssetId === asset.id ? undefined : asset.id
    if (selected.value.data.preferredAssetId) useVersion(asset)
  }
  async function addSelectedTimeline() {
    const ticket = epoch
    const selected = selectedNodes.value.map(n => ({ assetId: n.data.assetId, title: n.data.title }))
    for (const node of selected) {
      if (!current(ticket)) return
      if (node.assetId) await addTimeline(node.assetId, node.title)
    }
  }
  function runCounts(ids = pendingNodeIds.value) {
    const planned = new Set<string>()
    const selectedIDs = new Set(ids)
    const visit = (id: string) => {
      if (planned.has(id)) return
      const node = project.value?.nodes.find(n => n.id === id)
      if (!node || node.type === 'text') return
      planned.add(id)
      for (const edge of project.value?.edges.filter(e => e.target === id) || []) {
        const source = project.value?.nodes.find(n => n.id === edge.source)
        const completed = jobs.value.find(j => j.node_id === edge.source)?.status === 'succeeded'
        if (source && (selectedIDs.has(source.id) || (!source.data.assetId && !completed))) visit(source.id)
      }
    }
    ids.forEach(visit)
    const image = project.value?.nodes.filter(n => planned.has(n.id) && n.type === 'image').length || 0
    return { image, video: planned.size - image, total: planned.size }
  }
  async function checkPlan(ids = pendingNodeIds.value) {
    let result: StudioPreflight | undefined
    await action(async ticket => {
      if (!ids.length || !await save() || !current(ticket)) return
      const revision = project.value!.revision
      const content = signature(project.value!)
      const response = await api.preflight(project.value!.id, ids, revision, controller.signal)
      if (current(ticket) && project.value?.revision === revision && signature(project.value!) === content) {
        result = response; preflight.value = response
      } else if (current(ticket)) notice.value = tr('planChanged')
    })
    return result
  }
  function add(kind: CreativeCanvasKind, title?: string, prompt = '', position?: { x: number; y: number }) {
    if (!project.value) return undefined
    const n = project.value.nodes.length
    const node: CreativeCanvasNode = { id: uid(), type: kind, position: position || { x: (n % 3) * 350 + 60, y: Math.floor(n / 3) * 390 + 50 }, data: { kind, title: title || tr(kind === 'image' ? 'images' : kind === 'video' ? 'videos' : 'text'), prompt, params: params(kind), paramsMode: 'project', status: 'draft' } }
    project.value.nodes.push(node)
    selectedId.value = node.id
    return node
  }
  function connect(source: string, target: string) {
    project.value?.edges.push({ id: uid(), source, target, type: 'smoothstep' })
  }
  function derive(id: string, kind: 'image' | 'video') {
    const source = project.value?.nodes.find(n => n.id === id)
    if (!source || source.type === 'video') return
    const position = { x: source.position.x + 370, y: source.position.y }
    while (project.value?.nodes.some(n => Math.abs(n.position.x - position.x) < 300 && Math.abs(n.position.y - position.y) < 380)) position.y += 410
    const node = add(kind, undefined, kind === 'video' ? tr('defaultMotion') : '', position)
    if (node) connect(id, node.id)
    return node
  }
  function template() {
    if (!project.value) return
    const offset = project.value.nodes.reduce((max, n) => Math.max(max, n.position.y + 440), 60)
    const overview = add('image', tr('overview'), '', { x: 50, y: offset + 400 })
    if (!overview) return
    for (let i = 1; i <= 4; i++) {
      const node = add('image', tr(`detail${i}`), tr(`detailPrompt${i}`), { x: 430 + (i > 2 ? 370 : 0), y: offset + ((i - 1) % 2) * 430 })
      if (node) connect(overview.id, node.id)
    }
    selectedId.value = overview.id; notice.value = tr('templateAdded')
  }
  async function createQuickCompose(input: QuickComposeInput): Promise<QuickComposeFlow | undefined> {
    if (busy.value || loading.value || switching || !current(epoch)) return
    if (pendingRun.value) { error.value = tr('requestRetry'); return }
    if (!input.subject.trim() || !Number.isInteger(input.detailCount) || input.detailCount < 0 || input.detailCount > 4 ||
      !['none', 'two', 'all'].includes(input.videoMode)) {
      error.value = tr('quickInvalid'); return
    }
    const encoder = new TextEncoder()
    if (encoder.encode(input.subject.trim()).length > 4000 || encoder.encode(input.style.trim()).length > 4000 ||
      encoder.encode(input.overview.trim()).length > 12000) {
      error.value = tr('quickTextTooLong'); return
    }
    // Copy the form before any async request. The new flow is a separate
    // project so its shared subject and models cannot change existing work.
    const draft = JSON.parse(JSON.stringify(input)) as QuickComposeInput
    let ticket = epoch
    switching = true; busy.value = true; error.value = ''; notice.value = ''
    try {
      if (project.value && !await save()) return
      if (!current(ticket)) return
      const snapshot = await api.create(draft.subject.trim().slice(0, 60), controller.signal)
      if (!current(ticket)) return
      const next = snapshot.project
      next.settings = { subject: draft.subject.trim(), style: draft.style.trim(), image_params: draft.imageParams, video_params: draft.videoParams }
      next.timeline = { ...emptyTimeline(), auto_add_node_ids: draft.autoTimeline === false ? undefined : [] }
      next.nodes = []; next.edges = []
      const make = (kind: 'image' | 'video', title: string, prompt: string, x: number, y: number) => {
        const node: CreativeCanvasNode = { id: uid(), type: kind, position: { x, y }, data: {
          kind, title, prompt, paramsMode: 'project', status: 'draft',
          params: { ...(kind === 'image' ? draft.imageParams : draft.videoParams) }
        } }
        next.nodes.push(node)
        return node
      }
      const link = (source: string, target: string) => next.edges.push({ id: uid(), source, target, type: 'smoothstep' })
      const overview = make('image', tr('overview'), draft.overview.trim() || tr('quickOverviewDefault'), 60, draft.detailCount > 1 ? 260 : 50)
      const images = [overview]
      for (let i = 0; i < draft.detailCount; i++) {
        const detail = make('image', tr(`detail${i + 1}`), tr(`detailPrompt${i + 1}`), 430 + Math.floor(i / 2) * 370, 50 + (i % 2) * 410)
        link(overview.id, detail.id); images.push(detail)
      }
      const videoSources = draft.videoMode === 'all' ? images : draft.videoMode === 'two'
        ? [overview, ...(images.length > 1 ? [images[Math.min(3, images.length - 1)]!] : [])] : []
      videoSources.forEach((source, index) => {
        const video = make('video', `${source.data.title} · ${tr('quickVideoTitle')}`, tr('defaultMotion'),
          draft.detailCount > 2 ? 1170 : draft.detailCount ? 800 : 430, 50 + index * 410)
        link(source.id, video.id)
      })
      if (draft.autoTimeline !== false) next.timeline!.auto_add_node_ids = next.nodes.filter(n => n.type !== 'text').map(n => n.id)
      const flow = { projectId: next.id, nodeIds: next.nodes.map(n => n.id), graphSignature: signature(next), autoTimeline: draft.autoTimeline !== false }
      // Install a recoverable local draft before saving its graph. A failed
      // save must return these IDs so retry never creates a second flow.
      reset(); ticket = epoch
      loading.value = true
      project.value = next; projects.value.unshift(next)
      selectedId.value = overview.id
      savedSignature = ''
      loading.value = false
      writeLocal(`creative-studio:${owner}:last`, next.id)
      retainDraft()
      await save()
      if (!current(ticket)) return
      pollTimer = setTimeout(() => { void poll() }, 4500)
      return flow
    } catch (e) { if (current(ticket)) error.value = errorMessage(e) }
    finally { switching = false; if (current(ticket)) { busy.value = false; loading.value = false } }
  }
  function remove(id: string) {
    if (!project.value) return
    project.value.nodes = project.value.nodes.filter(n => n.id !== id)
    project.value.edges = project.value.edges.filter(e => e.source !== id && e.target !== id)
    if (selectedId.value === id) selectedId.value = null
  }
  function duplicate(id: string) {
    const source = project.value?.nodes.find(n => n.id === id)
    if (!source) return
    const node = add(source.type, source.data.title, source.data.prompt, { x: source.position.x + 60, y: source.position.y + 70 })
    if (node) { node.data.params = { ...effectiveParams(source) }; node.data.paramsMode = source.data.paramsMode || 'custom' }
  }
  function arrange() {
    const p = project.value
    if (!p) return
    const levels = new Map<string, number>()
    const level = (id: string, seen = new Set<string>()): number => {
      if (levels.has(id)) return levels.get(id)!
      if (seen.has(id)) return 0
      seen.add(id)
      const parents = p.edges.filter(e => e.target === id)
      const value = parents.length ? Math.max(...parents.map(e => level(e.source, new Set(seen)))) + 1 : 0
      levels.set(id, value); return value
    }
    const rows = new Map<number, number>()
    for (const n of p.nodes) {
      const col = level(n.id); const row = rows.get(col) || 0
      n.position = { x: col * 370 + 50, y: row * 410 + 50 }; rows.set(col, row + 1)
    }
  }
  async function action(work: (ticket: number) => Promise<void>) {
    if (busy.value || loading.value || !project.value || !current(epoch)) return
    const ticket = epoch
    busy.value = true; error.value = ''; notice.value = ''
    try { await work(ticket) } catch (e) { if (current(ticket)) error.value = errorMessage(e) }
    finally { if (current(ticket)) busy.value = false }
  }
  async function run(nodeIds?: string[]) {
    let submitted = false
    await action(async ticket => {
      if (!pendingRun.value) {
        const ids = nodeIds || pendingNodeIds.value
        if (!ids.length) { notice.value = tr('nothingPending'); return }
        if (!await save() || !current(ticket)) return
        const checkedRevision = project.value!.revision
        const checkedSignature = signature(project.value!)
        const plan = await api.preflight(project.value!.id, ids, checkedRevision, controller.signal)
        if (!current(ticket)) return
        preflight.value = plan
        if (!plan.valid) { error.value = tr('preflightFix'); return }
        // Edits made while validating need a fresh plan, never a different graph behind the same button.
        if (signature(project.value!) !== checkedSignature || project.value!.revision !== checkedRevision) { notice.value = tr('planChanged'); return }
        pendingRun.value = { requestId: uid(), nodeIds: ids, revision: checkedRevision }
        retainDraft()
      }
      const pending = pendingRun.value
      try {
        const sequence = ++snapshotSequence
        const snapshot = await api.run(project.value!.id, pending.nodeIds, pending.requestId, pending.revision, controller.signal)
        if (!current(ticket)) return
        pendingRun.value = null; acceptResults(snapshot, sequence); retainDraft(); notice.value = tr('submitted'); submitted = true
      } catch (e) {
        if (current(ticket) && [400, 403, 404, 409, 413, 422].includes(errorStatus(e) || 0)) { pendingRun.value = null; retainDraft() }
        throw e
      }
    })
    return submitted
  }
  async function runNode(id: string) {
    const job = jobs.value.find(j => j.node_id === id)
    if (job && active(job)) return
    if (job?.status === 'interrupted') {
      selectedId.value = id; notice.value = tr('interruptedHint'); return
    }
    if (job && ['failed', 'cancelled'].includes(job.status)) await jobAction(job, 'retry')
    else await run([id])
  }
  async function jobAction(job: StudioJob, operation: 'retry' | 'cancel') {
    await action(async ticket => {
      const sequence = ++snapshotSequence
      const snapshot = await api[operation](project.value!.id, job.id, controller.signal)
      if (current(ticket)) acceptResults(snapshot, sequence)
    })
  }
  function useVersion(asset: StudioAsset) {
    const n = selected.value
    if (!n || n.type !== asset.kind) return
    n.data.assetId = asset.id; n.data.previewUrl = previewUrls.value[asset.id]
    if (asset.kind === 'image') void preview(asset.id).catch(() => {})
  }
  function addAssetNode(asset: StudioAsset, prompt = '') {
    if (asset.kind === 'audio') return
    const n = add(asset.kind, asset.name, prompt)
    if (n) { n.data.assetId = asset.id; n.data.status = 'succeeded'; n.data.previewUrl = previewUrls.value[asset.id] }
    if (asset.kind === 'image') void preview(asset.id).catch(() => {})
  }
  async function upload(files: File[], prompts: string[] = []) {
    await action(async ticket => {
      for (let i = 0; i < files.length; i++) {
        const file = files[i]!
        if (file.size > (file.type.startsWith('image/') ? 32 : 128) * 1024 * 1024) throw new Error(tr('fileTooLarge'))
        const asset = await api.upload(project.value!.id, file, controller.signal)
        if (!current(ticket)) return
        assets.value.push(asset)
        addAssetNode(asset, prompts[i])
      }
      notice.value = tr('imported')
    })
  }
  function videoDuration(assetId: string) {
    const existing = durationPromises.get(assetId)
    if (existing) return existing
    const ticket = epoch, signal = controller.signal
    const promise = preview(assetId).then(url => {
      if (!current(ticket) || !url) throw new DOMException('Aborted', 'AbortError')
      return readVideoDuration(url, signal)
    }).catch(e => {
      if (current(ticket)) durationPromises.delete(assetId)
      throw e
    })
    durationPromises.set(assetId, promise)
    return promise
  }
  async function insertTimeline(assetId: string, title: string, index?: number, autoNodeId?: string) {
    const p = project.value, ticket = epoch
    const asset = assets.value.find(a => a.id === assetId && a.kind !== 'audio')
    if (!p || !asset || !current(ticket)) return false
    p.timeline ||= emptyTimeline()
    const stillWanted = () => current(ticket) && project.value === p && (!autoNodeId || (
      p.timeline?.auto_add_node_ids?.includes(autoNodeId) && p.nodes.some(n => n.id === autoNodeId && n.data.assetId === assetId && n.data.status === 'succeeded')
    ))
    const consume = () => {
      if (!autoNodeId) return
      const remaining = p.timeline!.auto_add_node_ids?.filter(id => id !== autoNodeId)
      p.timeline!.auto_add_node_ids = remaining?.length ? remaining : undefined
    }
    const alreadyAdded = () => !!autoNodeId && p.timeline!.clips.some(c => c.asset_id === assetId)
    const hasRoom = () => {
      if (p.timeline!.clips.length < timelineLimit) { autoTimelineFull = false; return true }
      if (autoNodeId) {
        if (!autoTimelineFull) notice.value = tr('timelineFull')
        autoTimelineFull = true
      } else error.value = tr('timelineFull')
      return false
    }
    if (!stillWanted()) return false
    if (alreadyAdded()) { consume(); return true }
    if (!hasRoom()) return false
    let duration = 4
    if (asset.kind === 'video') {
      try { duration = Math.min(duration, await videoDuration(assetId)) }
      catch { if (stillWanted()) error.value = tr('timelineVideoUnavailable'); return false }
      if (!stillWanted()) return false
      if (duration < 1) { error.value = tr('timelineVideoTooShort'); return false }
    }
    // A user may have edited the timeline or selected another result while
    // metadata was loading. Recheck both deduplication and capacity at insertion.
    if (alreadyAdded()) { consume(); return true }
    if (!hasRoom()) return false
    const clips = p.timeline!.clips
    const target = Number.isInteger(index) ? Math.max(0, Math.min(index!, clips.length)) : clips.length
    clips.splice(target, 0, { id: uid(), asset_id: assetId, title, start: 0, duration })
    consume()
    return true
  }
  function addTimeline(assetId: string, title = '', index?: number) {
    return insertTimeline(assetId, title, index)
  }
  async function render() {
    await action(async ticket => {
      const value = JSON.parse(JSON.stringify(timeline.value)) as StudioTimeline
      if (!value.clips.length || value.clips.length > timelineLimit || value.clips.some(c => !Number.isFinite(c.duration) || c.duration < 1 || c.duration > 30 || !Number.isFinite(c.start) || c.start < 0) || value.clips.reduce((sum, c) => sum + c.duration, 0) > 90) throw new Error(tr('renderInvalid'))
      if (!await save() || !current(ticket)) return
      // Identical timelines share one request until the server confirms reception.
      const pendingKey = `${key(project.value!.id)}:export`
      let request = { id: uid(), value: JSON.stringify(value) }
      try { const old = JSON.parse(readLocal(pendingKey) || 'null'); if (old?.value === request.value) request = old } catch { /* ignore */ }
      writeLocal(pendingKey, JSON.stringify(request))
      const sequence = ++snapshotSequence
      const snapshot = await api.render(project.value!.id, value, request.id, controller.signal)
      if (current(ticket)) { writeLocal(pendingKey, null); acceptResults(snapshot, sequence); notice.value = tr('renderSubmitted') }
    })
  }
  function downloadBlob(blob: Blob, name: string) {
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a'); a.href = url; a.download = name; a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  async function download(asset: StudioAsset) {
    await action(async ticket => {
      const blob = await api.asset(project.value!.id, asset.id, controller.signal)
      if (current(ticket)) downloadBlob(blob, asset.name)
    })
  }
  function exportDraft() {
    if (project.value) downloadBlob(new Blob([JSON.stringify(persistedProject(project.value), null, 2)], { type: 'application/json' }), `${project.value.name}.json`)
  }
  function beforeUnload() { retainDraft() }
  onMounted(() => {
    window.addEventListener('studio-session-ended', sessionEnded)
    window.addEventListener('beforeunload', beforeUnload)
    void initialize()
  })
  onBeforeUnmount(() => {
    retainDraft(); reset(); stopped = true
    window.removeEventListener('studio-session-ended', sessionEnded)
    window.removeEventListener('beforeunload', beforeUnload)
  })
  return { projects, project, assets, jobs, imageModels, videoModels, selectedId, selected, loading, busy, error, notice, saveState, pendingRun, previewUrls, running, exports, timeline, preflight, selectedNodes, pendingNodeIds, effectiveParams, setParamsMode, applyProjectDefaults, selectNodes, markPreferred, addSelectedTimeline, runCounts, checkPlan,
    initialize, create, createQuickCompose, deleteProject, open, save, preview, add, derive, template, remove, duplicate, arrange, run, runNode, jobAction, useVersion, addAssetNode, upload, addTimeline, render, download, exportDraft,
    projectSignature: () => project.value ? signature(project.value) : '' }
}
