/**
 * 主页视频背景主题配置。
 * 视频文件位于 frontend/public/landing/，可自行替换（建议 5-10 秒循环风景、
 * H.264 压缩到 2-5MB）；替换后同步更新 src 与 gradient（缩略图渐变）。
 */
export interface HomeHeroTheme {
  key: string
  /** i18n key，位于 home.hero.themes 下 */
  labelKey: string
  src: string
  /** 主题缩略图的渐变配色（无静态缩略图时用于色块展示） */
  gradient: string
}

export const HOME_HERO_THEMES: HomeHeroTheme[] = [
  {
    key: 'aurora',
    labelKey: 'home.hero.themes.aurora',
    src: '/landing/aurora.mp4',
    gradient: 'from-indigo-400 via-purple-400 to-cyan-400'
  },
  {
    key: 'morning',
    labelKey: 'home.hero.themes.morning',
    src: '/landing/morning.mp4',
    gradient: 'from-amber-200 via-orange-300 to-sky-400'
  },
  {
    key: 'water',
    labelKey: 'home.hero.themes.water',
    src: '/landing/water.mp4',
    gradient: 'from-sky-300 via-cyan-400 to-blue-500'
  },
  {
    key: 'forest',
    labelKey: 'home.hero.themes.forest',
    src: '/landing/forest.mp4',
    gradient: 'from-emerald-300 via-green-400 to-teal-600'
  },
  {
    key: 'dawn',
    labelKey: 'home.hero.themes.dawn',
    src: '/landing/dawn.mp4',
    gradient: 'from-rose-300 via-pink-400 to-purple-500'
  }
]

/** 所有主题视频失败或用户偏好减少动效时的静态兜底图 */
export const HOME_HERO_FALLBACK_IMAGE = '/landing/hero-fallback.webp'

/**
 * 主页数据卡片的展示数值（营销数字，按需修改）。
 * 模型数量优先从模型广场接口实时统计，拉取失败时使用 modelsFallback。
 */
export const HOME_HERO_STATS = {
  modelsFallback: '30+',
  platformsValue: '10+',
  uptimeValue: '99.9%'
}
