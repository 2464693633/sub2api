import type { StudioParams } from '@/api/creativeStudio'

export interface QuickComposeInput {
  subject: string
  style: string
  overview: string
  detailCount: number
  videoMode: 'none' | 'two' | 'all'
  imageParams: StudioParams
  videoParams: StudioParams
}

export interface QuickComposeFlow {
  projectId: string
  nodeIds: string[]
}

export function quickComposeCounts(detailCount: number, videoMode: QuickComposeInput['videoMode']) {
  const image = detailCount + 1
  const video = videoMode === 'all' ? image : videoMode === 'two' ? Math.min(2, image) : 0
  return { image, video, total: image + video }
}
