<template>
  <AppLayout>
    <div class="flex w-full flex-col gap-4 text-gray-900 dark:text-gray-100">
    <!-- 页头 -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold">{{ t('imageStudio.title') }}</h1>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('imageStudio.subtitle') }}</p>
      </div>
    </div>

    <div v-if="initError" class="card p-6 text-center text-sm text-red-500">{{ initError }}</div>

    <div v-else class="grid grid-cols-1 gap-4 xl:h-[calc(100vh-8rem)] xl:grid-cols-[270px_minmax(0,1fr)_330px]">
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
            <option v-for="m in models" :key="m" :value="m">{{ m }}</option>
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

      <!-- 中列:提示词框 + 当前批次 -->
      <div class="flex min-h-0 flex-col gap-4">
        <div class="card p-4">
          <!-- 提示词框 tabs -->
          <div class="mb-2 flex flex-wrap items-center gap-1.5">
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
            rows="4"
            maxlength="4000"
            :placeholder="t('imageStudio.promptPlaceholder')"
            class="w-full rounded-lg border border-gray-300 bg-white p-3 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
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

          <!-- 生成按钮 -->
          <div class="mt-3 flex items-center gap-3">
            <button
              type="button"
              class="flex-1 rounded-lg bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="generating || !prompt.trim()"
              @click="generateBatch"
            >
              <span v-if="generating" class="mr-1 inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/40 border-t-white"></span>
              {{ generating ? t('imageStudio.generating') : t('imageStudio.generateN', { n: quantity }) }}
            </button>
          </div>
          <p v-if="mode === 'i2i' && refItems.length === 0" class="mt-1 text-[11px] text-amber-500">{{ t('imageStudio.refEmptyHint') }}</p>
        </div>

        <!-- 当前批次 -->
        <div class="card flex min-h-0 flex-col p-4 xl:flex-1">
          <div class="mb-3 flex items-center justify-between">
            <span class="text-sm font-semibold">{{ t('imageStudio.currentBatch') }}</span>
            <button
              v-if="batch.some(s => s.status === 'failed')"
              type="button"
              class="text-xs text-gray-400 hover:text-red-500"
              @click="clearFailed"
            >{{ t('imageStudio.clearFailed') }}</button>
          </div>
          <div class="min-h-0 flex-1 xl:overflow-y-auto">
            <div v-if="batch.length === 0" class="flex min-h-64 items-center justify-center text-sm text-gray-400">
              {{ t('imageStudio.emptyResult') }}
            </div>
            <div v-else class="grid grid-cols-2 gap-3 md:grid-cols-3">
              <div v-for="slot in batch" :key="slot.slotId" class="group relative overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
                <template v-if="slot.status === 'done' && slot.url">
                  <img
                    :src="slot.url"
                    class="aspect-square w-full cursor-zoom-in select-none object-cover"
                    :alt="slot.prompt.slice(0, 30)"
                    draggable="false"
                    @pointerdown="onBatchImgPointerDown($event, slot)"
                    @click="onBatchImgClick(slot)"
                  />
                <div class="absolute inset-x-0 bottom-0 flex flex-wrap items-center justify-center gap-x-2 gap-y-0.5 bg-black/60 px-2 py-1 opacity-0 transition-opacity group-hover:opacity-100">
                  <a :href="slot.url" :download="`image-${slot.slotId}.${outputFormat}`" class="text-[10px] text-white hover:underline">{{ t('imageStudio.download') }}</a>
                  <button type="button" class="text-[10px] text-white hover:underline" @click="copySlotImage(slot.slotId)">{{ t('imageStudio.copyImage') }}</button>
                  <button type="button" class="text-[10px] text-primary-300 hover:underline" @click="setRefFromSlot(slot.slotId)">{{ t('imageStudio.setRefImage') }}</button>
                  <button type="button" class="text-[10px] text-amber-300" :title="t('imageStudio.star')" @click="starSlot(slot.slotId, slot.blob)">★</button>
                  <button type="button" class="text-[10px] text-white" @click="removeSlot(slot.slotId)">✕</button>
                </div>
                <span class="absolute right-1 top-1 rounded bg-green-500/80 px-1 text-[10px] text-white">{{ t('imageStudio.doneTag') }}</span>
                <span v-if="slot.actualSize" class="absolute bottom-1 left-1 rounded bg-black/50 px-1 text-[10px] text-white" :title="t('imageStudio.actualSizeNote')">{{ slot.actualSize }}</span>
                <span v-if="slot.ms" class="absolute bottom-1 left-1 translate-x-[calc(100%+2px)] rounded bg-black/50 px-1 text-[10px] text-white">{{ (slot.ms / 1000).toFixed(1) }}s</span>
              </template>
              <template v-else-if="slot.status === 'failed'">
                <div class="flex aspect-square w-full flex-col items-center justify-center gap-1 bg-red-50 p-2 text-center dark:bg-red-900/20">
                  <span class="line-clamp-3 text-[10px] text-red-500">{{ slot.error || t('imageStudio.generateFailed') }}</span>
                  <button type="button" class="text-[10px] text-primary-500 underline" @click="retrySlot(slot.slotId)">{{ t('imageStudio.retry') }}</button>
                </div>
                <span class="absolute right-1 top-1 rounded bg-red-500/80 px-1 text-[10px] text-white">{{ t('imageStudio.failedShort') }}</span>
              </template>
              <template v-else>
                <div class="flex aspect-square w-full items-center justify-center bg-gray-50 dark:bg-dark-800">
                  <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600"></span>
                </div>
                <span class="absolute bottom-1 left-1/2 -translate-x-1/2 rounded bg-black/50 px-1.5 text-[10px] text-white">{{ slot.status === 'running' ? t('imageStudio.generating') : t('imageStudio.queued') }}</span>
              </template>
              </div>
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
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import JSZip from 'jszip'
import AppLayout from '@/components/layout/AppLayout.vue'
import { keysAPI } from '@/api/keys'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

// ===== 常量 =====
const MODES = [
  { value: 't2i' as const, label: 'imageStudio.modeText2Image' },
  { value: 'i2i' as const, label: 'imageStudio.modeImage2Image' }
]
const QUALITIES = ['auto', 'high', 'medium', 'low'] as const
const FORMATS = ['png', 'jpeg', 'webp'] as const
const CLARITIES = [
  { value: '1k' as const, label: '1K' },
  { value: '2k' as const, label: '2K' },
  { value: '4k' as const, label: '4K' }
]
const RATIOS = [
  { value: '1:1', w: 1, h: 1 },
  { value: '16:9', w: 16, h: 9 },
  { value: '9:16', w: 9, h: 16 },
  { value: '4:3', w: 4, h: 3 },
  { value: '3:4', w: 3, h: 4 },
  { value: '3:2', w: 3, h: 2 },
  { value: '2:3', w: 2, h: 3 },
  { value: '21:9', w: 21, h: 9 }
]
const QUANTITIES = [1, 4, 8, 16, 32, 50]
const MAX_REFS = 8
const MAX_PROMPT_BOXES = 8
const IMAGE_MODEL_PATTERN = /(image|dall|flux|seedream|banana|diffusion)/i
const FALLBACK_MODELS = ['gpt-image-2.5', 'gpt-image-2', 'gpt-image-1', 'dall-e-3']
const CLARITY_BASE: Record<string, number> = { '1k': 1024, '2k': 2048, '4k': 4096 }

// ===== 类型 =====
interface PromptBox { id: number; text: string; status: 'draft' | 'done' }
interface BatchSlot {
  slotId: number
  status: 'running' | 'done' | 'failed'
  prompt: string
  size: string
  actualSize?: string
  blob?: Blob
  url?: string
  error?: string
  ms?: number
}
interface HistoryImage { blob: Blob; failed: boolean }
interface HistoryItem {
  id: number
  ts: number
  model: string
  quality: string
  format: string
  size: string
  actualSize?: string
  prompt: string
  mode: 't2i' | 'i2i'
  images: HistoryImage[]
  starred: boolean
}

// ===== 状态 =====
const models = ref<string[]>([...FALLBACK_MODELS])
const model = ref(FALLBACK_MODELS[0])
const initError = ref('')
const mode = ref<'t2i' | 'i2i'>('t2i')
const quality = ref<'auto' | 'high' | 'medium' | 'low'>('auto')
const outputFormat = ref<'png' | 'jpeg' | 'webp'>('png')
const clarity = ref<'1k' | '2k' | '4k'>('1k')
const size = ref<string>('auto')
const quantity = ref(1)
const generating = ref(false)

const promptBoxes = ref<PromptBox[]>([{ id: 1, text: '', status: 'draft' }])
const activeBoxId = ref(1)
let boxSeq = 1
const activeBox = computed(() => promptBoxes.value.find(b => b.id === activeBoxId.value))
const prompt = computed({
  get: () => activeBox.value?.text ?? '',
  set: (v: string) => { if (activeBox.value) activeBox.value.text = v }
})

interface RefItem { file: File; url: string }
const refItems = ref<RefItem[]>([])

const batch = ref<BatchSlot[]>([])
let slotSeq = 0

const history = ref<HistoryItem[]>([])
const historySearch = ref('')
const objectUrlCache = new Map<number, string>()
const storageUsed = ref(0)
const storageQuota = ref(0)

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
    const res = await fetch(`${gatewayBase.value}/v1/models`, {
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
    const res = await fetch(`${gatewayBase.value}/v1/chat/completions`, {
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

// 解码图片 Blob 的真实像素尺寸(请求的清晰度只是参数,实际以输出为准)
async function imageSizeOf(blob: Blob): Promise<string> {
  try {
    const bmp = await createImageBitmap(blob)
    const s = `${bmp.width}x${bmp.height}`
    bmp.close()
    return s
  } catch {
    return ''
  }
}

// ===== 拖图变参考图(Pointer Events 自实现) =====
// 原生 HTML5 拖拽会进入 OS 拖拽循环,实测在本页面主线程反复阻塞(采样最大 1s+),
// 因此完全弃用原生拖拽:Pointer Events 跟踪手势,小图 ghost 跟随光标,
// 松手落 anywhere 即加入参考图;移动距离小于阈值视为点击(打开灯箱)。
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

function onBatchImgPointerDown(event: PointerEvent, slot: BatchSlot) {
  if (event.pointerType !== 'mouse' || event.button !== 0 || !slot.blob || !slot.url) return
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
  window.addEventListener('pointermove', onBatchImgPointerMove)
  window.addEventListener('pointerup', onBatchImgPointerUp)
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
  window.removeEventListener('pointermove', onBatchImgPointerMove)
  window.removeEventListener('pointerup', onBatchImgPointerUp)
  const d = pointerDrag.value
  pointerDrag.value = null
  if (!d) return
  if (!d.active) return
  suppressNextImgClick = true
  addRefFiles([new File([d.blob], d.name, { type: d.blob.type || 'image/png' })])
  appStore.showSuccess(t('imageStudio.refAdded'))
}
function onBatchImgClick(slot: BatchSlot) {
  if (suppressNextImgClick) {
    suppressNextImgClick = false
    return
  }
  if (slot.url) openViewer(slot.url, slot.prompt, slot.actualSize || slot.size)
}

const gatewayBase = computed(() => window.location.origin)
const sessionAuthHeader = computed(() => {
  const token = localStorage.getItem('auth_token') || ''
  return { Authorization: `Bearer ${token}` }
})

const computedSize = computed(() => {
  if (size.value === 'auto') {
    // 画幅 auto 时清晰度也要生效:2K/4K 请求方形大图(1K 保持 auto 由上游默认)
    if (clarity.value !== '1k') {
      const base = CLARITY_BASE[clarity.value] || 1024
      return `${base}x${base}`
    }
    return 'auto'
  }
  const [wStr, hStr] = size.value.split(':')
  const w = Number(wStr)
  const h = Number(hStr)
  if (!w || !h) return 'auto'
  const base = CLARITY_BASE[clarity.value] || 1024
  const ratio = w / h
  let width = base
  let height = base
  if (ratio >= 1) height = Math.round(base / ratio)
  else width = Math.round(base * ratio)
  return `${width}x${height}`
})

const filteredHistory = computed(() => {
  const kw = historySearch.value.trim().toLowerCase()
  if (!kw) return history.value
  return history.value.filter(it => it.prompt.toLowerCase().includes(kw) || it.model.toLowerCase().includes(kw))
})
const starredItems = computed(() => history.value.filter(it => it.starred))
const storagePercent = computed(() => (storageQuota.value > 0 ? Math.min(100, (storageUsed.value / storageQuota.value) * 100) : 0))

// ===== 提示词框 =====
function addPromptBox() {
  if (promptBoxes.value.length >= MAX_PROMPT_BOXES) return
  const id = ++boxSeq
  promptBoxes.value = [...promptBoxes.value, { id, text: '', status: 'draft' }]
  activeBoxId.value = id
}
function removePromptBox(id: number) {
  if (promptBoxes.value.length <= 1) return
  const idx = promptBoxes.value.findIndex(b => b.id === id)
  promptBoxes.value = promptBoxes.value.filter(b => b.id !== id)
  if (activeBoxId.value === id) {
    const next = promptBoxes.value[Math.max(0, idx - 1)]
    activeBoxId.value = next ? next.id : promptBoxes.value[0]?.id ?? 1
  }
}

// ===== 参考图 =====
function addRefFiles(incoming: File[]) {
  const images = incoming.filter(f => f.type.startsWith('image/'))
  if (!images.length) return
  const room = MAX_REFS - refItems.value.length
  if (room <= 0) {
    appStore.showError(t('imageStudio.maxRefs'))
    return
  }
  const accepted = images.slice(0, room)
  for (const f of accepted) {
    refItems.value.push({ file: f, url: URL.createObjectURL(f) })
  }
  mode.value = 'i2i'
  if (images.length > accepted.length) appStore.showError(t('imageStudio.maxRefs'))
}
function removeRefItem(idx: number) {
  const item = refItems.value[idx]
  if (!item) return
  URL.revokeObjectURL(item.url)
  refItems.value = refItems.value.filter((_, i) => i !== idx)
}
function clearRefItems() {
  refItems.value.forEach(r => URL.revokeObjectURL(r.url))
  refItems.value = []
}
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

// ===== 模型(会话直通,无需 API Key) =====
async function loadModels() {
  initError.value = ''
  try {
    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 15000)
    const res = await fetch(`${gatewayBase.value}/api/v1/image-studio/models`, {
      headers: sessionAuthHeader.value,
      signal: controller.signal
    })
    window.clearTimeout(timer)
    const body = await res.json().catch(() => null) as { message?: string; error?: { message?: string }; data?: Array<{ id: string }> } | null
    if (!res.ok) {
      throw new Error(body?.message || body?.error?.message || `HTTP ${res.status}`)
    }
    const ids: string[] = (body?.data || []).map((m) => m.id)
    const imageModels = ids.filter(id => IMAGE_MODEL_PATTERN.test(id))
    if (!imageModels.length) {
      initError.value = t('imageStudio.noImageModels')
      return
    }
    models.value = imageModels
    if (!models.value.includes(model.value)) model.value = models.value[0] || FALLBACK_MODELS[0]
  } catch (err: unknown) {
    const msg = err instanceof DOMException && err.name === 'AbortError'
      ? t('imageStudio.timeout')
      : (err instanceof Error ? err.message : t('imageStudio.noKeys'))
    initError.value = msg
    models.value = FALLBACK_MODELS
    model.value = FALLBACK_MODELS[0]
  }
}

// ===== IndexedDB =====
function openIdb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open('image-studio-db', 1)
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains('history')) {
        req.result.createObjectStore('history', { keyPath: 'id' })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}
async function idbPut(item: HistoryItem) {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    tx.objectStore('history').put(item)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}
async function idbPutMany(items: HistoryItem[]) {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    const store = tx.objectStore('history')
    for (const it of items) store.put(it)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}
async function idbAll(): Promise<HistoryItem[]> {
  const db = await openIdb()
  const items = await new Promise<HistoryItem[]>((resolve, reject) => {
    const req = db.transaction('history', 'readonly').objectStore('history').getAll()
    req.onsuccess = () => resolve(req.result as HistoryItem[])
    req.onerror = () => reject(req.error)
  })
  db.close()
  return items
}
async function idbDelete(id: number) {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    tx.objectStore('history').delete(id)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}
async function idbClear() {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    tx.objectStore('history').clear()
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}

async function loadHistory() {
  try {
    const items = await idbAll()
    // 过滤早期版本写入的不完整记录(缺少 images 数组会导致渲染崩溃)
    history.value = items
      .filter(it => it && Array.isArray(it.images))
      .sort((a, b) => b.ts - a.ts)
  } catch {
    history.value = []
  }
}

function updateStorageMeter() {
  if (navigator.storage?.estimate) {
    void navigator.storage.estimate().then(est => {
      storageUsed.value = est.usage ?? 0
      storageQuota.value = est.quota ?? 0
    })
  }
}
function fmtMB(n: number): string {
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

function objectUrlFor(item: HistoryItem): string {
  if (!item || !Array.isArray(item.images)) return ''
  const cached = objectUrlCache.get(item.id)
  if (cached) return cached
  const first = item.images.find(img => img && !img.failed)
  if (!first) return ''
  const url = URL.createObjectURL(first.blob)
  objectUrlCache.set(item.id, url)
  return url
}

async function toggleHistoryStar(item: HistoryItem) {
  item.starred = !item.starred
  await idbPut(item).catch(() => undefined)
}
async function deleteHistory(id: number) {
  const cached = objectUrlCache.get(id)
  if (cached) {
    URL.revokeObjectURL(cached)
    objectUrlCache.delete(id)
  }
  history.value = history.value.filter(it => it.id !== id)
  await idbDelete(id).catch(() => undefined)
  updateStorageMeter()
}
async function clearAllHistory() {
  if (!window.confirm(t('imageStudio.clearAllConfirm'))) return
  objectUrlCache.forEach(url => URL.revokeObjectURL(url))
  objectUrlCache.clear()
  history.value = []
  await idbClear().catch(() => undefined)
  updateStorageMeter()
}
function restoreHistory(item: HistoryItem) {
  batch.value.forEach(s => { if (s.url) URL.revokeObjectURL(s.url) })
  batch.value = item.images.map(img => ({
    slotId: ++slotSeq,
    status: (img.failed ? 'failed' : 'done') as BatchSlot['status'],
    prompt: item.prompt,
    size: item.size,
    actualSize: img.failed ? undefined : item.actualSize,
    blob: img.failed ? undefined : img.blob,
    url: img.failed ? undefined : URL.createObjectURL(img.blob),
    error: img.failed ? t('imageStudio.generateFailed') : undefined
  }))
  if (activeBox.value) activeBox.value.text = item.prompt
  if (models.value.includes(item.model)) model.value = item.model
  appStore.showSuccess(t('imageStudio.restored'))
}

// ===== ZIP 导出 / 导入 =====
function blobToB64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result).split(',')[1] || '')
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(blob)
  })
}
async function exportZip() {
  if (!history.value.length) return
  const zip = new JSZip()
  const metas: Record<string, unknown>[] = []
  for (const item of history.value) {
    const folder = zip.folder(String(item.ts))
    if (!folder) continue
    for (let i = 0; i < item.images.length; i++) {
      const img = item.images[i]
      const ext = img.failed ? 'txt' : (item.format || 'png')
      if (img.failed) {
        folder.file(`image-${i + 1}-failed.txt`, t('imageStudio.generateFailed'))
      } else {
        folder.file(`image-${i + 1}.${ext}`, await blobToB64(img.blob), { base64: true })
      }
    }
    metas.push({
      ts: item.ts, model: item.model, quality: item.quality, format: item.format,
      size: item.size, actualSize: item.actualSize, prompt: item.prompt, mode: item.mode, starred: item.starred
    })
  }
  zip.file('meta.json', JSON.stringify(metas, null, 2))
  const blob = await zip.generateAsync({ type: 'blob' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `image-studio-${Date.now()}.zip`
  a.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 30000)
}
async function onImportZipChange(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files || !input.files[0]) return
  try {
    const zip = await JSZip.loadAsync(input.files[0])
    let metas: Record<string, unknown>[] = []
    const metaFile = zip.file('meta.json')
    if (metaFile) {
      try { metas = JSON.parse(await metaFile.async('string')) as Record<string, unknown>[] } catch { metas = [] }
    }
    const folders = new Set<string>()
    Object.keys(zip.files).forEach(path => {
      const seg = path.split('/')[0]
      if (/^\d+$/.test(seg)) folders.add(seg)
    })
    const items: HistoryItem[] = []
    for (const folder of folders) {
      const meta = metas.find(m => String(m.ts) === folder) || {}
      const images: HistoryImage[] = []
      for (const path of Object.keys(zip.files)) {
        if (!path.startsWith(`${folder}/image-`) || zip.files[path].dir) continue
        if (path.endsWith('.txt')) {
          images.push({ blob: new Blob([await zip.files[path].async('string')], { type: 'text/plain' }), failed: true })
          continue
        }
        const blob = await zip.files[path].async('blob')
        images.push({ blob, failed: path.includes('-failed') })
      }
      if (!images.length) continue
      items.push({
        id: Number(folder) || Date.now() + items.length,
        ts: Number(meta.ts) || Date.now(),
        model: String(meta.model || 'unknown'),
        quality: String(meta.quality || 'auto'),
        format: String(meta.format || 'png'),
        size: String(meta.size || '1024x1024'),
        actualSize: meta.actualSize ? String(meta.actualSize) : undefined,
        prompt: String(meta.prompt || ''),
        mode: (meta.mode as 't2i' | 'i2i') || 't2i',
        images,
        starred: Boolean(meta.starred)
      })
    }
    if (items.length) {
      await idbPutMany(items)
      await loadHistory()
      appStore.showSuccess(t('imageStudio.importDone', { n: items.length }))
      updateStorageMeter()
    }
  } catch {
    appStore.showError(t('imageStudio.importFailed'))
  }
  input.value = ''
}

// ===== 生成 =====
function blobFromB64(b64: string): Blob {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return new Blob([bytes], { type: 'image/png' })
}
async function blobFromUrl(url: string): Promise<Blob> {
  const res = await fetch(url)
  return await res.blob()
}

async function generateOne(promptText: string, refList: File[]): Promise<{ blob: Blob | null; error?: string }> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 300000)
  try {
    let res: Response
    if (mode.value === 'i2i' && refList.length) {
      const form = new FormData()
      form.append('model', model.value)
      form.append('prompt', promptText)
      if (computedSize.value !== 'auto') form.append('size', computedSize.value)
      if (quality.value !== 'auto') form.append('quality', quality.value)
      form.append('output_format', outputFormat.value)
      for (const f of refList) form.append('image', f)
      res = await fetch(`${gatewayBase.value}/api/v1/image-studio/edits`, {
        method: 'POST',
        headers: sessionAuthHeader.value,
        body: form,
        signal: controller.signal
      })
    } else {
      res = await fetch(`${gatewayBase.value}/api/v1/image-studio/generations`, {
        method: 'POST',
        headers: {
          ...sessionAuthHeader.value,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          model: model.value,
          prompt: promptText,
          size: computedSize.value === 'auto' ? undefined : computedSize.value,
          quality: quality.value === 'auto' ? undefined : quality.value,
          output_format: outputFormat.value,
          n: 1
        }),
        signal: controller.signal
      })
    }
    const body = await res.json().catch(() => null) as { error?: { message?: string }; message?: string; data?: Array<{ b64_json?: string; url?: string }> } | null
    if (!res.ok) {
      const msg = body?.error?.message || body?.message || t('imageStudio.generateFailed')
      return { blob: null, error: msg }
    }
    const data = body?.data || []
    if (!data.length) return { blob: null, error: t('imageStudio.generateFailed') }
    const first = data[0]
    const blob = first?.b64_json
      ? blobFromB64(first.b64_json)
      : (first?.url ? await blobFromUrl(first.url) : null)
    if (!blob) return { blob: null, error: t('imageStudio.generateFailed') }
    return { blob }
  } catch (err: unknown) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      return { blob: null, error: t('imageStudio.timeout') }
    }
    return { blob: null, error: err instanceof Error ? err.message : t('imageStudio.generateFailed') }
  } finally {
    window.clearTimeout(timer)
  }
}

function clearFailed() {
  batch.value = batch.value.filter(s => s.status !== 'failed')
}
function removeSlot(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (slot?.url) URL.revokeObjectURL(slot.url)
  batch.value = batch.value.filter(s => s.slotId !== slotId)
}
function starSlot(slotId: number, blob?: Blob) {
  const slot = batch.value.find(s => s.slotId === slotId)
  const b = blob || slot?.blob
  if (!b) return
  const item: HistoryItem = {
    id: Date.now(),
    ts: Date.now(),
    model: model.value,
    quality: quality.value,
    format: outputFormat.value,
    size: slot?.size || computedSize.value,
    actualSize: slot?.actualSize,
    prompt: slot?.prompt || '',
    mode: mode.value,
    images: [{ blob: b, failed: false }],
    starred: true
  }
  history.value = [item, ...history.value]
  void idbPut(item)
  updateStorageMeter()
  appStore.showSuccess(t('imageStudio.starredSaved'))
}
async function copySlotImage(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (!slot?.blob) return
  try {
    const type = slot.blob.type || 'image/png'
    await navigator.clipboard.write([new ClipboardItem({ [type]: slot.blob })])
    appStore.showSuccess(t('imageStudio.copied'))
  } catch {
    appStore.showError(t('imageStudio.copyFailed'))
  }
}
function setRefFromSlot(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (!slot?.blob) return
  const file = new File([slot.blob], `ref-${slotId}.${outputFormat.value}`, { type: slot.blob.type || 'image/png' })
  addRefFiles([file])
  appStore.showSuccess(t('imageStudio.refAdded'))
}
async function retrySlot(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (!slot || generating.value) return
  slot.status = 'running'
  const refs = mode.value === 'i2i' ? refItems.value.map(r => r.file) : []
  const started = performance.now()
  const result = await generateOne(slot.prompt, refs)
  slot.ms = Math.round(performance.now() - started)
  slot.status = result.blob ? 'done' : 'failed'
  slot.error = result.error
  if (result.blob) {
    slot.blob = result.blob
    slot.url = URL.createObjectURL(result.blob)
    slot.actualSize = await imageSizeOf(result.blob)
  }
  batch.value = [...batch.value]
}

async function generateBatch() {
  const box = activeBox.value
  if (!box || initError.value || !box.text.trim()) return
  const n = quantity.value
  const promptText = box.text.trim()
  const refs = mode.value === 'i2i' ? refItems.value.map(r => r.file) : []
  batch.value.forEach(s => { if (s.url) URL.revokeObjectURL(s.url) })
  batch.value = []
  generating.value = true
  let okCount = 0
  try {
    for (let i = 0; i < n; i++) {
      const slotId = ++slotSeq
      const slot: BatchSlot = { slotId, status: 'running', prompt: promptText, size: computedSize.value }
      batch.value = [...batch.value, slot]
      const started = performance.now()
      const result = await generateOne(promptText, refs)
      slot.ms = Math.round(performance.now() - started)
      if (result.blob) {
        slot.blob = result.blob
        slot.url = URL.createObjectURL(result.blob)
        slot.actualSize = await imageSizeOf(result.blob)
        slot.status = 'done'
        okCount++
      } else {
        slot.status = 'failed'
        slot.error = result.error
      }
      batch.value = [...batch.value]
    }
    if (okCount > 0) {
      box.status = 'done'
      appStore.showSuccess(t('imageStudio.batchDone', { ok: okCount, total: n }))
    }
  } finally {
    generating.value = false
    const done = batch.value.filter(s => s.status === 'done' && s.blob)
    if (done.length) {
      const item: HistoryItem = {
        id: Date.now(),
        ts: Date.now(),
        model: model.value,
        quality: quality.value,
        format: outputFormat.value,
        size: computedSize.value,
        actualSize: done[0]?.actualSize,
        prompt: promptText,
        mode: mode.value,
        images: done.map(s => ({ blob: s.blob as Blob, failed: false })),
        starred: false
      }
      await idbPut(item).catch(() => undefined)
      history.value = [item, ...history.value]
      updateStorageMeter()
    }
  }
}

// ===== 生命周期 =====
onMounted(() => {
  void loadModels()
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
  batch.value.forEach(s => { if (s.url) URL.revokeObjectURL(s.url) })
  clearRefItems()
  objectUrlCache.forEach(url => URL.revokeObjectURL(url))
  objectUrlCache.clear()
})

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && viewer.value) closeViewer()
}
</script>

