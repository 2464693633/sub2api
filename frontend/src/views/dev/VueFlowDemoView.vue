<template>
  <div class="flex h-screen flex-col bg-dark-950 text-gray-100">
    <!-- 顶栏 -->
    <div class="flex flex-wrap items-center gap-2 border-b border-gray-800 px-4 py-2 text-xs">
      <span class="font-bold text-primary-400">Vue Flow 技术验证页</span>
      <button type="button" class="rounded border border-gray-700 px-2 py-1 hover:border-primary-400" @click="addText">+ 文字</button>
      <button type="button" class="rounded border border-gray-700 px-2 py-1 hover:border-primary-400" @click="addImage">+ 图片</button>
      <button type="button" class="rounded border border-gray-700 px-2 py-1 hover:border-primary-400" @click="addVideo">+ 视频</button>
      <label class="flex items-center gap-1 text-gray-400">
        <input v-model="snapToGrid" type="checkbox" class="accent-primary-500" /> 吸附网格
      </label>
      <button type="button" class="rounded border border-gray-700 px-2 py-1 hover:border-primary-400" @click="fitView({ padding: 0.2, duration: 400 })">适配视图</button>
      <span class="text-gray-500">拖节点 = 移动 · 拖圆点 = 连线 · Shift+拖拽 = 框选 · 滚轮 = 缩放 · Backspace = 删除选中</span>
      <router-link to="/studio" class="ml-auto text-primary-400 underline">返回创作中心</router-link>
    </div>

    <!-- 画布 -->
    <div class="relative min-h-0 flex-1">
      <VueFlow
        v-model:nodes="nodes"
        v-model:edges="edges"
        :snap-to-grid="snapToGrid"
        :snap-grid="[16, 16]"
        :min-zoom="0.2"
        :max-zoom="4"
        :delete-key-code="['Backspace', 'Delete']"
        fit-view-on-init
        class="h-full w-full"
        @connect="onConnect"
      >
        <Background :pattern-color="'#31415a'" :gap="24" />
        <MiniMap position="bottom-left" pannable zoomable />
        <Controls position="bottom-right" />
        <template #node-image="{ data }">
          <div class="vf-demo-node">
            <Handle type="target" :position="Position.Left" />
            <img :src="data.src" class="vf-demo-media" draggable="false" alt="" />
            <Handle type="source" :position="Position.Right" />
          </div>
        </template>
        <template #node-video="{ data }">
          <div class="vf-demo-node">
            <Handle type="target" :position="Position.Left" />
            <video :src="data.src" controls preload="metadata" class="vf-demo-media" />
            <Handle type="source" :position="Position.Right" />
          </div>
        </template>
        <template #node-text="{ data }">
          <div class="vf-demo-node vf-demo-text">
            <Handle type="target" :position="Position.Left" />
            <textarea
              v-model="data.text"
              class="vf-demo-textarea"
              placeholder="输入文字…"
              @mousedown.stop
              @keydown.stop
            ></textarea>
            <Handle type="source" :position="Position.Right" />
          </div>
        </template>
      </VueFlow>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { VueFlow, useVueFlow, Handle, Position } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'

const { addNodes, addEdges, fitView } = useVueFlow()

const snapToGrid = ref(false)
const nodes = ref<any[]>([])
const edges = ref<any[]>([])

let seq = 0
function nid() {
  return `n${++seq}_${Date.now().toString(36)}`
}

/** 程序生成一张可辨识的演示图(渐变+大字) */
function makeDemoImage(label: string, hue: number): string {
  const canvas = document.createElement('canvas')
  canvas.width = 480
  canvas.height = 320
  const ctx = canvas.getContext('2d')!
  const grad = ctx.createLinearGradient(0, 0, 480, 320)
  grad.addColorStop(0, `hsl(${hue} 70% 45%)`)
  grad.addColorStop(1, `hsl(${(hue + 60) % 360} 70% 30%)`)
  ctx.fillStyle = grad
  ctx.fillRect(0, 0, 480, 320)
  ctx.fillStyle = '#ffffff'
  ctx.font = 'bold 64px sans-serif'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(label, 240, 160)
  return canvas.toDataURL('image/png')
}

/** 程序生成一段 0.5 秒的演示视频(WebM,Canvas.captureStream) */
async function makeDemoVideo(label: string): Promise<string> {
  const canvas = document.createElement('canvas')
  canvas.width = 480
  canvas.height = 270
  const ctx = canvas.getContext('2d')!
  const stream = canvas.captureStream(30)
  const chunks: Blob[] = []
  const rec = new MediaRecorder(stream, { mimeType: 'video/webm' })
  rec.ondataavailable = e => chunks.push(e.data)
  const done = new Promise<string>(resolve => {
    rec.onstop = () => resolve(URL.createObjectURL(new Blob(chunks, { type: 'video/webm' })))
  })
  rec.start()
  let t = 0
  const timer = window.setInterval(() => {
    t += 0.05
    ctx.fillStyle = '#0b1020'
    ctx.fillRect(0, 0, 480, 270)
    ctx.fillStyle = `hsl(${(t * 80) % 360} 80% 55%)`
    ctx.beginPath()
    ctx.arc(240 + Math.sin(t * 3) * 140, 135 + Math.cos(t * 2) * 60, 40, 0, Math.PI * 2)
    ctx.fill()
    ctx.fillStyle = '#fff'
    ctx.font = 'bold 36px sans-serif'
    ctx.textAlign = 'center'
    ctx.fillText(label, 240, 60)
  }, 50)
  await new Promise(r => setTimeout(r, 1200))
  window.clearInterval(timer)
  rec.stop()
  return await done
}

function baseNode(kind: string, x: number, y: number, label: string) {
  seq++
  return {
    id: nid(),
    type: kind,
    position: { x, y },
    data: { label },
    style: { width: '300px' }
  }
}

function addText() {
  addNodes([{ ...baseNode('text', 80 + seq * 10, 60, '文字节点'), data: { text: '双击右侧节点试试' } }])
}
function addImage() {
  seq++
  addNodes([{
    id: nid(),
    type: 'image',
    position: { x: 60 + seq * 8, y: 200 + seq * 6 },
    data: { src: makeDemoImage(`IMG ${seq}`, (seq * 70) % 360) },
    style: { width: '280px' }
  }])
}
async function addVideo() {
  seq++
  const src = await makeDemoVideo(`VID ${seq}`)
  addNodes([{
    id: nid(),
    type: 'video',
    position: { x: 80 + seq * 8, y: 360 },
    data: { src },
    style: { width: '320px' }
  }])
}
function onConnect(params: any) {
  addEdges([{ ...params, animated: true }])
}

// 初始演示节点
nodes.value = [
  { id: nid(), type: 'text', position: { x: 60, y: 60 }, data: { text: 'Vue Flow 验证页\n\n试着拖动/缩放/框选/连线' }, style: { width: '240px' } },
  { id: nid(), type: 'image', position: { x: 420, y: 120 }, data: { src: makeDemoImage('IMG 1', 210), label: '演示图' }, style: { width: '300px' } },
]
</script>

<style>
.vf-demo-node {
  background: #101828;
  border: 2px solid #334155;
  border-radius: 10px;
  overflow: hidden;
  font-size: 12px;
}
.vf-demo-node:hover {
  border-color: #14b8a6;
}
.vf-demo-node.selected {
  border-color: #14b8a6;
  box-shadow: 0 0 0 3px rgba(20, 184, 166, 0.35);
}
.vf-demo-media {
  display: block;
  width: 100%;
  height: 100%;
  max-height: 220px;
  object-fit: contain;
  background: #0b1020;
}
.vf-demo-text {
  width: 220px;
  height: 120px;
}
.vf-demo-textarea {
  width: 100%;
  height: 100%;
  background: transparent;
  color: #e2e8f0;
  outline: none;
  resize: none;
  padding: 8px;
}
.vf-demo-label {
  font-size: 10px;
  color: #94a3b8;
}
.vue-flow__handle {
  width: 10px;
  height: 10px;
  background: #14b8a6;
}
</style>
