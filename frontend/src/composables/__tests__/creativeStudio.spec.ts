import { defineComponent, h } from 'vue'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { creativeStudioAPI as api, type StudioSnapshot, type StudioProject, type StudioAsset, type StudioJob } from '@/api/creativeStudio'
import { useCreativeStudio, persistedProject } from '../useCreativeStudio'
import type { QuickComposeInput } from '@/types/creativeQuickCompose'

vi.mock('@/api/creativeStudio', () => ({ creativeStudioAPI: Object.fromEntries(['list','create','get','save','remove','upload','asset','run','retry','cancel','render','models','preflight'].map(name => [name, vi.fn()])) }))
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T
const node = { id: 'image-1', type: 'image' as const, position: { x: 50, y: 80 }, data: { kind: 'image' as const, title: 'overview', prompt: 'leather bag', params: { model: 'gpt-image-2', group_id: 20, size: 'auto', quality: 'auto', format: 'png', seconds: 6, resolution: '720p' } } }
const imageAsset = (id: string): StudioAsset => ({ id, kind: 'image', name: `${id}.png`, mime: 'image/png', size: 10, created_at: 100, node_id: node.id })
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(yes => { resolve = yes })
  return { promise, resolve }
}
let server: StudioSnapshot
let wrappers: VueWrapper[] = []
async function start() {
  let studio!: ReturnType<typeof useCreativeStudio>
  const wrapper = mount(defineComponent({ setup() { studio = useCreativeStudio(key => key); return () => h('div') } }))
  wrappers.push(wrapper)
  await flushPromises()
  return studio
}

beforeEach(() => {
  vi.useFakeTimers(); vi.resetAllMocks(); localStorage.clear()
  localStorage.setItem('auth_user', JSON.stringify({ id: 1 }))
  localStorage.setItem('auth_token', 'test')
  vi.stubGlobal('URL', class extends URL { static createObjectURL = vi.fn(() => 'blob:preview'); static revokeObjectURL = vi.fn() })
  server = { project: { id: 'project-1', name: 'test', revision: 1, nodes: [copy(node)], edges: [], viewport: { x: 0, y: 0, zoom: 1 }, timeline: { clips: [], size: '1280x720' }, created_at: 1, updated_at: 1 }, assets: [], jobs: [] }
  vi.mocked(api.list).mockImplementation(async () => [copy(server.project)])
  vi.mocked(api.get).mockImplementation(async () => copy(server))
  vi.mocked(api.models).mockImplementation(async kind => [{ id: kind === 'image' ? 'gpt-image-2' : 'grok-imagine-video', group_id: 20, group_name: 'group' }])
  vi.mocked(api.save).mockImplementation(async p => {
    if (p.revision !== server.project.revision) throw { status: 409, message: 'conflict' }
    server.project = { ...copy(p), revision: p.revision + 1 }
    return copy(server)
  })
  vi.mocked(api.asset).mockResolvedValue(new Blob(['image'], { type: 'image/png' }))
  vi.mocked(api.preflight).mockImplementation(async (_id, ids, revision) => ({ revision, valid: true, counts: { image: ids.length, video: 0, total: ids.length }, nodes: [], errors: [] }))
  vi.mocked(api.run).mockImplementation(async () => copy(server))
  vi.mocked(api.render).mockImplementation(async () => copy(server))
})
afterEach(() => { for (const wrapper of wrappers) wrapper.unmount(); wrappers = []; vi.useRealTimers(); vi.unstubAllGlobals() })

describe('creative studio persistence and workflows', () => {
  it('saves stable graph data and strips canvas runtime fields from both draft and server', async () => {
    const studio = await start()
    studio.project.value!.nodes[0]!.data.previewUrl = 'blob:private'
    studio.project.value!.nodes[0]!.data.status = 'running'
    studio.project.value!.nodes[0]!.selected = true
    await vi.advanceTimersByTimeAsync(700)
    expect(api.save).not.toHaveBeenCalled()
    studio.project.value!.nodes[0]!.data.prompt = 'changed prompt'
    await vi.advanceTimersByTimeAsync(700)
    expect(server.project.nodes[0]!.data).toMatchObject({ prompt: 'changed prompt' })
    expect(server.project.nodes[0]!.data).not.toHaveProperty('previewUrl')
    expect(server.project.nodes[0]).not.toHaveProperty('selected')
    expect(studio.saveState.value).toBe('saved')
    expect(localStorage.getItem('creative-studio:user-1:project-1')).toBeNull()
  })

  it('serializes saves and retains edits made while a save is in flight', async () => {
    const studio = await start()
    const pending = deferred<StudioSnapshot>()
    vi.mocked(api.save).mockImplementationOnce(() => pending.promise)
    studio.project.value!.name = 'first'
    const saving = studio.save()
    const submitted = copy(vi.mocked(api.save).mock.calls[0]![0])
    studio.project.value!.name = 'second'
    server.project = { ...submitted, revision: 2 }
    pending.resolve(copy(server))
    await saving
    expect(api.save).toHaveBeenCalledTimes(2)
    expect(vi.mocked(api.save).mock.calls[1]![0]).toMatchObject({ revision: 2, name: 'second' })
    expect(server.project.name).toBe('second')
  })

  it('preserves a conflicting draft instead of overwriting another tab', async () => {
    const studio = await start()
    server.project.revision = 2; server.project.name = 'other tab'
    studio.project.value!.name = 'my draft'
    expect(await studio.save()).toBe(false)
    expect(studio.saveState.value).toBe('conflict')
    studio.project.value!.name = 'my later draft'
    await vi.advanceTimersByTimeAsync(1000)
    expect(api.save).toHaveBeenCalledTimes(1)
    expect(server.project.name).toBe('other tab')
    expect(localStorage.getItem('creative-studio:user-1:project-1')).toContain('my later draft')
  })

  it('replays the same revision and request ID after an unconfirmed submission, even after refresh', async () => {
    let studio = await start()
    vi.mocked(api.run).mockRejectedValueOnce(new Error('response lost'))
    await studio.run([node.id])
    const first = vi.mocked(api.run).mock.calls[0]!
    expect(first[3]).toBe(1)
    expect(studio.pendingRun.value).not.toBeNull()
    wrappers[0]!.unmount(); wrappers = []
    studio = await start()
    expect(studio.pendingRun.value?.requestId).toBe(first[2])
    await studio.run()
    const second = vi.mocked(api.run).mock.calls[1]!
    expect(second.slice(0, 4)).toEqual(first.slice(0, 4))
    expect(studio.pendingRun.value).toBeNull()
  })

  it('applies completed results once and preserves manual version selection on reopen', async () => {
    server.assets = [imageAsset('older'), imageAsset('newer')]
    server.jobs = [{ id: 'job-1', run_id: 'run-1', node_id: node.id, kind: 'image', status: 'succeeded', prompt: 'bag', params: node.data.params, asset_id: 'newer', created_at: 20, updated_at: 30 }]
    let studio = await start()
    expect(studio.project.value!.nodes[0]!.data.assetId).toBe('newer')
    studio.selectedId.value = node.id
    studio.useVersion(server.assets[0]!)
    await studio.save()
    expect(server.project.nodes[0]!.data.assetId).toBe('older')
    await vi.advanceTimersByTimeAsync(4500)
    expect(studio.project.value!.nodes[0]!.data.assetId).toBe('older')
    wrappers[0]!.unmount(); wrappers = []
    studio = await start()
    expect(studio.project.value!.nodes[0]!.data.assetId).toBe('older')
    expect(api.asset).toHaveBeenCalledWith('project-1', 'older', expect.any(AbortSignal), true)
  })

  it('does not expose a late previous-account project or media response', async () => {
    const late = deferred<StudioSnapshot>()
    vi.mocked(api.get).mockImplementationOnce(() => late.promise)
    const studio = await start()
    localStorage.setItem('auth_user', JSON.stringify({ id: 2 }))
    window.dispatchEvent(new Event('studio-session-ended'))
    late.resolve(copy(server)); await flushPromises()
    expect(studio.project.value).toBeNull()
    expect(studio.assets.value).toEqual([])
    expect(studio.projects.value).toEqual([])
    expect(studio.previewUrls.value).toEqual({})
  })

  it('does not reopen the previous project after logout during a pending save', async () => {
    const studio = await start()
    const pending = deferred<StudioSnapshot>()
    vi.mocked(api.save).mockImplementationOnce(() => pending.promise)
    studio.project.value!.name = 'edited'
    const opening = studio.open('project-2')
    window.dispatchEvent(new Event('studio-session-ended'))
    pending.resolve(copy(server)); await opening
    expect(studio.project.value).toBeNull()
    expect(api.get).toHaveBeenCalledTimes(1)
  })

  it('builds overview plus four dependent details and pins timeline versions', async () => {
    const studio = await start()
    studio.project.value!.nodes = []; studio.template()
    const p = studio.project.value!
    expect(p.nodes).toHaveLength(5)
    expect(p.edges).toHaveLength(4)
    expect(p.edges.every(e => e.source === p.nodes[0]!.id)).toBe(true)
    server.assets = [imageAsset('older'), imageAsset('newer')]
    studio.assets.value = copy(server.assets)
    studio.addTimeline('older', 'caption')
    expect(studio.timeline.value.clips[0]!.asset_id).toBe('older')
    await studio.render()
    expect(api.render).toHaveBeenCalledWith('project-1', expect.objectContaining({ clips: [expect.objectContaining({ asset_id: 'older', title: 'caption' })] }), expect.any(String), expect.any(AbortSignal))
    expect(api.run).not.toHaveBeenCalled()
  })

  it('only submits unfinished nodes for batch runs', async () => {
    const studio = await start()
    studio.project.value!.nodes[0]!.data.assetId = 'old-output'
    const other = studio.add('video')!
    studio.project.value!.nodes.push({ ...copy(node), id: 'active-image' })
    studio.jobs.value = [{ id: 'active-job', node_id: 'active-image', kind: 'image', status: 'running' } as StudioJob]
    await studio.run()
    expect(vi.mocked(api.run).mock.calls[0]![1]).toEqual([other.id])
  })

  it('retries existing failed video jobs without creating a new run, and pauses uncertain retries', async () => {
    server.project.nodes[0]!.type = 'video'
    server.project.nodes[0]!.data.kind = 'video'
    server.jobs = [{ id: 'video-job', run_id: 'run-1', node_id: node.id, kind: 'video', status: 'failed', prompt: 'motion', params: node.data.params, remote_id: 'upstream-existing', created_at: 20, updated_at: 30 }]
    vi.mocked(api.retry).mockImplementation(async () => copy(server))
    const studio = await start()
    await studio.runNode(node.id)
    expect(api.retry).toHaveBeenCalledWith('project-1', 'video-job', expect.any(AbortSignal))
    expect(api.run).not.toHaveBeenCalled()
    await studio.run()
    expect(api.run).not.toHaveBeenCalled()
    studio.jobs.value[0]!.status = 'interrupted'
    await studio.runNode(node.id)
    expect(api.retry).toHaveBeenCalledTimes(1)
    expect(studio.notice.value).toBe('interruptedHint')
  })

  it('keeps known new results when a stale snapshot arrives from another operation', async () => {
    server.assets = [imageAsset('older'), imageAsset('newer')]
    server.jobs = [{ id: 'job-1', run_id: 'run-1', node_id: node.id, kind: 'image', status: 'succeeded', prompt: 'bag', params: node.data.params, asset_id: 'older', created_at: 20, updated_at: 30 }]
    const old = copy(server)
    const studio = await start()
    await studio.save()
    server.jobs.push({ ...server.jobs[0]!, id: 'job-2', asset_id: 'newer', created_at: 50, updated_at: 60 })
    await vi.advanceTimersByTimeAsync(4500)
    expect(studio.project.value!.nodes[0]!.data.assetId).toBe('newer')
    studio.selectedId.value = node.id
    studio.useVersion(server.assets[0]!)
    vi.mocked(api.cancel).mockResolvedValue(old)
    await studio.jobAction(old.jobs[0]!, 'cancel')
    expect(studio.jobs.value.some(j => j.id === 'job-2')).toBe(true)
    expect(studio.project.value!.nodes[0]!.data.assetId).toBe('older')
    expect(studio.project.value!.nodes[0]!.data.resultId).toBe('job-2')
  })

  it('keeps legacy parameters custom and inherits project defaults for new cards', async () => {
    const studio = await start()
    const legacy = studio.project.value!.nodes[0]!
    studio.project.value!.settings!.image_params.size = '1536x1024'
    const fresh = studio.add('image')!
    expect(studio.effectiveParams(legacy).size).toBe('auto')
    expect(studio.effectiveParams(fresh).size).toBe('1536x1024')
    studio.setParamsMode(fresh.id, 'custom')
    studio.project.value!.settings!.image_params.size = '1024x1536'
    expect(studio.effectiveParams(fresh).size).toBe('1536x1024')
    studio.applyProjectDefaults([legacy.id, fresh.id])
    expect(studio.effectiveParams(legacy).size).toBe('1024x1536')
    expect(studio.effectiveParams(fresh).size).toBe('1024x1536')
    await studio.save()
    expect(server.project.nodes.every(n => n.data.paramsMode === 'project')).toBe(true)
  })

  it('blocks all generation when the server preflight rejects one dependency', async () => {
    const studio = await start()
    vi.mocked(api.preflight).mockResolvedValue({ revision: 1, valid: false, counts: { image: 1, video: 0, total: 1 }, nodes: [], errors: [{ node_id: node.id, code: 'invalid_params', message: 'unsupported image references' }] })
    await studio.run([node.id])
    expect(api.run).not.toHaveBeenCalled()
    expect(studio.pendingRun.value).toBeNull()
    expect(studio.preflight.value?.errors[0]?.node_id).toBe(node.id)
    expect(studio.error.value).toBe('preflightFix')
  })

  it('does not submit a graph edited while preflight was in flight', async () => {
    const studio = await start()
    const pending = deferred<Awaited<ReturnType<typeof api.preflight>>>()
    vi.mocked(api.preflight).mockImplementationOnce(() => pending.promise)
    const running = studio.run([node.id])
    await flushPromises()
    studio.project.value!.nodes[0]!.data.prompt = 'a different product'
    pending.resolve({ revision: 1, valid: true, counts: { image: 1, video: 0, total: 1 }, nodes: [], errors: [] })
    await running
    expect(api.run).not.toHaveBeenCalled()
    expect(studio.pendingRun.value).toBeNull()
    expect(studio.notice.value).toBe('planChanged')
  })

  it('pins marked versions in batch timeline actions and keeps selection transient', async () => {
    const studio = await start()
    server.assets = [imageAsset('preferred')]
    studio.assets.value = copy(server.assets)
    studio.selectedId.value = node.id
    studio.markPreferred(server.assets[0]!)
    studio.selectNodes([node.id])
    studio.addSelectedTimeline()
    await studio.save()
    expect(server.project.nodes[0]!.data.preferredAssetId).toBe('preferred')
    expect(server.project.timeline!.clips[0]!.asset_id).toBe('preferred')
    expect(server.project.nodes[0]).not.toHaveProperty('selected')
  })

  it('counts shared prerequisites once and reuses completed parent versions', async () => {
    const studio = await start()
    const first = studio.add('image')!, second = studio.add('image')!
    studio.project.value!.edges = [{ id: 'a', source: node.id, target: first.id }, { id: 'b', source: node.id, target: second.id }]
    expect(studio.runCounts([first.id, second.id])).toEqual({ image: 3, video: 0, total: 3 })
    studio.project.value!.nodes[0]!.data.assetId = 'completed-parent'
    expect(studio.runCounts([first.id, second.id])).toEqual({ image: 2, video: 0, total: 2 })
    expect(studio.runCounts([node.id, first.id, second.id])).toEqual({ image: 3, video: 0, total: 3 })
  })

  it('does not persist preview URLs in exported project metadata', () => {
    const p: StudioProject = copy(server.project)
    p.nodes[0]!.data.previewUrl = 'blob:transient'
    p.nodes[0]!.data.error = 'private provider diagnostic'
    const result = persistedProject(p)
    expect(JSON.stringify(result)).not.toContain('blob:')
    expect(result.nodes[0]!.data).not.toHaveProperty('error')
  })
})

describe('quick compose project creation', () => {
  const input = (detailCount = 4, videoMode: QuickComposeInput['videoMode'] = 'two'): QuickComposeInput => ({
    subject: 'A brown leather bag', style: 'Warm product photography', overview: '', detailCount, videoMode,
    imageParams: { ...node.data.params }, videoParams: { ...node.data.params, model: 'grok-imagine-video' }
  })

  beforeEach(() => {
    vi.mocked(api.create).mockImplementation(async name => {
      server = { project: { id: 'quick-project', name, revision: 1, nodes: [], edges: [], viewport: { x: 0, y: 0, zoom: 1 }, created_at: 100, updated_at: 100 }, assets: [], jobs: [] }
      return copy(server)
    })
  })

  it.each([
    { details: 0, mode: 'none' as const, images: 1, videos: 0 },
    { details: 0, mode: 'two' as const, images: 1, videos: 1 },
    { details: 4, mode: 'two' as const, images: 5, videos: 2 },
    { details: 2, mode: 'all' as const, images: 3, videos: 3 }
  ])('creates an independent $images image / $videos video graph without generating', async ({ details, mode, images, videos }) => {
    const studio = await start()
    const previous = studio.project.value!
    const previousDraft = copy(previous)
    const form = input(details, mode)
    const flow = await studio.createQuickCompose(form)
    expect(previous).toEqual(previousDraft)
    expect(studio.project.value?.id).toBe('quick-project')
    expect(flow?.nodeIds).toEqual(server.project.nodes.map(n => n.id))
    expect(server.project.nodes.filter(n => n.type === 'image')).toHaveLength(images)
    expect(server.project.nodes.filter(n => n.type === 'video')).toHaveLength(videos)
    const overview = server.project.nodes[0]!
    expect(overview.data.prompt).toBe('quickOverviewDefault')
    for (const n of server.project.nodes.slice(1)) {
      const incoming = server.project.edges.filter(e => e.target === n.id)
      expect(incoming).toHaveLength(1)
      const source = server.project.nodes.find(p => p.id === incoming[0]!.source)!
      expect(source.type).toBe('image')
      if (n.type === 'image') expect(source.id).toBe(overview.id)
    }
    form.subject = 'mutated'; form.imageParams.model = 'different'
    expect(server.project.settings?.subject).toBe('A brown leather bag')
    expect(server.project.settings?.image_params.model).toBe('gpt-image-2')
    expect(api.run).not.toHaveBeenCalled()
  })

  it('adds completed quick compose results to the timeline once after polling', async () => {
    const studio = await start()
    const flow = (await studio.createQuickCompose({ ...input(0, 'none'), autoTimeline: true }))!
    const nodeId = flow.nodeIds[0]!
    server.assets = [{ ...imageAsset('auto-result'), node_id: nodeId }]
    server.jobs = [{ id: 'auto-job', run_id: 'auto-run', node_id: nodeId, kind: 'image', status: 'succeeded', prompt: 'bag', params: node.data.params, asset_id: 'auto-result', created_at: 20, updated_at: 30 }]
    await vi.advanceTimersByTimeAsync(4500)
    expect(studio.timeline.value.clips).toHaveLength(1)
    expect(studio.timeline.value.clips[0]!.asset_id).toBe('auto-result')
    await vi.advanceTimersByTimeAsync(4500)
    expect(studio.timeline.value.clips).toHaveLength(1)
  })

  it('retains the new flow and its IDs after save failure, then saves and submits that flow', async () => {
    const studio = await start()
    vi.mocked(api.save).mockRejectedValueOnce(new Error('save failed'))
    const flow = await studio.createQuickCompose(input())
    expect(flow?.projectId).toBe('quick-project')
    expect(studio.saveState.value).toBe('saveError')
    expect(localStorage.getItem('creative-studio:user-1:quick-project')).toContain('A brown leather bag')
    expect(await studio.run(flow!.nodeIds)).toBe(true)
    expect(api.create).toHaveBeenCalledTimes(1)
    expect(api.run).toHaveBeenCalledWith('quick-project', flow!.nodeIds, expect.any(String), expect.any(Number), expect.any(AbortSignal))
  })

  it('does not create a flow when the current project has an unsaved conflict', async () => {
    const studio = await start()
    studio.project.value!.name = 'unsaved work'
    vi.mocked(api.save).mockRejectedValueOnce({ status: 409, message: 'conflict' })
    expect(await studio.createQuickCompose(input())).toBeUndefined()
    expect(api.create).not.toHaveBeenCalled()
    expect(studio.project.value?.name).toBe('unsaved work')
    expect(studio.busy.value).toBe(false)
  })

  it('keeps the same submission receipt and prevents new creation while a run is unconfirmed', async () => {
    const studio = await start()
    const flow = (await studio.createQuickCompose(input()))!
    vi.mocked(api.run).mockRejectedValueOnce(new Error('response lost'))
    expect(await studio.run(flow.nodeIds)).toBe(false)
    const original = vi.mocked(api.run).mock.calls[0]!
    expect(await studio.createQuickCompose(input())).toBeUndefined()
    expect(api.create).toHaveBeenCalledTimes(1)
    expect(await studio.run(flow.nodeIds)).toBe(true)
    expect(vi.mocked(api.run).mock.calls[1]!.slice(0, 4)).toEqual(original.slice(0, 4))
  })

  it('ignores a double click while creation is in flight and does not attach a late response after logout', async () => {
    const studio = await start()
    const delayed = deferred<StudioSnapshot>()
    vi.mocked(api.create).mockImplementationOnce(() => delayed.promise)
    const creating = studio.createQuickCompose(input())
    await flushPromises()
    expect(await studio.createQuickCompose(input())).toBeUndefined()
    expect(api.create).toHaveBeenCalledTimes(1)
    window.dispatchEvent(new Event('studio-session-ended'))
    delayed.resolve({ project: { ...copy(server.project), id: 'late-project', nodes: [], edges: [] }, assets: [], jobs: [] })
    expect(await creating).toBeUndefined()
    expect(studio.project.value).toBeNull()
    expect(api.run).not.toHaveBeenCalled()
  })
})
