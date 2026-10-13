import apiClient from './client'
import type { CreativeCanvasNode, CreativeCanvasEdge, CreativeCanvasViewport } from '@/types/creativeCanvas'

export interface StudioModel {
  id: string
  group_id: number
  group_name: string
  platform?: string
}
export interface StudioParams {
  model: string
  group_id: number
  platform?: string
  size: string
  quality: string
  format: string
  seconds: number
  resolution: string
}
export interface StudioSettings {
  subject: string
  style: string
  image_params: StudioParams
  video_params: StudioParams
}
export interface StudioCapabilities {
  profile: string
  max_references: number
  max_seconds?: number
  sizes: string[]
  qualities: string[]
  formats: string[]
  seconds: number[]
  resolutions: string[]
  custom_quality: boolean
  custom_format: boolean
  custom_size: boolean
  custom_seconds: boolean
  custom_resolution: boolean
  notes: string[]
}
export interface StudioPreflight {
  revision: number
  valid: boolean
  counts: { image: number; video: number; total: number }
  nodes: { node_id: string; title: string; kind: 'image' | 'video'; prompt: string; params: StudioParams; inputs: { node_id: string; kind: 'text' | 'asset' | 'job'; asset_id?: string }[] }[]
  errors: { node_id?: string; code: string; message: string }[]
}
export interface StudioAsset {
  id: string
  kind: 'image' | 'video' | 'audio'
  name: string
  mime: string
  size: number
  created_at: number
  node_id?: string
  run_id?: string
}
export interface StudioClip {
  id: string
  asset_id: string
  title: string
  start: number
  duration: number
}
export interface StudioTimeline {
  clips: StudioClip[]
  size: '1280x720' | '720x1280' | '720x720'
  audio_asset_id?: string
  cover_asset_id?: string
  /** Quick compose nodes whose first successful result should be added once. */
  auto_add_node_ids?: string[]
}
export interface StudioProject {
  id: string
  name: string
  revision: number
  nodes: CreativeCanvasNode[]
  edges: CreativeCanvasEdge[]
  viewport: CreativeCanvasViewport
  settings?: StudioSettings
  timeline?: StudioTimeline
  created_at: number
  updated_at: number
}
export interface StudioJob {
  id: string
  run_id: string
  node_id: string
  kind: string
  status: 'queued' | 'waiting' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'interrupted'
  prompt: string
  params: StudioParams
  asset_id?: string
  remote_id?: string
  error?: string
  upstream_terminal?: boolean
  /** ID of the prerequisite job that caused this job to fail before execution. */
  blocked_by?: string
  created_at: number
  updated_at: number
}
export interface StudioSnapshot {
  project: StudioProject
  assets: StudioAsset[]
  jobs: StudioJob[]
}
const base = '/studio/projects'
const projectPath = (id: string) => `${base}/${encodeURIComponent(id)}`

export const creativeStudioAPI = {
  async list(signal?: AbortSignal) {
    return (await apiClient.get<StudioProject[]>(base, { signal })).data
  },
  async create(name: string, signal?: AbortSignal) {
    return (await apiClient.post<StudioSnapshot>(base, { name }, { signal })).data
  },
  async get(id: string, signal?: AbortSignal) {
    return (await apiClient.get<StudioSnapshot>(projectPath(id), { signal })).data
  },
  async save(project: StudioProject, signal?: AbortSignal) {
    return (await apiClient.put<StudioSnapshot>(projectPath(project.id), project, { signal })).data
  },
  async remove(id: string, signal?: AbortSignal) {
    await apiClient.delete(projectPath(id), { signal })
  },
  async upload(id: string, file: File, signal?: AbortSignal) {
    const form = new FormData()
    form.append('file', file)
    return (await apiClient.post<StudioAsset>(`${projectPath(id)}/assets`, form, {
      headers: { 'Content-Type': 'multipart/form-data' }, timeout: 120000, signal
    })).data
  },
  async asset(id: string, assetId: string, signal?: AbortSignal, thumbnail = false) {
    return (await apiClient.get<Blob>(`${projectPath(id)}/assets/${encodeURIComponent(assetId)}`, {
      responseType: 'blob', timeout: 120000, signal, params: thumbnail ? { thumbnail: 1 } : undefined
    })).data
  },
  async run(id: string, nodeIds: string[], requestId: string, revision: number, signal?: AbortSignal) {
    return (await apiClient.post<StudioSnapshot>(`${projectPath(id)}/runs`, {
      request_id: requestId, node_ids: nodeIds, revision
    }, { signal })).data
  },
  async preflight(id: string, nodeIds: string[], revision: number, signal?: AbortSignal) {
    return (await apiClient.post<StudioPreflight>(`${projectPath(id)}/preflight`, { node_ids: nodeIds, revision }, { signal })).data
  },
  async capabilities(kind: 'image' | 'video', model: string, platform?: string, signal?: AbortSignal) {
    return (await apiClient.get<StudioCapabilities>(`${base}/capabilities`, { params: { kind, model, platform }, signal })).data
  },
  async retry(id: string, jobId: string, signal?: AbortSignal) {
    return (await apiClient.post<StudioSnapshot>(`${projectPath(id)}/jobs/${encodeURIComponent(jobId)}/retry`, {}, { signal })).data
  },
  async cancel(id: string, jobId: string, signal?: AbortSignal) {
    return (await apiClient.post<StudioSnapshot>(`${projectPath(id)}/jobs/${encodeURIComponent(jobId)}/cancel`, {}, { signal })).data
  },
  async render(id: string, timeline: StudioTimeline, requestId: string, signal?: AbortSignal) {
    return (await apiClient.post<StudioSnapshot>(`${projectPath(id)}/exports`, {
      request_id: requestId, ...timeline
    }, { signal })).data
  },
  async models(kind: 'image' | 'video', signal?: AbortSignal) {
    // These gateway-style endpoints return {data:[...]}, without the panel envelope.
    const response = (await apiClient.get<{ data: StudioModel[] }>(`/${kind}-studio/models`, { signal })).data
    return response.data || []
  }
}
