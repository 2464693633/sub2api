import type { StudioParams } from '@/api/creativeStudio'

export interface QuickComposeInput {
  subject: string
  style: string
  overview: string
  detailCount: number
  videoMode: 'none' | 'two' | 'all'
  /** Automatically append each first successful output to the project timeline. */
  autoTimeline?: boolean
  imageParams: StudioParams
  videoParams: StudioParams
}

export interface QuickComposeFlow {
  projectId: string
  nodeIds: string[]
  /** Signature of the graph when the quick compose draft was created. */
  graphSignature?: string
  autoTimeline?: boolean
}

export function quickComposeCounts(detailCount: number, videoMode: QuickComposeInput['videoMode']) {
  const image = detailCount + 1
  const video = videoMode === 'all' ? image : videoMode === 'two' ? Math.min(2, image) : 0
  return { image, video, total: image + video }
}
