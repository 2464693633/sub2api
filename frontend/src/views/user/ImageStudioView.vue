<template>
  <component :is="standalone ? 'div' : AppLayout" :class="standalone ? 'min-h-screen bg-gray-50 p-3 dark:bg-dark-950' : ''">
    <div class="flex w-full flex-col gap-3 text-gray-900 dark:text-gray-100">
    <!-- 独立窗口标题 / 主界面新窗口入口 -->
    <div v-if="standalone" class="flex items-center justify-between px-1">
      <span class="text-sm font-semibold">{{ t('imageStudio.title') }} <span class="ml-1 text-xs font-normal text-gray-400">{{ t('imageStudio.standaloneTag') }}</span></span>
      <button
        type="button"
        class="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
        @click="reloadStandalone"
      >{{ t('imageStudio.reload') }}</button>
    </div>
    <div v-else class="flex justify-end">
      <button
        type="button"
        class="flex items-center gap-1 rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-500 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600"
        :title="t('imageStudio.openStandaloneTip')"
        @click="openStandalone"
      >⧉ {{ t('imageStudio.openStandalone') }}</button>
    </div>

    <!-- 初始化失败降级:错误条 + 重试,不锁整页(历史仍可浏览) -->
    <div v-if="initError" class="card flex flex-wrap items-center justify-between gap-3 border border-red-200 p-3 dark:border-red-900/50">
      <span class="text-sm text-red-500">{{ initError }}</span>
      <button
        type="button"
        class="rounded-md border border-red-200 px-3 py-1 text-xs font-medium text-red-500 transition-colors hover:bg-red-50 dark:border-red-900/60 dark:hover:bg-red-900/20"
        @click="retryInit"
      >{{ t('imageStudio.retryInit') }}</button>
    </div>

    <div class="grid grid-cols-1 gap-4 xl:grid-cols-[250px_minmax(0,300px)_minmax(0,1fr)_300px]" :class="standalone ? 'xl:h-[calc(100vh-5rem)]' : 'xl:h-[calc(100vh-9rem)]'">
      <!-- 左列:参数 -->
      <div class="card space-y-4 p-4 xl:h-full xl:overflow-y-auto">
        <!-- 模式 -->
        <div class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
          <button
            v-for="m in MODES"
            :key="m.value"
            type="button"
            class="rounded-md px-2 py-1.5 text-xs font-medium transition-colors"
            :class="mode === m.value ? 'bg-white text-primary-600 shadow dark:bg-dark-700' : 'text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'"
            @click="mode = m.value"
          >{{ t(m.label) }}</button>
        </div>

        <!-- 模型 -->
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.model') }}</label>
          <select
            v-model="model"
            class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
          >
            <option v-for="mo in models" :key="mo.id + ':' + mo.group_id" :value="mo.id">{{ mo.id }}{{ mo.group_name ? ' · ' + mo.group_name : '' }}</option>
          </select>
        </div>

        <!-- 高级参数 -->
        <details open>
          <summary class="cursor-pointer text-xs font-medium text-gray-500">{{ t('imageStudio.advancedParams') }}</summary>
          <div class="mt-2 space-y-3">
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('imageStudio.quality') }}</label>
              <div class="grid grid-cols-4 gap-1">
                <button
                  v-for="q in QUALITIES"
                  :key="q"
                  type="button"
                  class="rounded-md border px-1 py-1 text-xs transition-colors"
                  :class="quality === q ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
                  @click="quality = q"
                >{{ q }}</button>
              </div>
            </div>
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('imageStudio.outputFormat') }}</label>
              <div class="grid grid-cols-3 gap-1">
                <button
                  v-for="f in FORMATS"
                  :key="f"
                  type="button"
                  class="rounded-md border px-1 py-1 text-xs uppercase transition-colors"
                  :class="outputFormat === f ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
                  @click="outputFormat = f"
                >{{ f }}</button>
              </div>
            </div>
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('imageStudio.clarity') }}</label>
              <div class="grid grid-cols-3 gap-1">
                <button
                  v-for="c in CLARITIES"
                  :key="c.value"
                  type="button"
                  class="rounded-md border px-1 py-1 text-xs transition-colors"
                  :class="clarity === c.value ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
                  @click="clarity = c.value"
                >{{ c.label }}</button>
              </div>
              <p class="mt-1 text-[11px] text-gray-400">{{ t('imageStudio.clarityNote') }}</p>
            </div>
          </div>
        </details>

        <!-- 画幅 -->
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.size') }}</label>
          <div class="grid grid-cols-3 gap-1">
            <button
              type="button"
              class="rounded-md border px-1 py-1.5 text-xs transition-colors"
              :class="size === 'auto' ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
              @click="size = 'auto'"
            >auto</button>
            <button
              v-for="r in RATIOS"
              :key="r.value"
              type="button"
              class="flex flex-col items-center gap-0.5 rounded-md border px-1 py-1 text-xs transition-colors"
              :class="size === r.value ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
              @click="size = r.value"
            >
              <span class="inline-block border border-current" :style="{ width: `${Math.min(22, 18 * r.w / Math.max(r.w, r.h))}px`, height: `${Math.min(22, 18 * r.h / Math.max(r.w, r.h))}px` }"></span>
              {{ r.value }}
            </button>
          </div>
          <p v-if="size !== 'auto'" class="mt-1 text-[11px] text-gray-400">{{ computedSize }}</p>
        </div>

        <!-- 数量 -->
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.quantity') }}</label>
          <div class="grid grid-cols-5 gap-1">
            <button
              v-for="q in QUANTITIES"
              :key="q"
              type="button"
              class="rounded-md border px-1 py-1 text-xs transition-colors"
              :class="quantity === q ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
              @click="quantity = q"
            >{{ q }}</button>
          </div>
        </div>

        <!-- 预留底部说明 -->
        <p class="text-xs text-gray-400">{{ t('imageStudio.feeNote') }}</p>
      </div>

      <!-- 提示词列 -->
      <div class="card flex flex-col gap-3 p-4 xl:h-full xl:overflow-y-auto">
        <!-- 提示词框 tabs -->
        <div class="flex flex-wrap items-center gap-1.5">
            <span class="text-xs text-gray-400">{{ t('imageStudio.currentEdit') }}</span>
            <button
              v-for="(box, idx) in promptBoxes"
              :key="box.id"
              type="button"
              class="group flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs transition-colors"
              :class="activeBoxId === box.id ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
              @click="activeBoxId = box.id"
            >
              {{ t('imageStudio.promptBoxN', { n: idx + 1 }) }}
              <span
                class="inline-block rounded-full px-1 text-[10px] leading-4"
                :class="box.status === 'done' ? 'bg-green-100 text-green-600 dark:bg-green-900/40 dark:text-green-400' : 'bg-gray-100 text-gray-400 dark:bg-dark-700'"
              >{{ box.status === 'done' ? t('imageStudio.doneTag') : t('imageStudio.draft') }}</span>
              <span
                v-if="promptBoxes.length > 1"
                class="ml-0.5 hidden text-gray-400 hover:text-red-500 group-hover:inline"
                :title="t('imageStudio.removePromptBox')"
                @click.stop="removePromptBox(box.id)"
              >✕</span>
            </button>
            <button
              type="button"
              class="flex h-6 w-6 items-center justify-center rounded-full border border-dashed border-gray-300 text-gray-400 transition-colors hover:border-primary-400 hover:text-primary-500 dark:border-dark-600"
              :title="t('imageStudio.addPromptBox')"
              :disabled="promptBoxes.length >= 8"
              @click="addPromptBox"
            >+</button>
          </div>

          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.prompt') }}</label>
          <textarea
            v-model="prompt"
            rows="8"
            maxlength="4000"
            :placeholder="t('imageStudio.promptPlaceholder')"
            class="w-full h-[280px] min-h-[160px] resize-y rounded-lg border border-gray-300 bg-white p-3 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
          ></textarea>
          <!-- AI 优化提示词:选择密钥与文本模型,一键改写当前提示词 -->
          <div class="mt-1.5 flex flex-wrap items-center justify-end gap-1.5">
            <label class="text-[11px] text-gray-400">{{ t('imageStudio.optKeyLabel') }}</label>
            <select
              v-model="optKeyId"
              class="max-w-[160px] rounded-md border border-gray-200 bg-white px-1.5 py-1 text-xs outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
            >
              <option v-for="k in optKeys" :key="k.id" :value="k.id">{{ k.name }}</option>
            </select>
            <select
              v-model="optModel"
              class="max-w-[170px] rounded-md border border-gray-200 bg-white px-1.5 py-1 text-xs outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
              :disabled="!optKeyId"
            >
              <option v-for="m in optModels" :key="m" :value="m">{{ m }}</option>
            </select>
            <button
              type="button"
              class="rounded-md border border-primary-200 px-2.5 py-1 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-primary-900/60 dark:hover:bg-primary-900/20"
              :disabled="optimizing || !prompt.trim() || !optKeyId || !optModel"
              @click="optimizePrompt"
            >
              <span v-if="optimizing" class="mr-1 inline-block h-3 w-3 animate-spin rounded-full border border-primary-300 border-t-primary-600"></span>
              {{ optimizing ? t('imageStudio.aiOptimizing') : `✨ ${t('imageStudio.aiOptimize')}` }}
            </button>
          </div>
          <div class="mt-1 text-right text-[11px] text-gray-400">{{ 4000 - prompt.length }} / 4000</div>

          <!-- 参考图(图生图) -->
          <div v-if="mode === 'i2i'" class="mt-2">
            <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.refImage') }} {{ refItems.length }}/8</label>
            <div class="flex flex-wrap gap-2">
              <div v-for="(ref, idx) in refItems" :key="ref.url" class="group relative h-20 w-20 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                <img :src="ref.url" class="h-full w-full object-cover" draggable="false" alt="ref" />
                <button
                  type="button"
                  class="absolute right-0.5 top-0.5 hidden h-5 w-5 items-center justify-center rounded-full bg-black/60 text-[10px] text-white group-hover:flex"
                  @click="removeRefItem(idx)"
                >✕</button>
              </div>
              <label
                class="flex h-20 w-20 cursor-pointer flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed border-gray-300 text-gray-400 transition-colors hover:border-primary-400 hover:text-primary-500 dark:border-dark-600"
                @dragover.prevent
                @drop.prevent="onDrop"
              >
                <span class="text-lg leading-none">+</span>
                <span class="px-1 text-center text-[10px] leading-tight">{{ t('imageStudio.pasteHint') }}</span>
                <input type="file" accept="image/png,image/jpeg,image/webp" multiple class="hidden" @change="onRefFilesChange" />
              </label>
            </div>
            <p class="mt-1 text-[11px] text-gray-400">{{ t('imageStudio.refFormats') }}</p>
          </div>

          <!-- 生成/停止按钮 -->
          <div class="mt-3 flex items-center gap-3">
            <button
              v-if="!generating"
              type="button"
              class="flex-1 rounded-lg bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="!!initError || !prompt.trim()"
              @click="generateBatch"
            >
              {{ t('imageStudio.generateN', { n: quantity }) }}
            </button>
            <button
              v-else
              type="button"
              class="flex-1 rounded-lg border border-red-300 bg-transparent px-4 py-2.5 text-sm font-semibold text-red-500 transition-colors hover:bg-red-50 dark:border-red-900/60 dark:hover:bg-red-900/20"
              :disabled="batchCancelRequested"
              @click="cancelBatch"
            >
              {{ batchCancelRequested ? t('imageStudio.cancelling') : t('imageStudio.stopBatch') }}
            </button>
          </div>
          <p v-if="mode === 'i2i' && refItems.length === 0" class="mt-1 text-[11px] text-amber-500">{{ t('imageStudio.refEmptyHint') }}</p>
          <p v-if="generating" class="mt-2 text-center text-[11px] text-gray-400">{{ t('imageStudio.genTimeHint') }}</p>
      </div>

      <!-- 结果区:占满剩余宽度 -->
      <div class="card flex min-h-0 flex-col p-4 xl:h-full">
        <div class="mb-3 flex shrink-0 items-center justify-between">
            <span class="text-sm font-semibold">{{ t('imageStudio.currentBatch') }}</span>
            <button
              v-if="batch.some(s => s.status === 'failed')"
              type="button"
              class="text-xs text-gray-400 hover:text-red-500"
              @click="clearFailed"
            >{{ t('imageStudio.clearFailed') }}</button>
          </div>
          <div class="flex min-h-0 flex-1 flex-col xl:overflow-y-auto">
            <div v-if="batch.length === 0" class="flex flex-1 items-center justify-center text-sm text-gray-400">
              {{ t('imageStudio.emptyResult') }}
            </div>
            <div v-else class="grid grid-cols-[repeat(auto-fit,minmax(240px,1fr))] gap-3">
              <div v-for="slot in batch" :key="slot.slotId" class="group relative overflow-hidden rounded-lg border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800/60">
                <template v-if="slot.status === 'done' && slot.url">
                  <img
                    :src="slot.url"
                    class="aspect-square w-full cursor-zoom-in select-none object-contain"
                    :alt="slot.prompt.slice(0, 30)"
                    draggable="false"
                    @pointerdown="onBatchImgPointerDown($event, slot)"
                    @click="onBatchImgClick(slot)"
                  />
                <div class="absolute inset-x-0 bottom-0 flex flex-wrap items-center justify-center gap-x-2 gap-y-0.5 bg-black/60 px-2 py-1.5 opacity-0 transition-opacity group-hover:opacity-100">
                  <a :href="slot.url" :download="`image-${slot.slotId}.${outputFormat}`" class="text-xs text-white hover:underline">{{ t('imageStudio.download') }}</a>
                  <button type="button" class="text-xs text-white hover:underline" @click="copySlotImage(slot.slotId)">{{ t('imageStudio.copyImage') }}</button>
                  <button type="button" class="text-xs text-primary-300 hover:underline" @click="setRefFromSlot(slot.slotId)">{{ t('imageStudio.setRefImage') }}</button>
                  <button type="button" class="text-xs text-amber-300" :title="t('imageStudio.star')" @click="starSlot(slot.slotId, slot.blob)">★</button>
                  <button type="button" class="text-xs text-white" @click="removeSlot(slot.slotId)">✕</button>
                </div>
                <span class="absolute right-1 top-1 rounded bg-green-500/80 px-1 text-[10px] text-white">{{ t('imageStudio.doneTag') }}</span>
                <div class="absolute bottom-1 left-1 flex items-center gap-1">
                  <span v-if="slot.actualSize" class="rounded bg-black/50 px-1 text-[10px] text-white" :title="t('imageStudio.actualSizeNote')">{{ slot.actualSize }}</span>
                  <span v-if="slot.ms" class="rounded bg-black/50 px-1 text-[10px] text-white">{{ (slot.ms / 1000).toFixed(1) }}s</span>
                </div>
              </template>
              <template v-else-if="slot.status === 'failed'">
                <div class="flex aspect-square w-full flex-col items-center justify-center gap-1 bg-red-50 p-2 text-center dark:bg-red-900/20">
                  <span class="line-clamp-3 text-[10px] text-red-500">{{ slot.error || t('imageStudio.generateFailed') }}</span>
                  <button type="button" class="text-[10px] text-primary-500 underline" @click="retrySlot(slot.slotId)">{{ t('imageStudio.retry') }}</button>
                </div>
                <span class="absolute right-1 top-1 rounded bg-red-500/80 px-1 text-[10px] text-white">{{ t('imageStudio.failedShort') }}</span>
              </template>
              <template v-else>
                <div class="flex aspect-square w-full flex-col items-center justify-center gap-2 bg-gray-50 px-3 text-center dark:bg-dark-800">
                  <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600"></span>
                  <span class="text-[11px] font-medium text-gray-500 dark:text-gray-300">{{ t('imageStudio.elapsed', { n: elapsed }) }}</span>
                  <span class="text-[10px] leading-tight text-gray-400">{{ t('imageStudio.genTimeHint') }}</span>
                </div>
                <span class="absolute bottom-1 left-1/2 -translate-x-1/2 rounded bg-black/50 px-1.5 text-[10px] text-white">{{ slot.status === 'running' ? t('imageStudio.generating') : t('imageStudio.queued') }}</span>
              </template>
              </div>
            </div>
          </div>
        </div>

      <!-- 右列:本地历史 -->
      <div class="card flex flex-col space-y-3 p-4 xl:h-full xl:min-h-0">
        <div class="flex items-center justify-between">
          <span class="text-sm font-semibold">{{ t('imageStudio.history') }}</span>
          <span class="text-xs text-gray-400">{{ history.length }}</span>
        </div>
        <input
          v-model="historySearch"
          type="text"
          :placeholder="t('imageStudio.historySearch')"
          class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
        />

        <!-- 收藏栏 -->
        <div v-if="starredItems.length">
          <div class="mb-1 text-xs text-gray-400">{{ t('imageStudio.starredBar') }}</div>
          <div class="flex gap-2 overflow-x-auto pb-1">
            <div v-for="item in starredItems" :key="`star-${item.id}`" class="group relative h-16 w-16 shrink-0 overflow-hidden rounded-lg border border-amber-300 dark:border-amber-600">
              <img v-if="objectUrlFor(item)" :src="objectUrlFor(item)" class="h-full w-full cursor-pointer object-cover" draggable="false" :alt="item.prompt.slice(0, 20)" @click="restoreHistory(item)" />
              <div class="absolute inset-x-0 bottom-0 flex items-center justify-around bg-black/60 opacity-0 transition-opacity group-hover:opacity-100">
                <a v-if="objectUrlFor(item)" :href="objectUrlFor(item)" :download="`lyozc-${item.ts}.${item.format}`" class="text-[10px] text-white" :title="t('imageStudio.download')">⬇</a>
                <button type="button" class="text-[10px] text-amber-300" :title="t('imageStudio.unstar')" @click="toggleHistoryStar(item)">★</button>
                <button type="button" class="text-[10px] text-white" @click="deleteHistory(item.id)">✕</button>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="text-[11px] text-gray-400">{{ t('imageStudio.starredEmpty') }}</div>

        <!-- 历史列表 -->
        <div class="max-h-[420px] min-h-0 flex-1 space-y-2 overflow-y-auto pr-1 xl:max-h-none">
          <div v-if="filteredHistory.length === 0" class="py-6 text-center text-xs text-gray-400">
            {{ t('imageStudio.historyEmpty') }}
          </div>
          <div
            v-for="item in filteredHistory"
            :key="item.id"
            class="group flex cursor-pointer items-center gap-2 rounded-lg border border-gray-100 p-2 transition-colors hover:border-primary-300 dark:border-dark-700"
            @click="restoreHistory(item)"
          >
            <img v-if="objectUrlFor(item)" :src="objectUrlFor(item)" class="h-12 w-12 shrink-0 rounded object-cover" draggable="false" :alt="item.prompt.slice(0, 20)" />
            <div v-else class="flex h-12 w-12 shrink-0 items-center justify-center rounded bg-red-50 text-[10px] text-red-400 dark:bg-red-900/20">{{ t('imageStudio.failedShort') }}</div>
            <div class="min-w-0 flex-1">
              <div class="truncate text-xs">{{ item.prompt || t('imageStudio.prompt') }}</div>
              <div class="truncate text-[10px] text-gray-400">
                {{ item.model }} · {{ item.actualSize || item.size }} · {{ item.images.length }}p · {{ new Date(item.ts).toLocaleString() }}
              </div>
            </div>
            <div class="flex shrink-0 flex-col items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
              <button
                v-if="objectUrlFor(item)"
                type="button"
                class="text-xs text-gray-400 hover:text-primary-400"
                :title="t('imageStudio.zoomIn')"
                @click.stop="openViewer(objectUrlFor(item), item.prompt, item.actualSize || item.size)"
              >⤢</button>
              <button type="button" class="text-xs" :class="item.starred ? 'text-amber-400' : 'text-gray-300 hover:text-amber-400'" :title="item.starred ? t('imageStudio.unstar') : t('imageStudio.star')" @click.stop="toggleHistoryStar(item)">★</button>
              <button type="button" class="text-xs text-gray-400 hover:text-red-500" @click.stop="deleteHistory(item.id)">✕</button>
            </div>
          </div>
        </div>

        <!-- 存储计量 -->
        <div>
          <div class="mb-1 flex items-center justify-between text-[11px] text-gray-400">
            <span>{{ t('imageStudio.localStorage') }}</span>
            <span>{{ fmtMB(storageUsed) }} / {{ fmtMB(storageQuota) }}</span>
          </div>
          <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div class="h-full rounded-full bg-primary-500 transition-all" :style="{ width: `${storagePercent}%` }"></div>
          </div>
        </div>
        <p class="text-[11px] text-gray-400">{{ t('imageStudio.historyNote') }}</p>

        <!-- 操作按钮 -->
        <div class="grid grid-cols-3 gap-1.5">
          <button type="button" class="rounded-lg border border-gray-200 px-2 py-1.5 text-xs text-gray-600 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600" @click="exportZip">{{ t('imageStudio.exportZip') }}</button>
          <label class="cursor-pointer rounded-lg border border-gray-200 px-2 py-1.5 text-center text-xs text-gray-600 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600">
            {{ t('imageStudio.importZip') }}
            <input type="file" accept=".zip" class="hidden" @change="onImportZipChange" />
          </label>
          <button type="button" class="rounded-lg border border-red-200 px-2 py-1.5 text-xs text-red-500 transition-colors hover:bg-red-50 dark:border-red-900/50 dark:hover:bg-red-900/20" @click="clearAllHistory">{{ t('imageStudio.clearAll') }}</button>
        </div>
      </div>
    </div>

    <!-- 拖拽跟随光标的小图 -->
    <div
      v-if="pointerDrag?.active"
      class="pointer-events-none fixed z-[120] h-24 w-24 -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-lg border-2 border-white/70 opacity-90 shadow-2xl"
      :style="{ left: `${pointerDrag.x}px`, top: `${pointerDrag.y}px` }"
    >
      <img :src="pointerDrag.url" class="h-full w-full object-cover" alt="" draggable="false" />
    </div>

    <!-- 图片放大预览 -->
    <div
      v-if="viewer"
      class="fixed inset-0 z-[100] flex items-center justify-center bg-black/85 p-6"
      @click="closeViewer"
    >
      <button
        type="button"
        class="absolute right-4 top-4 flex h-9 w-9 items-center justify-center rounded-full bg-white/10 text-lg text-white transition-colors hover:bg-white/20"
        :title="t('common.close')"
        @click="closeViewer"
      >✕</button>
      <img
        :src="viewer.url"
        class="max-h-[90vh] max-w-[92vw] rounded-lg object-contain shadow-2xl"
        :alt="viewer.prompt.slice(0, 50)"
        draggable="false"
        @click.stop
      />
      <div class="absolute inset-x-0 bottom-5 mx-auto max-w-[80vw] rounded-lg bg-black/60 px-4 py-2 text-center text-xs text-white/90">
        <span class="block truncate">{{ viewer.prompt }}</span>
        <span v-if="viewer.size" class="mt-0.5 block text-[10px] text-white/70">{{ t('imageStudio.actualSizeNote') }}: {{ viewer.size }}</span>
      </div>
    </div>
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { keysAPI } from '@/api/keys'
import { useAppStore } from '@/stores/app'
import {
  MODES, QUALITIES, FORMATS, CLARITIES, RATIOS, QUANTITIES, IMAGE_MODEL_PATTERN,
  models, model, initError, retryInit,
  mode, quality, outputFormat, clarity, size, quantity, computedSize,
  promptBoxes, activeBoxId, activeBox, prompt, addPromptBox, removePromptBox,
  refItems, addRefFiles, removeRefItem,
  batch, generating, elapsed, batchCancelRequested, generateBatch, cancelBatch,
  clearFailed, removeSlot, retrySlot, starSlot, copySlotImage, setRefFromSlot,
  history, historySearch, filteredHistory, starredItems, storageUsed, storageQuota, storagePercent,
  objectUrlFor, fmtMB, toggleHistoryStar, deleteHistory, clearAllHistory, restoreHistory,
  exportZip, onImportZipChange, loadHistory, updateStorageMeter,
} from '@/composables/useImageStudioEngine'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()

// ===== 独立窗口模式(?standalone=1 时脱离门户布局独立显示) =====
const standalone = computed(() => route.query.standalone === '1')
function openStandalone() {
  window.open(`${window.location.origin}/image-studio?standalone=1`, 'lyozc-image-studio', 'width=1500,height=960')
}
function reloadStandalone() {
  window.location.reload()
}

// ===== AI 优化提示词(选择密钥 → 该分组文本模型 → chat 改写) =====
interface OptKeyItem { id: number; name: string; key: string }
const optKeys = ref<OptKeyItem[]>([])
const optKeyId = ref<number | null>(null)
const optModels = ref<string[]>([])
const optModel = ref('')
const optimizing = ref(false)
const OPT_MODEL_PREFERENCE = /(claude.*(sonnet|opus)|gpt-5|gpt-4o|o4-mini|gemini.*pro|deepseek.*(chat|v3|r1)|qwen.*max|glm-4)/i

async function loadOptKeys() {
  try {
    const res = await keysAPI.list(1, 100, { status: 'active' })
    optKeys.value = (res.items || []).map(k => ({ id: k.id, name: k.name, key: k.key }))
    const saved = Number(localStorage.getItem('image_studio_opt_key_id'))
    optKeyId.value = optKeys.value.some(k => k.id === saved) ? saved : (optKeys.value[0]?.id ?? null)
  } catch {
    optKeys.value = []
  }
}
watch(optKeyId, v => {
  if (v) localStorage.setItem('image_studio_opt_key_id', String(v))
  void loadOptModels()
})

async function loadOptModels() {
  const key = optKeys.value.find(k => k.id === optKeyId.value)
  if (!key) {
    optModels.value = []
    optModel.value = ''
    return
  }
  try {
    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 15000)
    const res = await fetch(`${window.location.origin}/v1/models`, {
      headers: { Authorization: `Bearer ${key.key}` },
      signal: controller.signal
    })
    window.clearTimeout(timer)
    if (!res.ok) throw new Error(String(res.status))
    const j = await res.json()
    const ids: string[] = (j.data || []).map((m: { id: string }) => m.id)
    // 文本模型(排除生图模型),优选对话能力强的
    const textModels = ids.filter(id => !IMAGE_MODEL_PATTERN.test(id))
    textModels.sort((a, b) => Number(OPT_MODEL_PREFERENCE.test(b)) - Number(OPT_MODEL_PREFERENCE.test(a)))
    optModels.value = textModels
    const savedModel = localStorage.getItem('image_studio_opt_model')
    optModel.value = savedModel && textModels.includes(savedModel) ? savedModel : (textModels[0] ?? '')
  } catch {
    optModels.value = []
    optModel.value = ''
  }
}
watch(optModel, v => {
  if (v) localStorage.setItem('image_studio_opt_model', v)
})

function sanitizeOptimizedPrompt(raw: string): string {
  let out = raw.trim()
  out = out.replace(/^```[a-z]*\n?/i, '').replace(/```$/i, '').trim()
  out = out.replace(/^["'「『]+/, '').replace(/["'」』]+$/, '').trim()
  return out
}

async function optimizePrompt() {
  const key = optKeys.value.find(k => k.id === optKeyId.value)
  const box = activeBox.value
  if (!key || !box || !optModel.value || !box.text.trim() || optimizing.value) return
  const original = box.text.trim()
  optimizing.value = true
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 120000)
  try {
    const res = await fetch(`${window.location.origin}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${key.key}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        model: optModel.value,
        stream: false,
        messages: [
          {
            role: 'system',
            content: '你是专业的 AI 绘画提示词优化专家。把用户的生图提示词改写为细节丰富的画面描述:明确画面主体与外观、环境场景、光线氛围、构图视角、艺术风格与质感细节。忠实保留用户原意与关键要素,不新增用户未提及的主题。直接输出优化后的提示词正文,不要任何解释、前缀或引号。'
          },
          { role: 'user', content: original }
        ]
      }),
      signal: controller.signal
    })
    const body = await res.json().catch(() => null) as { error?: { message?: string }; message?: string; choices?: Array<{ message?: { content?: string } }> } | null
    if (!res.ok) {
      throw new Error(body?.error?.message || body?.message || t('imageStudio.optFailed'))
    }
    const content = body?.choices?.[0]?.message?.content || ''
    const optimized = sanitizeOptimizedPrompt(content)
    if (!optimized) throw new Error(t('imageStudio.optFailed'))
    box.text = optimized.slice(0, 4000)
    appStore.showSuccess(t('imageStudio.optDone'))
  } catch (err: unknown) {
    const msg = err instanceof DOMException && err.name === 'AbortError'
      ? t('imageStudio.timeout')
      : (err instanceof Error ? err.message : t('imageStudio.optFailed'))
    appStore.showError(msg)
  } finally {
    optimizing.value = false
    window.clearTimeout(timer)
  }
}

// ===== 图片放大预览 =====
const viewer = ref<{ url: string; prompt: string; size: string } | null>(null)
function openViewer(url: string, promptText: string, size = '') {
  viewer.value = { url, prompt: promptText, size }
}
function closeViewer() {
  viewer.value = null
}

// ===== 拖图变参考图(Pointer Events 自实现) =====
// 原生 HTML5 拖拽会进入 OS 拖拽循环并冻结页面渲染,因此完全弃用原生拖拽:
// Pointer Events 跟踪手势,小图 ghost 跟随光标,松手落 anywhere 即加入参考图;
// 移动距离小于阈值视为点击(打开灯箱)。
interface PointerDragState {
  blob: Blob
  url: string
  name: string
  active: boolean
  startX: number
  startY: number
  x: number
  y: number
}
const POINTER_DRAG_THRESHOLD = 6
const pointerDrag = ref<PointerDragState | null>(null)
let suppressNextImgClick = false

function onBatchImgPointerDown(event: PointerEvent, slot: import('@/composables/useImageStudioEngine').BatchSlot) {
  if (event.pointerType !== 'mouse' || event.button !== 0 || !slot.blob || !slot.url) return
  // 阻止默认的文本选择/原生拖拽接管,否则浏览器会改发 pointercancel 导致拖拽卡死
  event.preventDefault()
  suppressNextImgClick = false
  pointerDrag.value = {
    blob: slot.blob,
    url: slot.url,
    name: `ref-${Date.now()}.${outputFormat.value}`,
    active: false,
    startX: event.clientX,
    startY: event.clientY,
    x: event.clientX,
    y: event.clientY
  }
  // 指针捕获:光标移出元素甚至窗口也能保证收到 move/up 事件
  try { (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId) } catch { /* 捕获失败时退回 window 监听 */ }
  window.addEventListener('pointermove', onBatchImgPointerMove)
  window.addEventListener('pointerup', onBatchImgPointerUp)
  window.addEventListener('pointercancel', onBatchImgPointerCancel)
  window.addEventListener('blur', onBatchImgPointerCancel)
}
function onBatchImgPointerMove(event: PointerEvent) {
  const d = pointerDrag.value
  if (!d) return
  d.x = event.clientX
  d.y = event.clientY
  if (!d.active && Math.hypot(event.clientX - d.startX, event.clientY - d.startY) > POINTER_DRAG_THRESHOLD) {
    d.active = true
  }
}
function onBatchImgPointerUp() {
  finishPointerDrag(true)
}
function onBatchImgPointerCancel() {
  finishPointerDrag(false)
}
function finishPointerDrag(commit: boolean) {
  window.removeEventListener('pointermove', onBatchImgPointerMove)
  window.removeEventListener('pointerup', onBatchImgPointerUp)
  window.removeEventListener('pointercancel', onBatchImgPointerCancel)
  window.removeEventListener('blur', onBatchImgPointerCancel)
  const d = pointerDrag.value
  pointerDrag.value = null
  if (!d || !commit || !d.active) return
  suppressNextImgClick = true
  addRefFiles([new File([d.blob], d.name, { type: d.blob.type || 'image/png' })])
  appStore.showSuccess(t('imageStudio.refAdded'))
}
function onBatchImgClick(slot: import('@/composables/useImageStudioEngine').BatchSlot) {
  if (suppressNextImgClick) {
    suppressNextImgClick = false
    return
  }
  if (slot.url) openViewer(slot.url, slot.prompt, slot.actualSize || slot.size)
}

// ===== 参考图事件薄封装 =====
function onRefFilesChange(event: Event) {
  const input = event.target as HTMLInputElement
  if (input.files?.length) addRefFiles(Array.from(input.files))
  input.value = ''
}
function onDrop(event: DragEvent) {
  const files = event.dataTransfer?.files
  if (files?.length) addRefFiles(Array.from(files))
}
function onPaste(event: ClipboardEvent) {
  const items = event.clipboardData?.items
  if (!items) return
  const files: File[] = []
  for (const it of items) {
    if (it.kind === 'file' && it.type.startsWith('image/')) {
      const f = it.getAsFile()
      if (f) files.push(f)
    }
  }
  if (!files.length) return
  event.preventDefault()
  addRefFiles(files)
}

// ===== 生命周期 =====
// 注意:引擎是模块级单例,生成任务/历史/草稿在路由切换后继续存活,
// 这里只清理本组件自身的窗口监听与 UI 状态,不清理引擎资源。
onMounted(() => {
  void retryInit()
  void loadOptKeys()
  void loadHistory()
  updateStorageMeter()
  window.addEventListener('paste', onPaste)
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  window.removeEventListener('paste', onPaste)
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('pointermove', onBatchImgPointerMove)
  window.removeEventListener('pointerup', onBatchImgPointerUp)
  finishPointerDrag(false)
})

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (pointerDrag.value) {
      finishPointerDrag(false)
      return
    }
    if (viewer.value) closeViewer()
  }
}
</script>
