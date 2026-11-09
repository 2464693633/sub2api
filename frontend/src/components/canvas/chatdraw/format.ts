/** 对话画图视图的纯展示工具函数 */
import type { ChatQuality } from '@/composables/useChatDrawEngine'

type Tr = (key: string, named?: Record<string, unknown>) => string

export type ToolbarValue = string | number | null
export interface ToolbarOption { value: ToolbarValue; label: string; hint?: string }
export interface LightboxItem { url: string; caption: string; downloadable?: boolean }

export function qualityLabelOf(t: Tr, q: ChatQuality): string {
  return q === 'auto' ? t('canvas.qualityAuto') : q === 'high' ? t('canvas.qualityHigh') : q === 'medium' ? t('canvas.qualityMedium') : t('canvas.qualityLow')
}

/** '16:9' → '16 / 9'(CSS aspect-ratio) */
export function aspectOf(ratio: string): string {
  const [w, h] = ratio.split(':').map(Number)
  return w && h ? `${w} / ${h}` : '1 / 1'
}

/** 竖图时收窄单图宽度(参照:横图/方图定宽,竖图按比例收窄并设下限) */
export function singleImageWidth(ratio: string): string {
  const [w, h] = ratio.split(':').map(Number)
  if (!w || !h || w >= h) return 'min(100%, 24rem)'
  return `min(100%, clamp(14rem, calc(24rem * ${w} / ${h}), 24rem))`
}

export function dayKey(ts: number): string {
  const d = new Date(ts)
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
}

/** 日期分隔条文字:今天 / 昨天 / 本地化日期 */
export function dayLabel(t: Tr, locale: string, ts: number): string {
  const d = new Date(ts)
  const today = new Date()
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1)
  if (dayKey(ts) === dayKey(today.getTime())) return t('canvasChat.today')
  if (dayKey(ts) === dayKey(yesterday.getTime())) return t('canvasChat.yesterday')
  return new Intl.DateTimeFormat(locale, d.getFullYear() === today.getFullYear()
    ? { month: 'long', day: 'numeric', weekday: 'short' }
    : { year: 'numeric', month: 'long', day: 'numeric' }).format(d)
}
