import type { Edge, Node, ViewportTransform } from '@vue-flow/core'

export type CreativeCanvasKind = 'text' | 'image' | 'video'
export type CreativeCanvasStatus = 'draft' | 'waiting' | 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'interrupted'

export interface CreativeCanvasNodeData {
  kind: CreativeCanvasKind
  title: string
  prompt: string
  status?: CreativeCanvasStatus
  /** A display URL only. Persist the asset/result identity and resolve this URL when opening a project. */
  previewUrl?: string
  assetId?: string
  resultId?: string
  error?: string
  progress?: number
  [key: string]: unknown
}

export type CreativeCanvasNode = Node<CreativeCanvasNodeData> & {
  type: CreativeCanvasKind
  selected?: boolean
  data: CreativeCanvasNodeData
}

export type CreativeCanvasEdge = Edge
export type CreativeCanvasViewport = ViewportTransform

export interface CreativeCanvasConnection {
  source: string
  target: string
}

export type CreativeCanvasConnectionError = 'missing' | 'self' | 'duplicate' | 'unsupported' | 'cycle'

/** Keep the project graph executable: only prompt/image inputs and no cyclic dependencies. */
export function validateCreativeCanvasConnection(
  connection: CreativeCanvasConnection,
  nodes: CreativeCanvasNode[],
  edges: CreativeCanvasEdge[],
): CreativeCanvasConnectionError | null {
  const source = nodes.find(node => node.id === connection.source)
  const target = nodes.find(node => node.id === connection.target)
  if (!source || !target) return 'missing'
  if (source.id === target.id) return 'self'
  if (edges.some(edge => edge.source === source.id && edge.target === target.id)) return 'duplicate'
  if (source.data.kind === 'video' || target.data.kind === 'text') return 'unsupported'

  const visited = new Set<string>()
  const pending = [target.id]
  while (pending.length) {
    const current = pending.pop()!
    if (current === source.id) return 'cycle'
    if (visited.has(current)) continue
    visited.add(current)
    for (const edge of edges) {
      if (edge.source === current) pending.push(edge.target)
    }
  }
  return null
}
