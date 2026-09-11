<template>
  <div class="pointer-events-none absolute inset-0 overflow-hidden bg-dark-950" aria-hidden="true">
    <!-- 静态兜底：视频不可用（全部加载失败 / 减少动效偏好）时始终可见 -->
    <img
      v-if="useFallbackImage"
      :src="fallbackImage"
      class="absolute inset-0 h-full w-full object-cover"
      alt=""
    />
    <template v-else>
      <video
        v-for="(theme, index) in themes"
        :key="theme.src"
        :ref="(el) => setVideoRef(index, el)"
        class="absolute inset-0 h-full w-full object-cover transition-opacity duration-1000 ease-out"
        :class="index === activeIndex ? 'opacity-100' : 'opacity-0'"
        muted
        playsinline
        :autoplay="index === activeIndex && !switching"
        :preload="preloads[index]"
        @error="onError(index)"
        @timeupdate="onTimeUpdate(index)"
        @ended="onEnded(index)"
      >
        <source :src="theme.src" type="video/mp4" />
      </video>
    </template>
    <!-- 渐变蒙层：保证前景文字可读 -->
    <div class="absolute inset-0 bg-gradient-to-b from-black/40 via-black/20 to-black/70" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { HOME_HERO_FALLBACK_IMAGE, type HomeHeroTheme } from '@/config/homeThemes'

const props = withDefaults(
  defineProps<{
    themes: HomeHeroTheme[]
    /** 视频播放结束前多少秒开始预加载下一主题 */
    prewarmSeconds?: number
    fallbackImage?: string
  }>(),
  {
    prewarmSeconds: 12,
    fallbackImage: HOME_HERO_FALLBACK_IMAGE
  }
)

const activeIndex = defineModel<number>('activeIndex', { default: 0 })

const videoRefs = ref<(HTMLVideoElement | null)[]>([])
const preloads = ref<string[]>(props.themes.map(() => 'none'))
const failedIndexes = ref(new Set<number>())
const switching = ref(false)
// 减少动效偏好的用户不播放视频，直接落到静态兜底图
const reducedMotion =
  typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

const useFallbackImage = computed(() => reducedMotion || failedIndexes.value.size >= props.themes.length)

function setVideoRef(index: number, el: unknown) {
  videoRefs.value[index] = (el as HTMLVideoElement | null) ?? null
}

function nextAvailableIndex(from: number): number {
  const total = props.themes.length
  for (let step = 1; step <= total; step++) {
    const candidate = (from + step) % total
    if (!failedIndexes.value.has(candidate)) return candidate
  }
  return -1
}

function waitForReady(video: HTMLVideoElement): Promise<void> {
  // readyState >= 3 (HAVE_FUTURE_DATA) 时可以直接播放，无需重新 load
  if (video.readyState >= 3) return Promise.resolve()
  return new Promise((resolve) => {
    const done = () => {
      video.removeEventListener('canplay', done)
      resolve()
    }
    video.addEventListener('canplay', done)
    // 预加载等待上限，避免弱网下切换永久卡住
    window.setTimeout(done, 4000)
  })
}

/**
 * play() 带 4 秒超时上限；返回是否真正开始播放（收到 playing 事件）。
 * 非浏览器环境（如测试）play() 可能返回非 Promise 或抛错，此时按未播放处理。
 */
function playWithTimeout(video: HTMLVideoElement): Promise<boolean> {
  return new Promise((resolve) => {
    let settled = false
    const finish = (started: boolean) => {
      if (settled) return
      settled = true
      video.removeEventListener('playing', onPlaying)
      resolve(started)
    }
    const onPlaying = () => {
      window.clearTimeout(timeout)
      finish(true)
    }
    const timeout = window.setTimeout(() => finish(false), 4000)
    video.addEventListener('playing', onPlaying, { once: true })
    try {
      const result = video.play() as unknown
      if (result instanceof Promise) {
        result.then(
          () => {
            // play() resolve 不保证已出帧；仍等 playing 事件，最多到超时
            window.setTimeout(() => finish(false), 0)
          },
          () => {
            window.clearTimeout(timeout)
            finish(false)
          }
        )
      } else {
        window.clearTimeout(timeout)
        finish(false)
      }
    } catch {
      window.clearTimeout(timeout)
      finish(false)
    }
  })
}

function pauseAndReset(video: HTMLVideoElement | null) {
  if (!video) return
  video.pause()
  try {
    video.currentTime = 0
  } catch {
    /* 未开始播放前 seek 可能抛错，忽略 */
  }
}

async function selectTheme(index: number) {
  if (index === activeIndex.value || switching.value) return
  if (failedIndexes.value.has(index)) return
  const video = videoRefs.value[index]
  if (!video) return

  switching.value = true
  preloads.value[index] = 'auto'
  try {
    if (video.readyState < 3) video.load()
    await waitForReady(video)
    const started = await playWithTimeout(video)
    if (!started) {
      // 播放未真正开始：不切换画面（保持旧主题可见），标记失败并尝试下一个
      markFailed(index)
      const next = nextAvailableIndex(index === activeIndex.value ? index : activeIndex.value)
      if (next >= 0 && next !== index && next !== activeIndex.value) void selectTheme(next)
      return
    }
    const previous = activeIndex.value
    activeIndex.value = index
    if (previous !== index) pauseAndReset(videoRefs.value[previous])
  } finally {
    switching.value = false
  }
}

function onTimeUpdate(index: number) {
  if (index !== activeIndex.value) return
  const video = videoRefs.value[index]
  if (!video || !video.duration || Number.isNaN(video.duration)) return
  if (video.duration - video.currentTime > props.prewarmSeconds) return
  const next = nextAvailableIndex(index)
  if (next >= 0 && preloads.value[next] !== 'auto') {
    preloads.value[next] = 'auto'
    // 属性变更不会促使浏览器开始缓冲，对空闲元素主动 load() 启动预加载
    videoRefs.value[next]?.load()
  }
}

function onEnded(index: number) {
  if (index !== activeIndex.value) return
  const next = nextAvailableIndex(index)
  if (next === index) {
    // 仅剩这一个可用主题：原地重播，避免画面定格
    const video = videoRefs.value[index]
    if (video) {
      pauseAndReset(video)
      void playWithTimeout(video)
    }
    return
  }
  if (next >= 0) void selectTheme(next)
}

function onError(index: number) {
  markFailed(index)
  if (index === activeIndex.value) {
    const next = nextAvailableIndex(index)
    if (next >= 0) void selectTheme(next)
  }
}

function markFailed(index: number) {
  // 用不可变 Set 重建以触发响应式；从 index 起原子合并，避免并发覆盖
  const next = new Set(failedIndexes.value)
  next.add(index)
  failedIndexes.value = next
  pauseAndReset(videoRefs.value[index])
}

onMounted(() => {
  const initial = activeIndex.value
  if (props.themes[initial]) preloads.value[initial] = 'auto'
})

onBeforeUnmount(() => {
  videoRefs.value.forEach((video) => video?.pause())
})

defineExpose({
  selectTheme,
  isThemeFailed: (index: number) => failedIndexes.value.has(index),
  isSwitching: () => switching.value
})
</script>
